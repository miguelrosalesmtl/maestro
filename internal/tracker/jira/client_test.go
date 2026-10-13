package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"maestro/internal/secrets"
)

// fakeJira routes "METHOD /path" to handlers and records every request.
type fakeJira struct {
	t      *testing.T
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	calls  []string
	srv    *httptest.Server
}

func newFake(t *testing.T) *fakeJira {
	f := &fakeJira{t: t, routes: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		f.mu.Lock()
		f.calls = append(f.calls, key)
		h := f.routes[key]
		f.mu.Unlock()
		if h == nil {
			http.Error(w, `{"errorMessages":["no route `+key+`"]}`, http.StatusNotFound)
			return
		}
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeJira) on(route string, h http.HandlerFunc) { f.routes[route] = h }

func (f *fakeJira) count(route string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == route {
			n++
		}
	}
	return n
}

func jsonReply(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
}

// client builds a client against the fake with sleeps recorded, not slept.
func (f *fakeJira) client(t *testing.T, dep Deployment, opts ...func(*Options)) (*Client, *[]time.Duration) {
	var slept []time.Duration
	var mu sync.Mutex
	clock := time.Now()
	o := Options{
		BaseURL: f.srv.URL, Deployment: dep,
		Auth:       BasicToken{Email: "me@acme.com", Token: secrets.Secret("ATATT-secret-token")},
		HTTPClient: f.srv.Client(),
		// A fake clock: sleeping advances it, so limiter pauses expire at once.
		sleep: func(ctx context.Context, d time.Duration) error {
			mu.Lock()
			slept = append(slept, d)
			clock = clock.Add(d)
			mu.Unlock()
			return ctx.Err()
		},
		now: func() time.Time { mu.Lock(); defer mu.Unlock(); return clock },
	}
	for _, fn := range opts {
		fn(&o)
	}
	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return c, &slept
}

func TestRequestBasics(t *testing.T) {
	f := newFake(t)
	f.on("GET /rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("me@acme.com:ATATT-secret-token"))
		if r.Header.Get("Authorization") != want {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/json" || !strings.HasPrefix(r.Header.Get("User-Agent"), "Maestro/") {
			t.Errorf("headers: %v", r.Header)
		}
		jsonReply(User{AccountID: "abc", DisplayName: "Me"})(w, r)
	})
	f.on("GET /rest/api/2/myself", jsonReply(User{Name: "me", DisplayName: "Me DC"}))

	c, _ := f.client(t, Cloud)
	me, err := c.Myself(context.Background())
	if err != nil || me.ID() != "abc" {
		t.Fatalf("cloud Myself = %+v, %v", me, err)
	}
	dc, _ := f.client(t, DataCenter, func(o *Options) { o.Auth = BearerToken{Token: "pat"} })
	me, err = dc.Myself(context.Background())
	if err != nil || me.ID() != "me" {
		t.Fatalf("Data Center must use /rest/api/2: %+v, %v", me, err)
	}
}

func TestRateLimitRetriesWithRetryAfterAndAdaptsConcurrency(t *testing.T) {
	f := newFake(t)
	var n atomic.Int32
	f.on("GET /rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) <= 2 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		jsonReply(User{AccountID: "abc"})(w, r)
	})
	c, slept := f.client(t, Cloud)
	if _, err := c.Myself(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := *slept; len(got) != 2 || got[0] != 7*time.Second || got[1] != 7*time.Second {
		t.Fatalf("must wait Retry-After each time, slept %v", got)
	}
	if c.limit.current() != 1 {
		t.Fatalf("after a 429 the concurrency must drop to 1, got %d", c.limit.current())
	}
	for i := 0; i < recoverAfter; i++ {
		if _, err := c.Myself(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if c.limit.current() != 4 {
		t.Fatalf("after %d successes the concurrency must recover, got %d", recoverAfter, c.limit.current())
	}
}

func TestRateLimitGivesUp(t *testing.T) {
	f := newFake(t)
	f.on("GET /rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
	c, slept := f.client(t, Cloud)
	_, err := c.Myself(context.Background())
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
	if f.count("GET /rest/api/3/myself") != 6 || len(*slept) != 5 {
		t.Fatalf("want 1 + 5 retries, got %d calls", f.count("GET /rest/api/3/myself"))
	}
	for i, d := range *slept { // backoff 1,2,4,8,16 s with equal jitter
		step := time.Second << i
		if d < step/2 || d > step {
			t.Errorf("backoff %d = %v, want within [%v, %v]", i, d, step/2, step)
		}
	}
}

func TestServerErrorsRetryOnlySafeRequests(t *testing.T) {
	f := newFake(t)
	f.on("GET /rest/api/3/field", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	f.on("POST /rest/api/3/issue", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	var searches atomic.Int32
	f.on("POST /rest/api/3/search/jql", func(w http.ResponseWriter, r *http.Request) {
		if searches.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		jsonReply(map[string]any{"issues": []any{}, "isLast": true})(w, r)
	})
	c, _ := f.client(t, Cloud)
	ctx := context.Background()

	if _, err := c.Fields(ctx); !errors.Is(err, ErrServer) || f.count("GET /rest/api/3/field") != 4 {
		t.Fatalf("GET: err=%v calls=%d, want ErrServer after 1+3 calls", err, f.count("GET /rest/api/3/field"))
	}
	_, err := c.CreateIssue(ctx, CreateInput{ProjectKey: "PLAT", IssueTypeID: "1", Summary: "x"})
	if !errors.Is(err, ErrServer) || f.count("POST /rest/api/3/issue") != 1 {
		t.Fatalf("a create must never be retried: err=%v calls=%d", err, f.count("POST /rest/api/3/issue"))
	}
	if _, err := Collect(c.Search(ctx, SearchOptions{JQL: "project = PLAT"})); err != nil || searches.Load() != 2 {
		t.Fatalf("read-only search POST should retry: err=%v calls=%d", err, searches.Load())
	}
}

func TestAPIErrors(t *testing.T) {
	f := newFake(t)
	f.on("GET /rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-AREQUESTID", "req-1")
		w.WriteHeader(http.StatusUnauthorized)
	})
	f.on("POST /rest/api/3/issue/PLAT-1/transitions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"errorMessages":["Transition is not valid"],"errors":{"resolution":"Resolution is required."}}`)
	})
	f.on("GET /rest/api/3/field", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seraph-LoginReason", "AUTHENTICATION_DENIED_CAPTCHA")
		w.WriteHeader(http.StatusForbidden)
	})
	c, _ := f.client(t, Cloud)
	ctx := context.Background()

	_, err := c.Myself(ctx)
	var apiErr *APIError
	if !errors.Is(err, ErrUnauthorized) || !errors.As(err, &apiErr) || apiErr.RequestID != "req-1" || f.count("GET /rest/api/3/myself") != 1 {
		t.Fatalf("401 must fail at once with the request ID: %v", err)
	}
	err = c.DoTransition(ctx, "PLAT-1", "31", nil)
	if !errors.As(err, &apiErr) || !errors.Is(err, ErrBadRequest) || !apiErr.FieldOrTransitionStale() {
		t.Fatalf("400: %v", err)
	}
	if d := apiErr.Detail(); d != "Transition is not valid; resolution: Resolution is required." {
		t.Fatalf("detail = %q", d)
	}
	if _, err := c.Fields(ctx); !errors.Is(err, ErrCaptcha) || !errors.Is(err, ErrForbidden) {
		t.Fatalf("captcha: %v", err)
	}
}

func TestNothingSensitiveIsLogged(t *testing.T) {
	f := newFake(t)
	f.on("GET /rest/api/3/issue/PLAT-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"errorMessages":["boom"]}`)
	})
	var logs bytes.Buffer
	c, _ := f.client(t, Cloud, func(o *Options) {
		o.Logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})
	_, err := c.Issue(context.Background(), "PLAT-1", []string{"summary"}, "renderedFields")
	all := logs.String() + err.Error()
	for _, leak := range []string{"ATATT-secret-token", "Basic ", "renderedFields", "fields=summary"} {
		if strings.Contains(all, leak) {
			t.Errorf("%q leaked into logs or error:\n%s", leak, all)
		}
	}
	if !strings.Contains(logs.String(), "path=/rest/api/3/issue/PLAT-1") || !strings.Contains(logs.String(), "status=500") {
		t.Errorf("logs should keep method/path/status:\n%s", logs.String())
	}
}

func TestOffsetPaginationStopsEarly(t *testing.T) {
	f := newFake(t)
	f.on("GET /rest/api/3/project/search", func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		var vals []Project
		for i := start; i < start+2 && i < 5; i++ {
			vals = append(vals, Project{Key: fmt.Sprintf("P%d", i)})
		}
		jsonReply(map[string]any{"startAt": start, "values": vals, "isLast": start+2 >= 5, "total": 5})(w, r)
	})
	c, _ := f.client(t, Cloud)
	all, err := Collect(c.Projects(context.Background()))
	if err != nil || len(all) != 5 || all[4].Key != "P4" {
		t.Fatalf("got %v, %v", all, err)
	}
	pages := f.count("GET /rest/api/3/project/search")
	for p := range c.Projects(context.Background()) {
		_ = p
		break // must not fetch more pages
	}
	if f.count("GET /rest/api/3/project/search") != pages+1 {
		t.Fatal("breaking out of the loop must stop fetching")
	}
}

func TestSearchPagination(t *testing.T) {
	f := newFake(t)
	f.on("POST /rest/api/3/search/jql", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["jql"] != "project = PLAT" {
			t.Errorf("jql = %v", body["jql"])
		}
		switch body["nextPageToken"] {
		case nil:
			jsonReply(map[string]any{"issues": []map[string]any{{"key": "PLAT-1"}, {"key": "PLAT-2"}}, "nextPageToken": "p2"})(w, r)
		case "p2":
			jsonReply(map[string]any{"issues": []map[string]any{{"key": "PLAT-3"}}, "isLast": true})(w, r)
		}
	})
	f.on("POST /rest/api/2/search", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		start := int(body["startAt"].(float64))
		issues := []map[string]any{{"key": fmt.Sprintf("DC-%d", start+1)}}
		jsonReply(map[string]any{"issues": issues, "total": 2, "startAt": start})(w, r)
	})
	c, _ := f.client(t, Cloud)
	got, err := Collect(c.Search(context.Background(), SearchOptions{JQL: "project = PLAT"}))
	if err != nil || len(got) != 3 || got[2].Key != "PLAT-3" {
		t.Fatalf("cloud: %v %v", got, err)
	}
	dc, _ := f.client(t, DataCenter)
	got, err = Collect(dc.Search(context.Background(), SearchOptions{JQL: "project = DC"}))
	if err != nil || len(got) != 2 || got[1].Key != "DC-2" {
		t.Fatalf("data center: %v %v", got, err)
	}
}

func TestIssueCustomFields(t *testing.T) {
	raw := `{"id":"1","key":"PLAT-1","fields":{"summary":"S","status":{"name":"In Review","statusCategory":{"key":"indeterminate"}},
	  "customfield_10016":5,"customfield_10020":[{"id":1,"name":"Sprint 41","state":"closed"},{"id":2,"name":"Sprint 42","state":"active"}]}}`
	var is Issue
	if err := json.Unmarshal([]byte(raw), &is); err != nil {
		t.Fatal(err)
	}
	if is.Fields.Summary != "S" || is.Fields.Status.StatusCategory.Key != "indeterminate" {
		t.Fatalf("typed fields: %+v", is.Fields)
	}
	if v, ok := is.Number("customfield_10016"); !ok || v != 5 {
		t.Fatalf("story points = %v %v", v, ok)
	}
	if s := is.ActiveSprint("customfield_10020"); s == nil || s.Name != "Sprint 42" {
		t.Fatalf("active sprint = %+v", s)
	}
	if _, ok := is.Number("customfield_99999"); ok {
		t.Fatal("missing field must report !ok")
	}
}

func TestRetryAfterParsing(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"5":                             5 * time.Second,
		"Tue, 13 Oct 2026 12:00:30 GMT": 30 * time.Second,
		"garbage":                       0,
	}
	for in, want := range cases {
		if got := parseRetryAfter(in, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCancelDuringBackoff(t *testing.T) {
	f := newFake(t)
	f.on("GET /rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
	ctx, cancel := context.WithCancel(context.Background())
	c, _ := f.client(t, Cloud, func(o *Options) {
		o.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	})
	if _, err := c.Myself(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalizeSite(t *testing.T) {
	cases := map[string]string{
		"acme.atlassian.net":                                              "https://acme.atlassian.net",
		"https://ACME.atlassian.net/browse/PLAT-1":                        "https://acme.atlassian.net",
		"https://acme.atlassian.net/jira/software/projects/PLAT/boards/1": "https://acme.atlassian.net",
		"https://jira.example.com/jira/browse/X-1?x=1":                    "https://jira.example.com/jira",
		"https://jira.example.com/":                                       "https://jira.example.com",
	}
	for in, want := range cases {
		if got, err := NormalizeSite(in); err != nil || got != want {
			t.Errorf("NormalizeSite(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := NormalizeSite("  "); err == nil {
		t.Error("empty site must fail")
	}
}

func TestDetectSite(t *testing.T) {
	cloud := newFake(t)
	cloud.on("GET /_edge/tenant_info", jsonReply(map[string]string{"cloudId": "c-123"}))
	info, err := DetectSite(context.Background(), cloud.srv.Client(), cloud.srv.URL)
	if err != nil || info.Deployment != Cloud || info.CloudID != "c-123" {
		t.Fatalf("cloud: %+v %v", info, err)
	}

	dc := newFake(t)
	dc.on("GET /rest/api/2/serverInfo", jsonReply(ServerInfo{DeploymentType: "DataCenter", Version: "9.12.0"}))
	info, err = DetectSite(context.Background(), dc.srv.Client(), dc.srv.URL)
	if err != nil || info.Deployment != DataCenter || info.Version != "9.12.0" {
		t.Fatalf("data center: %+v %v", info, err)
	}

	other := newFake(t)
	if _, err := DetectSite(context.Background(), other.srv.Client(), other.srv.URL); !errors.Is(err, ErrNotJira) {
		t.Fatalf("non-Jira: %v", err)
	}
}

func TestDiscoverDegradesGracefully(t *testing.T) {
	f := newFake(t)
	f.on("GET /rest/api/3/myself", jsonReply(User{AccountID: "me"}))
	f.on("GET /rest/api/3/serverInfo", jsonReply(ServerInfo{DeploymentType: "Cloud"}))
	f.on("GET /rest/api/3/field", jsonReply([]Field{
		{ID: "customfield_10020", Name: "Sprint", Custom: true, Schema: FieldSchema{Custom: SchemaSprint}},
		{ID: "customfield_10016", Name: "Story point estimate", Custom: true, Schema: FieldSchema{Type: "number", Custom: SchemaStoryPointEst}},
	}))
	f.on("GET /rest/api/3/priority/search", jsonReply(map[string]any{"values": []Priority{{ID: "3", Name: "Medium"}}, "isLast": true}))
	f.on("GET /rest/api/3/resolution/search", jsonReply(map[string]any{"values": []Resolution{{ID: "1", Name: "Done"}}, "isLast": true}))
	f.on("GET /rest/api/3/issueLinkType", jsonReply(map[string]any{"issueLinkTypes": []IssueLinkType{{ID: "1", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"}}}))
	f.on("GET /rest/api/3/project/PLAT", jsonReply(Project{ID: "10", Key: "PLAT"}))
	f.on("GET /rest/api/3/issuetype/project", jsonReply([]IssueType{{ID: "1", Name: "Story"}}))
	f.on("GET /rest/api/3/statuses/search", jsonReply(map[string]any{"values": []StatusDef{{ID: "1", Name: "To Do", StatusCategory: "TODO"}}, "isLast": true}))
	f.on("GET /rest/api/3/mypermissions", jsonReply(map[string]any{"permissions": map[string]Permission{"CREATE_ISSUES": {HavePermission: true}}}))
	f.on("GET /rest/api/3/user/assignable/search", jsonReply([]User{{AccountID: "me"}}))
	f.on("GET /rest/api/3/project/PLAT/components", jsonReply([]Component{}))
	f.on("GET /rest/agile/1.0/board", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	// GHOST is not configured: 404.

	c, _ := f.client(t, Cloud)
	d, err := c.Discover(context.Background(), []string{"PLAT", "GHOST"})
	if err != nil {
		t.Fatalf("optional failures must not fail discovery: %v", err)
	}
	if d.SprintField() != "customfield_10020" || d.StoryPointsField() != "customfield_10016" {
		t.Errorf("field lookup: sprint=%q points=%q", d.SprintField(), d.StoryPointsField())
	}
	if p := d.Projects[0]; len(p.Statuses) != 1 || !p.Permissions["CREATE_ISSUES"] || p.Permissions["EDIT_ISSUES"] {
		t.Errorf("project discovery: %+v", p)
	}
	joined := strings.Join(d.Warnings, "\n")
	if !strings.Contains(joined, "PLAT boards") || !strings.Contains(joined, "GHOST: not found") {
		t.Errorf("warnings = %v", d.Warnings)
	}

	f.on("GET /rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	if _, err := c.Discover(context.Background(), []string{"PLAT"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a bad token must abort discovery: %v", err)
	}
}

func TestCreateIssueCloudPayload(t *testing.T) {
	f := newFake(t)
	f.on("POST /rest/api/3/issue", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Fields map[string]json.RawMessage `json:"fields"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		var desc ADFNode
		json.Unmarshal(body.Fields["description"], &desc)
		if desc.Type != "doc" || string(body.Fields["assignee"]) != `{"accountId":"abc"}` || string(body.Fields["customfield_10020"]) != "42" {
			t.Errorf("payload: %s", mustJSON(body.Fields))
		}
		w.WriteHeader(http.StatusCreated)
		jsonReply(CreatedIssue{ID: "100", Key: "PLAT-153"})(w, r)
	})
	c, _ := f.client(t, Cloud)
	got, err := c.CreateIssue(context.Background(), CreateInput{
		ProjectKey: "PLAT", IssueTypeID: "10001", Summary: " Canary ", Description: "Hello **world**",
		AssigneeID: "abc", Extra: map[string]any{"customfield_10020": 42},
	})
	if err != nil || got.Key != "PLAT-153" {
		t.Fatalf("create: %+v %v", got, err)
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
