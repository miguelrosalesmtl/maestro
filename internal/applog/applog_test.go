package applog

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("not a JSON line: %q", sc.Text())
		}
		out = append(out, m)
	}
	return out
}

func TestSetupWritesJSONAndRedacts(t *testing.T) {
	t.Setenv("MAESTRO_LOG_LEVEL", "")
	dir := t.TempDir()
	l, err := Setup(Options{Dir: dir, Version: "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	l.Logger.Info("jira request", "path", "/rest/api/3/myself", "status", 200, "api_token", "ATATT-secret", "Authorization", "Basic abc")
	l.Logger.Debug("hidden at info level")
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	lines := readLines(t, filepath.Join(dir, FileName))
	if lines[0]["msg"] != "maestro started" || lines[0]["version"] != "1.2.3" {
		t.Errorf("first line = %v", lines[0])
	}
	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	if strings.Contains(string(raw), "ATATT-secret") || strings.Contains(string(raw), "Basic abc") {
		t.Fatalf("secret leaked:\n%s", raw)
	}
	if strings.Contains(string(raw), "hidden at info level") {
		t.Error("debug line written at info level")
	}
	if lines[len(lines)-1]["msg"] != "maestro stopped" {
		t.Error("clean shutdown must be logged")
	}
	if _, err := os.Stat(filepath.Join(dir, CrashFileName)); !os.IsNotExist(err) {
		t.Error("a clean shutdown must remove the empty crash file")
	}
}

func TestEnvLevelOverrides(t *testing.T) {
	t.Setenv("MAESTRO_LOG_LEVEL", "debug")
	l, err := Setup(Options{Dir: t.TempDir(), Level: slog.LevelError})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.SetLevel(slog.LevelWarn) // config must not beat the environment
	if !l.Logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("MAESTRO_LOG_LEVEL=debug must win")
	}
}

func TestRecoverLogsPanic(t *testing.T) {
	dir := t.TempDir()
	l, err := Setup(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	Go("test worker", func() { defer close(done); panic("kaboom") })
	<-done
	time.Sleep(20 * time.Millisecond) // Recover runs after the deferred close
	l.Close()
	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	if !strings.Contains(string(raw), `"panic":"kaboom"`) || !strings.Contains(string(raw), `"where":"test worker"`) || !strings.Contains(string(raw), "applog_test.go") {
		t.Fatalf("panic not logged with stack:\n%s", raw)
	}
}

// TestCrashReportSurvivesProcessDeath crashes a child process for real and
// checks that the next start finds the report.
func TestCrashReportSurvivesProcessDeath(t *testing.T) {
	if dir := os.Getenv("APPLOG_CRASH_DIR"); dir != "" {
		if _, err := Setup(Options{Dir: dir}); err != nil {
			os.Exit(3)
		}
		go func() { panic("unrecovered crash in goroutine") }()
		select {}
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashReportSurvivesProcessDeath$")
	cmd.Env = append(os.Environ(), "APPLOG_CRASH_DIR="+dir)
	if err := cmd.Run(); err == nil {
		t.Fatal("child should have crashed")
	}

	l, err := Setup(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if l.PreviousCrash == "" {
		t.Fatal("the next start must detect the crash")
	}
	report, _ := os.ReadFile(l.PreviousCrash)
	if !strings.Contains(string(report), "unrecovered crash in goroutine") {
		t.Fatalf("crash report missing the panic:\n%s", report)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	if !strings.Contains(string(raw), "previous run crashed") {
		t.Error("the crash must be noted in maestro.log")
	}
}

func TestFrontendRateLimit(t *testing.T) {
	dir := t.TempDir()
	l, err := Setup(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(0, 0)
	f := &Frontend{now: func() time.Time { return now }}
	for i := 0; i < 100; i++ {
		f.Log("error", "TypeError: x is undefined", "at render (App.tsx:10)", "window.onerror")
	}
	now = now.Add(time.Minute)
	f.Log("warn", "after the window", "", "console")
	l.Close()

	var frontend, limited int
	for _, m := range readLines(t, filepath.Join(dir, FileName)) {
		if m["component"] == "frontend" {
			frontend++
		}
		if m["msg"] == "frontend log rate limited" && m["dropped"] == float64(70) {
			limited++
		}
	}
	if frontend != frontendPerMinute+1 || limited != 1 {
		t.Fatalf("frontend lines = %d, rate-limit notes = %d", frontend, limited)
	}
}
