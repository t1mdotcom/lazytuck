package tui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// In (all), files are ordered: 0 aero_linux/.aerospace.toml, 1 tmux/.config/tmux/tmux.conf,
// 2 vim/.vimrc, 3 zsh/.zshrc.

func linkTo(p string) string { s, _ := os.Readlink(p); return s }

func TestFileLinkUnlink(t *testing.T) {
	f := newTfx(t)
	m := newModel(t, f)
	m, _ = press(t, m, "2", "j", "j", "space")
	if linkTo(f.h(".vimrc")) != f.c("vim/.vimrc") {
		t.Fatalf(".vimrc not linked; note %q", m.note)
	}
	if !strings.Contains(screen(m), "linked .vimrc") {
		t.Error("status bar should confirm the link")
	}
	m, _ = press(t, m, "j", "space")
	if _, err := os.Lstat(f.h(".zshrc")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".zshrc still present after unlink; note %q", m.note)
	}
	if _, err := os.Stat(f.c("zsh/.zshrc")); err != nil {
		t.Error("unlink removed the repo file")
	}
}

func TestRestoreAsksAndBacksUp(t *testing.T) {
	f := newTfx(t)
	m := newModel(t, f)
	m, _ = press(t, m, "2", "j", "r")
	if m.confirm == nil || !strings.Contains(screen(m), "[y/N]") {
		t.Fatal("restore must ask for confirmation")
	}
	m, _ = press(t, m, "n")
	if b, _ := os.ReadFile(f.h(".config/tmux/tmux.conf")); string(b) != "home tmux" {
		t.Fatal("cancelled restore changed the file")
	}
	if !strings.Contains(screen(m), "cancelled") {
		t.Error("cancel should be reported")
	}
	m, _ = press(t, m, "r", "y")
	if linkTo(f.h(".config/tmux/tmux.conf")) != f.c("tmux/.config/tmux/tmux.conf") {
		t.Fatalf("not restored; note %q", m.note)
	}
	backups, _ := filepath.Glob(filepath.Join(f.tmp, "state", "lazytuck", "backup", "*", ".config", "tmux", "tmux.conf"))
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	if b, _ := os.ReadFile(backups[0]); string(b) != "home tmux" {
		t.Errorf("backup content = %q", b)
	}
}

func TestAdoptCopiesHomeIntoRepo(t *testing.T) {
	f := newTfx(t)
	m := newModel(t, f)
	m, _ = press(t, m, "2", "j", "a")
	if b, _ := os.ReadFile(f.c("tmux/.config/tmux/tmux.conf")); string(b) != "home tmux" {
		t.Fatalf("repo not updated; note %q", m.note)
	}
	if linkTo(f.h(".config/tmux/tmux.conf")) != f.c("tmux/.config/tmux/tmux.conf") {
		t.Error("not linked after adopt")
	}
}

func TestGroupLinkAllSkipsInactive(t *testing.T) {
	f := newTfx(t)
	m := newModel(t, f)
	m, _ = press(t, m, "space")
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "link 1 file(s) in (all)") {
		t.Fatalf("group op prompt = %+v", m.confirm)
	}
	m, _ = press(t, m, "y")
	if linkTo(f.h(".vimrc")) != f.c("vim/.vimrc") {
		t.Errorf(".vimrc not linked; note %q", m.note)
	}
	if _, err := os.Lstat(f.h(".aerospace.toml")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("inactive group was linked (V7)")
	}
	if linkTo(f.h(".config/tmux/tmux.conf")) != "" {
		t.Error("link-all must not replace a drifted file")
	}
}

func TestOpErrorsShowInStatusBar(t *testing.T) {
	f := newTfx(t)
	m := newModel(t, f)
	// inactive file: op not applicable
	m, _ = press(t, m, "2", "space")
	if !m.noteErr || !strings.Contains(m.note, ".aerospace.toml is inactive; cannot link it") {
		t.Errorf("note = %q (err=%v)", m.note, m.noteErr)
	}
	// file changed after the scan
	if err := os.Remove(f.h(".config/tmux/tmux.conf")); err != nil {
		t.Fatal(err)
	}
	m, _ = press(t, m, "j", "a")
	if !m.noteErr || !strings.Contains(m.note, "adopt .config/tmux/tmux.conf") {
		t.Errorf("note = %q (err=%v)", m.note, m.noteErr)
	}
	if got := m.files()[1].State; got != "missing" {
		t.Errorf("state after failed op = %s, want rescanned missing (V8)", got)
	}
}
