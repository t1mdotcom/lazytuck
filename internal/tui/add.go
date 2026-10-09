package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// addFlow is the two-step "add file" dialog: path below the target dir, then group.
type addFlow struct {
	step  int // 0 path, 1 group
	path  textinput.Model
	group textinput.Model
	err   string
}

const maxSuggestions = 200

func (m *Model) startAdd() tea.Cmd {
	p := textinput.New()
	p.Prompt = "path  ~/"
	p.Placeholder = ".config/tool/config.toml"
	p.ShowSuggestions = true
	g := textinput.New()
	g.Prompt = "group "
	g.Placeholder = "existing or new, e.g. tool or tool_macos"
	g.ShowSuggestions = true
	var groups []string
	for _, gr := range m.ws.Repo.Groups {
		if gr.Active {
			groups = append(groups, gr.Name)
		}
	}
	g.SetSuggestions(groups)
	m.add = &addFlow{path: p, group: g}
	m.add.path.SetSuggestions(m.pathSuggestions(""))
	return m.add.path.Focus()
}

// pathSuggestions lists entries of the directory typed so far, relative to the target.
func (m *Model) pathSuggestions(value string) []string {
	dir := value
	if !strings.HasSuffix(dir, "/") {
		dir = filepath.Dir(value)
		if dir == "." {
			dir = ""
		} else {
			dir += "/"
		}
	}
	entries, err := os.ReadDir(filepath.Join(m.ws.Loc.Target, dir))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := dir + e.Name()
		if e.IsDir() {
			name += "/"
		}
		out = append(out, name)
		if len(out) == maxSuggestions {
			break
		}
	}
	sort.Strings(out)
	return out
}

func (m Model) handleAddKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	a := m.add
	switch k.String() {
	case "esc", "ctrl+c":
		m.add = nil
		m.setNote(nil, "add cancelled")
		m.syncDetail()
		return m, nil
	case "enter":
		if a.step == 0 {
			rel := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(a.path.Value()), "~/"), "/")
			if rel == "" {
				a.err = "enter a path below ~"
				return m, nil
			}
			if _, err := os.Lstat(filepath.Join(m.ws.Loc.Target, rel)); err != nil {
				a.err = err.Error()
				return m, nil
			}
			a.err = ""
			a.step = 1
			a.path.Blur()
			return m, a.group.Focus()
		}
		group := strings.TrimSpace(a.group.Value())
		rel := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(a.path.Value()), "~/"), "/")
		added, err := m.ops.Add(filepath.Join(m.ws.Loc.Target, rel), group, m.ws.Platform)
		if err != nil && len(added) == 0 {
			a.err = err.Error()
			return m, nil
		}
		m.add = nil
		m.afterOp()
		if err != nil {
			m.setNote(err, "")
		} else {
			m.setNote(nil, "added %d file(s) to %s", len(added), group)
		}
		m.selectGroup(group)
		m.syncDetail()
		return m, nil
	}

	var cmd tea.Cmd
	if a.step == 0 {
		a.path, cmd = a.path.Update(k)
		a.path.SetSuggestions(m.pathSuggestions(a.path.Value()))
	} else {
		a.group, cmd = a.group.Update(k)
	}
	return m, cmd
}

// selectGroup moves the Groups selection to name, if present.
func (m *Model) selectGroup(name string) {
	for i, g := range m.groups {
		if g.name == name {
			m.gSel, m.fSel, m.fOff = i, 0, 0
			return
		}
	}
}

func (m Model) addLines() []string {
	a := m.add
	lines := []string{
		"",
		stBold.Render(" Add a file or directory to the repo"),
		stDim.Render(" It is copied into Configs/<group>/, backed up, and replaced by a link."),
		"",
		" " + a.path.View(),
	}
	if a.step == 1 {
		lines = append(lines, " "+a.group.View())
	}
	if a.err != "" {
		lines = append(lines, "", stBad.Render(" ✗ "+a.err))
	}
	return append(lines, "", stDim.Render(" tab complete · ↑/↓ cycle suggestions · enter next · esc cancel"))
}
