package ops

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/t1mdotcom/lazytuck/internal/state"
)

// homeState captures what sits at a target path, for "untouched" checks.
func homeState(p string) string {
	fi, err := os.Lstat(p)
	if err != nil {
		return "absent"
	}
	lt, _ := os.Readlink(p)
	b, _ := os.ReadFile(p)
	return fi.Mode().String() + "|" + lt + "|" + string(b)
}

func TestRemoveMatrix(t *testing.T) {
	keys := []string{
		"a/.linked", "a/.missing", "a/.same", "a/.drift", "a/.foreign", "a/.dir", "a/.dangling",
		"fold/.config/fold/init.lua", "b_linux/.inactive", "s/.shadow", "p/%P/c.txt",
	}
	for _, k := range []Kind{Unmanage, Delete} {
		for _, key := range keys {
			t.Run(string(k)+" "+key, func(t *testing.T) {
				f := newFx(t)
				write(t, filepath.Join(f.root, "Hooks", "b_linux", "post.sh"), "hook")
				r, entries := f.load(t)
				e := entries[key]
				homeBefore := homeState(e.Target)
				before := snapshot(t, f.tmp)
				o := New(r, f.home, f.stateHome, time.Unix(0, 0))

				err := o.Run(k, e)
				checkInvariants(t, f)
				if key == "fold/.config/fold/init.lua" { // V13
					if err == nil {
						t.Fatal("removal through a folded directory must be refused")
					}
					if after := snapshot(t, f.tmp); !reflect.DeepEqual(before, after) {
						t.Error("refused removal modified the filesystem")
					}
					return
				}
				if err != nil {
					t.Fatalf("%s %s (%s): %v", k, key, e.State, err)
				}

				if _, err := os.Lstat(e.Source); !errors.Is(err, fs.ErrNotExist) {
					t.Error("repo file still present")
				}
				if b, err := os.ReadFile(filepath.Join(o.BackupRoot, "repo", "Configs", e.Group, e.Rel)); err != nil || string(b) != "repo:"+key {
					t.Errorf("repo backup = %q, %v (V15)", b, err)
				}

				got := homeState(e.Target)
				switch {
				case k == Unmanage && e.State == state.Linked:
					if fi, _ := os.Lstat(e.Target); fi == nil || !fi.Mode().IsRegular() {
						t.Fatalf("unmanage left %s, want a real file (V16)", got)
					}
					if b, _ := os.ReadFile(e.Target); string(b) != "repo:"+key {
						t.Errorf("real copy = %q", b)
					}
				case k == Delete && (e.State == state.Linked || e.State == state.Same):
					if got != "absent" {
						t.Errorf("delete left %s in home", got)
					}
					if e.State == state.Same {
						if b, _ := os.ReadFile(filepath.Join(o.BackupRoot, e.Rel)); string(b) != "repo:a/.same" {
							t.Errorf("home backup = %q (V1)", b)
						}
					}
				default:
					if got != homeBefore {
						t.Errorf("home target touched: %s → %s", homeBefore, got)
					}
				}
			})
		}
	}
}

func TestRemovePrunesGroupAndHooks(t *testing.T) {
	f := newFx(t)
	write(t, filepath.Join(f.root, "Hooks", "b_linux", "post.sh"), "hook")
	write(t, filepath.Join(f.root, "Configs", "b_linux", ".DS_Store"), "junk")
	r, entries := f.load(t)
	o := New(r, f.home, f.stateHome, time.Unix(0, 0))

	if err := o.Run(Delete, entries["b_linux/.inactive"]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.c("b_linux")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("empty group dir (only .DS_Store) not removed")
	}
	if _, err := os.Stat(filepath.Join(f.root, "Hooks", "b_linux")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("hooks of a removed group stay in the repo")
	}
	if b, _ := os.ReadFile(filepath.Join(o.BackupRoot, "repo", "Hooks", "b_linux", "post.sh")); string(b) != "hook" {
		t.Errorf("hook backup = %q", b)
	}

	if err := o.Run(Unmanage, entries["p/%P/c.txt"]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.c("p")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("nested empty dirs not pruned")
	}

	if err := o.Run(Unmanage, entries["a/.linked"]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.c("a")); err != nil {
		t.Error("group dir with remaining files was removed")
	}
	if _, err := os.Stat(f.c("")); err != nil {
		t.Error("Configs/ itself must stay")
	}
}

func TestRemoveBackupFailureAborts(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	f := newFx(t)
	r, entries := f.load(t)
	if err := os.MkdirAll(f.stateHome, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(f.stateHome, 0o700)
	before := snapshot(t, f.root)
	homeBefore := snapshot(t, f.home)
	for _, k := range []Kind{Unmanage, Delete} {
		if err := New(r, f.home, f.stateHome, time.Now()).Run(k, entries["a/.linked"]); err == nil {
			t.Errorf("%s succeeded without a writable backup", k)
		}
	}
	if !reflect.DeepEqual(before, snapshot(t, f.root)) || !reflect.DeepEqual(homeBefore, snapshot(t, f.home)) {
		t.Error("repo or home changed although the backup failed (V15)")
	}
}
