package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/t1mdotcom/lazytuck/internal/gitx"
	"github.com/t1mdotcom/lazytuck/internal/secrets"
)

// gate blocks a commit or push that contains likely secrets until the user types "yes" (V10).
type gate struct {
	action   string
	findings []secrets.Finding
	input    textinput.Model
	err      string
	proceed  func(m *Model) tea.Cmd
}

// guard scans diff and either runs proceed right away or opens the gate.
func (m *Model) guard(action string, diff string, proceed func(m *Model) tea.Cmd) tea.Cmd {
	findings := secrets.Scan(secrets.ParseDiff(diff))
	if len(findings) == 0 {
		return proceed(m)
	}
	in := textinput.New()
	in.Prompt = "type yes to " + action + " anyway: "
	m.gate = &gate{action: action, findings: findings, input: in, proceed: proceed}
	return m.gate.input.Focus()
}

// guardCommit scans what the commit would record, then commits.
func (m *Model) guardCommit(msg string) tea.Cmd {
	diff, err := gitx.PendingDiff(m.ws.Loc.Repo)
	if err != nil {
		m.setNote(fmt.Errorf("secret scan: %w", err), "")
		return nil
	}
	return m.guard("commit", diff, func(m *Model) tea.Cmd {
		m.doCommit(msg)
		return nil
	})
}

// guardPush scans every outgoing commit, then pushes in the background.
func (m *Model) guardPush() tea.Cmd {
	diff, err := gitx.OutgoingDiff(m.ws.Loc.Repo)
	if err != nil {
		m.setNote(fmt.Errorf("secret scan: %w", err), "")
		return nil
	}
	return m.guard("push", diff, func(m *Model) tea.Cmd {
		return m.runGit("push", func(dir string) error { _, err := gitx.Push(dir); return err })
	})
}

func (m Model) handleGateKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	g := m.gate
	switch k.String() {
	case "esc", "ctrl+c":
		m.gate = nil
		m.setNote(nil, "%s cancelled: possible secrets", g.action)
		return m, nil
	case "enter":
		if strings.TrimSpace(g.input.Value()) != "yes" {
			g.err = fmt.Sprintf("type yes to %s anyway, or esc to cancel", g.action)
			return m, nil
		}
		m.gate = nil
		cmd := g.proceed(&m)
		m.syncDetail()
		return m, cmd
	}
	var cmd tea.Cmd
	g.input, cmd = g.input.Update(k)
	return m, cmd
}

func (m Model) gateLines() []string {
	g := m.gate
	lines := []string{
		"",
		stBad.Render(fmt.Sprintf(" %s blocked: %d possible secret(s)", g.action, len(g.findings))),
		"",
	}
	for _, f := range g.findings {
		lines = append(lines,
			fmt.Sprintf("   %s %s", stWarn.Render(fmt.Sprintf("%s:%d", f.File, f.Num)), stDim.Render(f.Rule)),
			"     "+stDim.Render(f.Excerpt),
		)
	}
	lines = append(lines, "", " "+g.input.View())
	if g.err != "" {
		lines = append(lines, "", stBad.Render(" ✗ "+g.err))
	}
	return append(lines, "", stDim.Render(" Move secrets to an untracked file (e.g. ~/.zshrc.local) instead."), stDim.Render(" esc cancel"))
}
