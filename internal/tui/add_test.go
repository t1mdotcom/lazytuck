package tui

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = press(t, m, string(r))
	}
	return m
}

func TestAddFlowCreatesGroupAndLinks(t *testing.T) {
	f := newTfx(t)
	write(t, f.h(".newrc"), "fresh")
	m := newModel(t, f)
	before := snapshot(t, f.tmp)

	m, _ = press(t, m, "n")
	if m.add == nil || !strings.Contains(screen(m), "Add file") {
		t.Fatal("n should open the add dialog")
	}
	m = typeText(t, m, ".new")
	m, _ = press(t, m, "tab") // accept suggestion
	if got := m.add.path.Value(); got != ".newrc" {
		t.Fatalf("tab completion = %q, want .newrc", got)
	}
	m, _ = press(t, m, "enter")
	if m.add.step != 1 {
		t.Fatalf("enter on a valid path should move to the group step; err %q", m.add.err)
	}
	if after := snapshot(t, f.tmp); !reflect.DeepEqual(before, after) {
		t.Fatal("dialog modified files before confirming the group")
	}
	m = typeText(t, m, "shell")
	m, _ = press(t, m, "enter")
	if m.add != nil {
		t.Fatalf("dialog still open: %q", m.add.err)
	}
	if b, _ := os.ReadFile(f.c("shell/.newrc")); string(b) != "fresh" {
		t.Errorf("repo copy = %q", b)
	}
	if linkTo(f.h(".newrc")) != f.c("shell/.newrc") {
		t.Error(".newrc not linked")
	}
	if m.groups[m.gSel].name != "shell" || !strings.Contains(m.note, "added 1 file(s) to shell") {
		t.Errorf("selected %s, note %q", m.groups[m.gSel].name, m.note)
	}
}

func TestAddFlowErrorsStayInDialog(t *testing.T) {
	f := newTfx(t)
	write(t, f.h(".newrc"), "fresh")
	m := newModel(t, f)
	before := snapshot(t, f.tmp)

	m, _ = press(t, m, "n")
	m = typeText(t, m, "does-not-exist")
	m, _ = press(t, m, "enter")
	if m.add == nil || m.add.step != 0 || m.add.err == "" {
		t.Fatalf("missing path must keep the dialog at step 0 with an error: %+v", m.add)
	}
	for range len("does-not-exist") {
		m, _ = press(t, m, "backspace")
	}
	m = typeText(t, m, ".newrc")
	m, _ = press(t, m, "enter")
	m = typeText(t, m, "x_linux")
	m, _ = press(t, m, "enter")
	if m.add == nil || !strings.Contains(m.add.err, "does not apply on this platform") {
		t.Fatalf("inactive group must be refused in the dialog: %+v", m.add)
	}
	m, _ = press(t, m, "esc")
	if m.add != nil || !strings.Contains(m.note, "add cancelled") {
		t.Errorf("esc should cancel, note %q", m.note)
	}
	if after := snapshot(t, f.tmp); !reflect.DeepEqual(before, after) {
		t.Error("refused or cancelled add modified the filesystem")
	}
}
