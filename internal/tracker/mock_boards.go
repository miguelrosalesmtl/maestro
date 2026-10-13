package tracker

import (
	"fmt"
	"strings"
	"time"
)

// Demo boards: two scrum boards (one per project) and a kanban board.
var mockBoards = []BoardInfo{
	{ID: 7, Name: "PLAT board", Type: "scrum", Project: "PLAT"},
	{ID: 8, Name: "Onboarding squad", Type: "scrum", Project: "ONB"},
	{ID: 9, Name: "Platform kanban", Type: "kanban", Project: "PLAT"},
}

func mockSprints() []SprintInfo {
	day := func(d int) string { return time.Now().AddDate(0, 0, d).UTC().Format(time.RFC3339) }
	return []SprintInfo{
		{ID: 42, Name: CurrentSprint, State: "active", Goal: "Ship health-gated deploys", Start: day(-8), End: day(6)},
		{ID: 43, Name: "Sprint 43", State: "future"},
		{ID: 41, Name: "Sprint 41", State: "closed", Start: day(-22), End: day(-8)},
		{ID: 40, Name: "Sprint 40", State: "closed", Start: day(-36), End: day(-22)},
	}
}

// mockColumns: board columns → statuses.
var mockColumns = []BoardColumn{
	{Name: "To Do", Statuses: []string{StatusTodo}},
	{Name: "In Progress", Statuses: []string{StatusInProgress, StatusBlocked}},
	{Name: "Review", Statuses: []string{StatusInReview}},
	{Name: "Done", Statuses: []string{StatusDone}},
}

func (m *Mock) SearchBoards(query string) ([]BoardInfo, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	out := []BoardInfo{}
	for _, b := range mockBoards {
		if q == "" || strings.Contains(strings.ToLower(b.Name), q) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *Mock) board(id int) (BoardInfo, error) {
	for _, b := range mockBoards {
		if b.ID == id {
			return b, nil
		}
	}
	return BoardInfo{}, fmt.Errorf("board %d not found", id)
}

func (m *Mock) BoardSprints(boardID int) ([]SprintInfo, error) {
	b, err := m.board(boardID)
	if err != nil {
		return nil, err
	}
	if b.Type == "kanban" {
		return []SprintInfo{}, nil
	}
	return mockSprints(), nil
}

func (m *Mock) BoardIssues(boardID, sprintID int) (BoardView, error) {
	b, err := m.board(boardID)
	if err != nil {
		return BoardView{}, err
	}
	view := BoardView{Board: b, Issues: []Issue{}}
	sprint := ""
	for _, s := range mockSprints() {
		if s.ID == sprintID && b.Type != "kanban" {
			s := s
			view.Sprint, sprint = &s, s.Name
		}
	}
	for _, c := range mockColumns {
		view.Columns = append(view.Columns, BoardColumn{Name: c.Name, Statuses: c.Statuses, Keys: []string{}})
	}
	for _, is := range m.ListIssues() {
		if !strings.HasPrefix(is.Key, b.Project+"-") || is.Type == TypeEpic || is.Type == TypeInitiative || is.Type == TypeSubtask {
			continue
		}
		if sprint != "" && is.Sprint != sprint {
			continue
		}
		if sprint == "" && is.StatusCategory == "done" {
			continue
		}
		for i, c := range view.Columns {
			for _, s := range c.Statuses {
				if s == is.Status {
					view.Columns[i].Keys = append(view.Columns[i].Keys, is.Key)
					view.Issues = append(view.Issues, is)
				}
			}
		}
	}
	return view, nil
}
