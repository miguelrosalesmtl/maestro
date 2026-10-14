package envpath

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeShell(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fakesh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoginPathReadsTheShellsPath(t *testing.T) {
	// Like a real login shell: rc files print noise, then our command runs.
	sh := fakeShell(t, `echo "Welcome!"; PATH=/opt/homebrew/bin:/usr/bin; shift; eval "$1"`)
	if got := LoginPath(context.Background(), sh); got != "/opt/homebrew/bin:/usr/bin" {
		t.Fatalf("LoginPath = %q", got)
	}
}

func TestLoginPathGivesUpOnASlowShell(t *testing.T) {
	old := Timeout
	Timeout = 200 * time.Millisecond
	defer func() { Timeout = old }()
	sh := fakeShell(t, `sleep 5`)
	start := time.Now()
	if got := LoginPath(context.Background(), sh); got != "" {
		t.Fatalf("LoginPath = %q", got)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %s", d)
	}
}

func TestMergeKeepsOrderWithoutDuplicates(t *testing.T) {
	got := Merge("/opt/homebrew/bin:/usr/bin", "/usr/bin:/bin::", "/usr/local/bin:/opt/homebrew/bin")
	if got != "/opt/homebrew/bin:/usr/bin:/bin:/usr/local/bin" {
		t.Fatalf("Merge = %q", got)
	}
}

// Started from the Dock: no TERM, minimal PATH. gh in a Homebrew folder is found.
func TestFixFindsToolsOutsideTheMinimalPath(t *testing.T) {
	brew := t.TempDir()
	if err := os.WriteFile(filepath.Join(brew, "gh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TERM", "")
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SHELL", fakeShell(t, `PATH=`+brew+`:/usr/bin; shift; eval "$1"`))
	Fix(slogDiscard())
	if p := os.Getenv("PATH"); filepath.SplitList(p)[0] != brew {
		t.Fatalf("PATH = %q", p)
	}
}

func slogDiscard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
