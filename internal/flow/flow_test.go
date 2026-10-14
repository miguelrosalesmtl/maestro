package flow

import (
	"testing"

	"maestro/internal/tracker"
)

func TestTerminalCommandsDriveTicket(t *testing.T) {
	tr := tracker.NewMock()
	e := NewEngine(tr)
	const key = "PLAT-143" // To Do, no branch yet

	if _, ok := e.OnCommand(key, "ls -la"); ok {
		t.Fatal("non-dev command should be ignored")
	}

	steps := []struct{ cmd, status string }{
		{"git checkout -b feature/PLAT-143-rollback", tracker.StatusInProgress},
		{"git push -u origin feature/PLAT-143-rollback", tracker.StatusInProgress},
		{`gh pr create --title "[PLAT-143] Automatic rollback"`, tracker.StatusInReview},
		{"gh pr merge --squash", tracker.StatusInReview}, // "PR merged" ships off: QA comes first
	}
	for _, s := range steps {
		res, ok := e.OnCommand(key, s.cmd)
		if !ok {
			t.Fatalf("%q: not handled", s.cmd)
		}
		if res.Issue.Status != s.status {
			t.Fatalf("%q: status = %q, want %q", s.cmd, res.Issue.Status, s.status)
		}
	}
	if f := e.Flow(key); f.Branch != "feature/PLAT-143-rollback" || f.PR == nil || f.PR.State != "merged" {
		t.Fatalf("unexpected flow: branch=%q pr=%+v", f.Branch, f.PR)
	}
}
