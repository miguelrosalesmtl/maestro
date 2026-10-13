package github

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type User struct {
	Login string `json:"login"`
	Name  string `json:"name,omitempty"`
}

type Ref struct {
	Ref string `json:"ref"` // branch name
	SHA string `json:"sha"`
}

type PullRequest struct {
	Number             int    `json:"number"`
	Title              string `json:"title"`
	Body               string `json:"body"`
	HTMLURL            string `json:"html_url"`
	State              string `json:"state"` // open | closed
	Draft              bool   `json:"draft"`
	Merged             bool   `json:"merged"` // only on single-PR reads
	MergedAt           string `json:"merged_at"`
	UpdatedAt          string `json:"updated_at"`
	Head               Ref    `json:"head"`
	Base               Ref    `json:"base"`
	User               User   `json:"user"`
	RequestedReviewers []User `json:"requested_reviewers"`
	Commits            int    `json:"commits"` // only on single-PR reads
}

// IsMerged works for list and single reads (lists carry merged_at only).
func (p PullRequest) IsMerged() bool { return p.Merged || p.MergedAt != "" }

type Review struct {
	ID          int64  `json:"id"`
	User        User   `json:"user"`
	State       string `json:"state"` // APPROVED | CHANGES_REQUESTED | COMMENTED | DISMISSED | PENDING
	SubmittedAt string `json:"submitted_at"`
}

type CheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`     // queued | in_progress | completed
	Conclusion string `json:"conclusion"` // success | failure | neutral | cancelled | skipped | timed_out | action_required
	HTMLURL    string `json:"html_url"`
}

type CommitStatus struct {
	Context string `json:"context"`
	State   string `json:"state"` // pending | success | failure | error
}

type Commit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

// Viewer is the signed-in user (checks the token works).
func (c *Client) Viewer(ctx context.Context) (*User, error) {
	var u User
	return &u, c.get(ctx, "/user", nil, &u)
}

func repoPath(repo string) (string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("github: repository must be owner/name, got %q", repo)
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name), nil
}

// OpenPullRequests lists open PRs, most recently updated first (≤ 300).
func (c *Client) OpenPullRequests(ctx context.Context, repo string) ([]PullRequest, error) {
	return c.pulls(ctx, repo, "open", 3)
}

// RecentlyClosedPullRequests is the latest page of closed PRs (merged or not).
func (c *Client) RecentlyClosedPullRequests(ctx context.Context, repo string) ([]PullRequest, error) {
	return c.pulls(ctx, repo, "closed", 1)
}

func (c *Client) pulls(ctx context.Context, repo, state string, maxPages int) ([]PullRequest, error) {
	p, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var all []PullRequest
	for page := 1; page <= maxPages; page++ {
		var batch []PullRequest
		q := url.Values{"state": {state}, "sort": {"updated"}, "direction": {"desc"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
		if err := c.get(ctx, p+"/pulls", q, &batch); err != nil {
			return all, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
	}
	return all, nil
}

// PullRequest reads one PR (has Merged and Commits).
func (c *Client) PullRequest(ctx context.Context, repo string, number int) (*PullRequest, error) {
	p, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var pr PullRequest
	return &pr, c.get(ctx, p+"/pulls/"+strconv.Itoa(number), nil, &pr)
}

func (c *Client) Reviews(ctx context.Context, repo string, number int) ([]Review, error) {
	p, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var out []Review
	return out, c.get(ctx, p+"/pulls/"+strconv.Itoa(number)+"/reviews", url.Values{"per_page": {"100"}}, &out)
}

func (c *Client) PullRequestCommits(ctx context.Context, repo string, number int) ([]Commit, error) {
	p, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var out []Commit
	return out, c.get(ctx, p+"/pulls/"+strconv.Itoa(number)+"/commits", url.Values{"per_page": {"100"}}, &out)
}

// CheckRuns are GitHub Actions and other apps' checks on a commit.
func (c *Client) CheckRuns(ctx context.Context, repo, sha string) ([]CheckRun, error) {
	p, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var out struct {
		CheckRuns []CheckRun `json:"check_runs"`
	}
	return out.CheckRuns, c.get(ctx, p+"/commits/"+url.PathEscape(sha)+"/check-runs", url.Values{"per_page": {"100"}}, &out)
}

// Statuses are commit statuses (older CI systems), latest per context.
func (c *Client) Statuses(ctx context.Context, repo, sha string) ([]CommitStatus, error) {
	p, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var out struct {
		Statuses []CommitStatus `json:"statuses"`
	}
	return out.Statuses, c.get(ctx, p+"/commits/"+url.PathEscape(sha)+"/status", nil, &out)
}

// LatestReviews keeps each reviewer's latest decisive review (APPROVED or
// CHANGES_REQUESTED; a later DISMISSED clears it). COMMENTED does not change
// a decision, as on GitHub.
func LatestReviews(rs []Review) map[string]string {
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].SubmittedAt < rs[j].SubmittedAt })
	out := map[string]string{}
	for _, r := range rs {
		switch r.State {
		case "APPROVED", "CHANGES_REQUESTED":
			out[r.User.Login] = r.State
		case "DISMISSED":
			delete(out, r.User.Login)
		}
	}
	return out
}
