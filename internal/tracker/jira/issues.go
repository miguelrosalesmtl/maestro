package jira

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ── Reads ────────────────────────────────

// ListFields is the small field set for list views (5.2). Add the discovered
// story points and sprint field IDs.
var ListFields = []string{"summary", "status", "issuetype", "priority", "assignee", "parent", "labels", "updated"}

type SearchOptions struct {
	JQL      string
	Fields   []string // default ListFields; never "*all" in list views (7.2)
	Expand   []string
	PageSize int // ≤ 100, default 100
}

// Search runs a JQL search. Cloud uses /search/jql with nextPageToken (the old
// /search endpoint is removed on Cloud); Data Center uses /search with startAt.
func (c *Client) Search(ctx context.Context, o SearchOptions) iter.Seq2[Issue, error] {
	if len(o.Fields) == 0 {
		o.Fields = ListFields
	}
	if o.PageSize <= 0 || o.PageSize > 100 {
		o.PageSize = 100
	}
	return func(yield func(Issue, error) bool) {
		var token string
		startAt := 0
		for {
			body := map[string]any{"jql": o.JQL, "fields": o.Fields, "maxResults": o.PageSize}
			if len(o.Expand) > 0 {
				body["expand"] = strings.Join(o.Expand, ",")
			}
			var page struct {
				Issues        []Issue `json:"issues"`
				NextPageToken string  `json:"nextPageToken"`
				IsLast        *bool   `json:"isLast"`
				Total         int     `json:"total"`
			}
			path := c.platform("/search/jql")
			if c.deployment == DataCenter {
				path = c.platform("/search")
				body["startAt"] = startAt
			} else if token != "" {
				body["nextPageToken"] = token
			}
			err := c.do(ctx, request{method: http.MethodPost, path: path, body: body, out: &page, idempotent: true})
			if err != nil {
				yield(Issue{}, err)
				return
			}
			for _, is := range page.Issues {
				if !yield(is, nil) {
					return
				}
			}
			startAt += len(page.Issues)
			if c.deployment == DataCenter {
				if len(page.Issues) == 0 || startAt >= page.Total {
					return
				}
				continue
			}
			if page.NextPageToken == "" || (page.IsLast != nil && *page.IsLast) || len(page.Issues) == 0 {
				return
			}
			token = page.NextPageToken
		}
	}
}

// IssuesByKey batches key lookups into searches of ≤ 100 keys (rule 7.3).
func (c *Client) IssuesByKey(ctx context.Context, keys []string, fields []string) ([]Issue, error) {
	var out []Issue
	for start := 0; start < len(keys); start += 100 {
		chunk := keys[start:min(start+100, len(keys))]
		quoted := make([]string, len(chunk))
		for i, k := range chunk {
			if !validKey(k) {
				return nil, fmt.Errorf("jira: invalid issue key %q", k)
			}
			quoted[i] = k
		}
		got, err := Collect(c.Search(ctx, SearchOptions{JQL: "key in (" + strings.Join(quoted, ",") + ")", Fields: fields}))
		if err != nil {
			return out, err
		}
		out = append(out, got...)
	}
	return out, nil
}

// Issue reads one issue. fields nil = all navigable fields.
func (c *Client) Issue(ctx context.Context, key string, fields []string, expand ...string) (*Issue, error) {
	q := url.Values{}
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	}
	if len(expand) > 0 {
		q.Set("expand", strings.Join(expand, ","))
	}
	var is Issue
	return &is, c.get(ctx, c.platform("/issue/"+url.PathEscape(key)), q, &is)
}

// Children lists the child issues of an issue.
func (c *Client) Children(ctx context.Context, parentKey string, fields []string) iter.Seq2[Issue, error] {
	if !validKey(parentKey) { // keys go into JQL unquoted
		return func(yield func(Issue, error) bool) {
			yield(Issue{}, fmt.Errorf("jira: invalid issue key %q", parentKey))
		}
	}
	return c.Search(ctx, SearchOptions{JQL: fmt.Sprintf("parent = %s ORDER BY rank", parentKey), Fields: fields})
}

// Transitions lists the transitions available on an issue, with the fields each transition screen requires (WFL-05).
func (c *Client) Transitions(ctx context.Context, key string) ([]Transition, error) {
	var out struct {
		Transitions []Transition `json:"transitions"`
	}
	q := url.Values{"expand": {"transitions.fields"}}
	return out.Transitions, c.get(ctx, c.platform("/issue/"+url.PathEscape(key)+"/transitions"), q, &out)
}

// Comments lists comments, newest first.
func (c *Client) Comments(ctx context.Context, key string) iter.Seq2[Comment, error] {
	q := url.Values{"orderBy": {"-created"}, "expand": {"renderedBody"}}
	return func(yield func(Comment, error) bool) {
		start := 0
		for {
			q.Set("startAt", strconv.Itoa(start))
			var page struct {
				Comments []Comment `json:"comments"`
				Total    int       `json:"total"`
			}
			if err := c.get(ctx, c.platform("/issue/"+url.PathEscape(key)+"/comment"), q, &page); err != nil {
				yield(Comment{}, err)
				return
			}
			for _, cm := range page.Comments {
				if !yield(cm, nil) {
					return
				}
			}
			start += len(page.Comments)
			if len(page.Comments) == 0 || start >= page.Total {
				return
			}
		}
	}
}

// ChangelogEntry is one history record. Used for the History tab and
// to detect Jira Automation moving issues (AUT-03).
type ChangelogEntry struct {
	ID      string `json:"id"`
	Author  *User  `json:"author"`
	Created string `json:"created"`
	Items   []struct {
		Field      string `json:"field"`
		FieldID    string `json:"fieldId"`
		From       string `json:"from"`
		FromString string `json:"fromString"`
		To         string `json:"to"`
		ToString   string `json:"toString"`
	} `json:"items"`
}

// Changelog lists an issue's history (Cloud). Data Center returns history via
// expand=changelog on Issue.
func (c *Client) Changelog(ctx context.Context, key string) iter.Seq2[ChangelogEntry, error] {
	return pagesByOffset[ChangelogEntry](ctx, c, c.platform("/issue/"+url.PathEscape(key)+"/changelog"), nil, 100)
}

// SearchUsers is (mentions).
func (c *Client) SearchUsers(ctx context.Context, query string) ([]User, error) {
	var out []User
	q := url.Values{"query": {query}, "maxResults": {"20"}}
	if c.deployment == DataCenter {
		q = url.Values{"username": {query}, "maxResults": {"20"}}
	}
	return out, c.get(ctx, c.platform("/user/search"), q, &out)
}

// ── Writes ───────────────────────────────
//
// Rule 5.3.5: POSTs that create data are never retried on 5xx or timeouts
// (client.do enforces it). After a timeout, search for the result before
// trying again.

// CreateInput uses Markdown for the description; the client converts it to
// ADF (Cloud) or plain text (Data Center).
type CreateInput struct {
	ProjectKey  string
	IssueTypeID string
	Summary     string
	Description string // Markdown
	ParentKey   string
	AssigneeID  string // accountId (Cloud) or name (DC)
	PriorityID  string
	Labels      []string
	// Extra holds discovered custom fields, e.g. story points or sprint ID.
	Extra map[string]any
}

// CreateIssue creates an issue.
func (c *Client) CreateIssue(ctx context.Context, in CreateInput) (*CreatedIssue, error) {
	if in.ProjectKey == "" || in.IssueTypeID == "" || strings.TrimSpace(in.Summary) == "" {
		return nil, errors.New("jira: project, issue type and summary are required")
	}
	f := map[string]any{
		"project":   map[string]string{"key": in.ProjectKey},
		"issuetype": map[string]string{"id": in.IssueTypeID},
		"summary":   strings.TrimSpace(in.Summary),
	}
	if in.Description != "" {
		f["description"] = c.richText(in.Description)
	}
	if in.ParentKey != "" {
		f["parent"] = map[string]string{"key": in.ParentKey}
	}
	if in.AssigneeID != "" {
		f["assignee"] = c.userRef(in.AssigneeID)
	}
	if in.PriorityID != "" {
		f["priority"] = map[string]string{"id": in.PriorityID}
	}
	if len(in.Labels) > 0 {
		f["labels"] = in.Labels
	}
	for k, v := range in.Extra {
		f[k] = v
	}
	var out CreatedIssue
	return &out, c.send(ctx, http.MethodPost, c.platform("/issue"), map[string]any{"fields": f}, &out)
}

// EditIssue edits fields of an issue. fields are raw Jira field values.
func (c *Client) EditIssue(ctx context.Context, key string, fields map[string]any) error {
	return c.do(ctx, request{method: http.MethodPut, path: c.platform("/issue/" + url.PathEscape(key)),
		query: url.Values{"notifyUsers": {"true"}}, body: map[string]any{"fields": fields}})
}

// Assign assigns an issue. An empty id unassigns.
func (c *Client) Assign(ctx context.Context, key, userID string) error {
	var body any = map[string]any{"accountId": nil}
	if userID != "" {
		body = c.userRef(userID)
	} else if c.deployment == DataCenter {
		body = map[string]any{"name": nil}
	}
	return c.send(ctx, http.MethodPut, c.platform("/issue/"+url.PathEscape(key)+"/assignee"), body, nil)
}

// DoTransition moves an issue through a transition. fields carries the screen values the transition
// requires (see Transition.RequiredFields), e.g. {"resolution":{"name":"Done"}}.
func (c *Client) DoTransition(ctx context.Context, key, transitionID string, fields map[string]any) error {
	body := map[string]any{"transition": map[string]string{"id": transitionID}}
	if len(fields) > 0 {
		body["fields"] = fields
	}
	return c.send(ctx, http.MethodPost, c.platform("/issue/"+url.PathEscape(key)+"/transitions"), body, nil)
}

// AddComment adds a comment.
func (c *Client) AddComment(ctx context.Context, key, markdown string) (*Comment, error) {
	var out Comment
	return &out, c.send(ctx, http.MethodPost, c.platform("/issue/"+url.PathEscape(key)+"/comment"), map[string]any{"body": c.richText(markdown)}, &out)
}

// LinkIssues links two issues. The arguments mirror the REST payload exactly: Jira
// decides which side reads "blocks" and which "is blocked by" from
// inwardIssue/outwardIssue. Verify the direction with a contract test
// (§14) before mapping Maestro's "A blocks B" onto it.
func (c *Client) LinkIssues(ctx context.Context, linkTypeName, inwardKey, outwardKey string) error {
	body := map[string]any{
		"type":         map[string]string{"name": linkTypeName},
		"inwardIssue":  map[string]string{"key": inwardKey},
		"outwardIssue": map[string]string{"key": outwardKey},
	}
	return c.send(ctx, http.MethodPost, c.platform("/issueLink"), body, nil)
}

// DeleteIssueLink deletes an issue link.
func (c *Client) DeleteIssueLink(ctx context.Context, linkID string) error {
	return c.send(ctx, http.MethodDelete, c.platform("/issueLink/"+url.PathEscape(linkID)), nil, nil)
}

// UpsertRemoteLink adds or updates a web link on an issue. The same GlobalID (the PR URL) updates the link.
// It is safe to retry, so it is marked idempotent.
func (c *Client) UpsertRemoteLink(ctx context.Context, key string, link RemoteLink) error {
	return c.do(ctx, request{method: http.MethodPost, path: c.platform("/issue/" + url.PathEscape(key) + "/remotelink"), body: link, idempotent: link.GlobalID != ""})
}

// MoveToSprint moves issues into a sprint, in chunks of 50 issues.
func (c *Client) MoveToSprint(ctx context.Context, sprintID int, keys []string) error {
	for start := 0; start < len(keys); start += 50 {
		chunk := keys[start:min(start+50, len(keys))]
		if err := c.do(ctx, request{method: http.MethodPost, path: agile("/sprint/" + strconv.Itoa(sprintID) + "/issue"), body: map[string]any{"issues": chunk}, idempotent: true}); err != nil {
			return err
		}
	}
	return nil
}

// MoveToBacklog moves issues to the backlog.
func (c *Client) MoveToBacklog(ctx context.Context, keys []string) error {
	for start := 0; start < len(keys); start += 50 {
		chunk := keys[start:min(start+50, len(keys))]
		if err := c.do(ctx, request{method: http.MethodPost, path: agile("/backlog/issue"), body: map[string]any{"issues": chunk}, idempotent: true}); err != nil {
			return err
		}
	}
	return nil
}

// Rank reorders an issue on the board. Give exactly one of before or after.
func (c *Client) Rank(ctx context.Context, key, before, after string) error {
	if (before == "") == (after == "") {
		return errors.New("jira: rank needs exactly one of before or after")
	}
	body := map[string]any{"issues": []string{key}}
	if before != "" {
		body["rankBeforeIssue"] = before
	} else {
		body["rankAfterIssue"] = after
	}
	return c.do(ctx, request{method: http.MethodPut, path: agile("/issue/rank"), body: body})
}

// Watch and Unwatch are for the current user.
func (c *Client) Watch(ctx context.Context, key string) error {
	return c.send(ctx, http.MethodPost, c.platform("/issue/"+url.PathEscape(key)+"/watchers"), nil, nil)
}

func (c *Client) Unwatch(ctx context.Context, key, userID string) error {
	q := url.Values{"accountId": {userID}}
	if c.deployment == DataCenter {
		q = url.Values{"username": {userID}}
	}
	return c.do(ctx, request{method: http.MethodDelete, path: c.platform("/issue/" + url.PathEscape(key) + "/watchers"), query: q})
}

// ── helpers ─────────────────────────────────────────────────────

// richText is the description/comment format for the deployment.
func (c *Client) richText(markdown string) any {
	if c.deployment == DataCenter {
		return markdown // wiki markup conversion is a later step; Markdown reads acceptably
	}
	return FromMarkdown(markdown)
}

func (c *Client) userRef(id string) map[string]any {
	if c.deployment == DataCenter {
		return map[string]any{"name": id}
	}
	return map[string]any{"accountId": id}
}

func validKey(k string) bool {
	project, num, ok := strings.Cut(k, "-")
	if !ok || project == "" || num == "" {
		return false
	}
	for _, r := range project {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	_, err := strconv.Atoi(num)
	return err == nil
}
