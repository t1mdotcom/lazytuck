package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/t1mdotcom/lazytuck/internal/gitx"
	"github.com/t1mdotcom/lazytuck/internal/state"
)

const (
	minWidth  = 60
	minHeight = 16
	gitHeight = 7
)

type layout struct {
	leftW, rightW             int
	groupsH, filesH, contentH int
}

func (m Model) layout() layout {
	l := layout{contentH: m.height - 1}
	l.leftW = m.width * 2 / 5
	if l.leftW < 32 {
		l.leftW = min(32, m.width/2)
	}
	l.rightW = m.width - l.leftW
	l.groupsH = min(len(m.groups)+2, (l.contentH-gitHeight)/2)
	l.groupsH = max(l.groupsH, 4)
	l.filesH = l.contentH - gitHeight - l.groupsH
	return l
}

// listHeight is the number of visible rows of the focused list.
func (m Model) listHeight() int {
	l := m.layout()
	if m.focus == paneGroups {
		return max(l.groupsH-2, 1)
	}
	return max(l.filesH-2, 1)
}

func ensureVisible(sel, off, rows int) int {
	if rows < 1 {
		return 0
	}
	if sel < off {
		return sel
	}
	if sel >= off+rows {
		return sel - rows + 1
	}
	return off
}

// syncDetail keeps scroll windows and the detail viewport consistent with the selection.
func (m *Model) syncDetail() {
	if m.width == 0 || m.height == 0 {
		return
	}
	l := m.layout()
	m.gOff = ensureVisible(m.gSel, m.gOff, l.groupsH-2)
	m.fOff = ensureVisible(m.fSel, m.fOff, l.filesH-2)
	m.detail.SetWidth(max(l.rightW-2, 1))
	m.detail.SetHeight(max(l.contentH-2, 1))
	content := m.detailContent()
	if content != m.detail.GetContent() {
		m.detail.SetContent(content)
		m.detail.GotoTop()
	}
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "lazytuck"
	return v
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("terminal too small (%dx%d, need %dx%d)", m.width, m.height, minWidth, minHeight)
	}
	l := m.layout()
	var body string
	if m.showHelp {
		body = box("Help", helpLines(), m.width, l.contentH, true)
	} else {
		left := lipgloss.JoinVertical(lipgloss.Left,
			box(paneTitles[paneGroups], m.groupLines(l.groupsH-2), l.leftW, l.groupsH, m.focus == paneGroups),
			box(m.filesTitle(), m.fileLines(l.filesH-2), l.leftW, l.filesH, m.focus == paneFiles),
			box(paneTitles[paneGit], m.gitLines(), l.leftW, gitHeight, m.focus == paneGit),
		)
		right := box(paneTitles[paneDetail], strings.Split(m.detail.View(), "\n"), l.rightW, l.contentH, m.focus == paneDetail)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return body + "\n" + m.statusBar()
}

// box draws a bordered pane of exactly w×h cells with the title in the top border.
func box(title string, lines []string, w, h int, focused bool) string {
	border, titleStyle := stDim, stDim
	if focused {
		border, titleStyle = stAccent, stAccent.Bold(true)
	}
	iw, ih := max(w-2, 0), max(h-2, 0)
	t := ansi.Truncate(" "+title+" ", max(iw-1, 0), "…")
	top := border.Render("╭─") + titleStyle.Render(t) + border.Render(strings.Repeat("─", max(iw-1-lipgloss.Width(t), 0))+"╮")
	var b strings.Builder
	b.WriteString(top)
	for i := range ih {
		line := ""
		if i < len(lines) {
			line = fit(lines[i], iw)
		} else {
			line = strings.Repeat(" ", iw)
		}
		b.WriteString("\n" + border.Render("│") + line + border.Render("│"))
	}
	b.WriteString("\n" + border.Render("╰"+strings.Repeat("─", iw)+"╯"))
	return b.String()
}

// fit truncates or pads s to exactly w cells.
func fit(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

func (m Model) groupLines(rows int) []string {
	var out []string
	for i := m.gOff; i < len(m.groups) && i < m.gOff+rows; i++ {
		g := m.groups[i]
		var mark string
		switch {
		case !g.active:
			mark = stDim.Render("·")
		case g.needsAction():
			mark = stWarn.Render("!")
		default:
			mark = stOK.Render("✓")
		}
		name := g.name
		if !g.active {
			name = stDim.Render(name)
		}
		count := stDim.Render(fmt.Sprintf("%d/%d", g.counts[state.Linked], g.total))
		out = append(out, m.row(i == m.gSel, paneGroups, mark+" "+name, count))
	}
	return out
}

func (m Model) filesTitle() string {
	if len(m.groups) == 0 {
		return paneTitles[paneFiles]
	}
	return fmt.Sprintf("%s · %s", paneTitles[paneFiles], m.groups[m.gSel].name)
}

func (m Model) fileLines(rows int) []string {
	fs := m.files()
	if len(fs) == 0 {
		return []string{stDim.Render(" no files")}
	}
	showGroup := m.groups[m.gSel].name == allGroups
	var out []string
	for i := m.fOff; i < len(fs) && i < m.fOff+rows; i++ {
		e := fs[i]
		g, st := glyph(e.State)
		right := ""
		if showGroup {
			right = stDim.Render(e.Group)
		}
		out = append(out, m.row(i == m.fSel, paneFiles, st.Render(g)+" "+e.Rel, right))
	}
	return out
}

// row renders a list row; the selected row is highlighted only in the focused pane.
func (m Model) row(selected bool, p pane, left, right string) string {
	w := m.layout().leftW - 2
	gap := max(w-lipgloss.Width(left)-lipgloss.Width(right)-2, 1)
	s := " " + left + strings.Repeat(" ", gap) + right + " "
	if selected {
		if m.focus == p {
			return stSelected.Render(ansi.Strip(fit(s, w)))
		}
		return stBold.Render(">") + fit(s[1:], w-1)
	}
	return s
}

func (m Model) gitLines() []string {
	g := m.git
	switch {
	case m.gitErr != nil:
		return []string{stBad.Render(" " + m.gitErr.Error())}
	case !g.IsRepo:
		return []string{stDim.Render(" not a git repository")}
	}
	head := " " + stAccent.Render(g.Branch)
	if g.Upstream != "" {
		head += stDim.Render(" → "+g.Upstream) + fmt.Sprintf(" ↑%d ↓%d", g.Ahead, g.Behind)
	} else {
		head += stDim.Render(" (no upstream)")
	}
	out := []string{head}
	if len(g.Changes) == 0 {
		return append(out, stDim.Render(" clean"))
	}
	for _, c := range g.Changes {
		out = append(out, " "+stWarn.Render(fmt.Sprintf("%-2s", c.Code))+" "+c.Path)
	}
	return out
}

func (m Model) detailContent() string {
	if m.focus == paneGroups && len(m.groups) > 0 {
		return m.groupDetail(m.groups[m.gSel])
	}
	e, ok := m.selected()
	if !ok {
		return stDim.Render("no file selected")
	}
	return m.fileDetail(e)
}

func kv(k, v string) string { return stDim.Render(fmt.Sprintf("%-9s", k)) + " " + v }

func (m Model) groupDetail(g groupRow) string {
	var lines []string
	if g.name == allGroups {
		lines = append(lines,
			kv("Repo", m.ws.Loc.Repo),
			kv("Target", m.ws.Loc.Target),
			kv("Platform", m.ws.Platform.GOOS),
		)
	} else {
		target := "none (always active)"
		if g.target != "" {
			target = "_" + g.target
		}
		active := stOK.Render("active")
		if !g.active {
			active = stDim.Render("inactive on " + m.ws.Platform.GOOS)
		}
		hooks := stDim.Render("none")
		if len(g.hooks) > 0 {
			hooks = strings.Join(g.hooks, ", ") + stDim.Render("  (run with `tuckr set`)")
		}
		lines = append(lines,
			kv("Group", stBold.Render(g.name)),
			kv("Suffix", target),
			kv("Status", active),
			kv("Priority", fmt.Sprint(g.prio)),
			kv("Hooks", hooks),
		)
	}
	lines = append(lines, "", kv("Files", fmt.Sprint(g.total)))
	states := make([]string, 0, len(g.counts))
	for s := range g.counts {
		states = append(states, string(s))
	}
	sort.Strings(states)
	for _, s := range states {
		gl, st := glyph(state.State(s))
		lines = append(lines, fmt.Sprintf("  %s %-11s %d", st.Render(gl), s, g.counts[state.State(s)]))
	}
	return strings.Join(lines, "\n")
}

func (m Model) fileDetail(e state.Entry) string {
	gl, st := glyph(e.State)
	lines := []string{
		kv("Path", stBold.Render(e.Rel)),
		kv("Group", e.Group),
		kv("State", st.Render(gl+" "+string(e.State))),
		kv("Target", e.Target),
		kv("Source", e.Source),
	}
	if e.LinkTo != "" {
		lines = append(lines, kv("Link to", e.LinkTo))
	}
	if e.Winner != "" {
		lines = append(lines, kv("Shadowed", "by group "+e.Winner))
	}
	if e.Folded {
		lines = append(lines, kv("Note", "linked through a directory symlink"))
	}
	if e.InRepo {
		lines = append(lines, kv("Note", stWarn.Render("target lies inside the repo; lazytuck will not touch it")))
	}
	if keys := fileKeys(e); keys != "" {
		lines = append(lines, "", kv("Keys", keys))
	}
	if e.State == state.Drift {
		lines = append(lines, "", stBold.Render(diffHeader), m.diff(e))
	}
	return strings.Join(lines, "\n")
}

// diffHeader starts the diff section of the file detail; `d` scrolls to it.
const diffHeader = "Diff  repo ↔ home"

// diff renders the colored repo → home diff of a drifted file (cached per scan).
func (m Model) diff(e state.Entry) string {
	if d, ok := m.diffs[e.Target]; ok {
		return d
	}
	raw, err := gitx.DiffFiles(e.Source, e.Target)
	var out string
	if err != nil {
		out = stBad.Render(err.Error())
	} else {
		var lines []string
		for _, l := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
			switch {
			case strings.HasPrefix(l, "diff "), strings.HasPrefix(l, "index "):
				continue // the paths are already shown above
			case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
				lines = append(lines, stDim.Render(l))
			case strings.HasPrefix(l, "@@"):
				lines = append(lines, stAccent.Render(l))
			case strings.HasPrefix(l, "+"):
				lines = append(lines, stOK.Render(l))
			case strings.HasPrefix(l, "-"):
				lines = append(lines, stBad.Render(l))
			default:
				lines = append(lines, l)
			}
		}
		out = strings.Join(lines, "\n")
	}
	m.diffs[e.Target] = out
	return out
}

func helpLines() []string {
	keys := [][2]string{
		{"tab / shift+tab", "next / previous pane"},
		{"1 2 3 4", "focus Groups, Files, Detail, Git"},
		{"j k ↑ ↓", "move selection / scroll detail"},
		{"pgup pgdown", "page"},
		{"enter", "open group's files"},
		{"space", "link / unlink file; on a group: link all or unlink all"},
		{"a", "adopt: copy home version into repo, then link"},
		{"r", "restore: back up home version, link repo version"},
		{"d", "jump to the diff of a drifted file"},
		{"R", "rescan repo and git status"},
		{"?", "toggle this help"},
		{"q ctrl+c", "quit"},
	}
	out := []string{"", "  " + stBold.Render("Keys"), ""}
	for _, k := range keys {
		out = append(out, "  "+stAccent.Render(fmt.Sprintf("%-16s", k[0]))+" "+k[1])
	}
	out = append(out, "", "  "+stBold.Render("States"), "")
	for _, s := range []state.State{state.Linked, state.Missing, state.Same, state.Drift, state.Foreign, state.Dangling, state.Shadowed, state.Inactive, state.Unsupported} {
		g, st := glyph(s)
		out = append(out, "  "+st.Render(g)+" "+string(s))
	}
	return append(out, "", stDim.Render("  any key closes this help"))
}

func (m Model) statusBar() string {
	if m.confirm != nil {
		return fit(stWarn.Render(" "+m.confirm.prompt+" [y/N]"), m.width)
	}
	if m.note != "" {
		if m.noteErr {
			return fit(stBad.Render(" ✗ "+m.note), m.width)
		}
		return fit(stOK.Render(" "+m.note), m.width)
	}
	hints := "j/k move · tab pane · R rescan · ? help · q quit"
	switch m.focus {
	case paneFiles:
		if e, ok := m.selected(); ok {
			if k := fileKeys(e); k != "" {
				hints = k + " · ? help"
			}
		}
	case paneGroups:
		hints = "enter files · space link/unlink all · a adopt all · r restore all · ? help"
	}
	return fit(stDim.Render(" "+hints), m.width)
}
