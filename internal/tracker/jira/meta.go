package jira

import (
	"context"
	"fmt"
	"iter"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Discovery reads. Callers cache the results.

// Myself returns the current user (proves the token works).
func (c *Client) Myself(ctx context.Context) (*User, error) {
	var u User
	return &u, c.get(ctx, c.platform("/myself"), nil, &u)
}

// ServerInfo is. Its ServerTime is the clock for incremental sync.
func (c *Client) ServerInfo(ctx context.Context) (*ServerInfo, error) {
	var s ServerInfo
	return &s, c.get(ctx, c.platform("/serverInfo"), nil, &s)
}

// Projects returns every project the user can browse, with issue types.
func (c *Client) Projects(ctx context.Context) iter.Seq2[Project, error] {
	if c.deployment == DataCenter {
		return arrayAsSeq[Project](ctx, c, c.platform("/project"), url.Values{"expand": {"issueTypes,lead"}})
	}
	return pagesByOffset[Project](ctx, c, c.platform("/project/search"), url.Values{"expand": {"issueTypes,lead"}}, 50)
}

// Project reads one project with its issue types.
func (c *Client) Project(ctx context.Context, keyOrID string) (*Project, error) {
	var p Project
	return &p, c.get(ctx, c.platform("/project/"+url.PathEscape(keyOrID)), url.Values{"expand": {"issueTypes"}}, &p)
}

// ProjectIssueTypes lists a project's issue types.
func (c *Client) ProjectIssueTypes(ctx context.Context, projectID string) ([]IssueType, error) {
	if c.deployment == DataCenter {
		p, err := c.Project(ctx, projectID)
		if err != nil {
			return nil, err
		}
		return p.IssueTypes, nil
	}
	var out []IssueType
	return out, c.get(ctx, c.platform("/issuetype/project"), url.Values{"projectId": {projectID}}, &out)
}

// Fields returns all system and custom fields.
func (c *Client) Fields(ctx context.Context) ([]Field, error) {
	var out []Field
	return out, c.get(ctx, c.platform("/field"), nil, &out)
}

// CreateMeta returns the fields of the create screen for one issue type (FLD-11).
func (c *Client) CreateMeta(ctx context.Context, projectKey, issueTypeID string) ([]CreateMetaField, error) {
	path := c.platform(fmt.Sprintf("/issue/createmeta/%s/issuetypes/%s", url.PathEscape(projectKey), url.PathEscape(issueTypeID)))
	if c.deployment == DataCenter {
		return Collect(pagesByOffset[CreateMetaField](ctx, c, path, nil, 50))
	}
	// Cloud returns {"fields":[…]} or a paged {"values":[…]} depending on version.
	var page struct {
		Fields []CreateMetaField `json:"fields"`
		Values []CreateMetaField `json:"values"`
	}
	if err := c.get(ctx, path, url.Values{"maxResults": {"200"}}, &page); err != nil {
		return nil, err
	}
	return append(page.Fields, page.Values...), nil
}

// Statuses is. projectID may be empty for all statuses.
func (c *Client) Statuses(ctx context.Context, projectID string) iter.Seq2[StatusDef, error] {
	if c.deployment == DataCenter {
		return func(yield func(StatusDef, error) bool) {
			for s, err := range arrayAsSeq[Status](ctx, c, c.platform("/status"), nil) {
				d := StatusDef{ID: s.ID, Name: s.Name, StatusCategory: categoryFromKey(s.StatusCategory.Key)}
				if !yield(d, err) || err != nil {
					return
				}
			}
		}
	}
	q := url.Values{}
	if projectID != "" {
		q.Set("projectId", projectID)
	}
	return pagesByOffset[StatusDef](ctx, c, c.platform("/statuses/search"), q, 200)
}

// categoryFromKey maps an issue's statusCategory.key to /statuses/search's form.
func categoryFromKey(k string) string {
	switch k {
	case "new":
		return "TODO"
	case "done":
		return "DONE"
	default:
		return "IN_PROGRESS"
	}
}

// Priorities lists priorities.
func (c *Client) Priorities(ctx context.Context) iter.Seq2[Priority, error] {
	if c.deployment == DataCenter {
		return arrayAsSeq[Priority](ctx, c, c.platform("/priority"), nil)
	}
	return pagesByOffset[Priority](ctx, c, c.platform("/priority/search"), nil, 100)
}

// Resolutions lists resolutions.
func (c *Client) Resolutions(ctx context.Context) iter.Seq2[Resolution, error] {
	if c.deployment == DataCenter {
		return arrayAsSeq[Resolution](ctx, c, c.platform("/resolution"), nil)
	}
	return pagesByOffset[Resolution](ctx, c, c.platform("/resolution/search"), nil, 100)
}

// IssueLinkTypes lists issue link types.
func (c *Client) IssueLinkTypes(ctx context.Context) ([]IssueLinkType, error) {
	var out struct {
		IssueLinkTypes []IssueLinkType `json:"issueLinkTypes"`
	}
	return out.IssueLinkTypes, c.get(ctx, c.platform("/issueLinkType"), nil, &out)
}

// Components lists a project's components.
func (c *Client) Components(ctx context.Context, projectKey string) ([]Component, error) {
	var out []Component
	return out, c.get(ctx, c.platform("/project/"+url.PathEscape(projectKey)+"/components"), nil, &out)
}

// Versions returns every version of a project, newest release first.
func (c *Client) Versions(ctx context.Context, projectKey string) iter.Seq2[Version, error] {
	path := c.platform("/project/" + url.PathEscape(projectKey) + "/version")
	if c.deployment == DataCenter {
		return arrayAsSeq[Version](ctx, c, c.platform("/project/"+url.PathEscape(projectKey)+"/versions"), nil)
	}
	return pagesByOffset[Version](ctx, c, path, url.Values{"orderBy": {"-releaseDate"}}, 50)
}

// EditMeta returns the fields that can be edited on one issue, with
// allowed values.
func (c *Client) EditMeta(ctx context.Context, key string) (map[string]CreateMetaField, error) {
	var out struct {
		Fields map[string]CreateMetaField `json:"fields"`
	}
	if err := c.get(ctx, c.platform("/issue/"+url.PathEscape(key)+"/editmeta"), nil, &out); err != nil {
		return nil, err
	}
	for id, f := range out.Fields { // editmeta's map key is the field ID
		f.FieldID = id
		out.Fields[id] = f
	}
	return out.Fields, nil
}

// TimeTrackingConfig is. Enabled is false when time tracking is off.
type TimeTrackingConfig struct {
	Enabled            bool    `json:"enabled"`
	WorkingHoursPerDay float64 `json:"workingHoursPerDay"`
	WorkingDaysPerWeek float64 `json:"workingDaysPerWeek"`
	TimeFormat         string  `json:"timeFormat"`
	DefaultUnit        string  `json:"defaultUnit"`
}

func (c *Client) TimeTracking(ctx context.Context) (*TimeTrackingConfig, error) {
	var provider struct {
		Key string `json:"key"`
	}
	if err := c.get(ctx, c.platform("/configuration/timetracking"), nil, &provider); err != nil {
		return nil, err
	}
	out := &TimeTrackingConfig{Enabled: provider.Key != ""}
	if !out.Enabled {
		return out, nil
	}
	if err := c.get(ctx, c.platform("/configuration/timetracking/options"), nil, out); err != nil {
		return nil, err
	}
	out.Enabled = true
	return out, nil
}

// UnreleasedVersions lists a project's unreleased versions.
func (c *Client) UnreleasedVersions(ctx context.Context, projectKey string) ([]Version, error) {
	var out []Version
	if err := c.get(ctx, c.platform("/project/"+url.PathEscape(projectKey)+"/versions"), nil, &out); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(out, func(v Version) bool { return v.Released || v.Archived }), nil
}

// AssignableUsers lists users who can be assigned issues in a project.
func (c *Client) AssignableUsers(ctx context.Context, projectKey string) iter.Seq2[User, error] {
	return pagesOfArray[User](ctx, c, c.platform("/user/assignable/search"), url.Values{"project": {projectKey}}, 100)
}

// Permissions Maestro checks before writes (USR-04).
var WritePermissions = []string{
	"BROWSE_PROJECTS", "CREATE_ISSUES", "EDIT_ISSUES", "TRANSITION_ISSUES", "LINK_ISSUES",
	"ASSIGN_ISSUES", "ADD_COMMENTS", "CREATE_ATTACHMENTS", "SCHEDULE_ISSUES",
}

// MyPermissions is. The result maps permission key → granted.
func (c *Client) MyPermissions(ctx context.Context, projectKey string, keys []string) (map[string]bool, error) {
	var out struct {
		Permissions map[string]Permission `json:"permissions"`
	}
	q := url.Values{"projectKey": {projectKey}, "permissions": {strings.Join(keys, ",")}}
	if err := c.get(ctx, c.platform("/mypermissions"), q, &out); err != nil {
		return nil, err
	}
	got := make(map[string]bool, len(keys))
	for _, k := range keys {
		got[k] = out.Permissions[k].HavePermission
	}
	return got, nil
}

// Boards lists a project's agile boards.
func (c *Client) Boards(ctx context.Context, projectKey string) iter.Seq2[Board, error] {
	return pagesByOffset[Board](ctx, c, agile("/board"), url.Values{"projectKeyOrId": {projectKey}}, 50)
}

// BoardConfiguration is (columns → statuses, filter, rank field).
func (c *Client) BoardConfiguration(ctx context.Context, boardID int) (*BoardConfig, error) {
	var bc BoardConfig
	return &bc, c.get(ctx, agile("/board/"+strconv.Itoa(boardID)+"/configuration"), nil, &bc)
}

// Filter returns a saved filter.
func (c *Client) Filter(ctx context.Context, id string) (*Filter, error) {
	var f Filter
	return &f, c.get(ctx, c.platform("/filter/"+url.PathEscape(id)), nil, &f)
}

// Sprints is. states: e.g. "active", "future".
func (c *Client) Sprints(ctx context.Context, boardID int, states ...string) iter.Seq2[Sprint, error] {
	q := url.Values{}
	if len(states) > 0 {
		q.Set("state", strings.Join(states, ","))
	}
	return pagesByOffset[Sprint](ctx, c, agile("/board/"+strconv.Itoa(boardID)+"/sprint"), q, 50)
}

// Labels is (suggestions for the create dialog).
func (c *Client) Labels(ctx context.Context) iter.Seq2[string, error] {
	return pagesByOffset[string](ctx, c, c.platform("/label"), nil, 1000)
}

// arrayAsSeq wraps an endpoint that returns the whole list in one array.
func arrayAsSeq[T any](ctx context.Context, c *Client, path string, q url.Values) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var all []T
		if err := c.get(ctx, path, q, &all); err != nil {
			var zero T
			yield(zero, err)
			return
		}
		for _, v := range all {
			if !yield(v, nil) {
				return
			}
		}
	}
}
