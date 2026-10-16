package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"regrow/engine/internal/vault"
)

func jsonResponse(body string) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}

func checkStreamSignature(t *testing.T, r *http.Request, c vault.Credentials) {
	t.Helper()
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

type keyCalls struct{ post, put, del int }

func streamTransport(t *testing.T, c vault.Credentials, calls *keyCalls, key string) roundTrip {
	return roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" {
			t.Fatal("non-TLS stream request")
		}
		path := r.URL.Path
		if path == "/fapi/v1/time" {
			if r.Method != "GET" || r.URL.Host != "fapi.binance.com" || r.Header.Get("X-MBX-APIKEY") != "" || r.URL.RawQuery != "" {
				t.Fatal("credential leaked on public request")
			}
			return jsonResponse(fmt.Sprintf(`{"serverTime":%d}`, time.Now().UnixMilli()))
		}
		if path == "/sapi/v1/account/apiRestrictions" {
			if r.URL.Host != "api.binance.com" || r.Method != "GET" {
				t.Fatal("unexpected restrictions request")
			}
			checkStreamSignature(t, r, c)
			raw, _ := json.Marshal(safePermissions())
			return jsonResponse(string(raw))
		}
		if path == "/fapi/v1/listenKey" {
			if r.URL.Host != "fapi.binance.com" {
				t.Fatal("unexpected listenKey host")
			}
			checkStreamSignature(t, r, c)
			switch r.Method {
			case http.MethodPost:
				calls.post++
				return jsonResponse(fmt.Sprintf(`{"listenKey":%q}`, key))
			case http.MethodPut:
				calls.put++
				return jsonResponse(`{}`)
			case http.MethodDelete:
				calls.del++
				return jsonResponse(`{}`)
			}
		}
		t.Fatal("endpoint outside stream allowlist")
		return nil, nil
	})
}

type connStep struct {
	payload []byte
	err     error
}
type scriptConn struct {
	steps     []connStep
	index     int
	deadlines []time.Time
	closed    bool
}

func (c *scriptConn) ReadMessage() (int, []byte, error) {
	if c.index >= len(c.steps) {
		return 0, nil, io.EOF
	}
	step := c.steps[c.index]
	c.index++
	return 0, step.payload, step.err
}
func (c *scriptConn) WriteMessage(int, []byte) error { return nil }
func (c *scriptConn) SetReadDeadline(t time.Time) error {
	c.deadlines = append(c.deadlines, t)
	return nil
}
func (c *scriptConn) Close() error { c.closed = true; return nil }

type scriptDialer struct {
	conns []streamConn
	urls  []string
	index int
}

func (d *scriptDialer) Dial(_ context.Context, url string) (streamConn, *http.Response, error) {
	d.urls = append(d.urls, url)
	if d.index >= len(d.conns) {
		return nil, nil, errors.New("no more connections")
	}
	conn := d.conns[d.index]
	d.index++
	return conn, nil, nil
}

type blockingConn struct {
	read   chan []byte
	closed chan struct{}
	once   sync.Once
}

func (c *blockingConn) ReadMessage() (int, []byte, error) {
	select {
	case p := <-c.read:
		return 0, p, nil
	case <-c.closed:
		return 0, nil, io.EOF
	}
}
func (c *blockingConn) WriteMessage(int, []byte) error  { return nil }
func (c *blockingConn) SetReadDeadline(time.Time) error { return nil }
func (c *blockingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}
func (c *blockingConn) wasClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

const orderUpdateEvent = `{"e":"ORDER_TRADE_UPDATE","E":1568879465651,"T":1568879465650,"o":{"s":"BTCUSDT","c":"client-1","S":"SELL","o":"LIMIT","f":"GTC","q":"0.001","p":"9910","ap":"0","sp":"0","x":"TRADE","X":"PARTIALLY_FILLED","i":9007199254740993,"l":"0.0005","z":"0.0005","L":"9910.00","n":"0.0010","N":"USDT","T":1568879465650,"t":12345678,"b":"0","a":"0","m":false,"R":true,"wt":"CONTRACT_PRICE","ot":"LIMIT","ps":"SHORT","cp":false,"rp":"-0.1","pP":false,"si":0,"ss":0}}`
const accountUpdateEvent = `{"e":"ACCOUNT_UPDATE","E":1564745798939,"T":1564745798938,"a":{"m":"ORDER","B":[{"a":"USDT","wb":"122624.12345678","cw":"100.12345678","bc":"50.12345678"}],"P":[{"s":"BTCUSDT","pa":"0","ep":"0.00000","cr":"200","up":"0","mt":"isolated","iw":"0","ps":"LONG"}]}}`

func TestListenKeyIsSignedAndAllowlisted(t *testing.T) {
	c := credentials()
	calls := &keyCalls{}
	client := newClient(streamTransport(t, c, calls, "stream-key-1"))
	key, err := client.userDataKey(context.Background(), c, http.MethodPost)
	if err != nil || key != "stream-key-1" || calls.post != 1 {
		t.Fatal("listenKey POST failed", err)
	}
	if _, err := client.userDataKey(context.Background(), c, http.MethodPut); err != nil || calls.put != 1 {
		t.Fatal("listenKey keepalive failed")
	}
	if _, err := client.userDataKey(context.Background(), c, http.MethodDelete); err != nil || calls.del != 1 {
		t.Fatal("listenKey close failed")
	}
	if _, err := client.userDataKey(context.Background(), c, http.MethodGet); !errors.Is(err, ErrUnavailable) {
		t.Fatal("GET accepted for listenKey")
	}
	if _, err := client.userDataKey(context.Background(), vault.Credentials{}, http.MethodPost); !errors.Is(err, ErrPermissions) {
		t.Fatal("invalid key accepted")
	}
}

func TestListenKeyRejectsMalformedOrUnsafe(t *testing.T) {
	for name, body := range map[string]string{"missing": `{}`, "empty": `{"listenKey":""}`, "control": `{"listenKey":"a\nb"}`, "oversized": `{"listenKey":"` + strings.Repeat("k", 200) + `"}`} {
		t.Run(name, func(t *testing.T) {
			c := credentials()
			client := newClient(roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.Path != "/fapi/v1/listenKey" {
					t.Fatal("unexpected request")
				}
				checkStreamSignature(t, r, c)
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			if _, err := client.userDataKey(context.Background(), c, http.MethodPost); !errors.Is(err, ErrResponse) {
				t.Fatal("malformed listenKey accepted")
			}
		})
	}
	denied := credentials()
	client := newClient(roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("private"))}, nil
	}))
	if _, err := client.userDataKey(context.Background(), denied, http.MethodPost); !errors.Is(err, ErrPermissions) {
		t.Fatal("403 did not map to permissions")
	}
}

func TestParseOrderTradeUpdate(t *testing.T) {
	event, err := ParseUserEvent([]byte(orderUpdateEvent))
	if err != nil {
		t.Fatal(err)
	}
	o := event.OrderUpdate
	if event.Type != EventOrderTradeUpdate || o.OrderID != "9007199254740993" || o.TradeID != "12345678" ||
		o.Quantity != "0.001" || o.LastFilledQty != "0.0005" || o.CumulativeQty != "0.0005" ||
		o.RealizedProfit != "-0.1" || !o.ReduceOnly || o.PositionSide != "SHORT" || o.Symbol != "BTCUSDT" {
		t.Fatal("order update lost identity or decimals")
	}
	raw, _ := json.Marshal(event)
	if strings.Contains(string(raw), "APIKey") || strings.Contains(string(raw), "APISecret") {
		t.Fatal("secret in event")
	}
	for name, mutate := range map[string]string{
		"bad symbol":     strings.Replace(orderUpdateEvent, `"BTCUSDT"`, `"BTCUSDC"`, 1),
		"bad order id":   strings.Replace(orderUpdateEvent, "9007199254740993", "0", 1),
		"bad quantity":   strings.Replace(orderUpdateEvent, `"q":"0.001"`, `"q":"NaN"`, 1),
		"unknown status": strings.Replace(orderUpdateEvent, `"X":"PARTIALLY_FILLED"`, `"X":"MYSTERY"`, 1),
		"unknown exec":   strings.Replace(orderUpdateEvent, `"x":"TRADE"`, `"x":"LOST"`, 1),
		"bad commission": strings.Replace(orderUpdateEvent, `"n":"0.0010"`, `"n":"-1"`, 1),
		"bad trade id":   strings.Replace(orderUpdateEvent, `"t":12345678`, `"t":1.5`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseUserEvent([]byte(mutate)); !errors.Is(err, ErrResponse) {
				t.Fatal("malformed order update accepted")
			}
		})
	}
}

func TestParseAccountUpdate(t *testing.T) {
	event, err := ParseUserEvent([]byte(accountUpdateEvent))
	if err != nil {
		t.Fatal(err)
	}
	a := event.AccountUpdate
	if a.Reason != "ORDER" || len(a.Balances) != 1 || a.Balances[0].WalletBalance != "122624.12345678" ||
		len(a.Positions) != 1 || a.Positions[0].Symbol != "BTCUSDT" || a.Positions[0].MarginType != "isolated" {
		t.Fatal("account update lost balances or positions")
	}
	for name, mutate := range map[string]string{
		"bad symbol":     strings.Replace(accountUpdateEvent, `"BTCUSDT"`, `"BTCUSDC"`, 1),
		"bad balance":    strings.Replace(accountUpdateEvent, `"wb":"122624.12345678"`, `"wb":"NaN"`, 1),
		"unknown margin": strings.Replace(accountUpdateEvent, `"mt":"isolated"`, `"mt":"hedged"`, 1),
		"bad side":       strings.Replace(accountUpdateEvent, `"ps":"LONG"`, `"ps":"UP"`, 1),
		"bad reason":     strings.Replace(accountUpdateEvent, `"m":"ORDER"`, `"m":"BORROW"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseUserEvent([]byte(mutate)); !errors.Is(err, ErrResponse) {
				t.Fatal("malformed account update accepted")
			}
		})
	}
}

func TestStreamEventTypeAndUnknownIgnored(t *testing.T) {
	if got := StreamEventType([]byte(`{"e":"MARGIN_CALL","E":1}`)); got != "MARGIN_CALL" {
		t.Fatalf("unknown event type not reported: %q", got)
	}
	if got := StreamEventType([]byte(orderUpdateEvent)); got != EventOrderTradeUpdate {
		t.Fatal("order event type not reported")
	}
	if _, err := ParseUserEvent([]byte(`{"e":"MARGIN_CALL","E":1}`)); !errors.Is(err, ErrResponse) {
		t.Fatal("unknown event should not parse as a private event")
	}
}

func TestDedupKeyStableAndDistinct(t *testing.T) {
	first, _ := ParseUserEvent([]byte(orderUpdateEvent))
	again, _ := ParseUserEvent([]byte(orderUpdateEvent))
	if first.DedupKey() != again.DedupKey() || first.DedupKey() == "" {
		t.Fatal("dedup key not stable")
	}
	changed := orderUpdateEvent
	changed = strings.Replace(changed, `"E":1568879465651`, `"E":1568879465652`, 1)
	other, _ := ParseUserEvent([]byte(changed))
	if first.DedupKey() == other.DedupKey() {
		t.Fatal("dedup key collapsed distinct events")
	}
	account, _ := ParseUserEvent([]byte(accountUpdateEvent))
	if account.DedupKey() == first.DedupKey() || account.DedupKey() == "" {
		t.Fatal("account/order dedup keys not distinct")
	}
}

func stream(c *Client, dialer streamDialer) *Stream {
	return &Stream{client: c, dial: dialer, keepAlive: time.Hour, minBackoff: time.Millisecond, maxBackoff: time.Millisecond, now: time.Now}
}

func TestRunReconnectsOnListenKeyExpiry(t *testing.T) {
	c := credentials()
	calls := &keyCalls{}
	client := newClient(streamTransport(t, c, calls, "stream-key-1"))
	expired := &scriptConn{steps: []connStep{{payload: []byte(`{"e":"listenKeyExpired","E":1}`)}}}
	delivering := &scriptConn{steps: []connStep{{payload: []byte(orderUpdateEvent)}}}
	dialer := &scriptDialer{conns: []streamConn{expired, delivering}}
	s := stream(client, dialer)
	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	var received []UserEvent
	sink := func(e UserEvent) { mu.Lock(); received = append(received, e); mu.Unlock(); cancel() }
	if err := s.Run(ctx, c, sink); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal("unexpected run error", err)
	}
	if calls.post != 2 || calls.del != 2 || len(dialer.urls) != 2 || dialer.urls[1] != "wss://fstream.binance.com/ws/stream-key-1" {
		t.Fatal("expiry did not re-establish the stream", calls)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 || received[0].OrderUpdate.OrderID != "9007199254740993" {
		t.Fatal("validated event not delivered")
	}
}

func TestRunStopsOnUnsafeKeyWithoutRetry(t *testing.T) {
	c := credentials()
	client := newClient(roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/fapi/v1/time" {
			return jsonResponse(fmt.Sprintf(`{"serverTime":%d}`, time.Now().UnixMilli()))
		}
		return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("private"))}, nil
	}))
	dialer := &scriptDialer{}
	s := stream(client, dialer)
	if err := s.Run(context.Background(), c, func(UserEvent) {}); !errors.Is(err, ErrPermissions) {
		t.Fatal("unsafe key did not fail closed", err)
	}
	if len(dialer.urls) != 0 {
		t.Fatal("stream dialed with an unsafe key")
	}
}

func TestRunOnceIgnoresUnknownAndFailsClosedOnMalformed(t *testing.T) {
	c := credentials()
	calls := &keyCalls{}
	client := newClient(streamTransport(t, c, calls, "stream-key-1"))
	unknown := &scriptConn{steps: []connStep{
		{payload: []byte(`{"e":"MARGIN_CALL","E":1}`)},
		{payload: []byte(orderUpdateEvent)},
		{payload: []byte(`{"e":"listenKeyExpired","E":2}`)},
	}}
	var received []UserEvent
	s := stream(client, &scriptDialer{conns: []streamConn{unknown}})
	if err := s.runOnce(context.Background(), c, func(e UserEvent) { received = append(received, e) }); err != nil {
		t.Fatal("unknown event broke the stream", err)
	}
	if len(received) != 1 || received[0].Type != EventOrderTradeUpdate {
		t.Fatal("unknown event was misread as trade")
	}
	malformed := &scriptConn{steps: []connStep{{payload: []byte(strings.Replace(orderUpdateEvent, `"q":"0.001"`, `"q":"NaN"`, 1))}}}
	if err := stream(client, &scriptDialer{conns: []streamConn{malformed}}).runOnce(context.Background(), c, func(UserEvent) {}); !errors.Is(err, ErrResponse) {
		t.Fatal("malformed event did not fail closed")
	}
	dead := &scriptConn{steps: []connStep{{err: io.EOF}}}
	if err := stream(client, &scriptDialer{conns: []streamConn{dead}}).runOnce(context.Background(), c, func(UserEvent) {}); !errors.Is(err, ErrStreamUnavailable) {
		t.Fatal("read failure did not surface as stream unavailable")
	}
}

func TestRunUnblocksBlockedReadOnCancellation(t *testing.T) {
	c := credentials()
	calls := &keyCalls{}
	client := newClient(streamTransport(t, c, calls, "stream-key-1"))
	conn := &blockingConn{read: make(chan []byte), closed: make(chan struct{})}
	dialer := &scriptDialer{conns: []streamConn{conn}}
	s := stream(client, dialer)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, c, func(UserEvent) {}) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(dialer.urls) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(dialer.urls) == 0 {
		t.Fatal("stream never dialed")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal("unexpected shutdown error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked read did not unblock on cancellation")
	}
	if !conn.wasClosed() {
		t.Fatal("stream did not close the connection on cancellation")
	}
}
