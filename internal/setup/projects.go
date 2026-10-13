package setup

import (
	"context"
	"sort"

	"maestro/internal/tracker/jira"
)

// SuggestedProject is a project the user recently worked in.
type SuggestedProject struct {
	Key    string
	Name   string
	Issues int
}

// SuggestProjects is setup: projects of issues the user is assigned
// to, reported or watches, updated in the last 60 days, most used first.
func SuggestProjects(ctx context.Context, c *jira.Client) ([]SuggestedProject, error) {
	const jql = "(assignee = currentUser() OR reporter = currentUser() OR watcher = currentUser()) AND updated >= -60d ORDER BY updated DESC"
	counts := map[string]*SuggestedProject{}
	seen := 0
	for is, err := range c.Search(ctx, jira.SearchOptions{JQL: jql, Fields: []string{"project"}}) {
		if err != nil {
			return nil, err
		}
		if p := is.Fields.Project; p != nil {
			if counts[p.Key] == nil {
				counts[p.Key] = &SuggestedProject{Key: p.Key, Name: p.Name}
			}
			counts[p.Key].Issues++
		}
		if seen++; seen >= 500 { // enough to rank; keeps setup fast
			break
		}
	}
	out := make([]SuggestedProject, 0, len(counts))
	for _, p := range counts {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Issues != out[j].Issues {
			return out[i].Issues > out[j].Issues
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}
