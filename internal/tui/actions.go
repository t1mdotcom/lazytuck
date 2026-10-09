package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/t1mdotcom/lazytuck/internal/ops"
	"github.com/t1mdotcom/lazytuck/internal/state"
)

// confirmation is a pending y/N question; run executes on "y" and may start a command.
type confirmation struct {
	prompt string
	run    func(m *Model) tea.Cmd
}

// fileOp runs k on the selected file. Restore, unmanage and delete ask first.
func (m *Model) fileOp(k ops.Kind) {
	e, ok := m.selected()
	if !ok {
		return
	}
	if k == ops.Link && e.State == state.Linked {
		k = ops.Unlink
	}
	if err := ops.Check(k, e); err != nil {
		m.setNote(err, "")
		return
	}
	run := func(m *Model) tea.Cmd {
		err := m.ops.Run(k, e)
		m.afterOp()
		if err != nil {
			m.setNote(fmt.Errorf("%s %s: %w", k, e.Rel, err), "")
			return nil
		}
		m.setNote(nil, "%s %s%s", pastTense(k), e.Rel, commitHint(k))
		return nil
	}
	switch k {
	case ops.Restore:
		m.confirm = &confirmation{prompt: fmt.Sprintf("restore %s from repo (current file goes to backup)?", e.Rel), run: run}
	case ops.Unmanage:
		m.confirm = &confirmation{prompt: fmt.Sprintf("remove %s from the repo; %s?", e.Rel, unmanageEffect(e)), run: run}
	case ops.Delete:
		m.confirm = &confirmation{prompt: fmt.Sprintf("delete %s from the repo; %s?", e.Rel, deleteEffect(e)), run: run}
	default:
		run(m)
	}
}

// unmanageEffect / deleteEffect describe what happens in the target dir, for the prompt.
func unmanageEffect(e state.Entry) string {
	if e.State == state.Linked {
		return "the link in ~ becomes a real copy"
	}
	return "~ stays as it is"
}

func deleteEffect(e state.Entry) string {
	switch e.State {
	case state.Linked:
		return "the link in ~ is removed too"
	case state.Same:
		return "the identical copy in ~ goes to the backup"
	}
	return fmt.Sprintf("~ stays as it is (%s)", e.State)
}

// commitHint reminds that repo removals are not committed automatically (V9).
func commitHint(k ops.Kind) string {
	if k == ops.Unmanage || k == ops.Delete {
		return " — press c to commit"
	}
	return ""
}

// groupOp runs k on every applicable file of the selected group, after confirmation.
// For k == Link the direction is decided per group: all linked → unlink, else link.
func (m *Model) groupOp(k ops.Kind) {
	if len(m.groups) == 0 {
		return
	}
	name := m.groups[m.gSel].name
	if name == allGroups && (k == ops.Unmanage || k == ops.Delete) {
		m.setNote(fmt.Errorf("select a single group to remove it from the repo"), "")
		return
	}
	entries := m.files()
	if k == ops.Link && allLinked(entries) {
		k = ops.Unlink
	}
	var todo []state.Entry
	for _, e := range entries {
		if ops.Check(k, e) == nil {
			todo = append(todo, e)
		}
	}
	if len(todo) == 0 {
		m.setNote(nil, "nothing to %s in %s", k, name)
		return
	}
	prompt := fmt.Sprintf("%s %d file(s) in %s?", k, len(todo), name)
	switch k {
	case ops.Unmanage:
		prompt = fmt.Sprintf("remove group %s (%d file(s)) from the repo; linked files become real copies in ~?", name, len(todo))
	case ops.Delete:
		prompt = fmt.Sprintf("delete group %s (%d file(s)) from the repo and its links/identical copies from ~?", name, len(todo))
	}
	m.confirm = &confirmation{
		prompt: prompt,
		run: func(m *Model) tea.Cmd {
			done, firstErr := 0, error(nil)
			for _, e := range todo {
				if err := m.ops.Run(k, e); err != nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("%s %s: %w", k, e.Rel, err)
					}
					continue
				}
				done++
			}
			m.afterOp()
			if firstErr != nil {
				m.setNote(fmt.Errorf("%d/%d done; %w", done, len(todo), firstErr), "")
				return nil
			}
			m.setNote(nil, "%s %d file(s) in %s%s", pastTense(k), done, name, commitHint(k))
			return nil
		},
	}
}

// allLinked reports whether every file an op could touch is already linked.
func allLinked(es []state.Entry) bool {
	found := false
	for _, e := range es {
		switch e.State {
		case state.Inactive, state.Shadowed, state.Unsupported:
			continue
		case state.Linked:
			found = true
		default:
			return false
		}
	}
	return found
}

// afterOp rescans so the UI shows the real state (V8), keeping ops pointed at the fresh scan.
func (m *Model) afterOp() {
	if err := m.rescan(); err != nil {
		m.setNote(err, "")
		return
	}
	m.ops.Repo = m.ws.Repo
}

func pastTense(k ops.Kind) string {
	switch k {
	case ops.Link:
		return "linked"
	case ops.Unlink:
		return "unlinked"
	case ops.Adopt:
		return "adopted"
	case ops.Restore:
		return "restored"
	case ops.Unmanage:
		return "unmanaged"
	case ops.Delete:
		return "deleted"
	}
	return string(k) + "ed"
}

// fileKeys lists the op keys that apply to e, for the detail pane and status bar.
func fileKeys(e state.Entry) string {
	var keys []string
	if ops.Check(ops.Link, e) == nil {
		keys = append(keys, "space link")
	}
	if ops.Check(ops.Unlink, e) == nil {
		keys = append(keys, "space unlink")
	}
	if ops.Check(ops.Adopt, e) == nil {
		keys = append(keys, "a adopt (home → repo)")
	}
	if ops.Check(ops.Restore, e) == nil {
		keys = append(keys, "r restore (repo → home)")
	}
	if ops.Check(ops.Unmanage, e) == nil {
		keys = append(keys, "x unmanage · X delete")
	}
	return strings.Join(keys, " · ")
}
