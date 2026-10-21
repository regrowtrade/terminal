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
	"strconv"
	"strings"
	"testing"
	"time"

	"regrow/engine/internal/vault"
)

func tradeRowAt(id, orderID, tms int64) tradeRow {
	return tradeRow{ID: json.Number(strconv.FormatInt(id, 10)), OrderID: json.Number(strconv.FormatInt(orderID, 10)), Symbol: "BTCUSDT", Side: "BUY", PositionSide: "LONG", Price: "100", Quantity: "1", Commission: "-0.1", CommissionAsset: "USDT", Time: tms}
}

func fullTradePage(startID, baseTime int64) []tradeRow {
	rows := make([]tradeRow, tradePageSize)
	for i := 0; i < tradePageSize; i++ {
		rows[i] = tradeRowAt(startID-int64(i), startID-int64(i)*2, baseTime-int64(i))
	}
	return rows
}

func tradesTransport(t *testing.T, c vault.Credentials, pages [][]tradeRow) roundTrip {
	t.Helper()
	idx := 0
	return roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/fapi/v1/time":
			return jsonResponse(fmt.Sprintf(`{"serverTime":%d}`, time.Now().UnixMilli()))
		case "/sapi/v1/account/apiRestrictions":
			raw, _ := json.Marshal(safePermissions())
			return jsonResponse(string(raw))
		case "/fapi/v1/userTrades":
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
			if idx >= len(pages) {
				return jsonResponse("[]")
			}
			body, _ := json.Marshal(pages[idx])
			idx++
			return jsonResponse(string(body))
		default:
			t.Fatal("unexpected path " + r.URL.Path)
			return nil, nil
		}
	})
}

type vaultCredentials = vault.Credentials

func TestRecoverTradesPaginatesValidatesAndDeduplicates(t *testing.T) {
	c := credentials()
	q := TradeQuery{Symbol: "BTCUSDT", StartTime: 1700000000000, EndTime: 1700000086400}
	page1 := fullTradePage(5000, 1700000086000)
	page2 := []tradeRow{tradeRowAt(3999, 3999, 1700000085000), tradeRowAt(3998, 3998, 1700000084999), tradeRowAt(3999, 3999, 1700000085000)}
	client := newClient(tradesTransport(t, c, [][]tradeRow{page1, page2}))
	fills, truncated, err := client.RecoverTrades(context.Background(), c, q)
	if err != nil || truncated || len(fills) != tradePageSize+2 {
		t.Fatal("recovery lost fills", err, len(fills), truncated)
	}
	for i := 1; i < len(fills); i++ {
		if fills[i-1].EventTime > fills[i].EventTime {
			t.Fatal("fills not ascending")
		}
	}
	if fills[0].TradeID != "3998" || fills[len(fills)-1].TradeID != "5000" {
		t.Fatal("boundary ordering wrong")
	}
	for _, f := range fills {
		if f.EventTime != f.TransactionTime || f.Symbol != "BTCUSDT" {
			t.Fatal("recovered fill lost clock or symbol semantics")
		}
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
		func(r *tradeRow) { r.Side = "HOLD" },
		func(r *tradeRow) { r.PositionSide = "NEUTRAL" },
		func(r *tradeRow) { r.Price = "0" },
		func(r *tradeRow) { r.Quantity = "-1" },
		func(r *tradeRow) { r.Commission = "NaN" },
		func(r *tradeRow) { r.CommissionAsset = strings.Repeat("a", 21) },
		func(r *tradeRow) { r.Time = 0 },
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
	q := TradeQuery{Symbol: "BTCUSDT", StartTime: 1700000000000, EndTime: 1700086400000}
	pages := make([][]tradeRow, maxTradePages)
	for p := range pages {
		pages[p] = fullTradePage(int64(100000-p*1000), 1700086399999-int64(p*1000))
	}
	client := newClient(tradesTransport(t, c, pages))
	fills, truncated, err := client.RecoverTrades(context.Background(), c, q)
	if err != nil || !truncated || len(fills) != maxTradePages*tradePageSize {
		t.Fatal("truncation not reported", err, len(fills), truncated)
	}
}

func TestValidTradeParams(t *testing.T) {
	valid := url.Values{"symbol": {"BTCUSDT"}, "startTime": {"1000"}, "endTime": {"2000"}, "limit": {"1000"}}
	if !validTradeParams(valid) {
		t.Fatal("valid params rejected")
	}
	withFrom := url.Values{"symbol": {"BTCUSDT"}, "startTime": {"1000"}, "endTime": {"2000"}, "limit": {"1000"}, "fromId": {"5"}}
	if !validTradeParams(withFrom) {
		t.Fatal("fromId params rejected")
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
