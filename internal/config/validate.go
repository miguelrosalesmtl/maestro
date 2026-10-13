package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"maestro/internal/secrets"
)

// Validate checks a merged configuration without contacting Jira.
//
// Checks that need Jira (configuration spec rules 18.2–18.6: project keys,
// statuses, transitions and custom fields exist) run after discovery, in the
// health check.
func Validate(cfg *Config) Problems {
	c := &collector{}
	if cfg.Version != CurrentVersion {
		c.errorf("version", "unsupported version %d; this Maestro reads version %d", cfg.Version, CurrentVersion)
	}
	validateJira(c, &cfg.Jira)
	validateProjects(c, cfg)
	validateHierarchy(c, &cfg.Hierarchy)
	validateWorkflow(c, &cfg.Workflow)
	validateDev(c, &cfg.Dev)
	validateAutomation(c, cfg)
	validateFieldsSprintsVersions(c, cfg)
	validateTerminalAgentUI(c, cfg)
	return c.ps
}

func validateJira(c *collector, j *Jira) {
	oneOf(c, "jira.transport", string(j.Transport), "rest", "mcp", "mock")
	oneOf(c, "jira.deployment", string(j.Deployment), "", "cloud", "datacenter")
	if j.Transport == TransportMock {
		return
	}

	switch u, err := url.Parse(j.Site); {
	case j.Site == "":
		c.errorf("jira.site", "required: the base URL of your Jira site, e.g. https://your-org.atlassian.net")
	case err != nil || u.Host == "":
		c.errorf("jira.site", "not a valid URL: %q", j.Site)
	case u.Scheme == "http" && !(j.TLS.AllowInsecure && j.Deployment == DataCenter):
		c.errorf("jira.site", "must use https (http is accepted only for Data Center with jira.tls.allowInsecure: true)")
	case u.Scheme != "https" && u.Scheme != "http":
		c.errorf("jira.site", "must use https")
	case strings.Contains(u.Path, "/browse") || strings.Contains(u.Path, "/jira/"):
		c.errorf("jira.site", "use the site root (e.g. https://%s), not a page URL", u.Host)
	case u.RawQuery != "" || u.Fragment != "":
		c.errorf("jira.site", "must not contain a query or fragment")
	}

	a := j.Auth
	if j.Transport == TransportREST {
		oneOf(c, "jira.auth.method", string(a.Method), "apiToken", "scopedApiToken", "oauth", "pat")
	}
	switch a.Method {
	case AuthOAuth:
		c.errorf("jira.auth.method", "oauth is planned for version 2; use apiToken")
	case AuthPAT:
		if j.Deployment == Cloud {
			c.errorf("jira.auth.method", "personal access tokens are for Data Center; Jira Cloud uses apiToken")
		}
	case AuthAPIToken, AuthScopedAPIToken:
		if j.Deployment == DataCenter {
			c.errorf("jira.auth.method", "API tokens are for Jira Cloud; Data Center uses pat")
		}
		if a.Email == "" {
			c.errorf("jira.auth.email", "required for API token authentication")
		} else if !strings.Contains(a.Email, "@") {
			c.errorf("jira.auth.email", "not an email address: %q", a.Email)
		}
	}
	if j.Transport == TransportREST && a.Method != AuthOAuth {
		if a.CredentialRef == "" {
			c.errorf("jira.auth.credentialRef", "required, e.g. keychain:maestro/jira:<host>:<email>")
		} else if _, err := secrets.ParseRef(a.CredentialRef); err != nil {
			c.errorf("jira.auth.credentialRef", "%v", err)
		}
	}
	if a.TokenExpires != "" {
		if d, err := time.Parse(time.DateOnly, a.TokenExpires); err != nil {
			c.errorf("jira.auth.tokenExpires", "use the format YYYY-MM-DD")
		} else if time.Until(d) < 14*24*time.Hour {
			c.warnf("jira.auth.tokenExpires", "the Jira token expires on %s", a.TokenExpires)
		}
	}
	if j.Transport == TransportMCP && len(j.MCP.Command) == 0 && j.MCP.URL == "" {
		c.errorf("jira.mcp", "transport mcp needs jira.mcp.command or jira.mcp.url")
	}

	between(c, "jira.http.timeoutSeconds", j.HTTP.TimeoutSeconds, 1, 300)
	between(c, "jira.http.maxConcurrency", j.HTTP.MaxConcurrency, 1, 16)
	if j.Proxy != "" {
		if u, err := url.Parse(j.Proxy); err != nil || u.Host == "" {
			c.errorf("jira.proxy", "not a valid proxy URL")
		}
	}
	if j.TLS.CAFile != "" {
		if _, err := os.Stat(ExpandHome(j.TLS.CAFile)); err != nil {
			c.errorf("jira.tls.caFile", "cannot read CA file: %v", err)
		}
	}
	between(c, "jira.sync.intervalSeconds", j.Sync.IntervalSeconds, 15, 3600)
	between(c, "jira.sync.burstSeconds", j.Sync.BurstSeconds, 5, 300)
	between(c, "jira.sync.fullResyncMinutes", j.Sync.FullResyncMinutes, 1, 1440)
	between(c, "jira.sync.discoveryTtlHours", j.Sync.DiscoveryTTLHours, 1, 24*30)
	if j.Sync.Webhooks {
		c.warnf("jira.sync.webhooks", "webhooks are not supported in version 1; Maestro polls instead")
	}
}

var projectKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,9}$`)
var repoName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func validateProjects(c *collector, cfg *Config) {
	seen := map[string]bool{}
	for i, p := range cfg.Projects {
		path := fmt.Sprintf("projects[%d]", i)
		if !projectKey.MatchString(p.Key) {
			c.errorf(path+".key", "%q is not a Jira project key (e.g. PLAT)", p.Key)
		}
		if seen[p.Key] {
			c.errorf(path+".key", "project %s is listed twice", p.Key)
		}
		seen[p.Key] = true
		if p.Board < 0 {
			c.errorf(path+".board", "board ID must be positive")
		}
		for j, r := range p.Repos {
			rp := fmt.Sprintf("%s.repos[%d]", path, j)
			if !repoName.MatchString(r.Name) {
				c.errorf(rp+".name", "use owner/name, e.g. acme/platform")
			}
			oneOf(c, rp+".host", r.Host, "", "github")
			if r.Path != "" {
				if st, err := os.Stat(ExpandHome(r.Path)); err != nil || !st.IsDir() {
					c.warnf(rp+".path", "directory %s does not exist on this computer", r.Path) // rule 18.7
				}
			}
		}
	}
	for _, k := range []struct{ path, key string }{
		{"defaults.epicProject", cfg.Defaults.EpicProject},
		{"hierarchy.initiativeProject", cfg.Hierarchy.InitiativeProject},
	} {
		if k.key != "" && !projectKey.MatchString(k.key) {
			c.errorf(k.path, "%q is not a Jira project key", k.key)
		}
	}
	if cfg.Defaults.EpicProject != "" && len(cfg.Projects) > 0 && !seen[cfg.Defaults.EpicProject] {
		c.errorf("defaults.epicProject", "%s is not in projects", cfg.Defaults.EpicProject)
	}
}

func validateHierarchy(c *collector, h *Hierarchy) {
	for i, t := range h.Types {
		path := fmt.Sprintf("hierarchy.types[%d]", i)
		if t.ID == "" && t.Name == "" {
			c.errorf(path, "needs id or name")
		}
		if !slices.Contains(Roles, t.Role) {
			c.errorf(path+".role", "unknown role %q; use one of %v", t.Role, Roles)
		}
	}
	for parent, kids := range h.Children {
		allowed, ok := DefaultChildren[parent]
		if !ok {
			c.errorf("hierarchy.children", "unknown role %q", parent)
			continue
		}
		for _, k := range kids {
			if !slices.Contains(allowed, k) {
				c.errorf("hierarchy.children."+string(parent), "%s cannot be a child of %s (the file can only narrow the hierarchy)", k, parent)
			}
		}
	}
}

func validateWorkflow(c *collector, w *Workflow) {
	for status, st := range w.States {
		if !slices.Contains(States, st) {
			c.errorf("workflow.states."+status, "unknown Maestro state %q; use one of %v", st, States)
		}
	}
	seen := map[string]bool{}
	for i, s := range w.Order {
		if seen[s] {
			c.errorf(fmt.Sprintf("workflow.order[%d]", i), "status %q is listed twice", s)
		}
		seen[s] = true
	}
}

func validateDev(c *collector, d *Dev) {
	if !strings.Contains(d.Branch.Template, "{key}") {
		c.errorf("dev.branch.template", "must contain {key} so Maestro can find the ticket")
	}
	for k := range d.Branch.Prefix {
		if k != "default" && !slices.Contains(Roles, Role(k)) {
			c.errorf("dev.branch.prefix."+k, "unknown role; use a role name or \"default\"")
		}
	}
	between(c, "dev.branch.slugWords", d.Branch.SlugWords, 1, 12)
	oneOf(c, "dev.pr.mergeMethod", d.PR.MergeMethod, "merge", "squash", "rebase")
	if _, err := regexp.Compile(d.KeyPattern); err != nil {
		c.errorf("dev.keyPattern", "not a valid regular expression: %v", err)
	}
	between(c, "dev.review.requiredApprovals", d.Review.RequiredApprovals, 0, 10)
	oneOf(c, "dev.github.auth", d.GitHub.Auth, "gh", "token")
	between(c, "dev.github.pollSeconds", d.GitHub.PollSeconds, 15, 3600)
	if u, err := url.Parse(d.GitHub.APIURL); err != nil || u.Scheme != "https" {
		c.errorf("dev.github.apiUrl", "must be an https URL")
	}
}

func validateAutomation(c *collector, cfg *Config) {
	type key struct {
		trigger Trigger
		scope   string
	}
	seen := map[key]int{}
	for i, r := range cfg.Automation.Rules {
		path := fmt.Sprintf("automation.rules[%d]", i)
		if !slices.Contains(Triggers, r.Trigger) {
			c.errorf(path+".trigger", "unknown trigger %q; use one of %v", r.Trigger, Triggers)
		}
		if strings.TrimSpace(r.To) == "" {
			c.errorf(path+".to", "required: a Maestro state (%v) or a Jira status", States)
		}
		for _, p := range r.Projects {
			if !projectKey.MatchString(p) {
				c.errorf(path+".projects", "%q is not a project key", p)
			}
		}
		if !r.IsEnabled() {
			continue
		}
		k := key{r.Trigger, strings.Join(r.Projects, ",") + "|" + strings.Join(r.IssueTypes, ",")}
		if j, dup := seen[k]; dup {
			c.errorf(path, "same trigger and scope as automation.rules[%d]; one event must move an issue to one status", j)
		}
		seen[k] = i
	}
}

func validateTerminalAgentUI(c *collector, cfg *Config) {
	for i, q := range cfg.Terminal.Commands {
		if q.Label == "" || q.Command == "" {
			c.errorf(fmt.Sprintf("terminal.commands[%d]", i), "needs label and command")
		}
	}
	between(c, "terminal.notify.minSeconds", cfg.Terminal.Notify.MinSeconds, 0, 3600)
	between(c, "terminal.notify.waitSeconds", cfg.Terminal.Notify.WaitSeconds, 2, 600)
	oneOf(c, "agent.mcp.server", cfg.Agent.MCP.Server, "atlassian-remote", "mcp-atlassian", "none")
	oneOf(c, "log.level", cfg.Log.Level, "debug", "info", "warn", "error")
	oneOf(c, "ui.defaultView", cfg.UI.DefaultView, "mine", "focus", "board", "roadmap", "automations")
	if tz := cfg.UI.TimeZone; tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			c.errorf("ui.timeZone", "unknown time zone %q", tz)
		}
	}
}

func oneOf(c *collector, path, v string, allowed ...string) {
	if !slices.Contains(allowed, v) {
		shown := slices.DeleteFunc(slices.Clone(allowed), func(s string) bool { return s == "" })
		c.errorf(path, "%q is not valid; use one of %s", v, strings.Join(shown, ", "))
	}
}

func between(c *collector, path string, v, lo, hi int) {
	if v < lo || v > hi {
		c.errorf(path, "%d is out of range (%d–%d)", v, lo, hi)
	}
}

// validateFieldsSprintsVersions checks custom fields, sprints and versions
// offline; the checks that need Jira run in setup.CheckAgainst.
func validateFieldsSprintsVersions(c *collector, cfg *Config) {
	seenRef, seenRole := map[string]int{}, map[string]int{}
	for i, f := range cfg.Fields.Custom {
		path := fmt.Sprintf("fields.custom[%d]", i)
		if f.ID == "" && f.Name == "" {
			c.errorf(path, "needs id or name")
			continue
		}
		if f.ID != "" && !strings.HasPrefix(f.ID, "customfield_") {
			c.errorf(path+".id", "%q is not a custom field ID (customfield_NNNNN); use name for system fields", f.ID)
		}
		if j, dup := seenRef[f.Ref()]; dup {
			c.errorf(path, "same field as fields.custom[%d]", j)
		}
		seenRef[f.Ref()] = i
		for _, s := range f.Show {
			if !slices.Contains(CustomPlaces, s) {
				c.errorf(path+".show", "%q is not valid; use one of %s", s, strings.Join(CustomPlaces, ", "))
			}
		}
		if f.Role != "" {
			if !slices.Contains(CustomRoles, f.Role) {
				c.errorf(path+".role", "%q is not valid; use one of %s", f.Role, strings.Join(CustomRoles, ", "))
			} else if j, dup := seenRole[f.Role]; dup {
				c.errorf(path+".role", "role %s is already given to fields.custom[%d]", f.Role, j)
			}
			seenRole[f.Role] = i
		}
		for _, r := range f.RequiredOn {
			if !slices.Contains(RequiredOns, r) {
				c.errorf(path+".requiredOn", "%q is not valid; use one of %s", r, strings.Join(RequiredOns, ", "))
			}
		}
		for _, r := range f.DefaultFor {
			if !slices.Contains(Roles, Role(r)) {
				c.errorf(path+".defaultFor", "%q is not an issue role", r)
			}
		}
		if len(f.DefaultFor) > 0 && f.Default == "" {
			c.warnf(path+".defaultFor", "has no effect without default")
		}
	}

	oneOf(c, "defaults.create.sprint", cfg.Defaults.Create.Sprint, "active", "next", "backlog")
	for _, k := range []struct{ path, v string }{
		{"defaults.create.fixVersion", cfg.Defaults.Create.FixVersion},
		{"defaults.create.affectsVersion", cfg.Defaults.Create.AffectsVersion},
	} {
		if strings.TrimSpace(k.v) == "" {
			c.errorf(k.path, "use none, next or a version name")
		}
	}

	between(c, "sprints.closedHistory", cfg.Sprints.ClosedHistory, 0, 20)
	between(c, "sprints.velocitySprints", cfg.Sprints.VelocitySprints, 1, 20)
	if t := cfg.Sprints.NameTemplate; !strings.Contains(t, "{n}") {
		c.errorf("sprints.nameTemplate", "must contain {n} (the sprint number)")
	}
	if cfg.Sprints.AllowManage {
		c.warnf("sprints.allowManage", "managing sprints is not in this version yet; Maestro reads sprints only")
	}

	if _, err := regexp.Compile(cfg.Versions.Pattern); err != nil {
		c.errorf("versions.pattern", "not a valid regular expression: %v", err)
	}
	if fb := cfg.Versions.FromBranch; fb.Enabled && !strings.Contains(fb.Template, "{version}") {
		c.errorf("versions.fromBranch.template", "must contain {version}")
	}
	if cfg.Versions.AllowManage {
		c.warnf("versions.allowManage", "managing versions is not in this version yet; Maestro reads versions only")
	}
	if !strings.Contains(cfg.Versions.ReleaseNotesTemplate, "{key}") {
		c.errorf("versions.releaseNotesTemplate", "must contain {key}")
	}
}
