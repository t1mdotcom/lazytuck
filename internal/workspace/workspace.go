// Package workspace ties lookup, scan and classification together for the CLI and the TUI.
package workspace

import (
	"github.com/t1mdotcom/lazytuck/internal/repo"
	"github.com/t1mdotcom/lazytuck/internal/state"
)

// Workspace is a located repo plus its current classification.
type Workspace struct {
	Loc      repo.Location
	Platform repo.Platform
	Repo     *repo.Repo
	Entries  []state.Entry
}

// Open locates the repo (flagRepo overrides the Tuckr lookup) and classifies it.
func Open(flagRepo string) (*Workspace, error) {
	env, err := repo.EnvFromOS()
	if err != nil {
		return nil, err
	}
	loc, err := repo.Locate(flagRepo, env)
	if err != nil {
		return nil, err
	}
	return Load(loc, repo.CurrentPlatform())
}

// Load classifies an already located repo.
func Load(loc repo.Location, p repo.Platform) (*Workspace, error) {
	w := &Workspace{Loc: loc, Platform: p}
	return w, w.Reload()
}

// Reload rescans the repo and reclassifies every file. Read-only.
func (w *Workspace) Reload() error {
	r, err := repo.Scan(w.Loc.Repo, w.Platform)
	if err != nil {
		return err
	}
	entries, err := state.Classify(r, w.Loc.Target)
	if err != nil {
		return err
	}
	w.Repo, w.Entries = r, entries
	return nil
}

// NeedsAction reports whether any entry is in a state that `status` treats as a failure.
func (w *Workspace) NeedsAction() bool {
	for _, e := range w.Entries {
		if e.State.NeedsAction() {
			return true
		}
	}
	return false
}
