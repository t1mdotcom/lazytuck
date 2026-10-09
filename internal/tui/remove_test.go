package tui

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// In (all): 0 aero_linux/.aerospace.toml, 1 tmux/.config/tmux/tmux.conf (drift), 2 vim/.vimrc (missing), 3 zsh/.zshrc (linked).

func TestUnmanageFileKeepsRealCopy(t *testing.T) {
	f := newTfx(t)
	m := newModel(t, f)
	m, _ = press(t, m, "2", "j", "j", "j", "x")
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "the link in ~ becomes a real copy") {
		t.Fatalf("unmanage must ask and describe the effect: %+v", m.confirm)
	}
	m, _ = press(t, m, "y")
	fi, err := os.Lstat(f.h(".zshrc"))
	if err != nil || !fi.Mode().IsRegular() {
		t.Fatalf(".zshrc in home is %v (%v), want a real file", fi, err)
	}
	if b, _ := os.ReadFile(f.h(".zshrc")); string(b) != "zsh" {
		t.Errorf("real copy = %q", b)
	}
	if _, err := os.Stat(f.c("zsh")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("zsh group still in repo")
	}
	for _, g := range m.groups {
		if g.name == "zsh" {
			t.Error("removed group still listed after rescan")
		}
	}
	if !strings.Contains(m.note, "unmanaged .zshrc — press c to commit") {
		t.Errorf("note = %q", m.note)
	}
}

func TestDeleteGroupLeavesForeignContent(t *testing.T) {
	f := newTfx(t)
	m := newModel(t, f)
	m, _ = press(t, m, "j", "j", "X") // groups: (all), aero_linux, tmux
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "delete group tmux (1 file(s))") {
		t.Fatalf("prompt = %+v", m.confirm)
	}
	m, _ = press(t, m, "y")
	if _, err := os.Stat(f.c("tmux")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("tmux still in repo; note %q", m.note)
	}
	if b, _ := os.ReadFile(f.h(".config/tmux/tmux.conf")); string(b) != "home tmux" {
		t.Errorf("drifted home file must stay untouched (V16), got %q", b)
	}
}

func TestRemoveRefusedOnAllAndCancellable(t *testing.T) {
	f := newTfx(t)
	m := newModel(t, f)
	before := snapshot(t, f.tmp)
	m, _ = press(t, m, "X")
	if m.confirm != nil || !m.noteErr || !strings.Contains(m.note, "select a single group") {
		t.Errorf("X on (all): confirm %v, note %q", m.confirm != nil, m.note)
	}
	m, _ = press(t, m, "2", "j", "j", "j", "x", "n")
	if !strings.Contains(m.note, "cancelled") {
		t.Errorf("note = %q", m.note)
	}
	if after := snapshot(t, f.tmp); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Error("refused or cancelled removal changed the filesystem")
	}
}
