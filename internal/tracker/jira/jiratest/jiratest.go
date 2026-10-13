// Package jiratest is a small stateful fake of Jira Cloud for tests and local
// development: discovery, search (full, incremental, by key), issue reads,
// transitions (one needs a resolution), create, comments and links.
//
// Fixture: PLAT-1 (epic, under OPS-1 which is outside the PLAT scope),
// PLAT-2 (story, 5 points, Sprint 42), PLAT-3 (sub-task of PLAT-2, Done).
package jiratest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeJira is a small stateful Jira Cloud: enough endpoints for discovery,
// search, issue reads and the writes Maestro makes.
type Server struct {
	t  testing.TB
	mu sync.Mutex

	issues    map[string]map[string]any // key → fields
	Links     []Link
	nextID    int
	Created   []map[string]any // POST /issue payloads
	Edits     []map[string]any // PUT /issue/{key} payloads
	recent    map[string]bool  // keys returned by "updated >= -Nm" searches
	posts     int
	LinkSwaps bool                             // true: this server applies POST /issueLink inward/outward the other way
	remote    map[string]map[string]RemoteLink // key → globalId → link
	srv       *httptest.Server
}

// RemoteLink is a stored web link (POST /issue/{key}/remotelink).
type RemoteLink struct {
	GlobalID string
	URL      string
	Title    string
	Resolved bool
	Posts    int // writes of this globalId
}

// RemoteLinks returns the web links of an issue, by globalId.
func (f *Server) RemoteLinks(key string) map[string]RemoteLink {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]RemoteLink{}
	for id, l := range f.remote[key] {
		out[id] = l
	}
	return out
}

// Link is a stored issue link: Outward <outward phrase> Inward.
type Link struct {
	ID, Type        string
	Outward, Inward string // "Outward blocks Inward"
}

var Statuses = map[string]map[string]any{
	"To Do":       {"id": "1", "name": "To Do", "statusCategory": map[string]any{"key": "new"}},
	"In Progress": {"id": "3", "name": "In Progress", "statusCategory": map[string]any{"key": "indeterminate"}},
	"Code Review": {"id": "4", "name": "Code Review", "statusCategory": map[string]any{"key": "indeterminate"}},
	"Done":        {"id": "5", "name": "Done", "statusCategory": map[string]any{"key": "done"}},
}

var types = map[string]map[string]any{
	"epic":    {"id": "10000", "name": "Epic", "hierarchyLevel": 1},
	"story":   {"id": "10001", "name": "Story", "hierarchyLevel": 0},
	"subtask": {"id": "10003", "name": "Subtask", "hierarchyLevel": -1, "subtask": true},
}

// New starts a fake Jira Cloud over HTTP; NewTLS over HTTPS.
func New(t testing.TB) *Server { return start(t, false) }

func NewTLS(t testing.TB) *Server { return start(t, true) }

func start(t testing.TB, useTLS bool) *Server {
	f := &Server{t: t, issues: map[string]map[string]any{}, recent: map[string]bool{}, nextID: 100}
	f.put("OPS-1", "Out-of-scope epic", "epic", "In Progress", "", nil) // parent outside scope
	f.put("PLAT-1", "Epic", "epic", "In Progress", "OPS-1", nil)
	f.put("PLAT-2", "Story", "story", "To Do", "PLAT-1", map[string]any{"customfield_10016": 5,
		// carried over from Sprint 41
		"customfield_10020": []any{map[string]any{"id": 41, "name": "Sprint 41", "state": "closed"}, map[string]any{"id": 42, "name": "Sprint 42", "state": "active"}},
		"customfield_10050": map[string]any{"id": "2", "value": "Major"},  // Severity
		"customfield_10021": []any{map[string]any{"value": "Impediment"}}, // Flagged
		"fixVersions":       []any{map[string]any{"id": "100", "name": "2.4.0"}},
		"duedate":           "2026-11-01",
		"timetracking":      map[string]any{"originalEstimate": "3d", "remainingEstimate": "1d", "timeSpent": "2d"},
		"description":       map[string]any{"type": "doc", "version": 1, "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Hello"}}}}},
	})
	f.put("PLAT-3", "Sub-task", "subtask", "Done", "PLAT-2", map[string]any{"security": map[string]any{"id": "1", "name": "Internal"}})
	if useTLS {
		f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	} else {
		f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	}
	t.Cleanup(f.srv.Close)
	return f
}

func (f *Server) put(key, summary, typ, status, parent string, extra map[string]any) {
	fields := map[string]any{
		"summary": summary, "issuetype": types[typ], "status": Statuses[status],
		"project": map[string]any{"key": strings.Split(key, "-")[0]},
		"updated": time.Now().Format("2006-01-02T15:04:05.000-0700"), "created": "2026-10-01T10:00:00.000-0400",
		"priority": map[string]any{"name": "Medium"}, "labels": []string{},
		"assignee": map[string]any{"accountId": "me", "displayName": "Miguel Rosales", "active": true},
	}
	if parent != "" {
		fields["parent"] = map[string]any{"key": parent}
	}
	for k, v := range extra {
		fields[k] = v
	}
	f.issues[key] = fields
}

func (f *Server) issueJSON(key string) map[string]any {
	fields := map[string]any{}
	for k, v := range f.issues[key] {
		fields[k] = v
	}
	var links []any
	for _, l := range f.Links {
		lt := map[string]any{"name": l.Type, "outward": "blocks", "inward": "is blocked by"}
		if l.Type == "Relates" {
			lt = map[string]any{"name": "Relates", "outward": "relates to", "inward": "relates to"}
		}
		if l.Outward == key {
			links = append(links, map[string]any{"id": l.ID, "type": lt, "outwardIssue": map[string]any{"key": l.Inward}})
		}
		if l.Inward == key {
			links = append(links, map[string]any{"id": l.ID, "type": lt, "inwardIssue": map[string]any{"key": l.Outward}})
		}
	}
	fields["issuelinks"] = links
	return map[string]any{"id": key, "key": key, "fields": fields}
}

var (
	keyIn       = regexp.MustCompile(`key in \(([^)]*)\)`)
	incremental = regexp.MustCompile(`updated >= -\d+m`)
)

func (f *Server) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(v any) { w.Header().Set("Content-Type", "application/json"); json.NewEncoder(w).Encode(v) }
	paged := func(vals any) { reply(map[string]any{"values": vals, "isLast": true}) }
	p := r.Method + " " + r.URL.Path

	switch {
	case p == "GET /rest/api/3/myself":
		reply(map[string]any{"accountId": "me", "displayName": "Miguel Rosales", "active": true})
	case p == "GET /rest/api/3/serverInfo":
		reply(map[string]any{"deploymentType": "Cloud"})
	case p == "GET /rest/api/3/field":
		reply([]any{
			map[string]any{"id": "customfield_10016", "name": "Story point estimate", "custom": true, "schema": map[string]any{"type": "number", "custom": "com.pyxis.greenhopper.jira:jsw-story-points"}},
			map[string]any{"id": "customfield_10020", "name": "Sprint", "custom": true, "schema": map[string]any{"custom": "com.pyxis.greenhopper.jira:gh-sprint"}},
			map[string]any{"id": "customfield_10021", "name": "Flagged", "custom": true, "schema": map[string]any{"type": "array", "items": "option", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:multicheckboxes"}},
			map[string]any{"id": "customfield_10050", "name": "Severity", "custom": true, "schema": map[string]any{"type": "option", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:select"}},
			map[string]any{"id": "customfield_10051", "name": "Acceptance Criteria", "custom": true, "schema": map[string]any{"type": "string", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}},
			map[string]any{"id": "customfield_10052", "name": "Customer", "custom": true, "schema": map[string]any{"type": "string", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:textfield"}},
		})
	case p == "GET /rest/api/3/project/PLAT/version":
		paged([]any{
			map[string]any{"id": "100", "name": "2.4.0", "released": false, "releaseDate": "2026-11-15"},
			map[string]any{"id": "101", "name": "2.5.0", "released": false},
			map[string]any{"id": "99", "name": "2.3.0", "released": true, "releaseDate": time.Now().AddDate(0, 0, -10).Format("2006-01-02")},
			map[string]any{"id": "90", "name": "1.0.0", "released": true, "archived": true},
		})
	case p == "GET /rest/api/3/configuration/timetracking":
		reply(map[string]any{"key": "JIRA", "name": "JIRA provided time tracking"})
	case p == "GET /rest/api/3/configuration/timetracking/options":
		reply(map[string]any{"workingHoursPerDay": 8, "workingDaysPerWeek": 5, "timeFormat": "pretty", "defaultUnit": "minute"})
	case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/editmeta"):
		reply(map[string]any{"fields": map[string]any{
			"fixVersions":       map[string]any{"name": "Fix versions", "schema": map[string]any{"type": "array", "items": "version", "system": "fixVersions"}, "allowedValues": []any{map[string]any{"name": "2.4.0"}, map[string]any{"name": "2.5.0"}}},
			"duedate":           map[string]any{"name": "Due date", "schema": map[string]any{"type": "date", "system": "duedate"}},
			"customfield_10050": map[string]any{"name": "Severity", "schema": map[string]any{"type": "option"}, "allowedValues": []any{map[string]any{"value": "Critical"}, map[string]any{"value": "Major"}, map[string]any{"value": "Minor"}}},
			"customfield_10051": map[string]any{"name": "Acceptance Criteria", "schema": map[string]any{"type": "string", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}},
		}})
	case r.Method == "GET" && strings.Contains(r.URL.Path, "/issue/createmeta/PLAT/issuetypes/"):
		reply(map[string]any{"fields": []any{
			map[string]any{"fieldId": "summary", "name": "Summary", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"fieldId": "customfield_10050", "name": "Severity", "required": false, "schema": map[string]any{"type": "option"}, "allowedValues": []any{map[string]any{"value": "Critical"}, map[string]any{"value": "Major"}}},
			map[string]any{"fieldId": "customfield_10052", "name": "Customer", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"fieldId": "customfield_10051", "name": "Acceptance Criteria", "required": false, "schema": map[string]any{"type": "string", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}},
		}})
	case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/"):
		key := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
		var body struct{ Fields map[string]any }
		json.NewDecoder(r.Body).Decode(&body)
		f.Edits = append(f.Edits, body.Fields)
		for k, v := range body.Fields {
			if v == nil {
				delete(f.issues[key], k)
				continue
			}
			f.issues[key][k] = v
		}
		f.touch(key)
		w.WriteHeader(204)
	case p == "GET /rest/api/3/priority/search":
		paged([]any{map[string]any{"id": "3", "name": "Medium"}, map[string]any{"id": "2", "name": "High"}})
	case p == "GET /rest/api/3/resolution/search":
		paged([]any{map[string]any{"id": "1", "name": "Done"}})
	case p == "GET /rest/api/3/issueLinkType":
		reply(map[string]any{"issueLinkTypes": []any{
			map[string]any{"id": "1", "name": "Blocks", "outward": "blocks", "inward": "is blocked by"},
			map[string]any{"id": "2", "name": "Relates", "outward": "relates to", "inward": "relates to"},
		}})
	case p == "GET /rest/api/3/project/PLAT":
		reply(map[string]any{"id": "10", "key": "PLAT"})
	case p == "GET /rest/api/3/issuetype/project":
		reply([]any{types["epic"], types["story"], types["subtask"]})
	case p == "GET /rest/api/3/statuses/search":
		paged([]any{
			map[string]any{"id": "1", "name": "To Do", "statusCategory": "TODO"},
			map[string]any{"id": "3", "name": "In Progress", "statusCategory": "IN_PROGRESS"},
			map[string]any{"id": "4", "name": "Code Review", "statusCategory": "IN_PROGRESS"},
			map[string]any{"id": "5", "name": "Done", "statusCategory": "DONE"},
		})
	case p == "GET /rest/api/3/mypermissions":
		reply(map[string]any{"permissions": map[string]any{}})
	case p == "GET /rest/api/3/user/assignable/search":
		reply([]any{map[string]any{"accountId": "me", "displayName": "Miguel Rosales", "active": true},
			map[string]any{"accountId": "ana", "displayName": "Ana Torres", "active": true}})
	case p == "GET /rest/api/3/project/PLAT/components":
		reply([]any{})
	case p == "GET /rest/agile/1.0/board":
		paged([]any{map[string]any{"id": 7, "name": "PLAT board", "type": "scrum"}})
	case p == "GET /rest/agile/1.0/board/7/configuration":
		reply(map[string]any{"columnConfig": map[string]any{"columns": []any{
			map[string]any{"name": "To Do", "statuses": []any{map[string]any{"id": "1"}}},
			map[string]any{"name": "Doing", "statuses": []any{map[string]any{"id": "3"}, map[string]any{"id": "4"}}},
			map[string]any{"name": "Done", "statuses": []any{map[string]any{"id": "5"}}},
		}}})
	case p == "GET /rest/agile/1.0/board/7/sprint":
		paged([]any{
			map[string]any{"id": 43, "name": "Sprint 43", "state": "future"},
			map[string]any{"id": 42, "name": "Sprint 42", "state": "active", "goal": "Ship it", "startDate": "2026-10-06T09:00:00.000Z", "endDate": "2026-10-20T17:00:00.000Z"},
			map[string]any{"id": 44, "name": "Hotfix sprint", "state": "active", "startDate": "2026-10-10T09:00:00.000Z"},
		})

	case p == "POST /rest/api/3/search/jql":
		var body struct{ JQL string }
		json.NewDecoder(r.Body).Decode(&body)
		var out []any
		switch {
		case keyIn.MatchString(body.JQL):
			for _, k := range strings.Split(keyIn.FindStringSubmatch(body.JQL)[1], ",") {
				if _, ok := f.issues[k]; ok {
					out = append(out, f.issueJSON(k))
				}
			}
		case incremental.MatchString(body.JQL):
			for k := range f.recent {
				out = append(out, f.issueJSON(k))
			}
		default:
			for k := range f.issues {
				if strings.HasPrefix(k, "PLAT-") {
					out = append(out, f.issueJSON(k))
				}
			}
		}
		reply(map[string]any{"issues": out, "isLast": true})

	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/") && strings.HasSuffix(r.URL.Path, "/transitions"):
		reply(map[string]any{"transitions": []any{
			map[string]any{"id": "11", "name": "Start", "to": Statuses["In Progress"]},
			map[string]any{"id": "21", "name": "Review", "to": Statuses["Code Review"]},
			map[string]any{"id": "31", "name": "Finish", "to": Statuses["Done"], "fields": map[string]any{"resolution": map[string]any{"required": true, "name": "Resolution"}}},
		}})
	case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/transitions"):
		key := strings.Split(r.URL.Path, "/")[5]
		var body struct {
			Transition struct{ ID string }
			Fields     map[string]any
		}
		json.NewDecoder(r.Body).Decode(&body)
		to := map[string]string{"11": "In Progress", "21": "Code Review", "31": "Done"}[body.Transition.ID]
		if to == "Done" && body.Fields["resolution"] == nil {
			w.WriteHeader(400)
			reply(map[string]any{"errors": map[string]string{"resolution": "Resolution is required."}})
			return
		}
		f.issues[key]["status"] = Statuses[to]
		f.touch(key)
		w.WriteHeader(204)
	case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/comment"):
		reply(map[string]any{"comments": []any{map[string]any{"id": "c1", "body": "plain text", "author": map[string]any{"accountId": "ana", "displayName": "Ana Torres"}, "created": "2026-10-02T10:00:00.000-0400"}}, "total": 1})
	case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/comment"):
		w.WriteHeader(201)
		reply(map[string]any{"id": "c2"})
	case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/remotelink"):
		key := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/"), "/remotelink")
		var body struct {
			GlobalID string `json:"globalId"`
			Object   struct {
				URL, Title string
				Status     *struct{ Resolved bool }
			}
		}
		json.NewDecoder(r.Body).Decode(&body)
		if f.remote == nil {
			f.remote = map[string]map[string]RemoteLink{}
		}
		if f.remote[key] == nil {
			f.remote[key] = map[string]RemoteLink{}
		}
		l := f.remote[key][body.GlobalID]
		l.GlobalID, l.URL, l.Title, l.Posts = body.GlobalID, body.Object.URL, body.Object.Title, l.Posts+1
		l.Resolved = body.Object.Status != nil && body.Object.Status.Resolved
		f.remote[key][body.GlobalID] = l
		w.WriteHeader(201)
		reply(map[string]any{"id": 1, "self": "x"})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/"):
		key := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
		if _, ok := f.issues[key]; !ok {
			w.WriteHeader(404)
			return
		}
		reply(f.issueJSON(key))
	case p == "POST /rest/api/3/issue":
		f.posts++
		var body struct{ Fields map[string]any }
		json.NewDecoder(r.Body).Decode(&body)
		f.Created = append(f.Created, body.Fields)
		project := body.Fields["project"].(map[string]any)["key"].(string)
		key := fmt.Sprintf("%s-%d", project, f.nextID)
		f.nextID++
		typ := map[string]string{"10000": "epic", "10001": "story", "10003": "subtask"}[body.Fields["issuetype"].(map[string]any)["id"].(string)]
		parent := ""
		if p, ok := body.Fields["parent"].(map[string]any); ok {
			parent = p["key"].(string)
		}
		f.put(key, body.Fields["summary"].(string), typ, "To Do", parent, nil)
		w.WriteHeader(201)
		reply(map[string]any{"id": key, "key": key})
	case p == "POST /rest/api/3/issueLink":
		var body struct {
			Type         struct{ Name string }
			InwardIssue  struct{ Key string }
			OutwardIssue struct{ Key string }
		}
		json.NewDecoder(r.Body).Decode(&body)
		l := Link{ID: fmt.Sprint(len(f.Links) + 1000), Type: body.Type.Name, Outward: body.InwardIssue.Key, Inward: body.OutwardIssue.Key}
		if f.LinkSwaps {
			l.Outward, l.Inward = l.Inward, l.Outward
		}
		f.Links = append(f.Links, l)
		w.WriteHeader(201)
	case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/issueLink/"):
		id := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issueLink/")
		for i, l := range f.Links {
			if l.ID == id {
				f.Links = append(f.Links[:i], f.Links[i+1:]...)
				break
			}
		}
		w.WriteHeader(204)
	default:
		f.t.Errorf("fake jira: unexpected %s", p)
		w.WriteHeader(404)
	}
}

func (f *Server) touch(key string) {
	f.issues[key]["updated"] = time.Now().Add(time.Duration(f.nextID) * time.Millisecond).Format("2006-01-02T15:04:05.000-0700")
	f.nextID++
}

// URL is the base URL of the fake site.
func (f *Server) URL() string { return f.srv.URL }

// Client is an HTTP client that trusts the fake's TLS certificate.
func (f *Server) Client() *http.Client { return f.srv.Client() }

// CertPEM is the TLS certificate in PEM form (for jira.tls.caFile).
func (f *Server) CertPEM() []byte {
	c := f.srv.Certificate()
	if c == nil {
		return nil
	}
	return []byte("-----BEGIN CERTIFICATE-----\n" + b64lines(c.Raw) + "-----END CERTIFICATE-----\n")
}

// SetStatus changes an issue's status as if someone did it in Jira, and makes
// the next incremental search return it.
func (f *Server) SetStatus(key, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issues[key]["status"] = Statuses[status]
	f.touch(key)
	f.recent[key] = true
}

// MarkRecent makes the next incremental search return key (unchanged).
func (f *Server) MarkRecent(key string) {
	f.mu.Lock()
	f.recent[key] = true
	f.mu.Unlock()
}

// Delete removes an issue.
func (f *Server) Delete(key string) {
	f.mu.Lock()
	delete(f.issues, key)
	f.mu.Unlock()
}

// Status reads an issue's current status name.
func (f *Server) Status(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.issues[key]["status"].(map[string]any)["name"].(string)
}

// Lock and Unlock give tests direct access to the stored state.
func (f *Server) Lock()   { f.mu.Lock() }
func (f *Server) Unlock() { f.mu.Unlock() }

func b64lines(der []byte) string {
	const enc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out strings.Builder
	n := 0
	for i := 0; i < len(der); i += 3 {
		var b [3]byte
		k := copy(b[:], der[i:])
		v := uint(b[0])<<16 | uint(b[1])<<8 | uint(b[2])
		chars := []byte{enc[v>>18&63], enc[v>>12&63], enc[v>>6&63], enc[v&63]}
		if k < 3 {
			chars[3] = '='
		}
		if k < 2 {
			chars[2] = '='
		}
		out.Write(chars)
		if n += 4; n == 64 {
			out.WriteByte('\n')
			n = 0
		}
	}
	if n > 0 {
		out.WriteByte('\n')
	}
	return out.String()
}
