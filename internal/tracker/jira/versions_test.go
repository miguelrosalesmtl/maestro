package jira

import (
	"strings"
	"testing"
	"time"
)

func TestRecentAndNextVersions(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	all := []Version{
		{Name: "1.0", Released: true, ReleaseDate: "2026-01-10"}, // too old
		{Name: "2.3", Released: true, ReleaseDate: "2026-09-01"},
		{Name: "2.2", Released: true, ReleaseDate: "2026-08-01"},
		{Name: "old", Archived: true},
		{Name: "3.0"},                            // unreleased, no date
		{Name: "2.5", ReleaseDate: "2026-12-01"}, // unreleased
		{Name: "2.4", ReleaseDate: "2026-11-01"}, // unreleased, soonest
	}
	got := RecentVersions(all, now)
	var names []string
	for _, v := range got {
		names = append(names, v.Name)
	}
	if strings.Join(names, ",") != "2.4,2.5,3.0,2.3,2.2" {
		t.Fatalf("order = %v", names)
	}
	if n := NextVersion(got); n == nil || n.Name != "2.4" {
		t.Fatalf("next = %+v", n)
	}
	if NextVersion([]Version{{Name: "x", Released: true}}) != nil {
		t.Fatal("no unreleased version means no next")
	}
}
