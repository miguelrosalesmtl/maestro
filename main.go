package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"maestro/internal/applog"
	"maestro/internal/config"
	"maestro/internal/setup"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	logs, err := applog.Setup(applog.Options{Version: version, Stderr: true})
	if err != nil {
		// Logging must never stop the app; fall back to stderr only.
		fmt.Fprintln(os.Stderr, "log file unavailable:", err)
	} else {
		defer logs.Close()
	}
	setup.Version = version
	cfg := loadConfig(logs)

	// WebKitGTK's DMA-BUF renderer is flaky on Wayland compositors (crashes in
	// libEGL on teardown, blank windows). The user can still override it.
	if runtime.GOOS == "linux" && os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
		os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}

	app := NewApp(cfg, logs)

	err = wails.Run(&options.App{
		Title: "Maestro",
		// Start maximized and follow the screen (tiling window managers resize
		// it freely); Width/Height are the size when un-maximized.
		Width:            1440,
		Height:           900,
		MinWidth:         900,
		MinHeight:        600,
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 15, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Logger:           applog.WailsLogger{L: slog.Default()},
		LogLevel:         logger.INFO,
		Bind:             []interface{}{app},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			About: &mac.AboutInfo{
				Title:   "Maestro",
				Message: "Your tickets, conducted.",
			},
		},
		Linux: &linux.Options{
			ProgramName:         "maestro",
			WebviewGpuPolicy:    linux.WebviewGpuPolicyOnDemand,
			WindowIsTranslucent: false,
		},
	})
	if err != nil {
		slog.Error("wails exited with an error", "err", err)
	}
}

// loadConfig reads config.yaml. Problems are logged, never fatal: the app
// still starts (with the mock tracker) so the user can fix them in Settings.
func loadConfig(logs *applog.Logs) *config.Config {
	path, err := config.PersonalPath()
	if err != nil {
		slog.Error("config path", "err", err)
		return config.Default()
	}
	l, err := config.Load(path)
	if l == nil {
		slog.Error("config unreadable", "path", path, "err", err)
		return config.Default()
	}
	if l.NeedsSetup() {
		// A fresh install is not an error: Jira is simply not connected yet.
		slog.Info("jira not connected yet; run setup", "path", path)
	} else {
		for _, p := range l.Problems {
			level := slog.LevelWarn
			if p.Severity == config.SeverityError {
				level = slog.LevelError
			}
			attrs := []any{"key", p.Path, "problem", p.Message}
			if p.File != "" {
				attrs = append(attrs, "file", p.File)
			}
			if p.Line > 0 {
				attrs = append(attrs, "line", p.Line)
			}
			slog.Log(context.Background(), level, "config problem", attrs...)
		}
	}
	if logs != nil {
		if lv, ok := applog.ParseLevel(l.Effective.Log.Level); ok {
			logs.SetLevel(lv)
		}
	}
	slog.Info("config loaded", "path", path, "needs_setup", l.NeedsSetup(), "transport", string(l.Effective.Jira.Transport))
	return l.Effective
}
