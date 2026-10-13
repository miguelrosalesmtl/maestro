// Package ghtest is a small stateful fake of the GitHub REST API: pull
// requests, reviews, check runs, commit statuses and commits, with ETags.
package ghtest

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const Token = "ghp_test"

type PR struct {
	Number    int
	Title     string
	Body      string
	Head      string // branch
	SHA       string
	Base      string
	Author    string
	State     string // open | closed
	Draft     bool
	MergedAt  string
	UpdatedAt string
	Reviews   []Review
	Checks    []Check
	Statuses  []Status
	Commits   []Commit
}

type Review struct{ User, State, At string }
type Check struct{ Name, Status, Conclusion string }
type Status struct{ Context, State string }
type Commit struct{ SHA, Message, Author, At string }

type Server struct {
	URL string

	mu       sync.Mutex
	repos    map[string][]*PR // owner/name → PRs
	Requests int              // all requests
	NotMod   int              // 304 answers
	failNext int              // answer the next n requests with 502
	limited  bool             // answer 403 rate limited once
	User     string
	srv      *httptest.Server
}

func New(t testing.TB) *Server {
	s := &Server{repos: map[string][]*PR{}, User: "octo"}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	s.URL = s.srv.URL
	t.Cleanup(s.srv.Close)
	return s
}

// Update changes a PR (or adds it) under the lock and bumps UpdatedAt.
func (s *Server) Update(repo string, number int, f func(*PR)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.repos[repo] {
		if p.Number == number {
			f(p)
			p.UpdatedAt = time.Now().UTC().Add(time.Duration(s.Requests) * time.Millisecond).Format(time.RFC3339Nano)
			return
		}
	}
	p := &PR{Number: number, State: "open", Base: "main", Author: "octo", SHA: fmt.Sprintf("%040d", number)}
	f(p)
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.repos[repo] = append(s.repos[repo], p)
}

func (s *Server) FailNext(n int) { s.mu.Lock(); s.failNext = n; s.mu.Unlock() }
func (s *Server) RateLimitOnce() { s.mu.Lock(); s.limited = true; s.mu.Unlock() }
func (s *Server) Counts() (requests, notModified int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Requests, s.NotMod
}

var (
	rePulls   = regexp.MustCompile(`^/repos/([^/]+/[^/]+)/pulls$`)
	rePull    = regexp.MustCompile(`^/repos/([^/]+/[^/]+)/pulls/(\d+)(/reviews|/commits)?$`)
	reCommit  = regexp.MustCompile(`^/repos/([^/]+/[^/]+)/commits/([^/]+)/(check-runs|status)$`)
	apiPrefix = "/api/v3" // accepted for GitHub Enterprise style URLs
)

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests++
	if r.Header.Get("Authorization") != "Bearer "+Token {
		w.WriteHeader(401)
		w.Write([]byte(`{"message":"Bad credentials"}`))
		return
	}
	if r.Header.Get("X-GitHub-Api-Version") == "" || r.Header.Get("User-Agent") == "" {
		w.WriteHeader(400)
		return
	}
	if s.failNext > 0 {
		s.failNext--
		w.WriteHeader(502)
		return
	}
	if s.limited {
		s.limited = false
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(403)
		w.Write([]byte(`{"message":"You have exceeded a secondary rate limit"}`))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, apiPrefix)
	var body any
	switch {
	case path == "/user":
		body = map[string]any{"login": s.User}
	case rePulls.MatchString(path):
		repo := rePulls.FindStringSubmatch(path)[1]
		state := r.URL.Query().Get("state")
		list := []map[string]any{}
		prs := append([]*PR(nil), s.repos[repo]...)
		sort.Slice(prs, func(i, j int) bool { return prs[i].UpdatedAt > prs[j].UpdatedAt })
		for _, p := range prs {
			if state == "" || state == p.State {
				list = append(list, prJSON(p))
			}
		}
		body = list
	case rePull.MatchString(path):
		m := rePull.FindStringSubmatch(path)
		n, _ := strconv.Atoi(m[2])
		p := s.find(m[1], n)
		if p == nil {
			w.WriteHeader(404)
			return
		}
		switch m[3] {
		case "":
			body = prJSON(p)
		case "/reviews":
			out := []map[string]any{}
			for i, rv := range p.Reviews {
				out = append(out, map[string]any{"id": i + 1, "user": map[string]any{"login": rv.User}, "state": rv.State, "submitted_at": rv.At})
			}
			body = out
		case "/commits":
			out := []map[string]any{}
			for _, c := range p.Commits {
				out = append(out, map[string]any{"sha": c.SHA, "commit": map[string]any{"message": c.Message, "author": map[string]any{"name": c.Author, "date": c.At}}})
			}
			body = out
		}
	case reCommit.MatchString(path):
		m := reCommit.FindStringSubmatch(path)
		var p *PR
		for _, q := range s.repos[m[1]] {
			if q.SHA == m[2] {
				p = q
			}
		}
		if p == nil {
			w.WriteHeader(404)
			return
		}
		if m[3] == "check-runs" {
			runs := []map[string]any{}
			for _, c := range p.Checks {
				runs = append(runs, map[string]any{"name": c.Name, "status": c.Status, "conclusion": c.Conclusion, "html_url": "https://example.test/run"})
			}
			body = map[string]any{"total_count": len(runs), "check_runs": runs}
		} else {
			sts := []map[string]any{}
			for _, st := range p.Statuses {
				sts = append(sts, map[string]any{"context": st.Context, "state": st.State})
			}
			body = map[string]any{"statuses": sts}
		}
	default:
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"Not Found"}`))
		return
	}
	b, _ := json.Marshal(body)
	etag := fmt.Sprintf(`W/"%x"`, sha1.Sum(b))
	if r.Header.Get("If-None-Match") == etag {
		s.NotMod++
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Remaining", "4999")
	w.Write(b)
}

func (s *Server) find(repo string, n int) *PR {
	for _, p := range s.repos[repo] {
		if p.Number == n {
			return p
		}
	}
	return nil
}

func prJSON(p *PR) map[string]any {
	var merged any
	if p.MergedAt != "" {
		merged = p.MergedAt
	}
	return map[string]any{
		"number": p.Number, "title": p.Title, "body": p.Body, "state": p.State, "draft": p.Draft,
		"html_url": fmt.Sprintf("https://github.com/x/pull/%d", p.Number), "merged_at": merged, "updated_at": p.UpdatedAt,
		"head": map[string]any{"ref": p.Head, "sha": p.SHA}, "base": map[string]any{"ref": p.Base},
		"user": map[string]any{"login": p.Author}, "requested_reviewers": []any{},
	}
}
