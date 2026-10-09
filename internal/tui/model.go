// Package tui is the lazygit-style terminal UI.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/t1mdotcom/lazytuck/internal/gitx"
	"github.com/t1mdotcom/lazytuck/internal/ops"
	"github.com/t1mdotcom/lazytuck/internal/state"
	"github.com/t1mdotcom/lazytuck/internal/workspace"
)

type pane int

const (
	paneGroups pane = iota
	paneFiles
	paneDetail
	paneGit
	paneCount
)

var paneTitles = [paneCount]string{"1 Groups", "2 Files", "3 Detail", "4 Git"}

// allGroups is the pseudo-group at the top of the Groups pane that lists every file.
const allGroups = "(all)"

type groupRow struct {
	name   string
	active bool
	target string
	prio   int
	hooks  []string
	counts map[state.State]int
	total  int
}

// needsAction reports whether any file of the group needs attention.
func (g groupRow) needsAction() bool {
	for s, n := range g.counts {
		if n > 0 && s.NeedsAction() {
			return true
		}
	}
	return false
}

// Model is the Bubble Tea model.
type Model struct {
	ws     *workspace.Workspace
	git    gitx.Status
	gitErr error

	focus      pane
	groups     []groupRow
	gSel, gOff int
	fSel, fOff int
	detail     viewport.Model

	width, height int
	note          string
	noteErr       bool
	showHelp      bool

	ops     *ops.Ops
	confirm *confirmation

	// diffs caches `git diff` output per target until the next rescan.
	diffs map[string]string

	add    *addFlow
	commit *commitFlow
	busy   string // running background git op ("push", "pull"), "" when idle
	gate   *gate
}

// New builds the model for an opened workspace. Backups of this session share one
// timestamped directory below $XDG_STATE_HOME (or ~/.local/state).
func New(ws *workspace.Workspace) Model {
	home, _ := os.UserHomeDir()
	m := Model{
		ws:     ws,
		detail: viewport.New(),
		ops:    ops.New(ws.Repo, ws.Loc.Target, ops.StateHome(home), time.Now()),
		diffs:  map[string]string{},
	}
	m.refreshGit()
	m.rebuild()
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

func (m *Model) refreshGit() {
	m.git, m.gitErr = gitx.Read(m.ws.Loc.Repo)
}

// rebuild derives group rows from the workspace and clamps selections.
func (m *Model) rebuild() {
	all := groupRow{name: allGroups, active: true, counts: map[state.State]int{}}
	byName := map[string]*groupRow{}
	rows := []groupRow{}
	for _, g := range m.ws.Repo.Groups {
		rows = append(rows, groupRow{name: g.Name, active: g.Active, target: g.Target, prio: g.Priority, hooks: g.Hooks, counts: map[state.State]int{}})
	}
	for i := range rows {
		byName[rows[i].name] = &rows[i]
	}
	for _, e := range m.ws.Entries {
		all.counts[e.State]++
		all.total++
		if g := byName[e.Group]; g != nil {
			g.counts[e.State]++
			g.total++
		}
	}
	m.groups = append([]groupRow{all}, rows...)
	clear(m.diffs)
	m.gSel = clamp(m.gSel, len(m.groups))
	m.fSel = clamp(m.fSel, len(m.files()))
	m.syncDetail()
}

func clamp(i, n int) int {
	if i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	return i
}

// files returns the entries of the selected group.
func (m Model) files() []state.Entry {
	if len(m.groups) == 0 || m.groups[m.gSel].name == allGroups {
		return m.ws.Entries
	}
	name := m.groups[m.gSel].name
	var out []state.Entry
	for _, e := range m.ws.Entries {
		if e.Group == name {
			out = append(out, e)
		}
	}
	return out
}

// selected returns the selected file entry, if any.
func (m Model) selected() (state.Entry, bool) {
	fs := m.files()
	if len(fs) == 0 {
		return state.Entry{}, false
	}
	return fs[m.fSel], true
}

func (m *Model) setNote(err error, format string, args ...any) {
	if err != nil {
		m.note, m.noteErr = err.Error(), true
		return
	}
	m.note, m.noteErr = fmt.Sprintf(format, args...), false
}

// rescan reloads the workspace and git status (V5: read-only).
func (m *Model) rescan() error {
	if err := m.ws.Reload(); err != nil {
		return err
	}
	m.refreshGit()
	m.rebuild()
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncDetail()
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case gitDoneMsg:
		m.gitDone(msg)
		m.syncDetail()
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	if m.showHelp {
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		m.showHelp = false
		return m, nil
	}
	if m.add != nil {
		return m.handleAddKey(k)
	}
	if m.gate != nil {
		return m.handleGateKey(k)
	}
	if m.commit != nil {
		return m.handleCommitKey(k)
	}
	if m.confirm != nil {
		c := m.confirm
		m.confirm = nil
		var cmd tea.Cmd
		if key == "y" || key == "Y" {
			cmd = c.run(&m)
		} else {
			m.setNote(nil, "cancelled")
		}
		m.syncDetail()
		return m, cmd
	}
	m.note = ""

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "tab":
		m.focus = (m.focus + 1) % paneCount
	case "shift+tab":
		m.focus = (m.focus + paneCount - 1) % paneCount
	case "1", "2", "3", "4":
		m.focus = pane(key[0] - '1')
	case "R":
		if err := m.rescan(); err != nil {
			m.setNote(err, "")
		} else {
			m.setNote(nil, "rescanned: %d files", len(m.ws.Entries))
		}
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "pgdown", "ctrl+d":
		m.move(m.listHeight())
	case "pgup", "ctrl+u":
		m.move(-m.listHeight())
	case "enter":
		if m.focus == paneGroups {
			m.focus = paneFiles
		}
	case "n":
		cmd := m.startAdd()
		return m, cmd
	case "c":
		cmd := m.startCommit()
		return m, cmd
	case "p":
		m.startPush()
	case "P":
		cmd := m.startPull()
		return m, cmd
	case "d":
		if e, ok := m.selected(); ok && e.State == state.Drift {
			m.focus = paneDetail
			m.syncDetail()
			if i := strings.Index(m.detail.GetContent(), diffHeader); i >= 0 {
				m.detail.SetYOffset(strings.Count(m.detail.GetContent()[:i], "\n"))
			}
			return m, nil
		}
		m.setNote(nil, "diff is available for drifted files")
	case " ", "space", "a", "r", "x", "X":
		k := map[string]ops.Kind{" ": ops.Link, "space": ops.Link, "a": ops.Adopt, "r": ops.Restore, "x": ops.Unmanage, "X": ops.Delete}[key]
		switch m.focus {
		case paneFiles:
			m.fileOp(k)
		case paneGroups:
			m.groupOp(k)
		}
	}
	m.syncDetail()
	return m, nil
}

// move changes the selection of the focused pane by delta.
func (m *Model) move(delta int) {
	switch m.focus {
	case paneGroups:
		before := m.gSel
		m.gSel = clamp(m.gSel+delta, len(m.groups))
		if m.gSel != before {
			m.fSel, m.fOff = 0, 0
		}
	case paneFiles:
		m.fSel = clamp(m.fSel+delta, len(m.files()))
	case paneDetail:
		if delta > 0 {
			m.detail.ScrollDown(delta)
		} else {
			m.detail.ScrollUp(-delta)
		}
	}
}
