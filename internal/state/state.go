// Package state classifies every repo file against its target path. It only reads.
package state

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/t1mdotcom/lazytuck/internal/repo"
)

// State of one dotfile.
type State string

const (
	Linked      State = "linked"      // target is (or resolves to) the repo file
	Missing     State = "missing"     // nothing at target
	Same        State = "same"        // real file with identical bytes
	Drift       State = "drift"       // real file with different bytes
	Foreign     State = "foreign"     // symlink elsewhere, directory, or other non-file
	Dangling    State = "dangling"    // symlink to nothing
	Inactive    State = "inactive"    // group target does not match this platform
	Shadowed    State = "shadowed"    // a more specific group provides the same path
	Unsupported State = "unsupported" // Tuckr %ENV path segment
)

// NeedsAction reports whether a state counts as "something to fix" for `status`.
func (s State) NeedsAction() bool {
	switch s {
	case Missing, Same, Drift, Foreign, Dangling:
		return true
	}
	return false
}

// Entry is the classification of one repo file.
type Entry struct {
	Group  string `json:"group"`
	Rel    string `json:"path"`
	Source string `json:"source"`
	Target string `json:"target"`
	State  State  `json:"state"`
	// LinkTo is the raw readlink value when the target is a symlink.
	LinkTo string `json:"link_to,omitempty"`
	// Winner names the group that shadows this one.
	Winner string `json:"winner,omitempty"`
	// Folded: linked through a symlinked parent directory (Tuckr without --only-files).
	Folded bool `json:"folded,omitempty"`
	// InRepo: the target's real path lies inside the repo. Ops must not touch it (V13).
	InRepo bool `json:"in_repo,omitempty"`
}

// Classify returns one Entry per file of every group, sorted by group then path.
func Classify(r *repo.Repo, target string) ([]Entry, error) {
	realRoot, err := filepath.EvalSymlinks(r.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	winners := pickWinners(r)

	var out []Entry
	for _, g := range r.Groups {
		for _, f := range g.Files {
			e := Entry{
				Group:  g.Name,
				Rel:    f.Rel,
				Source: r.Source(g.Name, f.Rel),
				Target: filepath.Join(target, f.Rel),
			}
			switch {
			case !g.Active:
				e.State = Inactive
			case f.Unsupported:
				e.State = Unsupported
			case winners[f.Rel] != g.Name:
				e.State = Shadowed
				e.Winner = winners[f.Rel]
			default:
				if err := classifyTarget(&e, realRoot); err != nil {
					return nil, err
				}
			}
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Rel < out[j].Rel
	})
	return out, nil
}

// pickWinners maps each relative path to the active group that provides it.
// Highest priority wins; on a tie the group later in name order wins (groups are sorted).
func pickWinners(r *repo.Repo) map[string]string {
	type best struct {
		group string
		prio  int
	}
	win := map[string]best{}
	for _, g := range r.Groups {
		if !g.Active {
			continue
		}
		for _, f := range g.Files {
			if f.Unsupported {
				continue
			}
			if b, ok := win[f.Rel]; !ok || g.Priority >= b.prio {
				win[f.Rel] = best{g.Name, g.Priority}
			}
		}
	}
	out := make(map[string]string, len(win))
	for rel, b := range win {
		out[rel] = b.group
	}
	return out
}

func classifyTarget(e *Entry, realRoot string) error {
	fi, err := os.Lstat(e.Target)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		e.State = Missing
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", e.Target, err)
	}
	realSource, err := filepath.EvalSymlinks(e.Source)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", e.Source, err)
	}

	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		e.LinkTo, _ = os.Readlink(e.Target)
		realTarget, err := filepath.EvalSymlinks(e.Target)
		if err != nil {
			e.State = Dangling
			return nil
		}
		e.InRepo = within(realTarget, realRoot)
		if realTarget == realSource {
			e.State = Linked
		} else {
			e.State = Foreign
		}
	case fi.Mode().IsRegular():
		realTarget, err := filepath.EvalSymlinks(e.Target)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", e.Target, err)
		}
		e.InRepo = within(realTarget, realRoot)
		switch {
		case realTarget == realSource:
			e.State, e.Folded = Linked, true
		case e.InRepo:
			e.State = Foreign
		default:
			same, err := sameBytes(e.Target, e.Source)
			if err != nil {
				return err
			}
			if same {
				e.State = Same
			} else {
				e.State = Drift
			}
		}
	default:
		e.State = Foreign
	}
	return nil
}

func within(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func sameBytes(a, b string) (bool, error) {
	fa, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer fb.Close()
	sa, err := fa.Stat()
	if err != nil {
		return false, err
	}
	sb, err := fb.Stat()
	if err != nil {
		return false, err
	}
	if sa.Size() != sb.Size() {
		return false, nil
	}
	bufA, bufB := make([]byte, 32*1024), make([]byte, 32*1024)
	for {
		na, errA := io.ReadFull(fa, bufA)
		nb, errB := io.ReadFull(fb, bufB)
		if !bytes.Equal(bufA[:na], bufB[:nb]) {
			return false, nil
		}
		doneA := errA == io.EOF || errA == io.ErrUnexpectedEOF
		doneB := errB == io.EOF || errB == io.ErrUnexpectedEOF
		if doneA || doneB {
			return doneA && doneB, nil
		}
		if errA != nil {
			return false, errA
		}
		if errB != nil {
			return false, errB
		}
	}
}
