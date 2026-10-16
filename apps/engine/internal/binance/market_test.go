package binance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRejectIncompleteOrdersAndInstruments(t *testing.T) {
	tests := []struct{ name, path, body string }{
		{"null orders", "/fapi/v1/openOrders", `null`},
		{"object orders", "/fapi/v1/openOrders", `{}`},
		{"null algos", "/fapi/v1/openAlgoOrders", `null`},
		{"missing flag", "/fapi/v1/openOrders", strings.Replace(openOrders, `"reduceOnly":true,`, "", 1)},
		{"null flag", "/fapi/v1/openAlgoOrders", strings.Replace(algoOrders, `"closePosition":true`, `"closePosition":null`, 1)},
		{"unsupported quote", "/fapi/v1/openOrders", strings.ReplaceAll(openOrders, "BTCUSDT", "BTCUSDC")},
		{"oversized id", "/fapi/v1/openOrders", strings.Replace(openOrders, "9007199254740993", "9223372036854775808", 1)},
		{"fractional id", "/fapi/v1/openAlgoOrders", strings.Replace(algoOrders, "9007199254740995", "1.5", 1)},
		{"negative qty", "/fapi/v1/openOrders", strings.Replace(openOrders, `"origQty":"2"`, `"origQty":"-2"`, 1)},
		{"overfill", "/fapi/v1/openOrders", strings.Replace(openOrders, `"executedQty":"0.5"`, `"executedQty":"3"`, 1)},
		{"terminal order", "/fapi/v1/openOrders", strings.Replace(openOrders, "PARTIALLY_FILLED", "FILLED", 1)},
		{"terminal algo", "/fapi/v1/openAlgoOrders", strings.Replace(algoOrders, `"algoStatus":"NEW"`, `"algoStatus":"FINISHED"`, 1)},
		{"unknown order type", "/fapi/v1/openAlgoOrders", strings.Replace(algoOrders, "STOP_MARKET", "FUTURE_TYPE", 1)},
		{"missing trigger", "/fapi/v1/openAlgoOrders", strings.Replace(algoOrders, `"triggerPrice":"120",`, "", 1)},
		{"missing catalogue", "/fapi/v1/exchangeInfo", `{}`},
		{"null filter", "/fapi/v1/exchangeInfo", strings.Replace(exchangeInfo, `"tickSize":"0.10"`, `"tickSize":null`, 1)},
		{"precision is not a step", "/fapi/v1/exchangeInfo", strings.Replace(exchangeInfo, `,"tickSize":"0.10"`, "", 1)},
		{"negative filter", "/fapi/v1/exchangeInfo", strings.Replace(exchangeInfo, `"notional":"5"`, `"notional":"-5"`, 1)},
		{"missing limit", "/fapi/v1/exchangeInfo", strings.Replace(exchangeInfo, `,"limit":200`, "", 1)},
		{"missing instrument for exposure", "/fapi/v1/exchangeInfo", strings.ReplaceAll(exchangeInfo, "BTC", "ETH")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := credentials()
			snapshot, err := fakeClient(t, c, safePermissions(), map[string]string{test.path: test.body}).Snapshot(context.Background(), c)
			if !errors.Is(err, ErrResponse) || snapshot.Assets != nil || snapshot.OpenOrders != nil {
				t.Fatal("partial snapshot returned")
			}
		})
	}
}

func TestDuplicateRowsRejectWholeSnapshot(t *testing.T) {
	for path, body := range map[string]string{"/fapi/v1/openOrders": openOrders, "/fapi/v1/openAlgoOrders": algoOrders, "/fapi/v1/exchangeInfo": exchangeInfo} {
		t.Run(path, func(t *testing.T) {
			duplicate := strings.TrimSuffix(body, "]") + "," + strings.TrimPrefix(body, "[")
			if path == "/fapi/v1/exchangeInfo" {
				var data map[string][]json.RawMessage
				if err := json.Unmarshal([]byte(body), &data); err != nil {
					t.Fatal(err)
				}
				data["symbols"] = append(data["symbols"], data["symbols"][0])
				encoded, _ := json.Marshal(data)
				duplicate = string(encoded)
			}
			c := credentials()
			if _, err := fakeClient(t, c, safePermissions(), map[string]string{path: duplicate}).Snapshot(context.Background(), c); !errors.Is(err, ErrResponse) {
				t.Fatal("duplicate row accepted")
			}
		})
	}
}

func TestEmptyOrdersAreExplicitAndUnknownFiltersAreVisible(t *testing.T) {
	c := credentials()
	info := strings.Replace(exchangeInfo, `"filters":[`, `"filters":[{"filterType":"FUTURE_FILTER","value":"opaque"},`, 1)
	snapshot, err := fakeClient(t, c, safePermissions(), map[string]string{"/fapi/v1/openOrders": "[]", "/fapi/v1/openAlgoOrders": "[]", "/fapi/v1/exchangeInfo": info}).Snapshot(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.OpenOrders == nil || snapshot.AlgoOrders == nil || len(snapshot.OpenOrders) != 0 || snapshot.Instruments[0].UnknownFilters[0] != "FUTURE_FILTER" || snapshot.ExecutionReady {
		t.Fatal("unsupported filter hidden or empty arrays ambiguous")
	}
}

func TestTrailingAlgoRequiresActivationAndCallback(t *testing.T) {
	body := strings.Replace(algoOrders, "STOP_MARKET", "TRAILING_STOP_MARKET", 1)
	body = strings.Replace(body, `"quantity":"0"`, `"quantity":"1"`, 1)
	body = strings.Replace(body, `"closePosition":true`, `"closePosition":false`, 1)
	for _, rate := range []string{"0.3", "0", "11", "NaN", ""} {
		t.Run(rate, func(t *testing.T) {
			c := credentials()
			candidate := strings.Replace(body, `"price":"0"`, `"price":"0","activatePrice":"110","callbackRate":"`+rate+`"`, 1)
			_, err := fakeClient(t, c, safePermissions(), map[string]string{"/fapi/v1/openAlgoOrders": candidate}).Snapshot(context.Background(), c)
			if (rate == "0.3" && err != nil) || (rate != "0.3" && !errors.Is(err, ErrResponse)) {
				t.Fatal("trailing semantics not validated")
			}
		})
	}
}

func TestLateFailureNeverReturnsEarlierData(t *testing.T) {
	c := credentials()
	base := fakeClient(t, c, safePermissions(), nil)
	transport := base.http.Transport
	base.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/fapi/v1/openAlgoOrders" {
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"120"}}, Body: io.NopCloser(strings.NewReader("private"))}, nil
		}
		return transport.RoundTrip(r)
	})
	snapshot, err := base.Snapshot(context.Background(), c)
	if !errors.Is(err, ErrRateLimited) || snapshot.Assets != nil {
		t.Fatal("late error returned partial data")
	}
	if _, err := base.Snapshot(context.Background(), c); !errors.Is(err, ErrRateLimited) {
		t.Fatal("late cooldown bypassed")
	}
}

func TestPublicPrivateBoundaryAndAllowlist(t *testing.T) {
	c := credentials()
	client := newClient(roundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("forbidden request sent"); return nil, nil }))
	for _, path := range []string{"/fapi/v1/time", "/fapi/v1/exchangeInfo"} {
		if err := client.get(context.Background(), path, &c, 0, &struct{}{}); !errors.Is(err, ErrPermissions) {
			t.Fatal("public credential leak allowed")
		}
	}
	if err := client.get(context.Background(), "/fapi/v1/openOrders", nil, 0, &struct{}{}); !errors.Is(err, ErrPermissions) {
		t.Fatal("unsigned private read allowed")
	}
	if err := client.get(context.Background(), "/fapi/v1/order", &c, 0, &struct{}{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("generic order endpoint allowed")
	}
}

func TestInFlightReadCannotBeOverlappedAfterStartInterval(t *testing.T) {
	c := credentials()
	client := newClient(roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("overlapping read reached transport")
		return nil, nil
	}))
	if err := client.begin(); err != nil {
		t.Fatal(err)
	}
	// Simulate a slow ongoing collection after its ten-second start interval.
	client.mu.Lock()
	client.next = time.Time{}
	client.mu.Unlock()
	if err := client.Verify(context.Background(), c); !errors.Is(err, ErrRateLimited) {
		t.Fatal("in-flight collection overlapped")
	}
	client.end()
}
