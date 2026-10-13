package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Sentinel errors. Match them with errors.Is; the concrete error is *APIError
// (or a wrapped network error for ErrOffline).
var (
	ErrUnauthorized = errors.New("jira: token is not valid or has expired") // 401,
	ErrForbidden    = errors.New("jira: permission denied")                 // 403
	ErrNotFound     = errors.New("jira: not found or no access")            // 404
	ErrConflict     = errors.New("jira: conflicting change")                // 409, 412
	ErrRateLimited  = errors.New("jira: rate limited")                      // 429 after retries
	ErrBadRequest   = errors.New("jira: request rejected")                  // 400
	ErrServer       = errors.New("jira: server error")                      // 5xx after retries
	ErrOffline      = errors.New("jira: cannot reach the site")             // network
	ErrCaptcha      = errors.New("jira: log in to Jira once in the browser (CAPTCHA required)")
)

// APIError is a non-2xx response. It holds Jira's own messages, never request
// headers or bodies.
type APIError struct {
	Status     int
	Method     string
	Path       string // without query
	RequestID  string // X-AREQUESTID, for Atlassian support
	Messages   []string
	Fields     map[string]string // field ID → message
	RetryAfter time.Duration
	captcha    bool
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "jira: %s %s: %d %s", e.Method, e.Path, e.Status, http.StatusText(e.Status))
	if d := e.Detail(); d != "" {
		b.WriteString(": " + d)
	}
	return b.String()
}

// Detail is Jira's explanation, suitable to show in the UI.
func (e *APIError) Detail() string {
	parts := append([]string(nil), e.Messages...)
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, k+": "+e.Fields[k])
	}
	return strings.Join(parts, "; ")
}

func (e *APIError) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.Status == http.StatusUnauthorized
	case ErrForbidden:
		return e.Status == http.StatusForbidden
	case ErrCaptcha:
		return e.captcha
	case ErrNotFound:
		return e.Status == http.StatusNotFound
	case ErrConflict:
		return e.Status == http.StatusConflict || e.Status == http.StatusPreconditionFailed
	case ErrRateLimited:
		return e.Status == http.StatusTooManyRequests
	case ErrBadRequest:
		return e.Status == http.StatusBadRequest
	case ErrServer:
		return e.Status >= 500
	}
	return false
}

// FieldOrTransitionStale reports a 400 that means Maestro's discovery cache is
// out of date: an unknown field, transition or status.
func (e *APIError) FieldOrTransitionStale() bool {
	if e.Status != http.StatusBadRequest {
		return false
	}
	d := strings.ToLower(e.Detail())
	for _, s := range []string{"does not exist", "is not valid", "cannot be set", "not on the appropriate screen", "unknown"} {
		if strings.Contains(d, s) {
			return true
		}
	}
	return false
}

// newAPIError reads Jira's error body: {"errorMessages":[…],"errors":{…}}.
func newAPIError(resp *http.Response, method, path string, body []byte) *APIError {
	e := &APIError{
		Status:    resp.StatusCode,
		Method:    method,
		Path:      path,
		RequestID: resp.Header.Get("X-AREQUESTID"),
	}
	var payload struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
		Message       string            `json:"message"` // agile API, some gateways
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Messages = payload.ErrorMessages
		if payload.Message != "" {
			e.Messages = append(e.Messages, payload.Message)
		}
		if len(payload.Errors) > 0 {
			e.Fields = payload.Errors
		}
	}
	if resp.StatusCode == http.StatusForbidden {
		reason := resp.Header.Get("X-Seraph-LoginReason")
		e.captcha = strings.Contains(reason, "CAPTCHA") || strings.Contains(strings.ToUpper(string(body)), "CAPTCHA")
	}
	e.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	return e
}
