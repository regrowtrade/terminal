// Read-only user data stream transport: listenKey creation/keepalive, a single
// fixed WebSocket destination and bounded reconnection. It never submits,
// cancels or amends orders.
package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"regrow/engine/internal/vault"
)

var ErrStreamUnavailable = errors.New("flujo de eventos Binance no disponible; no se enviaron órdenes")

// userDataKey is the only non-GET surface in the adapter. It creates (POST),
// refreshes (PUT) or closes (DELETE) a read-only listenKey stream. No body is
// sent and the response is bounded; a missing/malformed key fails closed.
func (c *Client) userDataKey(ctx context.Context, credentials vault.Credentials, method string) (string, error) {
	if !credentials.Valid() {
		return "", ErrPermissions
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return "", ErrUnavailable
	}
	u, _ := url.Parse("https://fapi.binance.com/fapi/v1/listenKey")
	q := url.Values{"recvWindow": {"5000"}, "timestamp": {strconv.FormatInt(time.Now().UnixMilli(), 10)}}
	encoded := q.Encode()
	mac := hmac.New(sha256.New, []byte(credentials.APISecret))
	mac.Write([]byte(encoded))
	u.RawQuery = encoded + "&signature=" + hex.EncodeToString(mac.Sum(nil))
	request, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return "", ErrUnavailable
	}
	request.Header.Set("X-MBX-APIKEY", credentials.APIKey)
	response, err := c.http.Do(request)
	if err != nil {
		return "", ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 429 || response.StatusCode == 418 {
		c.cooldown(response.Header.Get("Retry-After"))
		return "", ErrRateLimited
	}
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return "", ErrPermissions
	}
	if response.StatusCode != 200 {
		return "", ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil || len(body) > 4096 {
		return "", ErrResponse
	}
	var key struct {
		ListenKey string `json:"listenKey"`
	}
	if json.Unmarshal(body, &key) != nil {
		return "", ErrResponse
	}
	if method == http.MethodPost {
		if key.ListenKey == "" || len(key.ListenKey) > 128 || strings.ContainsAny(key.ListenKey, "\x00\r\n") {
			return "", ErrResponse
		}
	}
	return key.ListenKey, nil
}

// streamConn is the subset of the WebSocket connection the stream needs. The
// production transport satisfies it; tests provide a scripted fake.
type streamConn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	SetReadDeadline(time.Time) error
	Close() error
}

type streamDialer interface {
	Dial(ctx context.Context, url string) (streamConn, *http.Response, error)
}

type gorillaDialer struct{}

func (gorillaDialer) Dial(ctx context.Context, url string) (streamConn, *http.Response, error) {
	return websocketDialer.DialContext(ctx, url, nil)
}

// No environment-selected proxy may observe the signed listenKey URL.
var websocketDialer = &websocket.Dialer{Proxy: nil, HandshakeTimeout: 10 * time.Second}

// Stream maintains one user data stream for one credential set, reconnecting
// with bounded backoff and refreshing the listenKey before it expires.
type Stream struct {
	client     *Client
	dial       streamDialer
	keepAlive  time.Duration
	minBackoff time.Duration
	maxBackoff time.Duration
	now        func() time.Time
}

func NewStream(client *Client) *Stream {
	return &Stream{
		client: client, dial: gorillaDialer{},
		keepAlive: 30 * time.Minute, minBackoff: time.Second, maxBackoff: 30 * time.Second,
		now: time.Now,
	}
}

// Run consumes private events until ctx is cancelled, invoking sink for each
// validated ORDER_TRADE_UPDATE or ACCOUNT_UPDATE. A revoked/unsafe key returns
// ErrPermissions without retrying; transient failures retry with backoff.
func (s *Stream) Run(ctx context.Context, credentials vault.Credentials, sink func(UserEvent)) error {
	if s == nil || s.client == nil || s.dial == nil || sink == nil {
		return ErrStreamUnavailable
	}
	backoff := s.minBackoff
	// Verify least-privilege permissions before opening the stream, retrying
	// transient rate limits so several accounts can share one client.
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.client.Verify(ctx, credentials); err == nil {
			break
		} else if errors.Is(err, ErrPermissions) {
			return err
		} else if !s.pause(ctx, backoff) {
			return ctx.Err()
		}
		backoff = s.grow(backoff)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.runOnce(ctx, credentials, sink)
		if err == nil {
			backoff = s.minBackoff
			continue
		}
		if errors.Is(err, ErrPermissions) {
			return err
		}
		if !s.pause(ctx, backoff) {
			return ctx.Err()
		}
		backoff = s.grow(backoff)
	}
}

func (s *Stream) pause(ctx context.Context, duration time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}

func (s *Stream) grow(duration time.Duration) time.Duration {
	duration *= 2
	if duration > s.maxBackoff {
		duration = s.maxBackoff
	}
	return duration
}

func (s *Stream) runOnce(ctx context.Context, credentials vault.Credentials, sink func(UserEvent)) error {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	listenKey, err := s.client.userDataKey(connCtx, credentials, http.MethodPost)
	if err != nil {
		return err
	}
	conn, _, err := s.dial.Dial(connCtx, "wss://fstream.binance.com/ws/"+listenKey)
	if err != nil {
		return err
	}
	// The watcher, keepalive and cleanup paths can all request a close; a single
	// sync.Once keeps the connection close race-free and idempotent.
	var closeOnce sync.Once
	closeConn := func() { closeOnce.Do(func() { _ = conn.Close() }) }
	// Close the connection when the context is cancelled so a blocked read
	// returns promptly on shutdown instead of waiting for the read deadline.
	go func() {
		<-connCtx.Done()
		closeConn()
	}()
	var keepalive sync.WaitGroup
	keepalive.Add(1)
	go func() {
		defer keepalive.Done()
		ticker := time.NewTicker(s.keepAlive)
		defer ticker.Stop()
		for {
			select {
			case <-connCtx.Done():
				return
			case <-ticker.C:
				if _, err := s.client.userDataKey(connCtx, credentials, http.MethodPut); err != nil {
					closeConn()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		closeConn()
		keepalive.Wait()
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer closeCancel()
		_, _ = s.client.userDataKey(closeCtx, credentials, http.MethodDelete)
	}()
	for {
		_ = conn.SetReadDeadline(s.now().Add(s.keepAlive + 15*time.Minute))
		_, message, err := conn.ReadMessage()
		if err != nil {
			return ErrStreamUnavailable
		}
		switch StreamEventType(message) {
		case EventListenKeyExpired:
			return nil
		case EventOrderTradeUpdate, EventAccountUpdate:
			event, err := ParseUserEvent(message)
			if err != nil {
				return err
			}
			sink(event)
		default:
			// Unknown event types are ignored, never treated as trades.
		}
	}
}
