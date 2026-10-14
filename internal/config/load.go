package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// File names inside Dir().
const (
	PersonalFileName = "config.yaml"
	TeamFileName     = "maestro.team.yaml" // conventional name when shared in a repository
)

// Layers keeps each file as written plus the merged result.
//
// Saving writes only Personal, so defaults and team values are never copied
// into the personal file (configuration spec rule 4.2).
type Layers struct {
	PersonalPath string
	TeamPath     string // resolved teamFile, empty if none

	Personal *Config // as written; zero fields mean "not set"
	Team     *Config // as written; nil if no team file

	Effective *Config  // Default() ← Team ← Personal
	Problems  Problems // warnings (and errors, if Load failed)

	fileProblems Problems // from parsing the files; kept across Remerge

	// Raw documents, merged as written so an explicit value (even 0 or false)
	// overrides the default instead of disappearing in a struct round trip.
	personalRaw, teamRaw []byte
}

// PersonalPath is <config dir>/maestro/config.yaml.
func PersonalPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, PersonalFileName), nil
}

// Load reads the personal file at path (missing = empty), the team file it
// points to, merges them over the defaults and validates the result.
//
// On validation errors it returns the Layers (for display) and an *InvalidError.
func Load(path string) (*Layers, error) {
	l := &Layers{PersonalPath: path}

	personalRaw, err := readOptional(path)
	if err != nil {
		return nil, err
	}
	var ps Problems
	l.personalRaw = personalRaw
	l.Personal, ps = parse(path, personalRaw)
	l.fileProblems = append(l.fileProblems, ps...)

	if l.Personal.TeamFile != "" {
		l.TeamPath = resolvePath(l.Personal.TeamFile, filepath.Dir(path))
		teamRaw, err := os.ReadFile(l.TeamPath)
		if err != nil {
			l.fileProblems = append(l.fileProblems, Problem{Severity: SeverityError, File: path, Path: "teamFile", Message: fmt.Sprintf("cannot read team file: %v", err)})
		} else {
			l.teamRaw = teamRaw
			l.Team, ps = parse(l.TeamPath, teamRaw)
			l.fileProblems = append(l.fileProblems, ps...)
			l.fileProblems = append(l.fileProblems, checkTeamOnly(l.TeamPath, l.Team)...)
		}
	}

	if err := l.Remerge(); err != nil {
		return l, err
	}
	if l.Problems.HasErrors() {
		return l, &InvalidError{Problems: l.Problems}
	}
	return l, nil
}

// SetPersonal replaces the personal layer (e.g. from the settings UI) and remerges.
func (l *Layers) SetPersonal(p *Config) error {
	raw, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	l.Personal, l.personalRaw = p, raw
	return l.Remerge()
}

// Base is the configuration without the personal file (defaults ← team), to
// tell which settings the personal file actually changes.
func (l *Layers) Base() (*Config, error) { return mergeRaw(l.teamRaw) }

// Remerge recomputes Effective from the layers and re-validates it.
func (l *Layers) Remerge() error {
	eff, err := mergeRaw(l.teamRaw, l.personalRaw)
	if err != nil {
		return err
	}
	l.Effective = eff
	l.Problems = append(append(Problems{}, l.fileProblems...), Validate(eff)...)
	return nil
}

// mergeRaw decodes each document over Default(). Later documents win; maps
// merge key by key; lists are replaced as a whole. Empty documents are skipped.
// Documents are already checked strictly by parse.
func mergeRaw(docs ...[]byte) (*Config, error) {
	out := Default()
	for _, d := range docs {
		if len(bytes.TrimSpace(d)) == 0 {
			continue
		}
		if err := yaml.Unmarshal(d, out); err != nil {
			return nil, fmt.Errorf("merge: %w", err)
		}
	}
	return out, nil
}

// SavePersonal writes the personal layer atomically with mode 0600.
func (l *Layers) SavePersonal() error {
	return writeYAML(l.PersonalPath, l.Personal)
}

// Import validates a file as the new personal configuration and, only if it
// has no errors, replaces the personal file (rules 18.9 and 18.10: report every
// problem, apply all or nothing). It returns every problem found.
func Import(src, personalPath string) (Problems, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	incoming, problems := parse(src, raw)
	if problems.HasErrors() {
		return problems, &InvalidError{Problems: problems}
	}

	trial := &Layers{PersonalPath: personalPath, Personal: incoming, personalRaw: raw}
	if incoming.TeamFile != "" {
		trial.TeamPath = resolvePath(incoming.TeamFile, filepath.Dir(personalPath))
		teamRaw, err := os.ReadFile(trial.TeamPath)
		if err != nil {
			problems = append(problems, Problem{Severity: SeverityError, File: src, Path: "teamFile", Message: fmt.Sprintf("cannot read team file: %v", err)})
		} else {
			var tp Problems
			trial.teamRaw = teamRaw
			trial.Team, tp = parse(trial.TeamPath, teamRaw)
			problems = append(problems, tp...)
			problems = append(problems, checkTeamOnly(trial.TeamPath, trial.Team)...)
		}
	}
	eff, err := mergeRaw(trial.teamRaw, raw)
	if err != nil {
		return problems, err
	}
	problems = append(problems, Validate(eff)...)
	if problems.HasErrors() {
		return problems, &InvalidError{Problems: problems}
	}
	return problems, writeFile(personalPath, raw) // the file as written, comments kept
}

// parse decodes one file strictly (unknown keys are errors, so typos surface)
// and scans it for secrets. It never returns nil.
func parse(file string, raw []byte) (*Config, Problems) {
	c := &collector{file: file}
	cfg := &Config{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return cfg, nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		c.at(lineOf(err), "", "not valid YAML: %s", cleanYAMLErr(err))
		return cfg, c.ps
	}
	scanSecrets(c, &root, "")

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		var te *yaml.TypeError
		if errors.As(err, &te) {
			for _, msg := range te.Errors {
				c.at(lineOf(errors.New(msg)), "", "%s", cleanYAMLErr(errors.New(msg)))
			}
		} else {
			c.at(lineOf(err), "", "%s", cleanYAMLErr(err))
		}
	}
	if cfg.Version == 0 {
		c.errorf("version", "missing; add \"version: %d\" at the top of the file", CurrentVersion)
	}
	return cfg, c.ps
}

// checkTeamOnly rejects personal-only settings in a shared team file.
func checkTeamOnly(file string, team *Config) Problems {
	c := &collector{file: file}
	if team == nil {
		return nil
	}
	if team.TeamFile != "" {
		c.errorf("teamFile", "team file cannot point to another team file")
	}
	if team.Jira.Auth != (Auth{}) {
		c.errorf("jira.auth", "team file cannot contain personal credentials; move jira.auth to %s", PersonalFileName)
	}
	for i, p := range team.Projects {
		for j, r := range p.Repos {
			if r.Path != "" {
				c.warnf(fmt.Sprintf("projects[%d].repos[%d].path", i, j), "local paths are personal; set them in %s", PersonalFileName)
			}
		}
	}
	return c.ps
}

// ── secret scanning (configuration spec rule 18.8) ──────

var (
	secretKey      = regexp.MustCompile(`(?i)(token|password|passwd|secret|api[-_]?key|private[-_]?key|authorization|cookie)`)
	allowedKeys    = map[string]bool{"credentialRef": true, "tokenExpires": true}
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^ATATT[A-Za-z0-9_\-=]{20,}`),                 // Atlassian API token
		regexp.MustCompile(`^(gh[pousr]_|github_pat_)[A-Za-z0-9_]{20,}`), // GitHub tokens
		regexp.MustCompile(`^xox[abprs]-`),                               // Slack
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
		regexp.MustCompile(`(?i)^(basic|bearer) [A-Za-z0-9+/=._\-]{16,}$`),
	}
)

func scanSecrets(c *collector, n *yaml.Node, path string) {
	switch n.Kind {
	case yaml.DocumentNode:
		for _, ch := range n.Content {
			scanSecrets(c, ch, path)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			p := joinPath(path, k.Value)
			if v.Kind == yaml.ScalarNode && v.Value != "" && !allowedKeys[k.Value] && secretKey.MatchString(k.Value) {
				c.at(v.Line, p, "looks like a secret; store it in the keychain and use jira.auth.credentialRef")
				continue
			}
			scanSecrets(c, v, p)
		}
	case yaml.SequenceNode:
		for i, ch := range n.Content {
			scanSecrets(c, ch, fmt.Sprintf("%s[%d]", path, i))
		}
	case yaml.ScalarNode:
		for _, re := range secretPatterns {
			if re.MatchString(n.Value) {
				c.at(n.Line, path, "value looks like a credential; files must not contain secrets")
				return
			}
		}
	}
}

func joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

// ── file helpers ────────────────────────────────────────────────

func readOptional(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

const fileHeader = "# Maestro configuration. Secrets are never stored here; see jira.auth.credentialRef.\n"

func writeYAML(path string, cfg *Config) error {
	var buf bytes.Buffer
	buf.WriteString(fileHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return writeFile(path, buf.Bytes())
}

// writeFile replaces path atomically (temp file, fsync, rename) with mode 0600.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ExpandHome turns "~/x" into an absolute path.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func resolvePath(p, relativeTo string) string {
	p = ExpandHome(p)
	if !filepath.IsAbs(p) {
		p = filepath.Join(relativeTo, p)
	}
	return filepath.Clean(p)
}

var lineRe = regexp.MustCompile(`line (\d+)`)

func lineOf(err error) int {
	m := lineRe.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	n := 0
	fmt.Sscanf(m[1], "%d", &n)
	return n
}

func cleanYAMLErr(err error) string {
	s := strings.TrimPrefix(err.Error(), "yaml: ")
	s = lineRe.ReplaceAllString(s, "")
	return strings.TrimLeft(s, ": ")
}

// NeedsSetup reports whether the first-start wizard must run (no Jira site yet).
func (l *Layers) NeedsSetup() bool {
	return l.Effective == nil || (l.Effective.Jira.Transport != TransportMock && l.Effective.Jira.Site == "")
}
