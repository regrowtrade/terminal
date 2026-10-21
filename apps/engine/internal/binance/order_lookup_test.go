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

const queriedNormalOrder = `{"orderId":9007199254740993,"clientOrderId":"synthetic-limit","symbol":"BTCUSDT","side":"BUY","positionSide":"SHORT","type":"LIMIT","status":"FILLED","origQty":"2.000","executedQty":"2.000","price":"90.00000001","avgPrice":"89.12345678","reduceOnly":true,"closePosition":false,"updateTime":1750514941540}`

func TestLookupConditionalOrdersValidatedAndPreserved(t *testing.T) {
	for _, tc := range []struct {
		name, body, kind, trigger, working string
		closePosition                      bool
	}{
		{"stop market", `{"orderId":9007199254740993,"clientOrderId":"synthetic-sl","symbol":"BTCUSDT","side":"SELL","positionSide":"LONG","type":"STOP_MARKET","status":"NEW","origQty":"2.000","executedQty":"0","price":"0","avgPrice":"0","stopPrice":"95.000","workingType":"MARK_PRICE","reduceOnly":true,"closePosition":false,"updateTime":1750514941540}`, "STOP_MARKET", "95.000", "MARK_PRICE", false},
		{"take profit market close", `{"orderId":9007199254740993,"clientOrderId":"synthetic-tp","symbol":"BTCUSDT","side":"SELL","positionSide":"LONG","type":"TAKE_PROFIT_MARKET","status":"NEW","origQty":"2.000","executedQty":"0","price":"0","avgPrice":"0","stopPrice":"106","workingType":"CONTRACT_PRICE","reduceOnly":true,"closePosition":true,"updateTime":1750514941540}`, "TAKE_PROFIT_MARKET", "106", "CONTRACT_PRICE", true},
		{"stop limit", `{"orderId":9007199254740993,"clientOrderId":"synthetic-stop","symbol":"BTCUSDT","side":"SELL","positionSide":"LONG","type":"STOP","status":"NEW","origQty":"2.000","executedQty":"0","price":"95.5","avgPrice":"0","stopPrice":"95.000","workingType":"CONTRACT_PRICE","reduceOnly":true,"closePosition":false,"updateTime":1750514941540}`, "STOP", "95.000", "CONTRACT_PRICE", false},
		{"trailing stop", `{"orderId":9007199254740993,"clientOrderId":"synthetic-trail","symbol":"BTCUSDT","side":"SELL","positionSide":"LONG","type":"TRAILING_STOP_MARKET","status":"NEW","origQty":"2.000","executedQty":"0","price":"0","avgPrice":"0","stopPrice":"0","activatePrice":"100","priceRate":"2","workingType":"MARK_PRICE","reduceOnly":true,"closePosition":false,"updateTime":1750514941540}`, "TRAILING_STOP_MARKET", "", "MARK_PRICE", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := credentials()
			result, err := fakeClient(t, key, safePermissions(), map[string]string{"/fapi/v1/order": tc.body}).LookupOrder(context.Background(), key, OrderQuery{Symbol: "BTCUSDT", OrderID: "9007199254740993"})
			if err != nil || result.Outcome != "observed" || result.ExecutionReady || result.Order == nil || result.Order.Type != tc.kind || result.Order.ClosePosition != tc.closePosition || result.Order.WorkingType != tc.working {
				t.Fatal("conditional order lost identity or semantics", err, result.Order)
			}
			if tc.trigger != "" && result.Order.StopPrice != tc.trigger {
				t.Fatal("conditional trigger lost", result.Order.StopPrice)
			}
			if tc.kind == "TRAILING_STOP_MARKET" && (result.Order.ActivatePrice != "100" || result.Order.PriceRate != "2") {
				t.Fatal("trailing activation or rate lost")
			}
			body, _ := json.Marshal(result)
			if strings.Contains(string(body), key.APIKey) || strings.Contains(string(body), key.APISecret) {
				t.Fatal("secret exposed")
			}
		})
	}
}

func TestLookupRejectsInvalidConditionalOrders(t *testing.T) {
	base := `{"orderId":9007199254740993,"clientOrderId":"synthetic-sl","symbol":"BTCUSDT","side":"SELL","positionSide":"LONG","type":"STOP_MARKET","status":"NEW","origQty":"2.000","executedQty":"0","price":"0","avgPrice":"0","stopPrice":"95.000","workingType":"MARK_PRICE","reduceOnly":true,"closePosition":false,"updateTime":1750514941540}`
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing working type", func(o map[string]any) { o["workingType"] = "" }},
		{"invalid working type", func(o map[string]any) { o["workingType"] = "LAST_PRICE" }},
		{"zero trigger", func(o map[string]any) { o["stopPrice"] = "0" }},
		{"negative trigger", func(o map[string]any) { o["stopPrice"] = "-1" }},
		{"close on stop market limit", func(o map[string]any) { o["type"] = "STOP"; o["price"] = "95"; o["closePosition"] = true }},
		{"limit stop without price", func(o map[string]any) { o["type"] = "STOP"; o["price"] = "0" }},
		{"trailing bad rate", func(o map[string]any) {
			o["type"] = "TRAILING_STOP_MARKET"
			o["price"] = "0"
			o["stopPrice"] = "0"
			o["activatePrice"] = "100"
			o["priceRate"] = "0.05"
		}},
		{"trailing missing activation", func(o map[string]any) {
			o["type"] = "TRAILING_STOP_MARKET"
			o["price"] = "0"
			o["stopPrice"] = "0"
			o["activatePrice"] = ""
			o["priceRate"] = "2"
		}},
	} {
		var order map[string]any
		decoder := json.NewDecoder(strings.NewReader(base))
		decoder.UseNumber()
		if err := decoder.Decode(&order); err != nil {
			t.Fatal(err)
		}
		tc.mutate(order)
		body, _ := json.Marshal(order)
		key := credentials()
		c := fakeClient(t, key, safePermissions(), map[string]string{"/fapi/v1/order": string(body)})
		if _, err := c.LookupOrder(context.Background(), key, OrderQuery{Symbol: "BTCUSDT", OrderID: "9007199254740993"}); !errors.Is(err, ErrResponse) {
			t.Fatal("invalid conditional order accepted", tc.name, err)
		}
	}
}

func TestLookupNormalOrderEchoesNoConditionalFields(t *testing.T) {
	// Binance can echo stopPrice/workingType even on plain orders; the evidence
	// must normalize them away rather than implying a trigger exists.
	withEcho := `{"orderId":9007199254740993,"clientOrderId":"synthetic-limit","symbol":"BTCUSDT","side":"BUY","positionSide":"SHORT","type":"LIMIT","status":"FILLED","origQty":"2.000","executedQty":"2.000","price":"90.00000001","avgPrice":"89.12345678","stopPrice":"0","workingType":"CONTRACT_PRICE","reduceOnly":true,"closePosition":false,"updateTime":1750514941540}`
	key := credentials()
	result, err := fakeClient(t, key, safePermissions(), map[string]string{"/fapi/v1/order": withEcho}).LookupOrder(context.Background(), key, OrderQuery{Symbol: "BTCUSDT", OrderID: "9007199254740993"})
	if err != nil || result.Order == nil || result.Order.StopPrice != "" || result.Order.WorkingType != "" || result.Order.ActivatePrice != "" || result.Order.PriceRate != "" {
		t.Fatal("normal order echoed conditional fields", err, result.Order)
	}
}

func TestLookupOrderSignedSelectorsAndPrecision(t *testing.T) {
	for _, query := range []OrderQuery{{Symbol: "BTCUSDT", OrderID: "9007199254740993"}, {Symbol: "BTCUSDT", ClientOrderID: "synthetic-limit"}} {
		key := credentials()
		client := fakeClient(t, key, safePermissions(), nil)
		transport := client.http.Transport
		calls := 0
		client.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path == "/fapi/v1/order" {
				p := r.URL.Query()
				if len(p) != 5 || p.Get("symbol") != query.Symbol || p.Get("orderId") != query.OrderID || p.Get("origClientOrderId") != query.ClientOrderID {
					t.Fatal("selector lost, duplicated or mixed")
				}
			}
			return transport.RoundTrip(r)
		})
		result, err := client.LookupOrder(context.Background(), key, query)
		if err != nil || result.Outcome != "observed" || result.Reason != "" || result.ExecutionReady || result.Query != query || result.Order == nil || result.Order.OrderID != "9007199254740993" || result.Order.Executed != "2.000" || result.Order.AveragePrice != "89.12345678" {
			t.Fatal("order query lost identity, decimals or semantics", err)
		}
		body, _ := json.Marshal(result)
		if strings.Contains(string(body), key.APIKey) || strings.Contains(string(body), key.APISecret) {
			t.Fatal("secret exposed")
		}
		if _, err := client.LookupOrder(context.Background(), key, query); !errors.Is(err, ErrRateLimited) || calls != 3 {
			t.Fatal("lookup retried or ignored shared cooldown")
		}
	}
}

func TestLookupRejectsInvalidSelectorsBeforeNetwork(t *testing.T) {
	for _, q := range []OrderQuery{
		{}, {Symbol: "BTCUSDT"}, {Symbol: "BTCUSDT", OrderID: "1", ClientOrderID: "both"},
		{Symbol: "BTCUSDC", OrderID: "1"}, {Symbol: "BTCUSDT", OrderID: "9223372036854775808"},
		{Symbol: "BTCUSDT", OrderID: "01"}, {Symbol: "BTCUSDT", ClientOrderID: strings.Repeat("a", 37)},
		{Symbol: "BTCUSDT", ClientOrderID: "inject&signature=bad"},
	} {
		c := newClient(roundTrip(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid selector reached network")
			return nil, nil
		}))
		if _, err := c.LookupOrder(context.Background(), credentials(), q); !errors.Is(err, ErrOrderQuery) {
			t.Fatal("invalid selector accepted")
		}
	}
	c := newClient(roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid query params reached network")
		return nil, nil
	}))
	key := credentials()
	for _, tc := range []struct {
		path   string
		params url.Values
	}{
		{"/fapi/v1/order", url.Values{"symbol": {"BTCUSDT"}, "orderId": {"1", "2"}}},
		{"/fapi/v1/order", url.Values{"symbol": {"BTCUSDT"}, "orderId": {"1"}, "signature": {"injected"}}},
		{"/fapi/v3/account", url.Values{"symbol": {"BTCUSDT"}}},
		{"/fapi/v1/order/other", url.Values{"symbol": {"BTCUSDT"}, "orderId": {"1"}}},
	} {
		if err := c.getWithParams(context.Background(), tc.path, &key, 1, tc.params, &struct{}{}); err == nil {
			t.Fatal("unsafe path/parameters accepted")
		}
	}
}

func TestLookupNotFoundRemainsUnresolvedAndOtherErrorsFail(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body       string
		unresolved bool
	}{
		{400, `{"code":-2013,"msg":"private-error"}`, true},
		{500, `{"code":-2013,"msg":"private-error"}`, false},
		{404, `{"code":-2013}`, false}, {400, `{"code":-2011}`, false},
		{400, `not json`, false}, {200, `null`, false}, {200, `{}`, false},
		{200, strings.Repeat(" ", 64*1024+1), false},
	} {
		key := credentials()
		c := fakeClient(t, key, safePermissions(), nil)
		transport := c.http.Transport
		c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			resp, err := transport.RoundTrip(r)
			if r.URL.Path == "/fapi/v1/order" {
				resp.Body.Close()
				resp.StatusCode = tc.status
				resp.Body = io.NopCloser(strings.NewReader(tc.body))
			}
			return resp, err
		})
		result, err := c.LookupOrder(context.Background(), key, OrderQuery{Symbol: "BTCUSDT", OrderID: "9007199254740993"})
		if tc.unresolved {
			if err != nil || result.Outcome != "unresolved" || result.Reason != "exchange_not_found" || result.Order != nil || result.ExecutionReady {
				t.Fatal("not found treated as a definitive order state", err)
			}
		} else if err == nil || result.Source != "" || strings.Contains(err.Error(), "private-error") {
			t.Fatal("unsafe or partial result returned")
		}
	}
}

func TestLookupRejectsMismatchesIncompleteAndInconsistentOrders(t *testing.T) {
	for _, mutate := range []func(map[string]any){
		func(o map[string]any) { o["symbol"] = "ETHUSDT" }, func(o map[string]any) { o["orderId"] = json.Number("123") },
		func(o map[string]any) { o["status"] = "UNKNOWN" }, func(o map[string]any) { o["status"] = "NEW" },
		func(o map[string]any) { o["executedQty"] = "3" }, func(o map[string]any) { o["executedQty"] = "1" },
		func(o map[string]any) { o["avgPrice"] = "NaN" }, func(o map[string]any) { o["avgPrice"] = "0" },
		func(o map[string]any) { o["updateTime"] = 0 }, func(o map[string]any) { delete(o, "reduceOnly") },
		func(o map[string]any) { o["closePosition"] = nil }, func(o map[string]any) { o["type"] = "STOP_MARKET" },
	} {
		var order map[string]any
		decoder := json.NewDecoder(strings.NewReader(queriedNormalOrder))
		decoder.UseNumber()
		if err := decoder.Decode(&order); err != nil {
			t.Fatal(err)
		}
		mutate(order)
		body, _ := json.Marshal(order)
		key := credentials()
		c := fakeClient(t, key, safePermissions(), map[string]string{"/fapi/v1/order": string(body)})
		if _, err := c.LookupOrder(context.Background(), key, OrderQuery{Symbol: "BTCUSDT", OrderID: "9007199254740993"}); !errors.Is(err, ErrResponse) {
			t.Fatal("invalid queried order accepted", err)
		}
	}
}

func TestLookupNormalTerminalStatusesDoNotBecomeFills(t *testing.T) {
	for _, status := range []string{"NEW", "PARTIALLY_FILLED", "FILLED", "CANCELED", "EXPIRED", "EXPIRED_IN_MATCH", "REJECTED", "PENDING_CANCEL"} {
		body := strings.Replace(queriedNormalOrder, `"status":"FILLED"`, `"status":"`+status+`"`, 1)
		if status == "NEW" || status == "REJECTED" {
			body = strings.Replace(body, `"executedQty":"2.000"`, `"executedQty":"0"`, 1)
			body = strings.Replace(body, `"avgPrice":"89.12345678"`, `"avgPrice":"0"`, 1)
		}
		if status == "PARTIALLY_FILLED" {
			body = strings.Replace(body, `"executedQty":"2.000"`, `"executedQty":"0.5"`, 1)
		}
		key := credentials()
		result, err := fakeClient(t, key, safePermissions(), map[string]string{"/fapi/v1/order": body}).LookupOrder(context.Background(), key, OrderQuery{Symbol: "BTCUSDT", ClientOrderID: "synthetic-limit"})
		if err != nil || result.Order == nil || result.Order.Status != status || result.ExecutionReady {
			t.Fatal("normal order status lost", status, err)
		}
	}
}

func TestLookupPermissionsAndRateLimitsFailClosed(t *testing.T) {
	for _, mutate := range []func(map[string]bool){
		func(p map[string]bool) { p["enableFutures"] = true },
		func(p map[string]bool) { p["ipRestrict"] = false },
		func(p map[string]bool) { p["enableReading"] = false },
	} {
		key := credentials()
		permissions := safePermissions()
		mutate(permissions)
		c := fakeClient(t, key, permissions, nil)
		transport := c.http.Transport
		c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/fapi/v1/order" {
				t.Fatal("query reached exchange before permission gate")
			}
			return transport.RoundTrip(r)
		})
		if _, err := c.LookupOrder(context.Background(), key, OrderQuery{Symbol: "BTCUSDT", OrderID: "1"}); !errors.Is(err, ErrPermissions) {
			t.Fatal("unsafe permissions accepted", err)
		}
	}
	for _, status := range []int{418, 429} {
		key := credentials()
		c := fakeClient(t, key, safePermissions(), nil)
		transport := c.http.Transport
		calls := 0
		c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path == "/fapi/v1/order" {
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader("private"))}, nil
			}
			return transport.RoundTrip(r)
		})
		if _, err := c.LookupOrder(context.Background(), key, OrderQuery{Symbol: "BTCUSDT", OrderID: "1"}); !errors.Is(err, ErrRateLimited) {
			t.Fatal("lookup ignored exchange cooldown")
		}
		if _, err := c.Snapshot(context.Background(), key); !errors.Is(err, ErrRateLimited) || calls != 3 {
			t.Fatal("snapshot bypassed lookup cooldown or lookup retried")
		}
	}
}
