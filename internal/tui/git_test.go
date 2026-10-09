package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/t1mdotcom/lazytuck/internal/testutil"
)

// newGitTfx is newTfx with the repo pushed to a bare remote; it returns the remote path.
func newGitTfx(t *testing.T) (tfx, string) {
	t.Helper()
	testutil.IsolateGit(t)
	f := newTfx(t)
	remote := filepath.Join(f.tmp, "remote.git")
	testutil.Remote(t, remote)
	testutil.Git(t, f.root, "init", "-q")
	testutil.Git(t, f.root, "remote", "add", "origin", remote)
	testutil.Seed(t, f.root)
	return f, remote
}

// runCmd executes a command synchronously and feeds its message back, like the runtime would.
func runCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

func TestCommitFlow(t *testing.T) {
	f, _ := newGitTfx(t)
	m := newModel(t, f)
	m, _ = press(t, m, "c")
	if m.commit != nil || !strings.Contains(m.note, "nothing to commit") {
		t.Fatalf("clean repo: commit=%v note %q", m.commit != nil, m.note)
	}

	m, _ = press(t, m, "2", "j", "a") // adopt drifted tmux.conf → repo file changes
	m, _ = press(t, m, "c")
	if m.commit == nil || !strings.Contains(screen(m), "Configs/tmux/.config/tmux/tmux.conf") {
		t.Fatalf("commit dialog should list the change:\n%s", screen(m))
	}
	m, _ = press(t, m, "enter")
	if m.commit == nil || m.commit.err == "" {
		t.Fatal("empty message must be refused")
	}
	m = typeText(t, m, "adopt tmux")
	m, _ = press(t, m, "enter")
	if got := testutil.Git(t, f.root, "log", "-1", "--format=%s"); got != "adopt tmux" {
		t.Errorf("last commit = %q; note %q", got, m.note)
	}
	if m.git.Ahead != 1 || len(m.git.Changes) != 0 {
		t.Errorf("git pane not refreshed: ahead %d, changes %v", m.git.Ahead, m.git.Changes)
	}
}

func TestPushAsksFirst(t *testing.T) {
	f, remote := newGitTfx(t)
	m := newModel(t, f)
	m, _ = press(t, m, "p")
	if !strings.Contains(m.note, "nothing to push") {
		t.Fatalf("note = %q", m.note)
	}
	write(t, f.c("vim/.gvimrc"), "g")
	testutil.CommitAll(t, f.root, "gvim")
	m, _ = press(t, m, "R", "p")
	if m.confirm == nil || m.confirm.prompt != "push 1 commit(s) to origin/main?" {
		t.Fatalf("push must ask first (V9), confirm = %+v", m.confirm)
	}
	m, _ = press(t, m, "n")
	if got := testutil.Git(t, remote, "log", "-1", "--format=%s"); got != "seed" {
		t.Fatalf("cancelled push reached the remote: %q", got)
	}
	m, _ = press(t, m, "p")
	m, cmd := press(t, m, "y")
	if m.busy != "push" {
		t.Errorf("busy = %q during push", m.busy)
	}
	m = runCmd(t, m, cmd)
	if got := testutil.Git(t, remote, "log", "-1", "--format=%s"); got != "gvim" {
		t.Errorf("remote head = %q, want gvim; note %q", got, m.note)
	}
	if m.busy != "" || m.note != "push done" || m.git.Ahead != 0 {
		t.Errorf("after push: busy %q, note %q, ahead %d", m.busy, m.note, m.git.Ahead)
	}
}

func TestPullRescansAndGuardsBusy(t *testing.T) {
	f, remote := newGitTfx(t)
	other := filepath.Join(f.tmp, "other")
	testutil.Clone(t, remote, other)
	write(t, filepath.Join(other, "Configs", "vim", ".gvimrc"), "g")
	testutil.CommitAll(t, other, "gvim")
	testutil.Git(t, other, "push", "-q")

	m := newModel(t, f)
	m, cmd := press(t, m, "P")
	m, again := press(t, m, "P")
	if again != nil || !strings.Contains(m.note, "pull still running") {
		t.Errorf("second pull while busy: cmd %v, note %q", again != nil, m.note)
	}
	m = runCmd(t, m, cmd)
	found := false
	for _, e := range m.ws.Entries {
		found = found || e.Rel == ".gvimrc"
	}
	if !found || m.note != "pull done" {
		t.Errorf("pulled file not in scan (found %v), note %q", found, m.note)
	}
}
