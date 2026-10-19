// Package binance exposes only a fixed allowlist of GET endpoints. There is no
// generic method/path request API, order submission, transfer or listenKey call.
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

	"regrow/engine/internal/vault"
)

var ErrUnavailable = errors.New("Binance no disponible; no se enviaron órdenes")
var ErrPermissions = errors.New("la clave debe permitir lectura, restringir IP y no permitir trading, retiros ni transferencias")
var ErrRateLimited = errors.New("lectura limitada temporalmente; espera antes de reintentar")
var ErrResponse = errors.New("Binance devolvió datos incompletos o no compatibles")

type Client struct {
	http   *http.Client
	mu     sync.Mutex
	next   time.Time
	active bool
}

func New() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // No environment-selected proxy may receive signed URLs.
	return newClient(transport)
}
func newClient(transport http.RoundTripper) *Client {
	return &Client{http: &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) begin() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active || time.Now().Before(c.next) {
		return ErrRateLimited
	}
	c.active = true
	c.next = time.Now().Add(10 * time.Second)
	return nil
}
func (c *Client) end() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = false
}
func (c *Client) cooldown(header string) {
	duration := time.Minute
	if seconds, err := strconv.ParseInt(header, 10, 32); err == nil && seconds > 60 {
		duration = time.Duration(seconds) * time.Second
	} else if until, err := http.ParseTime(header); err == nil && time.Until(until) > duration {
		duration = time.Until(until)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	until := time.Now().Add(duration)
	if until.After(c.next) {
		c.next = until
	}
}
func (c *Client) get(ctx context.Context, path string, credentials *vault.Credentials, timestamp int64, target any) error {
	origin := "https://fapi.binance.com"
	public := false
	max := 2 * 1024 * 1024
	switch path {
	case "/fapi/v1/time":
		public = true
	case "/fapi/v1/exchangeInfo":
		public = true
		max = 8 * 1024 * 1024
	case "/fapi/v3/account", "/fapi/v3/positionRisk", "/fapi/v1/openOrders", "/fapi/v1/openAlgoOrders":
	case "/sapi/v1/account/apiRestrictions":
		origin = "https://api.binance.com"
	default:
		return ErrUnavailable
	}
	// Public metadata never receives keys; private reads always require them.
	if public != (credentials == nil) {
		return ErrPermissions
	}
	u, _ := url.Parse(origin + path)
	if credentials != nil {
		if !credentials.Valid() {
			return ErrPermissions
		}
		q := url.Values{"recvWindow": {"5000"}, "timestamp": {strconv.FormatInt(timestamp, 10)}}
		encoded := q.Encode()
		mac := hmac.New(sha256.New, []byte(credentials.APISecret))
		mac.Write([]byte(encoded))
		u.RawQuery = encoded + "&signature=" + hex.EncodeToString(mac.Sum(nil))
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ErrUnavailable
	}
	if credentials != nil {
		r.Header.Set("X-MBX-APIKEY", credentials.APIKey)
	}
	response, err := c.http.Do(r)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 429 || response.StatusCode == 418 {
		c.cooldown(response.Header.Get("Retry-After"))
		return ErrRateLimited
	}
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return ErrPermissions
	}
	if response.StatusCode != 200 {
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(max)+1))
	if err != nil || len(body) > max {
		return ErrResponse
	}
	if json.Unmarshal(body, target) != nil {
		return ErrResponse
	}
	return nil
}

type permissions struct {
	Reading   *bool `json:"enableReading"`
	IP        *bool `json:"ipRestrict"`
	Withdraw  *bool `json:"enableWithdrawals"`
	Futures   *bool `json:"enableFutures"`
	Spot      *bool `json:"enableSpotAndMarginTrading"`
	Margin    *bool `json:"enableMargin"`
	Transfer  *bool `json:"enableInternalTransfer"`
	Universal *bool `json:"permitsUniversalTransfer"`
	Options   bool  `json:"enableVanillaOptions"`
	FixTrade  bool  `json:"enableFixApiTrade"`
	Portfolio bool  `json:"enablePortfolioMarginTrading"`
}

func (p permissions) safe() bool {
	if p.Reading == nil || !*p.Reading || p.IP == nil || !*p.IP || p.Options || p.FixTrade || p.Portfolio {
		return false
	}
	for _, flag := range []*bool{p.Withdraw, p.Futures, p.Spot, p.Margin, p.Transfer, p.Universal} {
		if flag == nil || *flag {
			return false
		}
	}
	return true
}
func (c *Client) checked(ctx context.Context, credentials vault.Credentials) (func() int64, error) {
	if !credentials.Valid() {
		return nil, ErrPermissions
	}
	start := time.Now()
	var clock struct {
		ServerTime int64 `json:"serverTime"`
	}
	if err := c.get(ctx, "/fapi/v1/time", nil, 0, &clock); err != nil {
		return nil, err
	}
	received := time.Now()
	if clock.ServerTime <= 0 || received.Sub(start) > 2*time.Second {
		return nil, ErrResponse
	}
	now := func() int64 { return clock.ServerTime + time.Since(received).Milliseconds() }
	var raw json.RawMessage
	if err := c.get(ctx, "/sapi/v1/account/apiRestrictions", &credentials, now(), &raw); err != nil {
		return nil, err
	}
	var p permissions
	var flags map[string]json.RawMessage
	if json.Unmarshal(raw, &p) != nil || json.Unmarshal(raw, &flags) != nil {
		return nil, ErrPermissions
	}
	// A future enabled capability must not silently bypass the known flags.
	for name, value := range flags {
		if name == "enableReading" || name == "enableFixReadOnly" {
			continue
		}
		if strings.HasPrefix(name, "enable") || strings.HasPrefix(name, "permits") {
			var enabled bool
			if strings.TrimSpace(string(value)) == "null" || json.Unmarshal(value, &enabled) != nil || enabled {
				return nil, ErrPermissions
			}
		}
	}
	if !p.safe() {
		return nil, ErrPermissions
	}
	return now, nil
}
func (c *Client) Verify(ctx context.Context, credentials vault.Credentials) error {
	if err := c.begin(); err != nil {
		return err
	}
	defer c.end()
	_, err := c.checked(ctx, credentials)
	return err
}

type Asset struct {
	Asset            string `json:"asset"`
	WalletBalance    string `json:"walletBalance"`
	UnrealizedProfit string `json:"unrealizedProfit"`
	AvailableBalance string `json:"availableBalance"`
}
type Position struct {
	Symbol           string `json:"symbol"`
	Side             string `json:"positionSide"`
	Quantity         string `json:"positionAmt"`
	EntryPrice       string `json:"entryPrice"`
	MarkPrice        string `json:"markPrice"`
	UnrealizedProfit string `json:"unRealizedProfit"`
	LiquidationPrice string `json:"liquidationPrice"`
}
type Snapshot struct {
	Source         string       `json:"source"`
	Mode           string       `json:"mode"`
	CapturedAt     time.Time    `json:"capturedAt"`
	CapturedAtMs   int64        `json:"capturedAtMs"`
	StartedAt      time.Time    `json:"startedAt"`
	Consistency    string       `json:"consistency"`
	ExecutionReady bool         `json:"executionReady"`
	Assets         []Asset      `json:"assets"`
	Positions      []Position   `json:"positions"`
	OpenOrders     []Order      `json:"openOrders"`
	AlgoOrders     []AlgoOrder  `json:"algoOrders"`
	Instruments    []Instrument `json:"instruments"`
}

func (c *Client) Snapshot(ctx context.Context, credentials vault.Credentials) (Snapshot, error) {
	var result Snapshot
	if err := c.begin(); err != nil {
		return result, err
	}
	defer c.end()
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	now, err := c.checked(ctx, credentials)
	if err != nil {
		return result, err
	}
	var account struct {
		Assets []Asset `json:"assets"`
	}
	if err := c.get(ctx, "/fapi/v3/account", &credentials, now(), &account); err != nil {
		return result, err
	}
	var positions []Position
	if err := c.get(ctx, "/fapi/v3/positionRisk", &credentials, now(), &positions); err != nil {
		return result, err
	}
	if err := validate(account.Assets, positions); err != nil {
		return result, err
	}
	orders, algos, err := c.orders(ctx, credentials, now)
	if err != nil {
		return result, err
	}
	instruments, err := c.instruments(ctx)
	if err != nil {
		return result, err
	}
	known := make(map[string]bool, len(instruments))
	for _, i := range instruments {
		known[i.Symbol] = true
	}
	for _, p := range positions {
		if !known[p.Symbol] {
			return result, ErrResponse
		}
	}
	for _, o := range orders {
		if !known[o.Symbol] {
			return result, ErrResponse
		}
	}
	for _, o := range algos {
		if !known[o.Symbol] {
			return result, ErrResponse
		}
	}
	capturedAt := time.Now().UTC()
	return Snapshot{Source: "binance-production", Mode: "read_only", StartedAt: started, CapturedAt: capturedAt, CapturedAtMs: now(), Consistency: "rest_non_atomic", ExecutionReady: false, Assets: account.Assets, Positions: positions, OpenOrders: orders, AlgoOrders: algos, Instruments: instruments}, nil
}
