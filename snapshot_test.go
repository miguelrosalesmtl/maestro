package main

import (
	"encoding/json"
	"testing"

	"maestro/internal/flow"
	"maestro/internal/tracker"
)

// emptySite is a live site that returned nothing: no issues, users or
// metadata (e.g. a 403 on statuses for a non-admin user).
type emptySite struct{ *tracker.Mock }

func (emptySite) Meta() tracker.Meta          { return tracker.Meta{Live: true} }
func (emptySite) ListIssues() []tracker.Issue { return nil }
func (emptySite) Users() []tracker.User       { return nil }

// Regression (issue #1): every list and map in the snapshot is [] or {},
// never null, because the UI indexes them on its first render.
func TestSnapshotHasNoNulls(t *testing.T) {
	tr := emptySite{tracker.NewMock()}
	a := &App{ready: make(chan struct{}), tracker: tr, flow: flow.NewEngineWith(tr, flow.Options{})}
	close(a.ready)

	b, err := json.Marshal(a.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	for name, raw := range fields {
		if string(raw) == "null" {
			t.Errorf("snapshot.%s is null, want an empty list or map", name)
		}
	}
	if string(fields["activity"]) != "[]" {
		t.Errorf("activity = %s, want []", fields["activity"])
	}
}
