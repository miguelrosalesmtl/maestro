package main

import (
	"errors"

	"maestro/internal/tracker"
)

// Boards: browse any Jira board and sprint (Board view → Find board).

func (a *App) boards() (tracker.BoardBrowser, error) {
	b, ok := a.tr().(tracker.BoardBrowser)
	if !ok {
		return nil, errors.New("this tracker cannot browse boards")
	}
	return b, nil
}

func (a *App) SearchBoards(query string) ([]tracker.BoardInfo, error) {
	b, err := a.boards()
	if err != nil {
		return nil, err
	}
	out, err := b.SearchBoards(query)
	return orEmpty(out), a.fail("search boards", err)
}

func (a *App) GetBoardSprints(boardID int) ([]tracker.SprintInfo, error) {
	b, err := a.boards()
	if err != nil {
		return nil, err
	}
	out, err := b.BoardSprints(boardID)
	return orEmpty(out), a.fail("board sprints", err, "board", boardID)
}

// GetBoard shows one sprint of a board; sprintID 0 = its open work (kanban).
func (a *App) GetBoard(boardID, sprintID int) (tracker.BoardView, error) {
	b, err := a.boards()
	if err != nil {
		return tracker.BoardView{}, err
	}
	v, err := b.BoardIssues(boardID, sprintID)
	v.Issues, v.Columns = orEmpty(v.Issues), orEmpty(v.Columns)
	for i := range v.Columns {
		v.Columns[i].Keys, v.Columns[i].Statuses = orEmpty(v.Columns[i].Keys), orEmpty(v.Columns[i].Statuses)
	}
	return v, a.fail("open board", err, "board", boardID, "sprint", sprintID)
}
