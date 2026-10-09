package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/t1mdotcom/lazytuck/internal/gitx"
)

// commitFlow is the commit dialog: changed files plus a message input.
type commitFlow struct {
	msg textinput.Model
	err string
}

// gitDoneMsg reports the end of a background push or pull.
type gitDoneMsg struct {
	op  string
	err error
}

func (m *Model) gitReady() bool {
	switch {
	case m.busy != "":
		m.setNote(nil, "%s still running", m.busy)
	case !m.git.IsRepo:
		m.setNote(errors.New("the dotfiles repo is not a git repository"), "")
	default:
		return true
	}
	return false
}

func (m *Model) startCommit() tea.Cmd {
	if !m.gitReady() {
		return nil
	}
	if len(m.git.Changes) == 0 {
		m.setNote(nil, "nothing to commit")
		return nil
	}
	in := textinput.New()
	in.Prompt = "message "
	in.Placeholder = "what changed"
	m.commit = &commitFlow{msg: in}
	return m.commit.msg.Focus()
}

func (m Model) handleCommitKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+c":
		m.commit = nil
		m.setNote(nil, "commit cancelled")
		return m, nil
	case "enter":
		msg := strings.TrimSpace(m.commit.msg.Value())
		if msg == "" {
			m.commit.err = "enter a commit message"
			return m, nil
		}
		m.commit = nil
		cmd := m.guardCommit(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.commit.msg, cmd = m.commit.msg.Update(k)
	return m, cmd
}

func (m *Model) doCommit(msg string) {
	err := gitx.Commit(m.ws.Loc.Repo, msg)
	m.refreshGit()
	if err != nil {
		m.setNote(fmt.Errorf("commit: %w", err), "")
		return
	}
	m.setNote(nil, "committed: %s", msg)
}

// startPush asks before pushing (V9) and pushes in the background.
func (m *Model) startPush() {
	if !m.gitReady() {
		return
	}
	switch {
	case m.git.Upstream == "":
		m.setNote(fmt.Errorf("branch %s has no upstream; set one with `git push -u`", m.git.Branch), "")
		return
	case m.git.Ahead == 0:
		m.setNote(nil, "nothing to push")
		return
	}
	n, up := m.git.Ahead, m.git.Upstream
	m.confirm = &confirmation{
		prompt: fmt.Sprintf("push %d commit(s) to %s?", n, up),
		run: func(m *Model) tea.Cmd {
			return m.guardPush()
		},
	}
}

func (m *Model) startPull() tea.Cmd {
	if !m.gitReady() {
		return nil
	}
	return m.runGit("pull", func(dir string) error { _, err := gitx.Pull(dir); return err })
}

// runGit marks the model busy and runs fn off the UI goroutine.
func (m *Model) runGit(op string, fn func(dir string) error) tea.Cmd {
	m.busy = op
	m.setNote(nil, "%s…", op)
	dir := m.ws.Loc.Repo
	return func() tea.Msg { return gitDoneMsg{op: op, err: fn(dir)} }
}

func (m *Model) gitDone(msg gitDoneMsg) {
	m.busy = ""
	if msg.err != nil {
		m.refreshGit()
		m.setNote(fmt.Errorf("%s: %w", msg.op, msg.err), "")
		return
	}
	if msg.op == "pull" {
		// Pulled files can change link state; rescan everything.
		if err := m.rescan(); err != nil {
			m.setNote(err, "")
			return
		}
		m.ops.Repo = m.ws.Repo
	} else {
		m.refreshGit()
	}
	m.setNote(nil, "%s done", msg.op)
}

func (m Model) commitLines() []string {
	lines := []string{"", stBold.Render(" Commit all changes"), ""}
	for _, c := range m.git.Changes {
		lines = append(lines, "   "+stWarn.Render(fmt.Sprintf("%-2s", c.Code))+" "+c.Path)
	}
	lines = append(lines, "", " "+m.commit.msg.View())
	if m.commit.err != "" {
		lines = append(lines, "", stBad.Render(" ✗ "+m.commit.err))
	}
	return append(lines, "", stDim.Render(" enter commit · esc cancel"))
}
