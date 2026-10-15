package binance

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regrow/engine/internal/vault"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func credentials() vault.Credentials {
	a, b := make([]byte, 32), make([]byte, 32)
	if _, err := rand.Read(a); err != nil {
		panic("random data unavailable")
	}
	if _, err := rand.Read(b); err != nil {
		panic("random data unavailable")
	}
	return vault.Credentials{APIKey: hex.EncodeToString(a), APISecret: hex.EncodeToString(b)}
}
func safePermissions() map[string]bool {
	return map[string]bool{"enableReading": true, "ipRestrict": true, "enableWithdrawals": false, "enableFutures": false, "enableSpotAndMarginTrading": false, "enableMargin": false, "enableInternalTransfer": false, "permitsUniversalTransfer": false}
}

const assets = `{"assets":[{"asset":"USDT","walletBalance":"100.00000001","unrealizedProfit":"-2","availableBalance":"80"}]}`
const positions = `[{"symbol":"BTCUSDT","positionSide":"SHORT","positionAmt":"-1","entryPrice":"100","markPrice":"102","unRealizedProfit":"-2","liquidationPrice":"150"}]`

func fakeClient(t *testing.T, c vault.Credentials, permissions map[string]bool, overrides map[string]string) *Client {
	t.Helper()
	return newClient(roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Scheme != "https" {
			t.Fatal("non-read request")
		}
		path := r.URL.Path
		host := "fapi.binance.com"
		if path == "/sapi/v1/account/apiRestrictions" {
			host = "api.binance.com"
		}
		if r.URL.Host != host {
			t.Fatal("unexpected destination")
		}
		if path == "/fapi/v1/time" {
			if r.Header.Get("X-MBX-APIKEY") != "" || r.URL.RawQuery != "" {
				t.Fatal("credential leaked on public request")
			}
		} else {
			if r.Header.Get("X-MBX-APIKEY") != c.APIKey {
				t.Fatal("missing signed key")
			}
			q := r.URL.Query()
			signature := q.Get("signature")
			q.Del("signature")
			mac := hmac.New(sha256.New, []byte(c.APISecret))
			mac.Write([]byte(q.Encode()))
			if signature != hex.EncodeToString(mac.Sum(nil)) || q.Get("recvWindow") != "5000" || q.Get("timestamp") == "" {
				t.Fatal("invalid signature")
			}
		}
		body := ""
		switch path {
		case "/fapi/v1/time":
			body = fmt.Sprintf(`{"serverTime":%d}`, time.Now().UnixMilli())
		case "/sapi/v1/account/apiRestrictions":
			raw, _ := json.Marshal(permissions)
			body = string(raw)
		case "/fapi/v3/account":
			body = assets
		case "/fapi/v3/positionRisk":
			body = positions
		default:
			t.Fatal("endpoint outside allowlist")
		}
		if value, ok := overrides[path]; ok {
			body = value
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
}
func TestSnapshotUsesOnlyAllowlistedSignedReads(t *testing.T) {
	c := credentials()
	client := fakeClient(t, c, safePermissions(), nil)
	snapshot, err := client.Snapshot(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ExecutionReady || snapshot.Consistency != "rest_non_atomic" || snapshot.Assets[0].WalletBalance != "100.00000001" || snapshot.Positions[0].Quantity != "-1" {
		t.Fatal("snapshot lost decimal/side/source semantics")
	}
	raw, _ := json.Marshal(snapshot)
	if strings.Contains(string(raw), c.APIKey) || strings.Contains(string(raw), c.APISecret) {
		t.Fatal("secret in snapshot")
	}
}
func TestRejectUnsafeOrIncompletePermissions(t *testing.T) {
	for _, flag := range []string{"enableWithdrawals", "enableFutures", "enableSpotAndMarginTrading", "enableMargin", "enableInternalTransfer", "permitsUniversalTransfer", "enableVanillaOptions", "enableFixApiTrade", "enablePortfolioMarginTrading", "enableFutureUnknownCapability", "enableReading", "ipRestrict", "missing"} {
		t.Run(flag, func(t *testing.T) {
			p := safePermissions()
			if flag == "missing" {
				delete(p, "enableWithdrawals")
			} else {
				p[flag] = flag != "enableReading" && flag != "ipRestrict"
			}
			c := credentials()
			if err := fakeClient(t, c, p, nil).Verify(context.Background(), c); !errors.Is(err, ErrPermissions) {
				t.Fatal("unsafe key accepted")
			}
		})
	}
}
func TestRejectPartialOrUnsupportedSnapshots(t *testing.T) {
	for _, override := range []map[string]string{{"/fapi/v3/account": `{}`}, {"/fapi/v3/positionRisk": `null`}, {"/fapi/v3/positionRisk": strings.ReplaceAll(positions, "BTCUSDT", "BTCUSDC")}, {"/fapi/v3/positionRisk": strings.ReplaceAll(positions, `"-1"`, `"1"`)}, {"/fapi/v3/account": strings.ReplaceAll(assets, `"100.00000001"`, `"NaN"`)}} {
		c := credentials()
		if _, err := fakeClient(t, c, safePermissions(), override).Snapshot(context.Background(), c); !errors.Is(err, ErrResponse) {
			t.Fatal("partial/malformed data accepted")
		}
	}
}
func TestCooldownAndTransportErrorsAreSafe(t *testing.T) {
	c := credentials()
	calls := 0
	client := newClient(roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"120"}}, Body: io.NopCloser(strings.NewReader("private body"))}, nil
	}))
	if err := client.Verify(context.Background(), c); !errors.Is(err, ErrRateLimited) {
		t.Fatal("rate limit ignored")
	}
	if err := client.Verify(context.Background(), c); !errors.Is(err, ErrRateLimited) || calls != 1 {
		t.Fatal("automatic retry during cooldown")
	}
	client = newClient(roundTrip(func(r *http.Request) (*http.Response, error) { return nil, errors.New(c.APISecret + r.URL.String()) }))
	err := client.Verify(context.Background(), c)
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), c.APISecret) {
		t.Fatal("unsafe transport error")
	}
}
func TestDoesNotFollowRedirects(t *testing.T) {
	c := credentials()
	calls := 0
	client := newClient(roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://untrusted.example/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if err := client.Verify(context.Background(), c); !errors.Is(err, ErrUnavailable) || calls != 1 {
		t.Fatal("redirect followed")
	}
}
