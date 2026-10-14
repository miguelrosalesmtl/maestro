// Package envpath gives Maestro the user's PATH when it was not started from
// a terminal. Apps opened from the macOS Dock or Finder (or a desktop
// launcher) get a minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin), so tools
// installed with Homebrew and friends (gh, …) are not found.
package envpath

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Timeout bounds asking the login shell: a slow or broken shell setup must
// not keep the window from opening.
var Timeout = 3 * time.Second

const marker = "__MAESTRO_PATH__"

// Fix sets PATH (and SHELL, when missing) from the user's login shell, plus
// the usual install folders. It does nothing when started from a terminal,
// whose PATH is already the user's.
func Fix(log *slog.Logger) {
	if os.Getenv("TERM") != "" {
		return
	}
	if os.Getenv("SHELL") == "" {
		if sh := userShell(); sh != "" {
			os.Setenv("SHELL", sh)
		}
	}
	before := os.Getenv("PATH")
	fromShell := LoginPath(context.Background(), Shell())
	after := Merge(fromShell, before, Extras())
	os.Setenv("PATH", after)
	log.Info("PATH set for an app started outside a terminal", "fromLoginShell", fromShell != "", "dirs", strings.Count(after, ":")+1)
}

// Shell is the user's shell ($SHELL, else the platform default).
func Shell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/sh"
}

// LoginPath asks a login, interactive shell for its PATH, the way a new
// terminal window would set it up. "" when the shell fails or is too slow.
func LoginPath(ctx context.Context, shell string) string {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	script := `printf '\n%s%s%s\n' ` + marker + ` "$PATH" ` + marker
	if filepath.Base(shell) == "fish" {
		script = `printf '\n%s%s%s\n' ` + marker + ` (string join : $PATH) ` + marker
	}
	cmd := exec.CommandContext(ctx, shell, "-ilc", script)
	cmd.Stdin = nil                        // /dev/null: nothing may wait for input
	cmd.WaitDelay = 500 * time.Millisecond // do not wait on helpers the shell started
	out, _ := cmd.Output()                 // rc files may fail and still print PATH
	s := string(out)
	i := strings.Index(s, marker)
	if i < 0 {
		return ""
	}
	s = s[i+len(marker):]
	j := strings.Index(s, marker)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(s[:j])
}

// Extras are the usual install folders that exist on this machine.
func Extras() string {
	dirs := []string{"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	var out []string
	for _, d := range dirs {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}

// Merge joins PATH lists in order of preference, without duplicates.
func Merge(lists ...string) string {
	var out []string
	seen := map[string]bool{}
	for _, l := range lists {
		for _, d := range filepath.SplitList(l) {
			if d = strings.TrimSpace(d); d != "" && !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}

// userShell is the login shell from the user database.
func userShell() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("dscl", ".", "-read", "/Users/"+u.Username, "UserShell").Output()
		if err != nil {
			return ""
		}
		_, sh, _ := strings.Cut(strings.TrimSpace(string(out)), ":")
		return strings.TrimSpace(sh)
	}
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Split(line, ":"); len(f) >= 7 && f[0] == u.Username {
			return f[6]
		}
	}
	return ""
}
