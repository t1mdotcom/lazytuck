package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestDriftDetailShowsDiff(t *testing.T) {
	m := newModel(t, newTfx(t))
	m, _ = press(t, m, "2", "j") // tmux/.config/tmux/tmux.conf (drift)
	s := screen(m)
	for _, want := range []string{diffHeader, "-repo tmux", "+home tmux"} {
		if !strings.Contains(s, want) {
			t.Errorf("detail lacks %q:\n%s", want, s)
		}
	}
	m, _ = press(t, m, "d")
	if m.focus != paneDetail {
		t.Errorf("d should focus the detail pane, focus = %d", m.focus)
	}
	m, _ = press(t, m, "2", "j", "d") // vim/.vimrc (missing): no diff
	if m.focus != paneFiles || !strings.Contains(m.note, "drifted files") {
		t.Errorf("d on non-drift: focus %d, note %q", m.focus, m.note)
	}
}

func TestTabsInDiffKeepPaneWidths(t *testing.T) {
	f := newTfx(t)
	write(t, f.h(".config/tmux/tmux.conf"), "\tindented\twith\ttabs\n")
	m := newModel(t, f)
	m, _ = press(t, m, "2", "j")
	lines := strings.Split(screen(m), "\n")
	for i, l := range lines[:len(lines)-1] {
		if strings.Contains(l, "\t") {
			t.Fatalf("line %d contains a raw tab: %q", i, l)
		}
		if w := ansi.StringWidth(l); w != 120 {
			t.Fatalf("line %d width %d, want 120: %q", i, w, l)
		}
	}
}
