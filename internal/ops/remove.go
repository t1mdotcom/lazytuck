package ops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/t1mdotcom/lazytuck/internal/state"
)

// unmanage removes e from the repo. A linked target first becomes a real copy so it
// never dangles (V16); every other target stays as it is.
func (o *Ops) unmanage(e state.Entry) error {
	if err := o.backupRepoFile(e); err != nil {
		return err
	}
	if e.State == state.Linked {
		if !isSymlink(e.Target) {
			return changed(e)
		}
		if err := writeFileAtomic(e.Source, e.Target); err != nil {
			return fmt.Errorf("copy to %s: %w", e.Target, err)
		}
	}
	return o.removeFromRepo(e)
}

// delete removes e from the repo and our copy from the target dir: the link, or an
// identical real file (backed up). Foreign, drifted or shadowed targets stay (V16).
func (o *Ops) delete(e state.Entry) error {
	if err := o.backupRepoFile(e); err != nil {
		return err
	}
	switch e.State {
	case state.Linked:
		if !isSymlink(e.Target) {
			return changed(e)
		}
		if err := os.Remove(e.Target); err != nil {
			return err
		}
	case state.Same:
		fi, err := os.Lstat(e.Target)
		if err != nil || !fi.Mode().IsRegular() {
			return changed(e)
		}
		if err := o.backup(e.Target, e.Rel); err != nil {
			return err
		}
		if err := os.Remove(e.Target); err != nil {
			return err
		}
	}
	return o.removeFromRepo(e)
}

func isSymlink(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&fs.ModeSymlink != 0
}

// backupRepoFile copies the repo file to <backup>/repo/Configs/<group>/<rel> (V15).
func (o *Ops) backupRepoFile(e state.Entry) error {
	return o.backup(e.Source, filepath.Join("repo", "Configs", e.Group, e.Rel))
}

// removeFromRepo deletes the repo file, prunes directories it leaves empty (Configs/
// itself stays), and moves Hooks/<group> to the backup once the group is gone.
func (o *Ops) removeFromRepo(e state.Entry) error {
	if err := os.Remove(e.Source); err != nil {
		return fmt.Errorf("remove %s: %w", e.Source, err)
	}
	configs := o.Repo.ConfigsDir()
	for dir := filepath.Dir(e.Source); dir != configs && within(dir, configs); dir = filepath.Dir(dir) {
		if !removeIfEmpty(dir) {
			break
		}
	}
	if _, err := os.Stat(filepath.Join(configs, e.Group)); !errors.Is(err, fs.ErrNotExist) {
		return nil // group still has files
	}
	hooks := filepath.Join(o.Repo.Root, "Hooks", e.Group)
	if _, err := os.Stat(hooks); err != nil {
		return nil
	}
	dst, err := o.backupPath(filepath.Join("repo", "Hooks", e.Group))
	if err != nil {
		return err
	}
	if err := moveDir(hooks, dst); err != nil {
		return fmt.Errorf("move hooks of %s to backup: %w", e.Group, err)
	}
	return nil
}

// removeIfEmpty removes dir if it is empty or holds only Finder metadata.
func removeIfEmpty(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	if len(entries) == 1 && entries[0].Name() == ".DS_Store" {
		if os.Remove(filepath.Join(dir, ".DS_Store")) != nil {
			return false
		}
	} else if len(entries) > 0 {
		return false
	}
	return os.Remove(dir) == nil
}
