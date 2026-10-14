package flow

import (
	"regexp"
	"strings"

	"maestro/internal/tracker"
)

// Naming turns a ticket into a branch name, pull request title and body
// (config dev.branch and dev.pr). The UI renders the same templates with
// frontend/src/lib/naming.ts; testdata/naming.json holds the cases both pass.
//
// Placeholders: {key} {project} {type} {summary} {slug} {prefix} {url} {branch}.
// A secured issue's summary never goes into a branch, title or body.
type Naming struct {
	BranchTemplate string            `json:"branchTemplate"`
	Prefix         map[string]string `json:"prefix"` // role → prefix; "default" for the rest (also for "")
	SlugWords      int               `json:"slugWords"`
	PRTitle        string            `json:"prTitle"`
	PRBody         string            `json:"prBody"`
	MergeMethod    string            `json:"mergeMethod"` // merge | squash | rebase
}

// DefaultNaming matches config.Default().
func DefaultNaming() Naming {
	return Naming{
		BranchTemplate: "{prefix}/{key}-{slug}",
		Prefix:         map[string]string{"bug": "fix", "default": "feature"},
		SlugWords:      5,
		PRTitle:        "[{key}] {summary}",
		PRBody:         "Jira: {url}",
		MergeMethod:    "squash",
	}
}

var (
	nonSlug     = regexp.MustCompile(`[^a-z0-9]+`)
	placeholder = regexp.MustCompile(`\{[a-z]+\}`)
	// What git refuses in a branch name, plus whitespace.
	badRef    = regexp.MustCompile(`[\s~^:?*\[\\]+|\.\.+|@\{`)
	dupSep    = regexp.MustCompile(`([-_/.])[-_/.]+`)
	edgeSep   = regexp.MustCompile(`^[-_/.]+|[-_/.]+$`)
	blankLine = regexp.MustCompile(`[ \t]+(\n|$)`)
)

// Project is the project key of an issue key ("PLAT-142" → "PLAT").
func Project(key string) string {
	if i := strings.LastIndex(key, "-"); i > 0 {
		return key[:i]
	}
	return key
}

func (n Naming) vars(is tracker.Issue) map[string]string {
	summary := is.Summary
	if is.Security != "" {
		summary = ""
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(summary), "-"), "-")
	if words := max(n.SlugWords, 1); slug != "" {
		if parts := strings.Split(slug, "-"); len(parts) > words {
			slug = strings.Join(parts[:words], "-")
		}
	}
	prefix := n.Prefix[string(is.Type)]
	if prefix == "" { // not set for this type
		prefix = n.Prefix["default"]
	}
	return map[string]string{
		"key": is.Key, "project": Project(is.Key), "type": string(is.Type), "summary": summary,
		"slug": slug, "prefix": prefix, "url": is.URL,
	}
}

func render(tmpl string, vars map[string]string) string {
	return placeholder.ReplaceAllStringFunc(tmpl, func(p string) string {
		if v, ok := vars[p[1:len(p)-1]]; ok {
			return v
		}
		return p
	})
}

// Branch is the git branch for a ticket, e.g. feature/PLAT-142-health-gated-traffic.
// Empty parts (no slug for a secured issue) leave no stray separators.
func (n Naming) Branch(is tracker.Issue) string {
	tmpl := n.BranchTemplate
	if tmpl == "" {
		tmpl = DefaultNaming().BranchTemplate
	}
	b := badRef.ReplaceAllString(render(tmpl, n.vars(is)), "-")
	b = dupSep.ReplaceAllString(b, "$1")
	b = edgeSep.ReplaceAllString(b, "")
	return strings.TrimSuffix(b, ".lock")
}

// Title is the pull request title for a ticket.
func (n Naming) Title(is tracker.Issue, branch string) string {
	v := n.vars(is)
	v["branch"] = branch
	return strings.TrimRight(strings.Join(strings.Fields(render(n.PRTitle, v)), " "), " :-|")
}

// Body is the pull request description for a ticket.
func (n Naming) Body(is tracker.Issue, branch string) string {
	v := n.vars(is)
	v["branch"] = branch
	return strings.TrimSpace(blankLine.ReplaceAllString(render(n.PRBody, v), "$1"))
}
