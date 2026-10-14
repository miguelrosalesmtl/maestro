// Command maestro-jira connects Maestro to Jira from the terminal and checks
// the connection. The desktop app uses the same packages.
//
//	go run ./cmd/maestro-jira login            # site + token → keychain + config.yaml
//	go run ./cmd/maestro-jira check            # validate config, test the token
//	go run ./cmd/maestro-jira discover [-apply] [-json] [-projects PLAT,ONB]
//	go run ./cmd/maestro-jira detect <site>
//	go run ./cmd/maestro-jira logout
//	go run ./cmd/maestro-jira github-login     # GitHub token → keychain (instead of gh)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"golang.org/x/term"

	"maestro/internal/config"
	"maestro/internal/github"
	"maestro/internal/secrets"
	"maestro/internal/setup"
	"maestro/internal/tracker/jira"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmds := map[string]func(context.Context, []string) error{
		"detect": detect, "login": login, "check": check, "discover": discover, "logout": logout,
		"github-login": githubLogin,
		"version": func(context.Context, []string) error { fmt.Println("maestro-jira", setup.Version); return nil },
	}
	run, ok := cmds[os.Args[1]]
	if !ok {
		usage()
		os.Exit(2)
	}
	if err := run(ctx, os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", explain(err))
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: maestro-jira <login|check|discover|detect|logout|github-login|version> [flags]")
}

// explain turns typed errors into the messages the spec asks for.
func explain(err error) string {
	switch {
	case errors.Is(err, jira.ErrUnauthorized):
		return "Jira token is not valid or expired. Run: maestro-jira login"
	case errors.Is(err, jira.ErrCaptcha):
		return jira.ErrCaptcha.Error()
	case errors.Is(err, secrets.ErrNotFound):
		return "Jira token not found in the keychain. Run: maestro-jira login"
	case errors.Is(err, setup.ErrNotConfigured):
		return "Maestro is not connected to Jira yet. Run: maestro-jira login"
	}
	return err.Error()
}

func logger(verbose bool) *slog.Logger {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func detect(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: maestro-jira detect <site>")
	}
	hc, err := jira.NewHTTPClient(jira.NetOptions{})
	if err != nil {
		return err
	}
	info, err := jira.DetectSite(ctx, hc, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("site:        %s\ndeployment:  %s\n", info.BaseURL, info.Deployment)
	if info.CloudID != "" {
		fmt.Printf("cloud ID:    %s\n", info.CloudID)
	}
	if info.Version != "" {
		fmt.Printf("version:     %s\n", info.Version)
	}
	return nil
}

func login(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	site := fs.String("site", "", "Jira site, e.g. your-org.atlassian.net")
	user := fs.String("email", "", "Atlassian account email (Cloud) or user name (Data Center)")
	verbose := fs.Bool("v", false, "log requests")
	fs.Parse(args)

	in := bufio.NewReader(os.Stdin)
	if *site == "" {
		*site = prompt(in, "Jira site (e.g. your-org.atlassian.net): ")
	}
	hc, err := jira.NewHTTPClient(jira.NetOptions{})
	if err != nil {
		return err
	}
	info, err := jira.DetectSite(ctx, hc, *site)
	if err != nil {
		return err
	}
	fmt.Printf("Found Jira %s at %s\n", info.Deployment, info.BaseURL)

	method := config.AuthAPIToken
	if info.Deployment == jira.DataCenter {
		method = config.AuthPAT
	}
	if *user == "" {
		label := "Atlassian account email"
		if method == config.AuthPAT {
			label = "Jira user name"
		}
		*user = prompt(in, label+": ")
	}
	if method == config.AuthAPIToken {
		host, _ := os.Hostname()
		fmt.Printf("\nCreate an API token at https://id.atlassian.com/manage-profile/security/api-tokens\n(name it \"Maestro on %s\"), then paste it below.\n", host)
	} else {
		fmt.Println("\nCreate a personal access token in Jira → Profile → Personal Access Tokens, then paste it below.")
	}
	token, err := readSecret(in, "Token: ")
	if err != nil {
		return err
	}

	var auth jira.Authenticator = jira.BasicToken{Email: *user, Token: token}
	if method == config.AuthPAT {
		auth = jira.BearerToken{Token: token}
	}
	c, err := jira.New(jira.Options{BaseURL: info.BaseURL, Deployment: info.Deployment, Auth: auth, HTTPClient: hc, Logger: logger(*verbose)})
	if err != nil {
		return err
	}
	me, err := c.Myself(ctx) // only keep a token that works
	if err != nil {
		return err
	}

	u, _ := url.Parse(info.BaseURL)
	ref := secrets.JiraRef(u.Host, *user)
	if err := secrets.NewStore().Set(ref, token); err != nil {
		return err
	}

	path, err := config.PersonalPath()
	if err != nil {
		return err
	}
	l, err := config.Load(path)
	if l == nil {
		return err
	}
	p := *l.Personal
	p.Version = config.CurrentVersion
	p.Jira.Site = info.BaseURL
	p.Jira.Deployment = config.Deployment(info.Deployment)
	p.Jira.CloudID = info.CloudID
	p.Jira.Auth.Method = method
	p.Jira.Auth.Email = ""
	if method != config.AuthPAT {
		p.Jira.Auth.Email = *user
	}
	p.Jira.Auth.CredentialRef = ref.String()
	if err := l.SetPersonal(&p); err != nil {
		return err
	}
	printProblems(l.Problems)
	if l.Problems.HasErrors() {
		return errors.New("configuration not saved")
	}
	if err := l.SavePersonal(); err != nil {
		return err
	}
	fmt.Printf("\nConnected as %s. Token saved in the OS keychain; settings in %s\n", me.DisplayName, path)
	fmt.Println("Next: maestro-jira discover -apply")
	return nil
}

func check(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	verbose := fs.Bool("v", false, "log requests")
	fs.Parse(args)

	l, c, err := connect(*verbose)
	if err != nil {
		return err
	}
	start := time.Now()
	me, err := c.Myself(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("✓ config    %s\n✓ token     %s (%s) in %s\n", l.PersonalPath, me.DisplayName, l.Effective.Jira.Deployment, time.Since(start).Round(time.Millisecond))
	if si, err := c.ServerInfo(ctx); err == nil && si.ServerTime != "" {
		if t, err := time.Parse("2006-01-02T15:04:05.000-0700", si.ServerTime); err == nil {
			if skew := time.Since(t); skew > 2*time.Minute || skew < -2*time.Minute {
				fmt.Printf("! clock     differs from Jira by %s\n", skew.Round(time.Second))
			}
		}
	}
	if keys := projectKeys(l.Effective); len(keys) > 0 {
		d, err := c.Discover(ctx, keys)
		if err != nil {
			return err
		}
		ps := setup.CheckAgainst(l.Effective, d)
		if len(ps) == 0 {
			fmt.Println("✓ fields    config agrees with Jira")
		}
		for _, p := range ps {
			mark := "!"
			if p.Severity == config.SeverityError {
				mark = "✗"
			}
			fmt.Printf("%s %-9s %s: %s\n", mark, "config", p.Path, p.Message)
		}
	}
	for _, p := range l.Effective.Projects {
		perms, err := c.MyPermissions(ctx, p.Key, jira.WritePermissions)
		if err != nil {
			fmt.Printf("✗ %-9s %v\n", p.Key, explain(err))
			continue
		}
		var missing []string
		for _, k := range jira.WritePermissions {
			if !perms[k] {
				missing = append(missing, k)
			}
		}
		if len(missing) == 0 {
			fmt.Printf("✓ %-9s all permissions\n", p.Key)
		} else {
			fmt.Printf("! %-9s missing %s\n", p.Key, strings.Join(missing, ", "))
		}
	}
	checkGitHub(ctx, l.Effective)
	return nil
}

// checkGitHub reports GitHub sign-in and access to each configured repo.
// Problems are printed, not returned: Jira works without GitHub.
func checkGitHub(ctx context.Context, cfg *config.Config) {
	gh := cfg.Dev.GitHub
	var repos []string
	for _, p := range cfg.Projects {
		for _, r := range p.Repos {
			repos = append(repos, r.Name)
		}
	}
	switch {
	case !gh.IsEnabled():
		fmt.Println("- github    off (dev.github.enabled: false)")
		return
	case len(repos) == 0:
		fmt.Println("- github    no repositories in projects[].repos; pull requests are not followed")
		return
	}
	host := github.WebHost(gh.APIURL)
	var tokens github.TokenSource = github.GHCLI{Host: host}
	if gh.Auth == "token" {
		tokens = github.Keychain{Store: secrets.NewStore(), Ref: secrets.GitHubRef(host)}
	}
	c, err := github.New(github.Options{APIURL: gh.APIURL, Tokens: tokens, UserAgent: "maestro-jira"})
	if err != nil {
		fmt.Printf("✗ github    %v\n", err)
		return
	}
	u, err := c.Viewer(ctx)
	if err != nil {
		fmt.Printf("✗ github    %v\n", githubHint(err, gh.Auth, host))
		return
	}
	fmt.Printf("✓ github    @%s on %s (auth: %s)\n", u.Login, host, gh.Auth)
	for _, r := range repos {
		prs, err := c.OpenPullRequests(ctx, r)
		if err != nil {
			fmt.Printf("✗ %-9s %v\n", "repo", githubHint(err, gh.Auth, host))
			continue
		}
		fmt.Printf("✓ %-9s %s · %d open pull requests\n", "repo", r, len(prs))
	}
}

func githubHint(err error, auth, host string) string {
	switch {
	case errors.Is(err, secrets.ErrNotFound):
		return "GitHub token not found in the keychain. Run: maestro-jira github-login"
	case errors.Is(err, github.ErrNotFound):
		return err.Error() + " (check the owner/name, and that the token can read this repository)"
	case errors.Is(err, github.ErrUnauthorized) && auth == "token":
		return "GitHub token rejected. Run: maestro-jira github-login"
	}
	return err.Error()
}

// githubLogin stores a GitHub token in the keychain, for dev.github.auth: token.
func githubLogin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("github-login", flag.ExitOnError)
	apiURL := fs.String("api", "", "API URL (GitHub Enterprise: https://HOST/api/v3); default from config")
	fs.Parse(args)
	if *apiURL == "" {
		if path, err := config.PersonalPath(); err == nil {
			if l, _ := config.Load(path); l != nil {
				*apiURL = l.Effective.Dev.GitHub.APIURL
			}
		}
	}
	host := github.WebHost(*apiURL)
	fmt.Printf("Create a fine-grained token on %s with read access to Pull requests, Checks,\nCommit statuses and Contents of your repositories.\n", host)
	tok, err := readSecret(bufio.NewReader(os.Stdin), "Token: ")
	if err != nil || tok == "" {
		return errors.New("no token entered")
	}
	c, err := github.New(github.Options{APIURL: *apiURL, Tokens: github.Static(tok.Reveal()), UserAgent: "maestro-jira"})
	if err != nil {
		return err
	}
	u, err := c.Viewer(ctx)
	if err != nil {
		return err
	}
	if err := secrets.NewStore().Set(secrets.GitHubRef(host), tok); err != nil {
		return err
	}
	fmt.Printf("Signed in as @%s. Token saved in the keychain.\nSet dev.github.auth: token in config.yaml to use it (the default, gh, uses `gh auth token`).\n", u.Login)
	return nil
}

func discover(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	projects := fs.String("projects", "", "comma-separated project keys (default: from config, else suggested)")
	apply := fs.Bool("apply", false, "write the proposal to config.yaml")
	asJSON := fs.Bool("json", false, "print the raw discovery as JSON")
	verbose := fs.Bool("v", false, "log requests")
	fs.Parse(args)

	l, c, err := connect(*verbose)
	if err != nil {
		return err
	}
	keys := splitKeys(*projects)
	if len(keys) == 0 {
		for _, p := range l.Effective.Projects {
			keys = append(keys, p.Key)
		}
	}
	if len(keys) == 0 {
		sugg, err := setup.SuggestProjects(ctx, c)
		if err != nil {
			return err
		}
		for i, s := range sugg {
			if i == 5 {
				break
			}
			keys = append(keys, s.Key)
			fmt.Printf("suggested project %-8s %s (%d recent issues)\n", s.Key, s.Name, s.Issues)
		}
		if len(keys) == 0 {
			return errors.New("no recent projects found; pass -projects KEY1,KEY2")
		}
	}

	start := time.Now()
	d, err := c.Discover(ctx, keys)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(d)
	}
	p := setup.Propose(d)
	fmt.Printf("\nDiscovery of %s took %s\n", strings.Join(keys, ", "), time.Since(start).Round(time.Millisecond))
	fmt.Printf("  story points field: %s\n  sprint field:       %s\n", orNone(p.Fields.StoryPoints), orNone(p.Fields.Sprint))
	fmt.Println("\nIssue types → roles")
	for _, r := range p.Roles {
		fmt.Printf("  %-22s %-11s %s\n", r.IssueType.Name, r.Role, r.Confidence)
	}
	fmt.Println("\nStatuses → Maestro states")
	for _, s := range p.States {
		fmt.Printf("  %-22s %-11s %s\n", s.Status.Name, s.State, s.Confidence)
	}
	for _, w := range d.Warnings {
		fmt.Println("! " + w)
	}
	std := d.Standard()
	fmt.Printf("\nStandard fields found by schema: sprint=%s points=%s flagged=%s start=%s team=%s\n",
		orNone(std.Sprint), orNone(std.StoryPoints), orNone(std.Flagged), orNone(std.StartDate), orNone(std.Team))
	for _, p := range setup.CheckAgainst(l.Effective, d) {
		fmt.Println("! config " + p.Path + ": " + p.Message)
	}
	if len(p.NeedsConfirmation) > 0 {
		fmt.Printf("\nPlease confirm these statuses (Jira cannot tell \"in review\" or \"blocked\" apart): %s\n", strings.Join(p.NeedsConfirmation, ", "))
	}
	if !*apply {
		fmt.Println("\nRun again with -apply to save this to config.yaml.")
		return nil
	}

	pers := *l.Personal
	have := map[string]bool{}
	for _, pr := range pers.Projects {
		have[pr.Key] = true
	}
	for _, k := range keys {
		if !have[k] {
			pers.Projects = append(pers.Projects, config.Project{Key: k})
		}
	}
	p.Apply(&pers)
	if err := l.SetPersonal(&pers); err != nil {
		return err
	}
	printProblems(l.Problems)
	if l.Problems.HasErrors() {
		return errors.New("configuration not saved")
	}
	if err := l.SavePersonal(); err != nil {
		return err
	}
	fmt.Println("\nSaved to", l.PersonalPath)
	return nil
}

func logout(ctx context.Context, args []string) error {
	path, err := config.PersonalPath()
	if err != nil {
		return err
	}
	l, _ := config.Load(path)
	if l == nil || l.Effective.Jira.Auth.CredentialRef == "" {
		return errors.New("not logged in")
	}
	ref, err := secrets.ParseRef(l.Effective.Jira.Auth.CredentialRef)
	if err != nil {
		return err
	}
	if err := secrets.NewStore().Delete(ref); err != nil {
		return err
	}
	fmt.Println("Token removed from the keychain.")
	return nil
}

// connect loads the config and builds a client, reporting config problems.
func connect(verbose bool) (*config.Layers, *jira.Client, error) {
	path, err := config.PersonalPath()
	if err != nil {
		return nil, nil, err
	}
	l, err := config.Load(path)
	if l == nil {
		return nil, nil, err
	}
	if l.NeedsSetup() {
		return nil, nil, setup.ErrNotConfigured
	}
	printProblems(l.Problems)
	if err != nil {
		return nil, nil, err
	}
	c, err := setup.Connect(l.Effective, secrets.NewStore(), logger(verbose))
	return l, c, err
}

func printProblems(ps config.Problems) {
	for _, p := range ps {
		fmt.Fprintln(os.Stderr, "  "+p.String())
	}
}

func prompt(in *bufio.Reader, label string) string {
	fmt.Print(label)
	s, _ := in.ReadString('\n')
	return strings.TrimSpace(s)
}

// readSecret reads without echo on a terminal; from a pipe it reads one line.
func readSecret(in *bufio.Reader, label string) (secrets.Secret, error) {
	fmt.Print(label)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return secrets.Secret(strings.TrimSpace(string(b))), nil
	}
	s, err := in.ReadString('\n')
	if err != nil && s == "" {
		return "", err
	}
	return secrets.Secret(strings.TrimSpace(s)), nil
}

func splitKeys(s string) []string {
	var out []string
	for _, k := range strings.Split(s, ",") {
		if k = strings.ToUpper(strings.TrimSpace(k)); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func orNone(s string) string {
	if s == "" {
		return "(not found)"
	}
	return s
}

func projectKeys(cfg *config.Config) []string {
	var out []string
	for _, p := range cfg.Projects {
		out = append(out, p.Key)
	}
	return out
}
