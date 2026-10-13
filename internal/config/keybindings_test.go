package config

import "testing"

func TestKeybindingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if kb, err := LoadKeybindings(dir); err != nil || len(kb) != 0 {
		t.Fatalf("missing file should load empty, got %v %v", kb, err)
	}
	want := Keybindings{"nav.board": "g b", "focus.terminal": ""}
	if _, err := SaveKeybindings(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadKeybindings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["nav.board"] != "g b" || got["focus.terminal"] != "" {
		t.Fatalf("got %v", got)
	}
}
