// Package github is Maestro's GitHub REST client: just the reads needed to
// follow a ticket's pull requests (PRs, reviews, checks, commits).
//
// Robustness: ETag caching (a 304 does not count against the rate limit),
// retries for GET on 5xx and network errors, waiting on rate limits
// (primary and secondary), typed errors, and logs without tokens or bodies.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"maestro/internal/secrets"
)

var (
	ErrUnauthorized = errors.New("github: not signed in or token expired")
	ErrForbidden    = errors.New("github: no access")
	ErrNotFound     = errors.New("github: not found or no access")
	ErrRateLimited  = errors.New("github: rate limited")
	ErrOffline      = errors.New("github: cannot reach the server")
)

// APIError is a non-2xx response.
type APIError struct {
	Status     int
	Path       string
	Message    string
	RetryAfter time.Duration
	RequestID  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github: GET %s: %d %s", e.Path, e.Status, firstNonEmpty(e.Message, http.StatusText(e.Status)))
}

func (e *APIError) Is(t error) bool {
	switch t {
	case ErrUnauthorized:
		return e.Status == 401
	case ErrForbidden:
		return e.Status == 403 && e.RetryAfter == 0
	case ErrNotFound:
		return e.Status == 404
	case ErrRateLimited:
		return e.Status == 429 || (e.Status == 403 && e.RetryAfter > 0)
	}
	return false
}

// TokenSource supplies the token for each request.
type TokenSource interface {
	Token(ctx context.Context) (secrets.Secret, error)
}

// GHCLI asks the GitHub CLI for its token (`gh auth token`) every time, so a
// `gh auth login` or token refresh is picked up without restarting Maestro.
// The token is never stored.
type GHCLI struct{ Host string }

func (g GHCLI) Token(ctx context.Context) (secrets.Secret, error) {
	host := firstNonEmpty(g.Host, "github.com")
	out, err := exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", host).Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("%w: the GitHub CLI (gh) is not installed; install it or use dev.github.auth: token", ErrUnauthorized)
		}
		return "", fmt.Errorf("%w: run `gh auth login --hostname %s`", ErrUnauthorized, host)
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return "", fmt.Errorf("%w: run `gh auth login --hostname %s`", ErrUnauthorized, host)
	}
	return secrets.Secret(tok), nil
}

// Keychain reads a fine-grained token from the OS keychain.
type Keychain struct {
	Store *secrets.Store
	Ref   secrets.Ref
}

func (k Keychain) Token(context.Context) (secrets.Secret, error) { return k.Store.Get(k.Ref) }

// Static is a fixed token (tests).
type Static string

func (s Static) Token(context.Context) (secrets.Secret, error) { return secrets.Secret(s), nil }

type Options struct {
	APIURL     string // default https://api.github.com
	Tokens     TokenSource
	HTTPClient *http.Client
	UserAgent  string
	Logger     *slog.Logger
	MaxRetries int // GET retries on 5xx/network (default 3)

	sleep func(context.Context, time.Duration) error
}

type Client struct {
	base   *url.URL
	tokens TokenSource
	http   *http.Client
	ua     string
	log    *slog.Logger
	retry  int
	sleep  func(context.Context, time.Duration) error

	mu         sync.Mutex
	etags      map[string]cached // URL → last 200 response
	pauseUntil time.Time         // rate limit: no requests before this
}

type cached struct {
	etag string
	body []byte
}

func New(o Options) (*Client, error) {
	raw := firstNonEmpty(o.APIURL, "https://api.github.com")
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("github: invalid API URL %q", raw)
	}
	if o.Tokens == nil {
		return nil, errors.New("github: a TokenSource is required")
	}
	c := &Client{base: u, tokens: o.Tokens, http: o.HTTPClient, ua: firstNonEmpty(o.UserAgent, "Maestro/dev"),
		log: o.Logger, retry: o.MaxRetries, sleep: o.sleep, etags: map[string]cached{}}
	if c.http == nil {
		c.http = &http.Client{Timeout: 20 * time.Second}
	}
	if c.log == nil {
		c.log = slog.New(slog.DiscardHandler)
	}
	if c.retry <= 0 {
		c.retry = 3
	}
	if c.sleep == nil {
		c.sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	return c, nil
}

// WebHost is the github.com-style host for this API URL (for `gh auth`).
func WebHost(apiURL string) string {
	u, err := url.Parse(firstNonEmpty(apiURL, "https://api.github.com"))
	if err != nil || u.Host == "api.github.com" || u.Host == "" {
		return "github.com"
	}
	return u.Host // GitHub Enterprise: https://ghe.example.com/api/v3
}

// get fetches path (relative to the API base) into out, using the ETag cache.
func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := *c.base
	u.Path = strings.TrimRight(c.base.Path, "/") + path
	u.RawQuery = q.Encode()
	key := u.String()

	for attempt := 0; ; attempt++ {
		if err := c.waitForRateLimit(ctx); err != nil {
			return err
		}
		body, err := c.once(ctx, key, path)
		if err == nil {
			if out == nil {
				return nil
			}
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("github: decode %s: %w", path, err)
			}
			return nil
		}
		var api *APIError
		retryable := errors.Is(err, ErrOffline) || (errors.As(err, &api) && (api.Status >= 500 || errors.Is(err, ErrRateLimited)))
		if !retryable || attempt >= c.retry || ctx.Err() != nil {
			return err
		}
		delay := time.Duration(1<<attempt) * time.Second
		delay = delay/2 + rand.N(delay/2+1)
		if errors.As(err, &api) && api.RetryAfter > 0 {
			delay = min(api.RetryAfter, 2*time.Minute)
		}
		c.log.Info("github retry", "path", path, "attempt", attempt+1, "delay", delay)
		if err := c.sleep(ctx, delay); err != nil {
			return err
		}
	}
}

func (c *Client) once(ctx context.Context, key, path string) ([]byte, error) {
	tok, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Authorization", "Bearer "+tok.Reveal())
	c.mu.Lock()
	prev, hasPrev := c.etags[key]
	c.mu.Unlock()
	if hasPrev {
		req.Header.Set("If-None-Match", prev.etag)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		c.log.Warn("github request", "path", path, "status", 0, "duration", time.Since(start))
		return nil, fmt.Errorf("%w: %s", ErrOffline, strings.SplitN(err.Error(), ": ", 2)[0])
	}
	defer resp.Body.Close()
	c.observeRateLimit(resp)
	level := slog.LevelDebug
	if resp.StatusCode >= 400 {
		level = slog.LevelWarn
	}
	c.log.Log(ctx, level, "github request", "path", path, "status", resp.StatusCode, "duration", time.Since(start),
		"request_id", resp.Header.Get("X-GitHub-Request-Id"), "remaining", resp.Header.Get("X-RateLimit-Remaining"))

	switch {
	case resp.StatusCode == http.StatusNotModified && hasPrev:
		return prev.body, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
		if err != nil {
			return nil, err
		}
		if et := resp.Header.Get("ETag"); et != "" {
			c.mu.Lock()
			c.etags[key] = cached{etag: et, body: body}
			c.mu.Unlock()
		}
		return body, nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	e := &APIError{Status: resp.StatusCode, Path: path, RequestID: resp.Header.Get("X-GitHub-Request-Id")}
	var msg struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &msg) == nil {
		e.Message = msg.Message
	}
	e.RetryAfter = rateLimitWait(resp, time.Now())
	return nil, e
}

// rateLimitWait reads Retry-After, or X-RateLimit-Reset when the budget is spent.
func rateLimitWait(resp *http.Response, now time.Time) time.Duration {
	if resp.StatusCode != 403 && resp.StatusCode != 429 {
		return 0
	}
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
		return time.Duration(max(s, 1)) * time.Second
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if r, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			if d := time.Unix(r, 0).Sub(now); d > 0 {
				return d
			}
			return time.Second
		}
	}
	return 0
}

func (c *Client) observeRateLimit(resp *http.Response) {
	if d := rateLimitWait(resp, time.Now()); d > 0 {
		c.mu.Lock()
		if until := time.Now().Add(min(d, time.Hour)); until.After(c.pauseUntil) {
			c.pauseUntil = until
		}
		c.mu.Unlock()
	}
}

func (c *Client) waitForRateLimit(ctx context.Context) error {
	c.mu.Lock()
	d := time.Until(c.pauseUntil)
	c.mu.Unlock()
	if d <= 0 {
		return nil
	}
	if d > 2*time.Minute {
		return fmt.Errorf("%w until %s", ErrRateLimited, time.Now().Add(d).Format(time.Kitchen))
	}
	return c.sleep(ctx, d)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
