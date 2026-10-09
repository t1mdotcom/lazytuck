package tui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/t1mdotcom/lazytuck/internal/repo"
	"github.com/t1mdotcom/lazytuck/internal/workspace"
)

type tfx struct{ root, home, tmp string }

func (f tfx) c(p string) string { return filepath.Join(f.root, "Configs", p) }
func (f tfx) h(p string) string { return filepath.Join(f.home, p) }

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTfx(t *testing.T) tfx {
	t.Helper()
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	f := tfx{tmp: tmp, root: filepath.Join(tmp, "repo"), home: filepath.Join(tmp, "home")}
	write(t, f.c("zsh/.zshrc"), "zsh")
	write(t, f.c("vim/.vimrc"), "vim")
	write(t, f.c("tmux/.config/tmux/tmux.conf"), "repo tmux")
	write(t, f.c("aero_linux/.aerospace.toml"), "a")
	write(t, f.h(".config/tmux/tmux.conf"), "home tmux") // drift
	if err := os.Symlink(f.c("zsh/.zshrc"), f.h(".zshrc")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	return f
}

func newModel(t *testing.T, f tfx) Model {
	t.Helper()
	ws, err := workspace.Load(repo.Location{Repo: f.root, Target: f.home}, repo.Platform{GOOS: "darwin"})
	if err != nil {
		t.Fatal(err)
	}
	m := New(ws)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return next.(Model)
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// press sends keys in order and returns the model plus the last command.
func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		var next tea.Model
		next, cmd = m.Update(key(k))
		m = next.(Model)
	}
	return m, cmd
}

func screen(m Model) string { return ansi.Strip(m.render()) }

func snapshot(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, _ := os.Lstat(p)
		lt, _ := os.Readlink(p)
		out = append(out, fmt.Sprintf("%s %v %d %s", p, fi.Mode(), fi.ModTime().UnixNano(), lt))
		return nil
	})
	return out
}

func TestFocusCycles(t *testing.T) {
	m := newModel(t, newTfx(t))
	order := []pane{paneFiles, paneDetail, paneGit, paneGroups}
	for _, want := range order {
		m, _ = press(t, m, "tab")
		if m.focus != want {
			t.Fatalf("tab: focus = %d, want %d", m.focus, want)
		}
	}
	m, _ = press(t, m, "shift+tab")
	if m.focus != paneGit {
		t.Errorf("shift+tab from groups: focus = %d, want git", m.focus)
	}
	m, _ = press(t, m, "2")
	if m.focus != paneFiles {
		t.Errorf("2: focus = %d, want files", m.focus)
	}
}

func TestGroupSelectionFiltersFiles(t *testing.T) {
	m := newModel(t, newTfx(t))
	if n := len(m.files()); n != 4 {
		t.Fatalf("(all) lists %d files, want 4", n)
	}
	// groups sorted: (all), aero_linux, tmux, vim, zsh
	m, _ = press(t, m, "2", "j", "j", "1", "j", "j")
	if m.groups[m.gSel].name != "tmux" {
		t.Fatalf("selected group = %s, want tmux", m.groups[m.gSel].name)
	}
	if m.fSel != 0 {
		t.Errorf("changing group must reset file selection, fSel = %d", m.fSel)
	}
	fs := m.files()
	if len(fs) != 1 || fs[0].Rel != ".config/tmux/tmux.conf" {
		t.Errorf("tmux files = %v", fs)
	}
	s := screen(m)
	for _, want := range []string{"2 Files · tmux", "Group     tmux"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
	m, _ = press(t, m, "k")
	if !strings.Contains(screen(m), "inactive on darwin") {
		t.Error("aero_linux detail should say inactive on darwin")
	}
	m, _ = press(t, m, "j", "enter")
	if m.focus != paneFiles {
		t.Error("enter on a group should focus files")
	}
	if !strings.Contains(screen(m), "State     ± drift") {
		t.Errorf("file detail lacks drift state:\n%s", screen(m))
	}
}

func TestHelpAndQuit(t *testing.T) {
	m := newModel(t, newTfx(t))
	m, _ = press(t, m, "?")
	if !strings.Contains(screen(m), "toggle this help") {
		t.Fatal("help not shown")
	}
	m, _ = press(t, m, "x")
	if m.showHelp {
		t.Error("any key should close help")
	}
	_, cmd := press(t, m, "q")
	if cmd == nil || !reflect.DeepEqual(cmd(), tea.Quit()) {
		t.Error("q must quit")
	}
}

func TestRescanSeesChangesAndNavigationIsReadOnly(t *testing.T) {
	f := newTfx(t)
	m := newModel(t, f)
	before := snapshot(t, f.tmp)
	m, _ = press(t, m, "j", "j", "tab", "j", "k", "tab", "j", "tab", "tab", "?", "x", "R")
	if after := snapshot(t, f.tmp); !reflect.DeepEqual(before, after) {
		t.Fatal("navigation or rescan modified the filesystem (V5)")
	}
	if err := os.Symlink(f.c("vim/.vimrc"), f.h(".vimrc")); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, "R")
	if got := m.groups[0].counts["linked"]; got != 2 {
		t.Errorf("after rescan linked = %d, want 2", got)
	}
	if !strings.Contains(screen(m), "rescanned: 4 files") {
		t.Error("rescan should report in the status bar")
	}
}

func TestTooSmall(t *testing.T) {
	m := newModel(t, newTfx(t))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	if !strings.Contains(screen(next.(Model)), "terminal too small") {
		t.Error("expected too-small message")
	}
}

func TestPanesKeepSize(t *testing.T) {
	m := newModel(t, newTfx(t))
	lines := strings.Split(screen(m), "\n")
	if len(lines) != 30 {
		t.Errorf("rendered %d lines, want terminal height 30", len(lines))
	}
	for i, l := range lines[:len(lines)-1] {
		if w := ansi.StringWidth(l); w != 120 {
			t.Errorf("line %d width %d, want 120: %q", i, w, l)
			break
		}
	}
}
