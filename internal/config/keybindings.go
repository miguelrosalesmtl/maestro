// Package config persists user preferences under the OS config dir
// (~/.config/maestro on Linux, ~/Library/Application Support/maestro on macOS).
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Keybindings maps an action id (e.g. "nav.board") to a key combo
// (e.g. "g b", "mod+k", "shift+f"). Only overrides of the defaults are stored;
// an empty combo means the action is unbound.
type Keybindings map[string]string

// Dir is where Maestro keeps its config files.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "maestro"), nil
}

func keybindingsPath(dir string) string { return filepath.Join(dir, "keybindings.json") }

// LoadKeybindings reads overrides from dir; a missing file means no overrides.
func LoadKeybindings(dir string) (Keybindings, error) {
	b, err := os.ReadFile(keybindingsPath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return Keybindings{}, nil
	}
	if err != nil {
		return nil, err
	}
	kb := Keybindings{}
	if err := json.Unmarshal(b, &kb); err != nil {
		return nil, err
	}
	return kb, nil
}

// SaveKeybindings writes overrides atomically (write temp file, then rename).
func SaveKeybindings(dir string, kb Keybindings) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(kb, "", "  ")
	if err != nil {
		return "", err
	}
	path := keybindingsPath(dir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}
