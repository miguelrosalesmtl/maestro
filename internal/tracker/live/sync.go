package live

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"maestro/internal/tracker/jira"
)

// scopeJQL is the set of issues Maestro keeps: open work in the projects
// in scope and anything assigned to you in any project (My work promises
// "wherever it lives"), plus anything finished in the last two weeks.
func (t *Tracker) scopeJQL() string {
	who := "assignee = currentUser()"
	if len(t.keys) > 0 {
		who = fmt.Sprintf("(project in (%s) OR %s)", quoteKeys(t.keys), who)
	}
	return who + " AND (statusCategory != Done OR updated >= -14d)"
}

// Refresh runs a full sync now (the Refresh button). It waits for a sync
// already running, and restarts polling if it had stopped (e.g. after the
// token was rejected and then renewed).
func (t *Tracker) Refresh(ctx context.Context) error {
	t.syncMu.Lock()
	defer t.syncMu.Unlock()
	t.setStatus(StateSyncing, "")
	if err := t.fullSync(ctx); err != nil {
		t.fail(err)
		return err
	}
	if t.done != nil {
		select {
		case <-t.done: // the loop had ended: start it again
			loopCtx, cancel := context.WithCancel(context.Background())
			t.cancel, t.done = cancel, make(chan struct{})
			go t.loop(loopCtx)
			t.log.Info("sync restarted by refresh")
		default:
		}
	}
	return nil
}

// fullSync replaces the local set with what Jira has now. It catches deleted
// and moved issues, which incremental syncs cannot see.
func (t *Tracker) fullSync(ctx context.Context) error {
	started := t.now()
	got := map[string]*jira.Issue{}
	for is, err := range t.c.Search(ctx, jira.SearchOptions{JQL: t.scopeJQL() + " ORDER BY updated DESC", Fields: t.fields()}) {
		if err != nil {
			return err
		}
		is := is
		got[is.Key] = &is
		if len(got) >= MaxIssues {
			t.log.Warn("sync capped", "max", MaxIssues, "hint", "narrow projects in config.yaml")
			break
		}
	}
	ancestors, err := t.fetchMissingParents(ctx, got)
	if err != nil {
		return err
	}

	t.mu.Lock()
	prev := t.issues
	next := make(map[string]*entry, len(got)+len(ancestors))
	for k, raw := range got {
		next[k] = &entry{issue: t.m.issue(raw), raw: *raw}
		if old, ok := prev[k]; ok {
			next[k].issue.Comments = old.issue.Comments
		}
	}
	for k, raw := range ancestors {
		next[k] = &entry{issue: t.m.issue(raw), raw: *raw, ancestor: true}
	}
	t.issues = next
	t.relink()
	change := diff(prev, next)
	t.lastSync, t.lastFull = started, started
	t.mu.Unlock()

	t.log.Info("full sync", "issues", len(got), "parents", len(ancestors), "changed", len(change.Issues), "removed", len(change.Removed), "took", time.Since(started).Round(time.Millisecond))
	t.setStatus(StateLive, "")
	t.emit(change)
	return nil
}

// incrementalSync fetches issues updated since the last sync.
// It uses relative JQL ("updated >= -Nm"), which Jira evaluates on its own
// clock and time zone, so the local clock and the user's Jira time zone do
// not matter. The extra 2 minutes cover JQL's minute precision.
func (t *Tracker) incrementalSync(ctx context.Context) error {
	started := t.now()
	t.mu.RLock()
	since := t.lastSync
	t.mu.RUnlock()
	mins := int(math.Ceil(started.Sub(since).Minutes())) + 2
	jql := fmt.Sprintf("(%s) AND updated >= -%dm ORDER BY updated ASC", t.scopeJQL(), mins)

	got := map[string]*jira.Issue{}
	for is, err := range t.c.Search(ctx, jira.SearchOptions{JQL: jql, Fields: t.fields()}) {
		if err != nil {
			return err
		}
		is := is
		got[is.Key] = &is
	}
	ancestors, err := t.fetchMissingParents(ctx, got)
	if err != nil {
		return err
	}

	t.mu.Lock()
	var change Change
	for k, raw := range ancestors {
		got[k] = raw
	}
	for k, raw := range got {
		old, had := t.issues[k]
		if had && old.raw.Fields.Updated == raw.Fields.Updated {
			continue // the overlap window returns unchanged issues again
		}
		e := &entry{issue: t.m.issue(raw), raw: *raw, ancestor: ancestors[k] != nil}
		if had {
			e.issue.Comments = old.issue.Comments
			if old.issue.Status != e.issue.Status {
				change.StatusChanges = append(change.StatusChanges, StatusChange{Key: k, From: old.issue.Status, To: e.issue.Status})
			}
		}
		t.issues[k] = e
		change.Issues = append(change.Issues, e.issue)
	}
	if len(change.Issues) > 0 {
		before := make(map[string][]string, len(t.issues))
		for k, e := range t.issues {
			before[k] = e.issue.ChildKeys
		}
		t.relink()
		changed := make(map[string]bool, len(change.Issues))
		for i := range change.Issues { // pick up rebuilt ChildKeys
			change.Issues[i] = t.issues[change.Issues[i].Key].issue
			changed[change.Issues[i].Key] = true
		}
		// A ticket created under (or moved to) an epic does not change the
		// epic's "updated" in Jira, so the epic is not in this sync; send it
		// (and the old parent) anyway, or the tree never shows the new child.
		for k, e := range t.issues {
			if !changed[k] && !slices.Equal(before[k], e.issue.ChildKeys) {
				change.Issues = append(change.Issues, e.issue)
			}
		}
	}
	t.lastSync = started
	t.mu.Unlock()

	if len(change.Issues) > 0 {
		t.log.Info("sync", "changed", len(change.Issues), "status_changes", len(change.StatusChanges))
	}
	t.setStatus(StateLive, "")
	t.emit(change)
	return nil
}

// fetchMissingParents loads parents (epics, initiatives) that are outside the
// scope, up to three levels, so the hierarchy is complete.
func (t *Tracker) fetchMissingParents(ctx context.Context, got map[string]*jira.Issue) (map[string]*jira.Issue, error) {
	out := map[string]*jira.Issue{}
	level := got
	for depth := 0; depth < 3; depth++ {
		var missing []string
		seen := map[string]bool{}
		for _, raw := range level {
			p := t.m.parentKey(raw)
			if p == "" || seen[p] || got[p] != nil || out[p] != nil {
				continue
			}
			t.mu.RLock()
			e, known := t.issues[p]
			t.mu.RUnlock()
			if known && !e.ancestor {
				continue // in scope and synced on its own
			}
			seen[p] = true
			missing = append(missing, p)
		}
		if len(missing) == 0 {
			break
		}
		parents, err := t.c.IssuesByKey(ctx, missing, t.fields())
		if err != nil {
			return nil, err
		}
		level = map[string]*jira.Issue{}
		for i := range parents {
			out[parents[i].Key] = &parents[i]
			level[parents[i].Key] = &parents[i]
		}
	}
	return out, nil
}

// loop polls until ctx is cancelled.
func (t *Tracker) loop(ctx context.Context) {
	defer close(t.done)
	interval := time.Duration(max(15, t.cfg.Jira.Sync.IntervalSeconds)) * time.Second
	burst := time.Duration(max(5, t.cfg.Jira.Sync.BurstSeconds)) * time.Second
	full := time.Duration(max(1, t.cfg.Jira.Sync.FullResyncMinutes)) * time.Minute
	failures := 0

	for {
		wait := interval
		t.mu.RLock()
		if t.now().Before(t.burstUntil) {
			wait = burst // faster right after a write
		}
		t.mu.RUnlock()
		if failures > 0 { // back off while offline: 1, 2, 4 … intervals, max 5 min
			wait = min(wait*time.Duration(1<<min(failures-1, 4)), 5*time.Minute)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		var err error
		t.syncMu.Lock() // never at the same time as a Refresh
		t.mu.RLock()
		dueFull := t.now().Sub(t.lastFull) >= full
		t.mu.RUnlock()
		if dueFull {
			err = t.fullSync(cctx)
		} else {
			err = t.incrementalSync(cctx)
		}
		t.syncMu.Unlock()
		cancel()

		switch {
		case err == nil:
			failures = 0
		case ctx.Err() != nil:
			return
		case errors.Is(err, jira.ErrUnauthorized):
			t.log.Error("sync stopped: token rejected", "err", err.Error())
			t.fail(err)
			return // do not retry with the same token
		default:
			failures++
			t.log.Warn("sync failed", "err", err.Error(), "failures", failures)
			t.fail(err)
		}
	}
}

// bumpBurst switches to fast polling for two minutes.
func (t *Tracker) bumpBurst() {
	t.mu.Lock()
	t.burstUntil = t.now().Add(2 * time.Minute)
	t.mu.Unlock()
}

func diff(prev, next map[string]*entry) Change {
	var c Change
	for k, e := range next {
		old, ok := prev[k]
		if !ok || old.raw.Fields.Updated != e.raw.Fields.Updated || len(old.issue.ChildKeys) != len(e.issue.ChildKeys) {
			c.Issues = append(c.Issues, e.issue)
		}
		if ok && old.issue.Status != e.issue.Status {
			c.StatusChanges = append(c.StatusChanges, StatusChange{Key: k, From: old.issue.Status, To: e.issue.Status})
		}
	}
	for k := range prev {
		if _, ok := next[k]; !ok {
			c.Removed = append(c.Removed, k)
		}
	}
	return c
}
