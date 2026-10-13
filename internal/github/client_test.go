package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"maestro/internal/github/ghtest"
)

func noSleep(context.Context, time.Duration) error { return nil }

func newClient(t *testing.T, url string, tok string) *Client {
	t.Helper()
	c, err := New(Options{APIURL: url, Tokens: Static(tok), sleep: noSleep})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestETagCacheAndListing(t *testing.T) {
	gh := ghtest.New(t)
	gh.Update("acme/app", 7, func(p *ghtest.PR) { p.Title = "PLAT-2 work"; p.Head = "feature/PLAT-2-x" })
	c := newClient(t, gh.URL, ghtest.Token)
	ctx := context.Background()

	prs, err := c.OpenPullRequests(ctx, "acme/app")
	if err != nil || len(prs) != 1 || prs[0].Head.Ref != "feature/PLAT-2-x" {
		t.Fatalf("prs = %+v, %v", prs, err)
	}
	again, err := c.OpenPullRequests(ctx, "acme/app")
	if _, nm := gh.Counts(); err != nil || nm != 1 || len(again) != 1 || again[0].Number != 7 {
		t.Fatalf("second read must be a 304 from cache: %d not-modified, %+v %v", nm, again, err)
	}
	gh.Update("acme/app", 7, func(p *ghtest.PR) { p.Title = "PLAT-2 renamed" })
	third, _ := c.OpenPullRequests(ctx, "acme/app")
	if third[0].Title != "PLAT-2 renamed" {
		t.Fatalf("changed list must be read again: %+v", third)
	}
}

func TestRetriesAndErrors(t *testing.T) {
	gh := ghtest.New(t)
	c := newClient(t, gh.URL, ghtest.Token)
	ctx := context.Background()

	gh.FailNext(2)
	if u, err := c.Viewer(ctx); err != nil || u.Login != "octo" {
		t.Fatalf("GET must retry 5xx: %+v %v", u, err)
	}
	gh.RateLimitOnce()
	if _, err := c.Viewer(ctx); err != nil {
		t.Fatalf("secondary rate limit must be waited out: %v", err)
	}
	if _, err := c.PullRequest(ctx, "acme/app", 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing PR: %v", err)
	}
	if _, err := c.OpenPullRequests(ctx, "not-a-repo"); err == nil {
		t.Fatal("bad repo name must fail")
	}
	bad := newClient(t, gh.URL, "wrong")
	if _, err := bad.Viewer(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bad token: %v", err)
	}
}

func TestPrimaryRateLimitPauses(t *testing.T) {
	reset := time.Now().Add(time.Hour).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(403)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer srv.Close()
	c := newClient(t, srv.URL, "x")
	_, err := c.Viewer(context.Background())
	if !errors.Is(err, ErrRateLimited) || errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v", err)
	}
	// Later calls fail fast instead of hammering the API until the reset.
	if _, err := c.Viewer(context.Background()); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("paused: %v", err)
	}
}

func TestLatestReviews(t *testing.T) {
	got := LatestReviews([]Review{
		{User: User{Login: "ana"}, State: "CHANGES_REQUESTED", SubmittedAt: "2026-01-01T10:00:00Z"},
		{User: User{Login: "ana"}, State: "COMMENTED", SubmittedAt: "2026-01-01T11:00:00Z"},
		{User: User{Login: "bo"}, State: "APPROVED", SubmittedAt: "2026-01-01T09:00:00Z"},
		{User: User{Login: "ana"}, State: "APPROVED", SubmittedAt: "2026-01-01T12:00:00Z"},
		{User: User{Login: "cy"}, State: "APPROVED", SubmittedAt: "2026-01-01T08:00:00Z"},
		{User: User{Login: "cy"}, State: "DISMISSED", SubmittedAt: "2026-01-01T13:00:00Z"},
	})
	if len(got) != 2 || got["ana"] != "APPROVED" || got["bo"] != "APPROVED" {
		t.Fatalf("got %v", got)
	}
}

func TestWebHost(t *testing.T) {
	for in, want := range map[string]string{"": "github.com", "https://api.github.com": "github.com", "https://ghe.example.com/api/v3": "ghe.example.com"} {
		if got := WebHost(in); got != want {
			t.Errorf("WebHost(%q) = %q", in, got)
		}
	}
}
