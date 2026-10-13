package live

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"maestro/internal/tracker"
	"maestro/internal/tracker/jira"
)

// Boards: any board of the site, not only those of the configured projects.
// Issues shown on a board are read on demand and are not added to the synced
// set.

const (
	boardSearchMax  = 50
	boardIssuesMax  = 500
	recentClosedMax = 5
)

func boardInfo(b jira.Board) tracker.BoardInfo {
	bi := tracker.BoardInfo{ID: b.ID, Name: b.Name, Type: b.Type}
	if b.Location != nil {
		bi.Project = b.Location.ProjectKey
	}
	return bi
}

func (t *Tracker) SearchBoards(query string) ([]tracker.BoardInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	bs, err := t.c.SearchBoards(ctx, strings.TrimSpace(query), boardSearchMax)
	if err != nil {
		return nil, fmt.Errorf("search boards: %w", err)
	}
	out := make([]tracker.BoardInfo, 0, len(bs))
	for _, b := range bs {
		out = append(out, boardInfo(b))
	}
	return out, nil
}

func sprintInfo(s jira.Sprint, project string) tracker.SprintInfo {
	return tracker.SprintInfo{ID: s.ID, Name: s.Name, State: s.State, Goal: s.Goal,
		Start: isoTime(s.StartDate), End: isoTime(firstOf(s.CompleteDate, s.EndDate)), Project: project}
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// noSprints is Jira's answer for a kanban board ("The board does not support sprints").
func noSprints(err error) bool {
	var api *jira.APIError
	return errors.As(err, &api) && api.Status == 400
}

func (t *Tracker) BoardSprints(boardID int) ([]tracker.SprintInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var open, closed []tracker.SprintInfo
	for s, err := range t.c.Sprints(ctx, boardID, "active", "future") {
		if err != nil {
			if noSprints(err) {
				return []tracker.SprintInfo{}, nil
			}
			return nil, fmt.Errorf("sprints of board %d: %w", boardID, err)
		}
		open = append(open, sprintInfo(s, ""))
	}
	// Closed sprints come oldest first; keep the last few (bounded read).
	n := 0
	for s, err := range t.c.Sprints(ctx, boardID, "closed") {
		if err != nil {
			t.log.Warn("closed sprints not loaded", "board", boardID, "err", err.Error())
			break
		}
		closed = append(closed, sprintInfo(s, ""))
		if n++; n >= 500 {
			break
		}
	}
	slices.SortStableFunc(open, func(a, b tracker.SprintInfo) int { // active first
		return strings.Compare(a.State, b.State)
	})
	if len(closed) > recentClosedMax {
		closed = closed[len(closed)-recentClosedMax:]
	}
	slices.Reverse(closed) // most recent first
	return append(open, closed...), nil
}

var orderBy = regexp.MustCompile(`(?is)\s+ORDER\s+BY\s+.*$`)

// boardJQL is the board's own filter (without ORDER BY), or "" if it cannot be read.
func (t *Tracker) boardJQL(ctx context.Context, filterID string) string {
	if filterID == "" {
		return ""
	}
	f, err := t.c.Filter(ctx, filterID)
	if err != nil {
		t.log.Info("board filter not readable; showing the sprint without it", "filter", filterID, "err", err.Error())
		return ""
	}
	return strings.TrimSpace(orderBy.ReplaceAllString(f.JQL, ""))
}

func (t *Tracker) BoardIssues(boardID, sprintID int) (tracker.BoardView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*callTimeout)
	defer cancel()
	b, err := t.c.Board(ctx, boardID)
	if err != nil {
		return tracker.BoardView{}, fmt.Errorf("board %d: %w", boardID, err)
	}
	cfg, err := t.c.BoardConfiguration(ctx, boardID)
	if err != nil {
		return tracker.BoardView{}, fmt.Errorf("board %d configuration: %w", boardID, err)
	}
	view := tracker.BoardView{Board: boardInfo(*b), Columns: []tracker.BoardColumn{}, Issues: []tracker.Issue{}}

	var jql string
	filter := t.boardJQL(ctx, cfg.Filter.ID)
	if sprintID > 0 {
		jql = "sprint = " + strconv.Itoa(sprintID)
		if filter != "" {
			jql = "(" + filter + ") AND " + jql
		}
		for s, err := range t.c.Sprints(ctx, boardID) {
			if err != nil {
				break
			}
			if s.ID == sprintID {
				si := sprintInfo(s, view.Board.Project)
				view.Sprint = &si
				break
			}
		}
	} else {
		// No sprint (kanban): open work plus what was resolved in the last two weeks.
		jql = "(statusCategory != Done OR resolved >= -14d)"
		if filter != "" {
			jql = "(" + filter + ") AND " + jql
		}
	}

	names := t.statusNames()
	colOf := map[string]int{} // status ID → column
	for i, c := range cfg.ColumnConfig.Columns {
		col := tracker.BoardColumn{Name: c.Name, Keys: []string{}, Statuses: []string{}}
		for _, st := range c.Statuses {
			colOf[st.ID] = i
			if n := names[st.ID]; n != "" {
				col.Statuses = append(col.Statuses, n)
			}
		}
		view.Columns = append(view.Columns, col)
	}

	for raw, err := range t.c.Search(ctx, jira.SearchOptions{JQL: jql + " ORDER BY Rank ASC", Fields: t.fields()}) {
		if err != nil {
			return view, fmt.Errorf("board %d issues: %w", boardID, err)
		}
		if len(view.Issues) >= boardIssuesMax {
			view.Truncated = true
			break
		}
		is := t.m.issueWith(&raw, false)
		view.Issues = append(view.Issues, is)
		if st := raw.Fields.Status; st != nil {
			i, ok := colOf[st.ID]
			if !ok {
				continue // status not mapped to a column: Jira hides it too
			}
			view.Columns[i].Keys = append(view.Columns[i].Keys, is.Key)
			if !slices.Contains(view.Columns[i].Statuses, st.Name) {
				view.Columns[i].Statuses = append(view.Columns[i].Statuses, st.Name)
			}
		}
	}
	return view, nil
}

// statusNames maps status IDs to names from discovery (may be partial when
// the user cannot list statuses; issue statuses fill the gaps).
func (t *Tracker) statusNames() map[string]string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := map[string]string{}
	if t.disc == nil {
		return out
	}
	for _, p := range t.disc.Projects {
		for _, s := range p.Statuses {
			out[s.ID] = s.Name
		}
	}
	return out
}
