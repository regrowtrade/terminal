package binance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const queriedAlgo = `{"algoId":9007199254740993,"clientAlgoId":"synthetic-algo","algoType":"CONDITIONAL","symbol":"BTCUSDT","side":"SELL","positionSide":"LONG","orderType":"STOP_MARKET","algoStatus":"NEW","quantity":"2.000","price":"0","triggerPrice":"95.123456789012345678","reduceOnly":true,"closePosition":false,"workingType":"MARK_PRICE","updateTime":1750514941540,"triggerTime":0,"actualOrderId":"","actualPrice":"0"}`

func TestAlgoLookupSignedProtocolPrecisionAndNoAutomaticChildQuery(t *testing.T) {
	for _, q := range []AlgoQuery{{Symbol: "BTCUSDT", AlgoID: "9007199254740993"}, {Symbol: "BTCUSDT", ClientAlgoID: "synthetic-algo"}} {
		key := credentials()
		c := fakeClient(t, key, safePermissions(), map[string]string{"/fapi/v1/algoOrder": queriedAlgo})
		transport := c.http.Transport
		calls := 0
		c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path == "/fapi/v1/algoOrder" {
				p := r.URL.Query()
				if len(p) != 4 || p.Get("algoId") != q.AlgoID || p.Get("clientAlgoId") != q.ClientAlgoID || p.Has("symbol") {
					t.Fatal("incorrect algo selectors", p)
				}
			}
			return transport.RoundTrip(r)
		})
		r, err := c.LookupAlgoOrder(context.Background(), key, q)
		if err != nil || r.Family != "algo" || r.ExecutionReady || r.Outcome != "observed" || r.Query != q || r.Order == nil || r.Order.ID != "9007199254740993" || r.Order.Quantity != "2.000" || r.Order.TriggerPrice != "95.123456789012345678" || calls != 3 {
			t.Fatal("invalid algo lookup", err, r)
		}
		b, _ := json.Marshal(r)
		if strings.Contains(string(b), key.APIKey) || strings.Contains(string(b), key.APISecret) {
			t.Fatal("secret exposed")
		}
		if _, err := c.LookupOrder(context.Background(), key, OrderQuery{Symbol: q.Symbol, OrderID: "1"}); !errors.Is(err, ErrRateLimited) || calls != 3 {
			t.Fatal("shared cooldown bypassed")
		}
	}
}

func TestAlgoLookupRejectsSelectorsBeforeNetwork(t *testing.T) {
	c := newClient(roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid query reached network")
		return nil, nil
	}))
	key := credentials()
	for _, q := range []AlgoQuery{
		{}, {Symbol: "BTCUSDT"}, {Symbol: "BTCUSDC", AlgoID: "1"},
		{Symbol: "BTCUSDT", AlgoID: "01"}, {Symbol: "BTCUSDT", AlgoID: "9223372036854775808"},
		{Symbol: "BTCUSDT", AlgoID: "1", ClientAlgoID: "both"},
		{Symbol: "BTCUSDT", ClientAlgoID: strings.Repeat("x", 37)},
		{Symbol: "BTCUSDT", ClientAlgoID: "inject&signature=bad"},
	} {
		if _, err := c.LookupAlgoOrder(context.Background(), key, q); !errors.Is(err, ErrAlgoQuery) {
			t.Fatal("invalid selector accepted", q, err)
		}
	}
	for _, params := range []url.Values{
		nil, {"algoId": {"1", "2"}}, {"algoId": {"1"}, "symbol": {"BTCUSDT"}},
		{"algoId": {"1"}, "signature": {"bad"}}, {"orderId": {"1"}}, {"clientAlgoId": {""}},
	} {
		if err := c.getWithParams(context.Background(), "/fapi/v1/algoOrder", &key, 1, params, &struct{}{}); err == nil {
			t.Fatal("unsafe params accepted")
		}
	}
	if err := c.get(context.Background(), "/fapi/v1/algoOrder", &key, 1, &struct{}{}); err == nil {
		t.Fatal("generic algo get accepted")
	}
}

func TestAlgoLookupRejectsInvalidOrMismatchedEvidence(t *testing.T) {
	for _, mutate := range []func(map[string]any){
		func(o map[string]any) { o["symbol"] = "ETHUSDT" },
		func(o map[string]any) { o["algoId"] = json.Number("123") },
		func(o map[string]any) { o["algoStatus"] = "FILLED" },
		func(o map[string]any) { o["algoType"] = "OTHER" },
		func(o map[string]any) { o["workingType"] = "LAST_PRICE" },
		func(o map[string]any) { o["triggerPrice"] = "0" },
		func(o map[string]any) { o["quantity"] = "0" },
		func(o map[string]any) { o["actualPrice"] = "NaN" },
		func(o map[string]any) { o["actualOrderId"] = "123" },
		func(o map[string]any) { o["triggerTime"] = 1 },
		func(o map[string]any) { o["actualQty"] = "1" },
		func(o map[string]any) { o["orderType"] = "LIMIT" },
		func(o map[string]any) { o["closePosition"] = nil },
		func(o map[string]any) { delete(o, "reduceOnly") },
		func(o map[string]any) { delete(o, "actualOrderId") },
		func(o map[string]any) { delete(o, "triggerTime") },
		func(o map[string]any) { delete(o, "actualPrice") },
		func(o map[string]any) {
			o["algoStatus"] = "TRIGGERED"
			o["actualOrderId"] = "9223372036854775808"
			o["triggerTime"] = 1
		},
	} {
		var o map[string]any
		d := json.NewDecoder(strings.NewReader(queriedAlgo))
		d.UseNumber()
		if err := d.Decode(&o); err != nil {
			t.Fatal(err)
		}
		mutate(o)
		b, _ := json.Marshal(o)
		key := credentials()
		c := fakeClient(t, key, safePermissions(), map[string]string{"/fapi/v1/algoOrder": string(b)})
		if r, err := c.LookupAlgoOrder(context.Background(), key, AlgoQuery{Symbol: "BTCUSDT", AlgoID: "9007199254740993"}); !errors.Is(err, ErrResponse) || r.Source != "" {
			t.Fatal("invalid algo response accepted", err)
		}
	}
}

func TestAlgoLookupStatusesDoNotImplyFillsOrProtection(t *testing.T) {
	for _, status := range []string{"NEW", "CANCELED", "EXPIRED", "REJECTED", "TRIGGERED", "FINISHED"} {
		b := strings.Replace(queriedAlgo, `"algoStatus":"NEW"`, `"algoStatus":"`+status+`"`, 1)
		if status == "TRIGGERED" || status == "FINISHED" {
			b = strings.Replace(b, `"actualOrderId":""`, `"actualOrderId":"9007199254740995"`, 1)
			b = strings.Replace(b, `"triggerTime":0`, `"triggerTime":1750514941539`, 1)
			b = strings.Replace(b, `"actualPrice":"0"`, `"actualPrice":"94.123456789012345678","actualQty":"0.500000000000000001","actualType":"MARKET"`, 1)
		}
		key := credentials()
		r, err := fakeClient(t, key, safePermissions(), map[string]string{"/fapi/v1/algoOrder": b}).LookupAlgoOrder(context.Background(), key, AlgoQuery{Symbol: "BTCUSDT", ClientAlgoID: "synthetic-algo"})
		if err != nil || r.Order == nil || r.Order.Status != status || r.ExecutionReady {
			t.Fatal("algo status lost", status, err)
		}
	}
	// Close-all has no fixed quantity; never invent one from account positions.
	b := strings.Replace(queriedAlgo, `"quantity":"2.000"`, `"quantity":"0"`, 1)
	b = strings.Replace(b, `"closePosition":false`, `"closePosition":true`, 1)
	key := credentials()
	r, err := fakeClient(t, key, safePermissions(), map[string]string{"/fapi/v1/algoOrder": b}).LookupAlgoOrder(context.Background(), key, AlgoQuery{Symbol: "BTCUSDT", AlgoID: "9007199254740993"})
	if err != nil || r.Order == nil || r.Order.Quantity != "0" || !r.Order.ClosePosition {
		t.Fatal("close-all lost", err)
	}
}

func TestAlgoLookupAbsenceErrorsPermissionsAndCooldown(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body       string
		want       error
		unresolved bool
	}{
		{400, `{"code":-2013}`, nil, true},
		{500, `{"code":-2013}`, ErrUnavailable, false},
		{404, `{"code":-2013}`, ErrUnavailable, false},
		{400, `{"code":-2011}`, ErrUnavailable, false},
		{400, `private`, ErrUnavailable, false},
		{200, `null`, ErrResponse, false},
		{200, `{}`, ErrResponse, false},
		{200, strings.Repeat(" ", 64*1024+1), ErrResponse, false},
		{429, `private`, ErrRateLimited, false},
		{418, `private`, ErrRateLimited, false},
		{403, `private`, ErrPermissions, false},
	} {
		key := credentials()
		c := fakeClient(t, key, safePermissions(), nil)
		transport := c.http.Transport
		calls := 0
		c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path == "/fapi/v1/algoOrder" {
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}
			return transport.RoundTrip(r)
		})
		r, err := c.LookupAlgoOrder(context.Background(), key, AlgoQuery{Symbol: "BTCUSDT", AlgoID: "1"})
		if !errors.Is(err, tc.want) || calls != 3 {
			t.Fatal("wrong algo error/no retry boundary", tc.status, err, calls)
		}
		if tc.unresolved {
			if r.Outcome != "unresolved" || r.Order != nil || r.Reason != "exchange_not_found" {
				t.Fatal("absence inferred state")
			}
		} else if r.Source != "" {
			t.Fatal("partial response")
		}
		if errors.Is(err, ErrRateLimited) {
			if _, err := c.Snapshot(context.Background(), key); !errors.Is(err, ErrRateLimited) || calls != 3 {
				t.Fatal("cooldown ignored")
			}
		}
	}
	key := credentials()
	p := safePermissions()
	p["enableFutures"] = true
	c := fakeClient(t, key, p, nil)
	if _, err := c.LookupAlgoOrder(context.Background(), key, AlgoQuery{Symbol: "BTCUSDT", AlgoID: "1"}); !errors.Is(err, ErrPermissions) {
		t.Fatal("trading key accepted", err)
	}
}
