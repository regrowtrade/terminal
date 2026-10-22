package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"regrow/engine/internal/vault"
)

func tradeRowAt(id, orderID, tms int64) tradeRow {
	return tradeRow{ID: json.Number(strconv.FormatInt(id, 10)), OrderID: json.Number(strconv.FormatInt(orderID, 10)), Symbol: "BTCUSDT", Side: "BUY", PositionSide: "LONG", Price: "100", Quantity: "1", Commission: "-0.1", CommissionAsset: "USDT", Time: tms}
}

func tradesTransport(t *testing.T, c vault.Credentials, pages [][]tradeRow) roundTrip {
	t.Helper()
	idx := 0
	return tradeResponses(t, c, func(url.Values) []tradeRow {
		if idx >= len(pages) {
			return []tradeRow{}
		}
		rows := pages[idx]
		idx++
		return rows
	})
}

func tradeResponses(t *testing.T, c vault.Credentials, rows func(url.Values) []tradeRow) roundTrip {
	t.Helper()
	return roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/fapi/v1/time":
			return jsonResponse(fmt.Sprintf(`{"serverTime":%d}`, time.Now().UnixMilli()))
		case "/sapi/v1/account/apiRestrictions":
			raw, _ := json.Marshal(safePermissions())
			return jsonResponse(string(raw))
		case "/fapi/v1/userTrades":
			if r.Method != http.MethodGet || r.URL.Host != "fapi.binance.com" {
				t.Fatal("recovery changed fixed read-only destination")
			}
			if r.Header.Get("X-MBX-APIKEY") != c.APIKey {
				t.Fatal("missing signed key")
			}
			q := r.URL.Query()
			signature := q.Get("signature")
			q.Del("signature")
			mac := hmac.New(sha256.New, []byte(c.APISecret))
			mac.Write([]byte(q.Encode()))
			if signature != hex.EncodeToString(mac.Sum(nil)) {
				t.Fatal("invalid signature")
			}
			params := url.Values{}
			for key, values := range q {
				params[key] = values
			}
			params.Del("timestamp")
			params.Del("recvWindow")
			if !validTradeParams(params) || params.Has("fromId") {
				t.Fatal("recovery sent incompatible or unbounded selectors", params)
			}
			body, _ := json.Marshal(rows(q))
			return jsonResponse(string(body))
		default:
			t.Fatal("unexpected path " + r.URL.Path)
			return nil, nil
		}
	})
}

type vaultCredentials = vault.Credentials

func TestRecoverTradesSplitsWindowsIndependentOfResponseOrder(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(strconv.FormatBool(reverse), func(t *testing.T) {
			c := credentials()
			q := TradeQuery{Symbol: "BTCUSDT", StartTime: 1700000000000, EndTime: 1700000002000}
			rows := make([]tradeRow, 1002)
			for i := range rows {
				rows[i] = tradeRowAt(9007199254740993+int64(i), 9007199254741111+int64(i), q.StartTime+int64(i))
			}
			calls := 0
			client := newClient(tradeResponses(t, c, func(params url.Values) []tradeRow {
				calls++
				start, _ := strconv.ParseInt(params.Get("startTime"), 10, 64)
				end, _ := strconv.ParseInt(params.Get("endTime"), 10, 64)
				page := []tradeRow{}
				for _, row := range rows {
					if row.Time >= start && row.Time <= end {
						page = append(page, row)
					}
				}
				if reverse {
					slices.Reverse(page)
				}
				if len(page) >= tradePageSize {
					return page[:tradePageSize]
				}
				// Identical redelivery must not duplicate evidence.
				if len(page) > 0 {
					page = append(page, page[0])
				}
				return page
			}))
			fills, truncated, err := client.RecoverTrades(context.Background(), c, q)
			if err != nil || truncated || len(fills) != len(rows) || calls <= 1 || calls > maxTradePages {
				t.Fatal("recovery lost fills or exceeded request budget", err, len(fills), truncated, calls)
			}
			for i, fill := range fills {
				if fill.TradeID != rows[i].ID.String() || fill.OrderID != rows[i].OrderID.String() || fill.EventTime != rows[i].Time || fill.TransactionTime != rows[i].Time || fill.Commission != "-0.1" {
					t.Fatal("recovery lost exact IDs, signed commission, clock or ordering")
				}
			}
		})
	}
}

func TestRecoverTradesRejectsInvalidInputBeforeNetwork(t *testing.T) {
	noNetwork := roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid query reached network")
		return nil, nil
	})
	client := newClient(noNetwork)
	for _, q := range []TradeQuery{
		{}, {Symbol: "BTCUSDT"}, {Symbol: "BTCUSDC", StartTime: 1, EndTime: 2},
		{Symbol: "BTCUSDT", StartTime: 0, EndTime: 2},
		{Symbol: "BTCUSDT", StartTime: 2, EndTime: 1},
		{Symbol: "BTCUSDT", StartTime: 1, EndTime: 1 + maxTradeWindowMs + 1},
	} {
		if _, _, err := client.RecoverTrades(context.Background(), credentials(), q); err == nil {
			t.Fatal("invalid trade query accepted", q)
		}
	}
}

func TestRecoverTradesRejectsMalformedRows(t *testing.T) {
	c := credentials()
	base := tradeRowAt(5000, 4001, 1700000086000)
	for _, mutate := range []func(*tradeRow){
		func(r *tradeRow) { r.Symbol = "BTCUSDC" },
		func(r *tradeRow) { r.Symbol = "ETHUSDT" },
		func(r *tradeRow) { r.Side = "HOLD" },
		func(r *tradeRow) { r.PositionSide = "NEUTRAL" },
		func(r *tradeRow) { r.Price = "0" },
		func(r *tradeRow) { r.Quantity = "-1" },
		func(r *tradeRow) { r.Commission = "NaN" },
		func(r *tradeRow) { r.CommissionAsset = strings.Repeat("a", 21) },
		func(r *tradeRow) { r.CommissionAsset = "USDT\x00" },
		func(r *tradeRow) { r.Time = 0 },
		func(r *tradeRow) { r.Time = 1700000086400 },
		func(r *tradeRow) { r.Time = 1699999999999 },
		func(r *tradeRow) { r.ID = json.Number("0") },
		func(r *tradeRow) { r.OrderID = json.Number("-1") },
	} {
		row := base
		mutate(&row)
		client := newClient(tradesTransport(t, c, [][]tradeRow{{row}}))
		if _, _, err := client.RecoverTrades(context.Background(), c, TradeQuery{Symbol: "BTCUSDT", StartTime: 1700000000000, EndTime: 1700000086400}); err == nil {
			t.Fatal("malformed trade accepted")
		}
	}
}

func TestRecoverTradesReportsTruncation(t *testing.T) {
	c := credentials()
	q := TradeQuery{Symbol: "BTCUSDT", StartTime: 1700000000000, EndTime: 1700000200000}
	calls := 0
	client := newClient(tradeResponses(t, c, func(params url.Values) []tradeRow {
		calls++
		start, _ := strconv.ParseInt(params.Get("startTime"), 10, 64)
		end, _ := strconv.ParseInt(params.Get("endTime"), 10, 64)
		rows := []tradeRow{}
		for ms := start; ms <= end && len(rows) < tradePageSize; ms++ {
			rows = append(rows, tradeRowAt(ms, ms, ms))
		}
		return rows
	}))
	fills, truncated, err := client.RecoverTrades(context.Background(), c, q)
	if err != nil || !truncated || len(fills) == 0 || len(fills) > maxTradePages*tradePageSize || calls != maxTradePages {
		t.Fatal("truncation or request bound lost", err, len(fills), truncated, calls)
	}
}

func TestRecoverTradesSaturatedMillisecondIsExplicitlyTruncated(t *testing.T) {
	c := credentials()
	q := TradeQuery{Symbol: "BTCUSDT", StartTime: 1700000000000, EndTime: 1700000000001}
	rows := make([]tradeRow, tradePageSize)
	for i := range rows {
		rows[i] = tradeRowAt(int64(i+1), int64(i+1), q.StartTime)
	}
	fills, truncated, err := newClient(tradesTransport(t, c, [][]tradeRow{rows})).RecoverTrades(context.Background(), c, q)
	if err != nil || !truncated || len(fills) != tradePageSize {
		t.Fatal("saturated millisecond was skipped or claimed complete", err, truncated)
	}
}

func TestRecoverTradesEmptyWindowAndConflictingDuplicates(t *testing.T) {
	c := credentials()
	q := TradeQuery{Symbol: "BTCUSDT", StartTime: 1700000000000, EndTime: 1700000000001}
	fills, truncated, err := newClient(tradesTransport(t, c, [][]tradeRow{{}})).RecoverTrades(context.Background(), c, q)
	if err != nil || fills == nil || len(fills) != 0 || truncated {
		t.Fatal("empty response was not a successful explicit empty window", err)
	}
	one := tradeRowAt(1, 2, q.StartTime)
	changed := one
	changed.Price = "101"
	if fills, _, err := newClient(tradesTransport(t, c, [][]tradeRow{{one, changed}})).RecoverTrades(context.Background(), c, q); err == nil || fills != nil {
		t.Fatal("conflicting duplicate produced partial evidence")
	}
}

func TestValidTradeParams(t *testing.T) {
	valid := url.Values{"symbol": {"BTCUSDT"}, "startTime": {"1000"}, "endTime": {"2000"}, "limit": {"1000"}}
	if !validTradeParams(valid) {
		t.Fatal("valid params rejected")
	}
	withFrom := url.Values{"symbol": {"BTCUSDT"}, "startTime": {"1000"}, "endTime": {"2000"}, "limit": {"1000"}, "fromId": {"5"}}
	if validTradeParams(withFrom) {
		t.Fatal("incompatible fromId/time selectors accepted")
	}
	if !validTradeParams(url.Values{"symbol": {"BTCUSDT"}, "startTime": {"1000"}, "endTime": {"1000"}, "limit": {"1000"}}) {
		t.Fatal("inclusive one-millisecond query rejected")
	}
	for _, bad := range []url.Values{
		{},
		{"symbol": {"BTCUSDT"}},
		{"symbol": {"BTCUSDC"}, "startTime": {"1000"}, "endTime": {"2000"}, "limit": {"1000"}},
		{"symbol": {"BTCUSDT"}, "startTime": {"0"}, "endTime": {"2000"}, "limit": {"1000"}},
		{"symbol": {"BTCUSDT"}, "startTime": {"2000"}, "endTime": {"1000"}, "limit": {"1000"}},
		{"symbol": {"BTCUSDT"}, "startTime": {"1000"}, "endTime": {"2000"}, "limit": {"1001"}},
		{"symbol": {"BTCUSDT"}, "startTime": {"1000"}, "endTime": {"2000"}, "limit": {"1000"}, "signature": {"x"}},
		{"symbol": {"BTCUSDT", "ETHUSDT"}, "startTime": {"1000"}, "endTime": {"2000"}, "limit": {"1000"}},
	} {
		if validTradeParams(bad) {
			t.Fatal("invalid params accepted", bad)
		}
	}
}
