package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox points every lookup input at temp dirs and returns (repo, home).
func sandbox(t *testing.T) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	repo := filepath.Join(home, ".dotfiles")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdg"))
	t.Setenv("TUCKR_HOME", "")
	t.Setenv("TUCKR_TARGET", "")
	src := filepath.Join(repo, "Configs", "zsh", ".zshrc")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo, home
}

func TestStatusExitCodes(t *testing.T) {
	repo, home := sandbox(t)
	var out, errb bytes.Buffer

	if code := run([]string{"status"}, &out, &errb); code != 1 {
		t.Fatalf("missing link: exit %d, want 1 (stderr %q)", code, errb.String())
	}
	if !strings.Contains(out.String(), "missing") {
		t.Errorf("table lacks state: %q", out.String())
	}

	if err := os.Symlink(filepath.Join(repo, "Configs", "zsh", ".zshrc"), filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := run([]string{"status", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("all linked: exit %d, want 0 (stderr %q)", code, errb.String())
	}
	var entries []map[string]any
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	if len(entries) != 1 || entries[0]["state"] != "linked" || entries[0]["path"] != ".zshrc" || entries[0]["group"] != "zsh" {
		t.Errorf("entries = %v", entries)
	}
}

func TestStatusNoRepo(t *testing.T) {
	_, home := sandbox(t)
	if err := os.RemoveAll(filepath.Join(home, ".dotfiles")); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"status"}, &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(errb.String(), filepath.Join(home, ".dotfiles")) {
		t.Errorf("error should list tried paths, got %q", errb.String())
	}
}

func TestRepoFlagOverridesLookup(t *testing.T) {
	sandbox(t)
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "Configs"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"--repo", other, "status", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("empty repo should print [], got %q", out.String())
	}
}
