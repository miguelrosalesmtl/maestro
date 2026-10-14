package main

import (
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"maestro/internal/config"
	"maestro/internal/flow"
)

// Settings is what the Settings view edits: branch and pull request naming
// and terminal setup. Saving writes only what differs from the defaults and
// the team file into the personal config.yaml.
type Settings struct {
	Naming   flow.Naming      `json:"naming"`
	Terminal TerminalSettings `json:"terminal"`

	Path     string            `json:"path"`     // personal config file
	TeamPath string            `json:"teamPath"` // shared team file, "" if none
	Writable bool              `json:"writable"` // false when config.yaml could not be read
	Saved    bool              `json:"saved"`    // set by SaveSettings
	Problems []SettingsProblem `json:"problems"` // why a save was refused
}

type TerminalSettings struct {
	Shell    string                            `json:"shell"`
	Env      map[string]string                 `json:"env"`
	Startup  []string                          `json:"startup"`
	Commands []config.QuickCommand             `json:"commands"`
	Projects map[string]config.ProjectTerminal `json:"projects"`
}

type SettingsProblem struct {
	Path    string `json:"path"` // config key, e.g. dev.branch.template
	Message string `json:"message"`
}

// UseConfigLayers lets Settings save to the config files (main sets it at startup).
func (a *App) UseConfigLayers(l *config.Layers) {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	a.layers = l
}

func (a *App) GetSettings() Settings {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.settingsLocked()
}

func (a *App) settingsLocked() Settings {
	c := a.cfg
	t := c.Terminal
	projects := map[string]config.ProjectTerminal{}
	for k, p := range t.Projects {
		projects[k] = config.ProjectTerminal{Env: orEmptyMap(p.Env), Startup: orEmpty(p.Startup)}
	}
	s := Settings{
		Naming: namingFrom(c),
		Terminal: TerminalSettings{Shell: t.Shell, Env: orEmptyMap(maps.Clone(t.Env)), Startup: orEmpty(t.Startup),
			Commands: orEmpty(t.Commands), Projects: projects},
		Problems: []SettingsProblem{},
	}
	if a.layers != nil {
		s.Path, s.TeamPath, s.Writable = a.layers.PersonalPath, a.layers.TeamPath, true
	}
	return s
}

// SaveSettings validates and saves naming and terminal settings. When the
// result is invalid nothing is saved and Problems says why.
func (a *App) SaveSettings(in Settings) (Settings, error) {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	if a.layers == nil {
		return a.settingsLocked(), errors.New("config.yaml could not be read, so settings cannot be saved; see the log")
	}
	base, err := a.layers.Base()
	if err != nil {
		return a.settingsLocked(), err
	}

	n, t := in.Naming, in.Terminal
	commands := slices.DeleteFunc(slices.Clone(t.Commands), func(q config.QuickCommand) bool {
		return strings.TrimSpace(q.Label) == "" && strings.TrimSpace(q.Command) == ""
	})

	// Validate exactly what was entered (an emptied field is an error, not "use the default").
	trial := *a.cfg
	trial.Dev.Branch = config.Branch{Template: strings.TrimSpace(n.BranchTemplate), Prefix: trimValues(n.Prefix), SlugWords: n.SlugWords}
	trial.Dev.PR = config.PR{TitleTemplate: strings.TrimSpace(n.PRTitle), BodyTemplate: n.PRBody, MergeMethod: n.MergeMethod}
	trial.Terminal.Env, trial.Terminal.Commands, trial.Terminal.Projects = trimKeys(t.Env), commands, t.Projects
	var problems []SettingsProblem
	for _, pr := range config.Validate(&trial) {
		if pr.Severity == config.SeverityError && (strings.HasPrefix(pr.Path, "dev.branch") || strings.HasPrefix(pr.Path, "dev.pr") || strings.HasPrefix(pr.Path, "terminal.")) {
			problems = append(problems, SettingsProblem{Path: pr.Path, Message: pr.Message})
		}
	}
	if sh := strings.TrimSpace(t.Shell); sh != "" {
		if _, err := exec.LookPath(config.ExpandHome(sh)); err != nil {
			problems = append(problems, SettingsProblem{Path: "terminal.shell", Message: fmt.Sprintf("%q is not an installed program", sh)})
		}
	}
	if len(problems) > 0 {
		s := a.settingsLocked()
		s.Problems = problems
		return s, nil
	}

	old := a.layers.Personal
	p := *old
	p.Dev.Branch = config.Branch{
		Template:  unlessSame(strings.TrimSpace(n.BranchTemplate), base.Dev.Branch.Template),
		Prefix:    changedKeys(trimValues(n.Prefix), base.Dev.Branch.Prefix),
		SlugWords: unlessSame(n.SlugWords, base.Dev.Branch.SlugWords),
	}
	p.Dev.PR = config.PR{
		TitleTemplate: unlessSame(strings.TrimSpace(n.PRTitle), base.Dev.PR.TitleTemplate),
		BodyTemplate:  unlessSame(strings.TrimRight(n.PRBody, " \n"), base.Dev.PR.BodyTemplate),
		MergeMethod:   unlessSame(n.MergeMethod, base.Dev.PR.MergeMethod),
	}
	p.Terminal.Shell = unlessSame(strings.TrimSpace(t.Shell), base.Terminal.Shell)
	p.Terminal.Env = changedKeys(trimKeys(t.Env), base.Terminal.Env)
	p.Terminal.Startup = unlessSameList(lines(t.Startup), base.Terminal.Startup)
	p.Terminal.Commands = unlessSameList(commands, base.Terminal.Commands)
	p.Terminal.Projects = nil
	for k, pt := range t.Projects {
		bp := base.Terminal.Projects[k]
		d := config.ProjectTerminal{Env: changedKeys(trimKeys(pt.Env), bp.Env), Startup: unlessSameList(lines(pt.Startup), bp.Startup)}
		if len(d.Env) > 0 || len(d.Startup) > 0 {
			if p.Terminal.Projects == nil {
				p.Terminal.Projects = map[string]config.ProjectTerminal{}
			}
			p.Terminal.Projects[k] = d
		}
	}

	if err := a.layers.SetPersonal(&p); err != nil {
		_ = a.layers.SetPersonal(old)
		return a.settingsLocked(), err
	}
	if problems = a.problemsUnder("dev.branch", "dev.pr", "terminal."); len(problems) > 0 {
		_ = a.layers.SetPersonal(old)
		s := a.settingsLocked()
		s.Problems = problems
		return s, nil
	}
	if err := a.layers.SavePersonal(); err != nil {
		_ = a.layers.SetPersonal(old)
		return a.settingsLocked(), a.fail("save settings", err)
	}
	a.cfg = a.layers.Effective
	if fl := a.flowIfReady(); fl != nil {
		fl.SetNaming(namingFrom(a.cfg))
	}
	a.log.Info("settings saved", "path", a.layers.PersonalPath)
	s := a.settingsLocked()
	s.Saved = true
	return s, nil
}

// SaveRules replaces the automation rules. With live Jira they are saved to
// config.yaml; demo rules only change in memory.
func (a *App) SaveRules(rules []flow.Rule) ([]flow.Rule, error) {
	fl := a.fl()
	a.mu.RLock()
	lt := a.live
	a.mu.RUnlock()

	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	known := slices.Clone(a.cfg.Automation.Rules)
	var resolve func(string) string
	if lt != nil {
		meta := lt.Meta()
		resolve = func(to string) string {
			for _, s := range meta.Statuses {
				if meta.StatusStates[s] == to {
					return s
				}
			}
			return to
		}
	}
	var base *config.Config
	if a.layers != nil {
		if b, err := a.layers.Base(); err == nil {
			base = b
			known = append(known, b.Automation.Rules...)
		}
	}
	cfgRules := make([]config.Rule, 0, len(rules))
	for _, r := range rules {
		cr := config.Rule{Trigger: config.Trigger(r.Trigger), To: r.ToStatus, Projects: nonEmpty(r.Projects), IssueTypes: nonEmpty(r.IssueTypes)}
		if !r.Enabled {
			cr.Enabled = new(false)
		}
		// Keep a Maestro state ("inReview") when it still names the chosen status.
		for _, k := range known {
			if resolve != nil && k.Trigger == cr.Trigger && k.To != r.ToStatus && resolve(k.To) == r.ToStatus {
				cr.To = k.To
				break
			}
		}
		cfgRules = append(cfgRules, cr)
	}

	trial := *a.cfg
	trial.Automation.Rules = cfgRules
	if ps := rulesProblems(config.Validate(&trial)); ps != nil {
		return fl.Rules(), ps
	}
	if lt == nil || a.layers == nil {
		out := make([]flow.Rule, len(rules))
		for i, r := range rules {
			r.ID, r.Label = fmt.Sprintf("r%d", i+1), flow.TriggerLabel(r.Trigger)
			r.Projects, r.IssueTypes = orEmpty(r.Projects), orEmpty(r.IssueTypes)
			out[i] = r
		}
		return fl.SetRules(out), nil
	}

	old := a.layers.Personal
	p := *old
	p.Automation.Rules = cfgRules
	if base != nil && slices.EqualFunc(cfgRules, base.Automation.Rules, sameRule) {
		p.Automation.Rules = nil // back to the team's / default rules
	}
	if err := a.layers.SetPersonal(&p); err != nil {
		_ = a.layers.SetPersonal(old)
		return fl.Rules(), err
	}
	if err := a.layers.SavePersonal(); err != nil {
		_ = a.layers.SetPersonal(old)
		return fl.Rules(), a.fail("save rules", err)
	}
	a.cfg = a.layers.Effective
	next, _ := liveRules(a.cfg, lt.Meta())
	a.log.Info("automation rules saved", "rules", len(next))
	return fl.SetRules(orEmpty(next)), nil
}

// flowIfReady is the flow engine, or nil before startup has chosen one.
func (a *App) flowIfReady() *flow.Engine {
	select {
	case <-a.ready:
		return a.fl()
	default:
		return nil
	}
}

// problemsUnder lists validation errors for the given config keys (callers hold cfgMu).
func (a *App) problemsUnder(prefixes ...string) []SettingsProblem {
	var out []SettingsProblem
	for _, p := range a.layers.Problems {
		if p.Severity != config.SeverityError {
			continue
		}
		for _, pre := range prefixes {
			if strings.HasPrefix(p.Path, pre) {
				out = append(out, SettingsProblem{Path: p.Path, Message: p.Message})
				break
			}
		}
	}
	return out
}

var ruleIndex = regexp.MustCompile(`automation\.rules\[(\d+)\]`)

// rulesProblems turns rule validation errors into one readable error ("Rule 2: …").
func rulesProblems(ps config.Problems) error {
	var msgs []string
	for _, p := range ps {
		if p.Severity != config.SeverityError || !strings.HasPrefix(p.Path, "automation.") {
			continue
		}
		human := func(s string) string {
			return ruleIndex.ReplaceAllStringFunc(s, func(m string) string {
				i, _ := strconv.Atoi(ruleIndex.FindStringSubmatch(m)[1])
				return fmt.Sprintf("rule %d", i+1)
			})
		}
		msgs = append(msgs, strings.ToUpper(human(p.Path)[:1])+human(p.Path)[1:]+": "+human(p.Message))
	}
	if len(msgs) == 0 {
		return nil
	}
	return errors.New(strings.Join(msgs, "; "))
}

func sameRule(a, b config.Rule) bool {
	return a.Trigger == b.Trigger && a.To == b.To && a.IsEnabled() == b.IsEnabled() &&
		slices.Equal(a.Projects, b.Projects) && slices.Equal(a.IssueTypes, b.IssueTypes)
}

// unlessSame is v, or the zero value when it equals what the lower layers already give.
func unlessSame[T comparable](v, base T) T {
	var zero T
	if v == base {
		return zero
	}
	return v
}

func unlessSameList[T comparable](v, base []T) []T {
	if slices.Equal(v, base) {
		return nil
	}
	return v
}

// changedKeys keeps the entries that differ from base (maps merge per key).
func changedKeys(v, base map[string]string) map[string]string {
	var out map[string]string
	for k, x := range v {
		if b, ok := base[k]; ok && b == x || !ok && x == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[k] = x
	}
	return out
}

func trimValues(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = strings.TrimSpace(v)
	}
	return out
}

func trimKeys(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		if k = strings.TrimSpace(k); k != "" {
			out[k] = v
		}
	}
	return out
}

func lines(l []string) []string {
	var out []string
	for _, s := range l {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func nonEmpty(l []string) []string {
	if len(l) == 0 {
		return nil
	}
	return l
}
