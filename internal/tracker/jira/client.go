// Package jira is Maestro's Jira REST client.
//
// One Client talks to one site. It owns authentication, the API version
// (Cloud v3, Data Center v2), retries with backoff, adaptive concurrency for
// rate limits, pagination and redacted logging. Operations live in the other
// files of this package, one function per REST operation.
package jira

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Deployment string

const (
	Cloud      Deployment = "cloud"
	DataCenter Deployment = "datacenter"
)

const (
	maxResponseBytes = 50 << 20 // a guard, not a limit Jira normally reaches
	maxErrorBytes    = 64 << 10
)

// RetryPolicy controls retries and backoff.
type RetryPolicy struct {
	MaxRateLimitRetries int           // 429, any method (the request was not processed)
	MaxServerRetries    int           // 5xx and network errors, GET/HEAD only
	BaseDelay           time.Duration // first backoff step
	MaxDelay            time.Duration // cap for backoff and Retry-After
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxRateLimitRetries: 5, MaxServerRetries: 3, BaseDelay: time.Second, MaxDelay: 60 * time.Second}
}

type Options struct {
	// BaseURL is the site root (https://acme.atlassian.net, or the Data Center
	// URL with its context path), or https://api.atlassian.com/ex/jira/{cloudId}
	// for scoped tokens and OAuth.
	BaseURL    string
	Deployment Deployment
	Auth       Authenticator

	HTTPClient     *http.Client // nil: NewHTTPClient(NetOptions{})
	UserAgent      string       // default "Maestro/dev"
	MaxConcurrency int          // default 4
	Retry          RetryPolicy  // zero: DefaultRetryPolicy()
	Logger         *slog.Logger // nil: discard

	sleep func(context.Context, time.Duration) error // tests
	now   func() time.Time                           // tests
}

type Client struct {
	base       *url.URL
	deployment Deployment
	auth       Authenticator
	http       *http.Client
	userAgent  string
	retry      RetryPolicy
	log        *slog.Logger
	limit      *limiter
	sleep      func(context.Context, time.Duration) error
	now        func() time.Time
}

func New(o Options) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(o.BaseURL, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("jira: invalid base URL %q", o.BaseURL)
	}
	if o.Deployment != Cloud && o.Deployment != DataCenter {
		return nil, fmt.Errorf("jira: deployment must be %q or %q", Cloud, DataCenter)
	}
	if o.Auth == nil {
		return nil, errors.New("jira: an Authenticator is required")
	}
	c := &Client{
		base:       u,
		deployment: o.Deployment,
		auth:       o.Auth,
		http:       o.HTTPClient,
		userAgent:  o.UserAgent,
		retry:      o.Retry,
		log:        o.Logger,
		sleep:      o.sleep,
		now:        o.now,
	}
	if c.http == nil {
		if c.http, err = NewHTTPClient(NetOptions{}); err != nil {
			return nil, err
		}
	}
	if c.userAgent == "" {
		c.userAgent = "Maestro/dev"
	}
	if c.retry == (RetryPolicy{}) {
		c.retry = DefaultRetryPolicy()
	}
	if c.log == nil {
		c.log = slog.New(slog.DiscardHandler)
	}
	if c.sleep == nil {
		c.sleep = sleepCtx
	}
	if c.now == nil {
		c.now = time.Now
	}
	n := o.MaxConcurrency
	if n <= 0 {
		n = 4
	}
	c.limit = newLimiter(n, c.now)
	return c, nil
}

func (c *Client) Deployment() Deployment { return c.deployment }

// BaseURL is the site URL without a trailing slash.
func (c *Client) BaseURL() string { return c.base.String() }

// BrowseURL is the web link to an issue.
func (c *Client) BrowseURL(key string) string { return c.BaseURL() + "/browse/" + key }

// platform and agile build API paths for the deployment (configuration spec §15).
func (c *Client) platform(p string) string {
	if c.deployment == DataCenter {
		return "/rest/api/2" + p
	}
	return "/rest/api/3" + p
}

func agile(p string) string { return "/rest/agile/1.0" + p }

// request describes one API call.
type request struct {
	method  string
	path    string     // API path, e.g. /rest/api/3/myself
	query   url.Values // may be nil
	body    any        // JSON-encoded if not nil
	out     any        // decoded from a 2xx JSON body if not nil
	noCheck bool       // X-Atlassian-Token: no-check
	// idempotent marks a POST that only reads (e.g. /search/jql), so it may be
	// retried on 5xx and network errors like a GET.
	idempotent bool
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	return c.do(ctx, request{method: http.MethodGet, path: path, query: q, out: out})
}

func (c *Client) send(ctx context.Context, method, path string, body, out any) error {
	return c.do(ctx, request{method: method, path: path, body: body, out: out})
}

// do executes a request with these retry rules:
//   - 429: wait Retry-After (or backoff) and retry any method; drop concurrency to 1.
//   - 5xx and network errors: retry GET/HEAD only. Never retry a POST that may
//     have created data (OP rule 5.3.5).
//   - Everything else returns at once.
func (c *Client) do(ctx context.Context, r request) error {
	var payload []byte
	if r.body != nil {
		var err error
		if payload, err = json.Marshal(r.body); err != nil {
			return fmt.Errorf("jira: encode %s %s: %w", r.method, r.path, err)
		}
	}
	safe := r.method == http.MethodGet || r.method == http.MethodHead || r.idempotent

	var rateRetries, serverRetries int
	for attempt := 1; ; attempt++ {
		err := c.once(ctx, r, payload, attempt)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var apiErr *APIError
		var delay time.Duration
		switch {
		case errors.As(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests:
			if rateRetries >= c.retry.MaxRateLimitRetries {
				return err
			}
			rateRetries++
			delay = apiErr.RetryAfter
			if delay <= 0 {
				delay = c.backoff(rateRetries)
			}
			delay = min(delay, c.retry.MaxDelay)
			c.limit.pause(c.now().Add(delay)) // every caller waits, not just this one
		case errors.As(err, &apiErr) && retryableStatus(apiErr.Status) && safe:
			if serverRetries >= c.retry.MaxServerRetries {
				return err
			}
			serverRetries++
			delay = c.backoff(serverRetries)
		case errors.Is(err, ErrOffline) && safe:
			if serverRetries >= c.retry.MaxServerRetries {
				return err
			}
			serverRetries++
			delay = c.backoff(serverRetries)
		default:
			return err
		}
		c.log.LogAttrs(ctx, slog.LevelInfo, "jira retry", slog.String("method", r.method), slog.String("path", r.path),
			slog.Int("attempt", attempt), slog.Duration("delay", delay), slog.String("reason", errReason(err)))
		if err := c.sleep(ctx, delay); err != nil {
			return err
		}
	}
}

func retryableStatus(s int) bool {
	return s == http.StatusInternalServerError || s == http.StatusBadGateway ||
		s == http.StatusServiceUnavailable || s == http.StatusGatewayTimeout
}

// backoff is 1, 2, 4, 8, 16 … seconds with jitter, capped at MaxDelay.
func (c *Client) backoff(n int) time.Duration {
	d := c.retry.BaseDelay << (n - 1)
	if d <= 0 || d > c.retry.MaxDelay {
		d = c.retry.MaxDelay
	}
	return d/2 + rand.N(d/2+1) // "equal jitter": never below half the step
}

func (c *Client) once(ctx context.Context, r request, payload []byte, attempt int) (err error) {
	if err := c.limit.acquire(ctx); err != nil {
		return err
	}
	throttled := false
	defer func() { c.limit.release(err == nil, throttled) }()

	u := *c.base
	u.Path = strings.TrimRight(c.base.Path, "/") + r.path
	u.RawQuery = r.query.Encode()

	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.noCheck {
		req.Header.Set("X-Atlassian-Token", "no-check")
	}
	if err := c.auth.Apply(req); err != nil {
		return err
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.logRequest(ctx, r, 0, time.Since(start), "", attempt)
		return fmt.Errorf("%w: %s", ErrOffline, netReason(err))
	}
	defer resp.Body.Close()
	c.logRequest(ctx, r, resp.StatusCode, time.Since(start), resp.Header.Get("X-AREQUESTID"), attempt)
	c.observeRateLimit(resp.Header)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
		apiErr := newAPIError(resp, r.method, r.path, b)
		throttled = resp.StatusCode == http.StatusTooManyRequests
		return apiErr
	}
	if r.out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return nil
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes))
	if err := dec.Decode(r.out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // empty 200/201 body
		}
		return fmt.Errorf("jira: decode %s %s: %w", r.method, r.path, err)
	}
	return nil
}

// observeRateLimit pauses all requests when Jira says the budget is spent.
// Header names per Atlassian's rate-limiting docs.
func (c *Client) observeRateLimit(h http.Header) {
	if h.Get("X-RateLimit-Remaining") != "0" {
		return
	}
	if reset := h.Get("X-RateLimit-Reset"); reset != "" {
		if t, err := time.Parse(time.RFC3339, reset); err == nil {
			if limit := c.now().Add(c.retry.MaxDelay); t.After(limit) {
				t = limit
			}
			c.limit.pause(t)
		}
	}
}

// logRequest records method, path, status, duration and request ID only:
// never the query, headers or body.
func (c *Client) logRequest(ctx context.Context, r request, status int, d time.Duration, reqID string, attempt int) {
	level := slog.LevelDebug
	if status == 0 || status >= 400 {
		level = slog.LevelWarn
	}
	c.log.LogAttrs(ctx, level, "jira request",
		slog.String("method", r.method), slog.String("path", r.path), slog.Int("status", status),
		slog.Duration("duration", d), slog.String("request_id", reqID), slog.Int("attempt", attempt))
}

func errReason(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return strconv.Itoa(apiErr.Status)
	}
	return "network"
}

// netReason describes a transport error without echoing the URL (which may
// carry a JQL query with issue text).
func netReason(err error) string {
	var dnsErr *net.DNSError
	var opErr *net.OpError
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.As(err, &certErr):
		return "TLS certificate not trusted (set jira.tls.caFile)"
	case errors.As(err, &dnsErr):
		return "DNS lookup failed for " + dnsErr.Name
	case errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err):
		return "timeout"
	case errors.As(err, &opErr):
		return opErr.Op + " failed"
	}
	return "connection failed"
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// parseRetryAfter accepts seconds or an HTTP date.
func parseRetryAfter(v string, now time.Time) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

// ── HTTP transport ─────────────────────────────────

type NetOptions struct {
	Timeout time.Duration // default 20s
	Proxy   string        // default HTTPS_PROXY / HTTP_PROXY
	CAFile  string        // added to the system pool
}

func NewHTTPClient(o NetOptions) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyFromEnvironment
	if o.Proxy != "" {
		pu, err := url.Parse(o.Proxy)
		if err != nil {
			return nil, fmt.Errorf("jira: invalid proxy URL: %w", err)
		}
		tr.Proxy = http.ProxyURL(pu)
	}
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if o.CAFile != "" {
		pem, err := os.ReadFile(o.CAFile)
		if err != nil {
			return nil, fmt.Errorf("jira: read CA file: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("jira: CA file has no PEM certificates")
		}
		tr.TLSClientConfig.RootCAs = pool
	}
	tr.MaxIdleConnsPerHost = 8
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &http.Client{Transport: tr, Timeout: timeout}, nil
}
