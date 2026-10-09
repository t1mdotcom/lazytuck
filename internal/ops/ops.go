// Package ops changes link state: link, unlink, adopt, restore and add. Every op that
// replaces or removes something in the target dir backs it up first (V1) and replaces
// files atomically (V8).
package ops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/t1mdotcom/lazytuck/internal/repo"
	"github.com/t1mdotcom/lazytuck/internal/state"
)

// Kind is a per-file operation.
type Kind string

const (
	Link     Kind = "link"
	Unlink   Kind = "unlink"
	Adopt    Kind = "adopt"
	Restore  Kind = "restore"
	Unmanage Kind = "unmanage" // remove from repo, keep a real copy in the target dir
	Delete   Kind = "delete"   // remove from repo and our copy from the target dir
)

var (
	ErrNotApplicable = errors.New("not applicable")
	ErrInRepo        = errors.New("target lies inside the repo (directory symlink); refusing to touch it")
)

// everyState: unmanage and delete accept any state; InRepo is still refused by Check.
var everyState = []state.State{
	state.Linked, state.Missing, state.Same, state.Drift, state.Foreign, state.Dangling,
	state.Inactive, state.Shadowed, state.Unsupported,
}

// allowed lists the states each op accepts (§I.ops).
var allowed = map[Kind][]state.State{
	Link:     {state.Missing, state.Same},
	Unlink:   {state.Linked},
	Adopt:    {state.Drift, state.Same},
	Restore:  {state.Drift, state.Foreign, state.Dangling},
	Unmanage: everyState,
	Delete:   everyState,
}

// Ops performs operations for one repo/target pair. Backups of one Ops value share
// a timestamped directory.
type Ops struct {
	Repo       *repo.Repo
	Target     string
	BackupRoot string // <state home>/lazytuck/backup/<YYYYMMDD-HHMMSS>
}

// StateHome returns $XDG_STATE_HOME, or ~/.local/state.
func StateHome(home string) string {
	if s := os.Getenv("XDG_STATE_HOME"); s != "" {
		return s
	}
	return filepath.Join(home, ".local", "state")
}

// New creates an Ops whose backups go below stateHome.
func New(r *repo.Repo, target, stateHome string, now time.Time) *Ops {
	return &Ops{
		Repo:       r,
		Target:     target,
		BackupRoot: filepath.Join(stateHome, "lazytuck", "backup", now.Format("20060102-150405")),
	}
}

// Check reports whether op k may run on e, without touching anything.
func Check(k Kind, e state.Entry) error {
	if e.InRepo {
		return ErrInRepo
	}
	if k == Unlink && e.Folded {
		return fmt.Errorf("%w: %s is linked through a directory symlink; unlink the directory instead", ErrNotApplicable, e.Rel)
	}
	for _, s := range allowed[k] {
		if e.State == s {
			return nil
		}
	}
	return fmt.Errorf("%w: %s is %s; cannot %s it", ErrNotApplicable, e.Rel, e.State, k)
}

// Run performs op k on e.
func (o *Ops) Run(k Kind, e state.Entry) error {
	if err := Check(k, e); err != nil {
		return err
	}
	switch k {
	case Link:
		return o.link(e)
	case Unlink:
		return o.unlink(e)
	case Adopt:
		return o.adopt(e)
	case Restore:
		return o.restore(e)
	case Unmanage:
		return o.unmanage(e)
	case Delete:
		return o.delete(e)
	}
	return fmt.Errorf("unknown op %q", k)
}

func (o *Ops) link(e state.Entry) error {
	fi, err := os.Lstat(e.Target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if e.State != state.Missing {
			return changed(e)
		}
	case err != nil:
		return err
	case fi.Mode().IsRegular() && e.State == state.Same:
		if err := o.backup(e.Target, e.Rel); err != nil {
			return err
		}
	default:
		return changed(e)
	}
	return placeLink(e.Source, e.Target)
}

func (o *Ops) unlink(e state.Entry) error {
	fi, err := os.Lstat(e.Target)
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		return changed(e)
	}
	resolved, err := filepath.EvalSymlinks(e.Target)
	if err != nil {
		return changed(e)
	}
	configs, err := filepath.EvalSymlinks(o.Repo.ConfigsDir())
	if err != nil {
		return err
	}
	if !within(resolved, configs) { // V2
		return fmt.Errorf("%w: %s does not point into %s", ErrNotApplicable, e.Target, configs)
	}
	return os.Remove(e.Target)
}

func (o *Ops) adopt(e state.Entry) error {
	fi, err := os.Lstat(e.Target)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return changed(e)
	}
	if err := o.backup(e.Target, e.Rel); err != nil {
		return err
	}
	if err := writeFileAtomic(e.Target, e.Source); err != nil {
		return fmt.Errorf("copy into repo: %w", err)
	}
	return placeLink(e.Source, e.Target)
}

func (o *Ops) restore(e state.Entry) error {
	fi, err := os.Lstat(e.Target)
	if err != nil {
		return changed(e)
	}
	if fi.IsDir() {
		// A directory cannot be replaced atomically; move it into the backup first.
		dst, err := o.backupPath(e.Rel)
		if err != nil {
			return err
		}
		if err := moveDir(e.Target, dst); err != nil {
			return fmt.Errorf("backup %s: %w", e.Target, err)
		}
	} else if err := o.backup(e.Target, e.Rel); err != nil {
		return err
	}
	return placeLink(e.Source, e.Target)
}

func changed(e state.Entry) error {
	return fmt.Errorf("%s changed since the last scan; rescan and retry", e.Target)
}

// backupPath returns a free path for rel inside the backup dir.
func (o *Ops) backupPath(rel string) (string, error) {
	p := filepath.Join(o.BackupRoot, rel)
	for i := 1; ; i++ {
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			return p, nil
		}
		p = fmt.Sprintf("%s.%d", filepath.Join(o.BackupRoot, rel), i)
	}
}

// backup copies a file or recreates a symlink below BackupRoot. The original stays in
// place so the caller can replace it atomically afterwards (V1).
func (o *Ops) backup(path, rel string) error {
	dst, err := o.backupPath(rel)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("backup %s: %w", path, err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		to, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if err := os.Symlink(to, dst); err != nil {
			return fmt.Errorf("backup %s: %w", path, err)
		}
		return nil
	}
	if err := writeFileAtomic(path, dst); err != nil {
		return fmt.Errorf("backup %s: %w", path, err)
	}
	return nil
}

// Add moves path (a file, or every regular file below a directory) into
// Configs/<group>/ and links it back. It returns the added relative paths.
func (o *Ops) Add(path, group string, p repo.Platform) ([]string, error) {
	if err := validGroupName(group); err != nil {
		return nil, err
	}
	if !groupActive(group, p) {
		return nil, fmt.Errorf("%w: group %s does not apply on this platform; adding would remove the file from %s", ErrNotApplicable, group, o.Target)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(o.Target, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("%w: %s is not below %s", ErrNotApplicable, abs, o.Target)
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	var rels []string
	switch {
	case fi.Mode().IsRegular():
		rels = []string{rel}
	case fi.IsDir():
		err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if d.Type().IsRegular() && d.Name() != ".DS_Store" {
				r, _ := filepath.Rel(o.Target, p)
				rels = append(rels, r)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: %s is not a regular file or directory", ErrNotApplicable, abs)
	}

	var added []string
	for _, r := range rels {
		if err := o.addFile(r, group); err != nil {
			return added, err
		}
		added = append(added, r)
	}
	return added, nil
}

func (o *Ops) addFile(rel, group string) error {
	for _, g := range o.Repo.Groups {
		if !g.Active {
			continue
		}
		for _, f := range g.Files {
			if f.Rel == rel {
				return fmt.Errorf("%w: %s is already managed by group %s", ErrNotApplicable, rel, g.Name)
			}
		}
	}
	target := filepath.Join(o.Target, rel)
	source := o.Repo.Source(group, rel)
	if _, err := os.Lstat(source); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s already exists in the repo", ErrNotApplicable, source)
	}
	realRoot, err := filepath.EvalSymlinks(o.Repo.Root)
	if err != nil {
		return err
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return err
	}
	if within(filepath.Join(realParent, filepath.Base(target)), realRoot) { // V13
		return ErrInRepo
	}
	if err := o.backup(target, rel); err != nil {
		return err
	}
	if err := writeFileAtomic(target, source); err != nil {
		return fmt.Errorf("copy into repo: %w", err)
	}
	return placeLink(source, target)
}

func validGroupName(g string) error {
	if g == "" || g == "." || g == ".." || strings.ContainsRune(g, filepath.Separator) {
		return fmt.Errorf("%w: invalid group name %q", ErrNotApplicable, g)
	}
	return nil
}

func groupActive(group string, p repo.Platform) bool {
	_, target := repo.SplitTarget(group)
	return p.Active(target)
}
