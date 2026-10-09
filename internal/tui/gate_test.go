package tui

import (
	"strings"
	"testing"

	"github.com/t1mdotcom/lazytuck/internal/testutil"
)

func TestCommitWithSecretNeedsYes(t *testing.T) {
	f, _ := newGitTfx(t)
	write(t, f.c("zsh/.zshrc"), "export SONARQUBE_TOKEN=squ_5316a0fa005ece102a80f4e72f51e9a8a0775a6d\n")
	m := newModel(t, f)
	m, _ = press(t, m, "R", "c")
	m = typeText(t, m, "token")
	m, _ = press(t, m, "enter")
	if m.gate == nil {
		t.Fatal("secret in pending changes must open the gate (V10)")
	}
	if s := screen(m); !strings.Contains(s, "commit blocked") || !strings.Contains(s, "Configs/zsh/.zshrc:1") {
		t.Errorf("gate should name file and line:\n%s", s)
	}
	m = typeText(t, m, "y")
	m, _ = press(t, m, "enter")
	if m.gate == nil || m.gate.err == "" {
		t.Fatal("anything but yes must keep the commit blocked")
	}
	m, _ = press(t, m, "esc")
	if got := testutil.Git(t, f.root, "log", "-1", "--format=%s"); got != "seed" {
		t.Fatalf("cancelled gate committed anyway: %q", got)
	}

	m, _ = press(t, m, "c")
	m = typeText(t, m, "token")
	m, _ = press(t, m, "enter")
	m = typeText(t, m, "yes")
	m, _ = press(t, m, "enter")
	if got := testutil.Git(t, f.root, "log", "-1", "--format=%s"); got != "token" {
		t.Errorf("override with yes should commit, last = %q, note %q", got, m.note)
	}
}

func TestPushScansEveryOutgoingCommit(t *testing.T) {
	f, remote := newGitTfx(t)
	write(t, f.c("zsh/.zshrc"), "password: hunter2\n")
	testutil.CommitAll(t, f.root, "oops")
	write(t, f.c("zsh/.zshrc"), "zsh\n")
	testutil.CommitAll(t, f.root, "remove it again")
	m := newModel(t, f)
	m, _ = press(t, m, "p")
	m, _ = press(t, m, "y")
	if m.gate == nil {
		t.Fatal("push of history containing a secret must open the gate")
	}
	if m.busy != "" {
		t.Fatalf("push started before the gate was answered (busy %q)", m.busy)
	}
	m, _ = press(t, m, "esc")
	if got := testutil.Git(t, remote, "log", "-1", "--format=%s"); got != "seed" {
		t.Errorf("remote received commits despite cancelled gate: %q", got)
	}
	if !strings.Contains(m.note, "push cancelled") {
		t.Errorf("note = %q", m.note)
	}
}
