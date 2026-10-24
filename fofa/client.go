// Package fofa is a small, dependency-free client for the FOFA API v1.
//
// It covers the four endpoints the agent needs:
//
//	GET /api/v1/info/my        account + remaining quota (0 query cost)
//	GET /api/v1/search/all     page-based search (start/size)
//	GET /api/v1/search/next    cursor-based search (search-after, large pulls)
//	GET /api/v1/search/stats   aggregation / result-set sizing
//	GET /api/v1/host/{host}    host (IP/domain) aggregation
//
// The client never logs or returns the API key. All requests share one rate
// limiter so concurrent agents cannot trip FOFA's frequency limit.
package fofa

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the mainland endpoint used by the official SDK.
	DefaultBaseURL = "https://fofa.info"
	// DefaultLang selects the response language for errmsg/field labels.
	DefaultLang = "zh-CN"

	defaultTimeout = 60 * time.Second
	maxRetries     = 3
	maxBodyBytes   = 64 << 20 // 64 MiB: a 10k-row page with banner fields is large
)

// Client talks to one FOFA account. Safe for concurrent use.
type Client struct {
	key     string
	baseURL string
	lang    string
	hc      *http.Client
	lim     *limiter

	// retryDelay is the base backoff; tests override it.
	retryDelay time.Duration
}

// New builds a client. An empty baseURL falls back to DefaultBaseURL.
func New(key, baseURL string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		key:        strings.TrimSpace(key),
		baseURL:    baseURL,
		lang:       DefaultLang,
		hc:         &http.Client{Timeout: defaultTimeout},
		lim:        newLimiter(1.5),
		retryDelay: time.Second,
	}
}

// SetRatePerSecond changes the shared request rate limit (requests/second).
// Values <= 0 are ignored.
func (c *Client) SetRatePerSecond(n float64) {
	if n > 0 {
		c.lim.setRate(n)
	}
}

// SetLang changes the API language (zh-CN / en).
func (c *Client) SetLang(lang string) {
	if strings.TrimSpace(lang) != "" {
		c.lang = strings.TrimSpace(lang)
	}
}

// SetHTTPClient replaces the underlying HTTP client (used by tests / custom
// proxies). A nil client is ignored.
func (c *Client) SetHTTPClient(hc *http.Client) {
	if hc != nil {
		c.hc = hc
	}
}

// Account returns the account profile and remaining quota. This endpoint costs
// no query quota.
func (c *Client) Account(ctx context.Context) (*Account, error) {
	var out Account
	if err := c.do(ctx, "/api/v1/info/my", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Search runs a page-based query (/api/v1/search/all).
func (c *Client) Search(ctx context.Context, query string, fields []string, page, size int) (*SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query 不能为空")
	}
	if page < 1 {
		page = 1
	}
	size = clampSize(size)
	q := url.Values{}
	q.Set("qbase64", EncodeQuery(query))
	q.Set("page", strconv.Itoa(page))
	q.Set("size", strconv.Itoa(size))
	q.Set("fields", normalizeFields(fields))

	var out SearchResult
	if err := c.do(ctx, "/api/v1/search/all", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SearchNext runs one cursor page (/api/v1/search/next). Pass next="" for the
// first page. size max is 10000.
func (c *Client) SearchNext(ctx context.Context, query string, fields []string, size int, next string, full bool) (*SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query 不能为空")
	}
	size = clampSize(size)
	q := url.Values{}
	q.Set("qbase64", EncodeQuery(query))
	q.Set("size", strconv.Itoa(size))
	q.Set("fields", normalizeFields(fields))
	q.Set("full", strconv.FormatBool(full))
	if strings.TrimSpace(next) != "" {
		q.Set("next", strings.TrimSpace(next))
	}

	var out SearchResult
	if err := c.do(ctx, "/api/v1/search/next", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Stats aggregates the result set (/api/v1/search/stats). It is the cheapest
// way to size a query before pulling rows.
func (c *Client) Stats(ctx context.Context, query string, fields []string, size int) (*StatsResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query 不能为空")
	}
	if size <= 0 {
		size = 5
	}
	q := url.Values{}
	q.Set("qbase64", EncodeQuery(query))
	q.Set("size", strconv.Itoa(size))
	q.Set("fields", normalizeFields(fields))

	var out StatsResult
	if err := c.do(ctx, "/api/v1/search/stats", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Host returns the host aggregation for an IP or domain.
func (c *Client) Host(ctx context.Context, host string, detail bool) (*HostInfo, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("host 不能为空")
	}
	q := url.Values{}
	q.Set("detail", strconv.FormatBool(detail))

	var out HostInfo
	if err := c.do(ctx, "/api/v1/host/"+url.PathEscape(host), q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EncodeQuery base64-encodes a FOFA query string (standard alphabet, as the
// official SDK does).
func EncodeQuery(q string) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(q)))
}

func clampSize(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 10000 {
		return 10000
	}
	return n
}

var fieldRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

// normalizeFields drops anything that is not a plausible field name. An empty
// list falls back to the default host/ip/port trio.
func normalizeFields(fields []string) string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" || !fieldRe.MatchString(f) {
			continue
		}
		out = append(out, f)
		if len(out) >= 24 {
			break
		}
	}
	if len(out) == 0 {
		return "host,ip,port"
	}
	return strings.Join(out, ",")
}

// envelope is the error wrapper every FOFA response carries.
type envelope struct {
	Error  bool   `json:"error"`
	ErrMsg string `json:"errmsg"`
	Code   int    `json:"code"`
}

func (c *Client) do(ctx context.Context, path string, params url.Values, out any) error {
	if c.key == "" {
		return ErrNoKey
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("key", c.key)
	params.Set("lang", c.lang)
	endpoint := c.baseURL + path + "?" + params.Encode()

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := c.lim.wait(ctx); err != nil {
			return err
		}
		body, status, err := c.get(ctx, endpoint)
		if err != nil {
			lastErr = err
			if !sleepCtx(ctx, c.backoff(attempt)) {
				return ctx.Err()
			}
			continue
		}

		var env envelope
		_ = json.Unmarshal(body, &env)
		if env.Error {
			apiErr := &APIError{Code: parseErrCode(env), Msg: strings.TrimSpace(env.ErrMsg), HTTP: status}
			if apiErr.Code == 0 {
				apiErr.Code = codeFromMessage(env.ErrMsg)
			}
			lastErr = apiErr
			if apiErr.Retryable() && attempt < maxRetries {
				if !sleepCtx(ctx, c.backoff(attempt)) {
					return ctx.Err()
				}
				continue
			}
			return apiErr
		}
		if status >= 500 {
			lastErr = &APIError{Code: 0, Msg: fmt.Sprintf("HTTP %d", status), HTTP: status}
			if attempt < maxRetries {
				if !sleepCtx(ctx, c.backoff(attempt)) {
					return ctx.Err()
				}
				continue
			}
			return lastErr
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("解析 FOFA 响应失败: %w", err)
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("FOFA 请求失败")
	}
	return lastErr
}

func (c *Client) get(ctx context.Context, endpoint string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "artex-fofa/1.0")
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return body, resp.StatusCode, nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return body, resp.StatusCode, nil
	}
	return body, resp.StatusCode, nil
}

func (c *Client) backoff(attempt int) time.Duration {
	d := c.retryDelay << attempt
	if d <= 0 {
		d = time.Second
	}
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// limiter is a tiny interval-based limiter shared by all calls on a client.
type limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

func newLimiter(perSec float64) *limiter {
	l := &limiter{}
	l.setRate(perSec)
	return l
}

func (l *limiter) setRate(perSec float64) {
	if perSec <= 0 {
		perSec = 1
	}
	l.mu.Lock()
	l.interval = time.Duration(float64(time.Second) / perSec)
	l.mu.Unlock()
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	d := l.next.Sub(now)
	l.next = l.next.Add(l.interval)
	l.mu.Unlock()
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
