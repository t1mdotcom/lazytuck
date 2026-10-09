package ops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/t1mdotcom/lazytuck/internal/repo"
	"github.com/t1mdotcom/lazytuck/internal/state"
)

var darwin = repo.Platform{GOOS: "darwin"}

type fx struct {
	tmp, root, home, stateHome string
}

func (f fx) c(p string) string { return filepath.Join(f.root, "Configs", p) }
func (f fx) h(p string) string { return filepath.Join(f.home, p) }

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, to, at string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(to, at); err != nil {
		t.Fatal(err)
	}
}

// newFx builds one file per state in group "a", plus inactive/shadowed/unsupported/folded cases.
func newFx(t *testing.T) fx {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir()) // /var → /private/var on macOS
	if err != nil {
		t.Fatal(err)
	}
	f := fx{tmp: tmp, root: filepath.Join(tmp, "repo"), home: filepath.Join(tmp, "home"), stateHome: filepath.Join(tmp, "state")}
	for _, p := range []string{
		"a/.linked", "a/.missing", "a/.same", "a/.drift", "a/.foreign", "a/.dir", "a/.dangling",
		"a/Library/Application Support/x.json",
		"fold/.config/fold/init.lua",
		"b_linux/.inactive", "s/.shadow", "s_macos/.shadow", "p/%P/c.txt",
	} {
		write(t, f.c(p), "repo:"+p)
	}
	symlink(t, f.c("a/.linked"), f.h(".linked"))
	write(t, f.h(".same"), "repo:a/.same")
	write(t, f.h(".drift"), "home version")
	write(t, filepath.Join(tmp, "elsewhere"), "x")
	symlink(t, filepath.Join(tmp, "elsewhere"), f.h(".foreign"))
	write(t, f.h(".dir/inner"), "inner")
	symlink(t, filepath.Join(tmp, "nope"), f.h(".dangling"))
	symlink(t, f.c("fold/.config/fold"), f.h(".config/fold"))
	return f
}

func (f fx) load(t *testing.T) (*repo.Repo, map[string]state.Entry) {
	t.Helper()
	r, err := repo.Scan(f.root, darwin)
	if err != nil {
		t.Fatal(err)
	}
	es, err := state.Classify(r, f.home)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]state.Entry{}
	for _, e := range es {
		m[e.Group+"/"+e.Rel] = e
	}
	return r, m
}

func snapshot(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, _ := os.Lstat(p)
		lt, _ := os.Readlink(p)
		var content string
		if fi.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			content = string(b)
		}
		out = append(out, fmt.Sprintf("%s|%v|%s|%s", p, fi.Mode(), lt, content))
		return nil
	})
	return out
}

// checkInvariants: no temp files anywhere, no symlinks inside Configs (V4).
func checkInvariants(t *testing.T, f fx) {
	t.Helper()
	_ = filepath.WalkDir(f.tmp, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".lazytuck-") {
			t.Errorf("temp file left behind: %s", p)
		}
		if d.Type()&fs.ModeSymlink != 0 && strings.HasPrefix(p, f.c("")) {
			t.Errorf("symlink created inside repo: %s", p)
		}
		return nil
	})
}

func TestOpMatrix(t *testing.T) {
	ok := map[Kind]map[string]bool{
		Link:    {"a/.missing": true, "a/.same": true},
		Unlink:  {"a/.linked": true},
		Adopt:   {"a/.drift": true, "a/.same": true},
		Restore: {"a/.drift": true, "a/.foreign": true, "a/.dir": true, "a/.dangling": true},
	}
	keys := []string{
		"a/.linked", "a/.missing", "a/.same", "a/.drift", "a/.foreign", "a/.dir", "a/.dangling",
		"fold/.config/fold/init.lua", "b_linux/.inactive", "s/.shadow", "p/%P/c.txt",
	}
	for _, k := range []Kind{Link, Unlink, Adopt, Restore} {
		for _, key := range keys {
			t.Run(string(k)+" "+key, func(t *testing.T) {
				f := newFx(t)
				r, entries := f.load(t)
				e, found := entries[key]
				if !found {
					t.Fatalf("fixture lacks %s", key)
				}
				homeBefore, _ := os.ReadFile(e.Target)
				linkBefore, _ := os.Readlink(e.Target)
				before := snapshot(t, f.tmp)

				err := New(r, f.home, f.stateHome, time.Unix(0, 0)).Run(k, e)
				checkInvariants(t, f)

				if !ok[k][key] {
					if err == nil {
						t.Fatalf("%s on %s (%s) succeeded, want error", k, key, e.State)
					}
					if after := snapshot(t, f.tmp); !reflect.DeepEqual(before, after) {
						t.Errorf("failed op modified the filesystem")
					}
					return
				}
				if err != nil {
					t.Fatalf("%s on %s (%s): %v", k, key, e.State, err)
				}

				if k == Unlink {
					if _, err := os.Lstat(e.Target); !errors.Is(err, fs.ErrNotExist) {
						t.Errorf("target still present after unlink")
					}
					if _, err := os.Stat(e.Source); err != nil {
						t.Errorf("unlink touched the repo file: %v", err)
					}
					return
				}
				if lt, _ := os.Readlink(e.Target); lt != e.Source {
					t.Errorf("target links to %q, want absolute %q (V3)", lt, e.Source)
				}
				if _, after := f.load(t); after[key].State != state.Linked {
					t.Errorf("state after %s = %s, want linked", k, after[key].State)
				}
				if k == Adopt {
					if b, _ := os.ReadFile(e.Source); string(b) != string(homeBefore) {
						t.Errorf("repo file = %q, want adopted home bytes %q", b, homeBefore)
					}
				}
				// Every replaced real file / symlink / dir must be in the backup (V1).
				bp := filepath.Join(f.stateHome, "lazytuck", "backup", time.Unix(0, 0).Format("20060102-150405"), e.Rel)
				switch e.State {
				case state.Same, state.Drift:
					if b, err := os.ReadFile(bp); err != nil || string(b) != string(homeBefore) {
						t.Errorf("backup %s = %q, %v; want %q", bp, b, err, homeBefore)
					}
				case state.Dangling:
					if lt, _ := os.Readlink(bp); lt != linkBefore {
						t.Errorf("backup link -> %q, want %q", lt, linkBefore)
					}
				case state.Foreign:
					if e.Rel == ".dir" {
						if b, _ := os.ReadFile(filepath.Join(bp, "inner")); string(b) != "inner" {
							t.Errorf("directory not moved into backup")
						}
					} else if lt, _ := os.Readlink(bp); lt != linkBefore {
						t.Errorf("backup link -> %q, want %q", lt, linkBefore)
					}
				}
			})
		}
	}
}

func TestBackupFailureAbortsOp(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	f := newFx(t)
	r, entries := f.load(t)
	if err := os.MkdirAll(f.stateHome, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(f.stateHome, 0o700)
	before := snapshot(t, f.home)
	for _, key := range []string{"a/.drift", "a/.same"} {
		k := Restore
		if key == "a/.same" {
			k = Link
		}
		if err := New(r, f.home, f.stateHome, time.Now()).Run(k, entries[key]); err == nil {
			t.Errorf("%s on %s succeeded although backup dir is unwritable", k, key)
		}
	}
	if after := snapshot(t, f.home); !reflect.DeepEqual(before, after) {
		t.Error("home changed although backup failed (V1)")
	}
}

func TestBackupNamesDoNotCollide(t *testing.T) {
	f := newFx(t)
	r, entries := f.load(t)
	o := New(r, f.home, f.stateHome, time.Unix(0, 0))
	if err := o.Run(Restore, entries["a/.drift"]); err != nil {
		t.Fatal(err)
	}
	write(t, f.h(".drift2"), "second")
	if err := o.backup(f.h(".drift2"), ".drift"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(o.BackupRoot, ".drift")); string(b) != "home version" {
		t.Errorf("first backup overwritten: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(o.BackupRoot, ".drift.1")); string(b) != "second" {
		t.Errorf("second backup = %q", b)
	}
}

func TestAdd(t *testing.T) {
	f := newFx(t)
	write(t, f.h(".newrc"), "new")
	write(t, f.h(".config/tool/a.conf"), "a")
	write(t, f.h(".config/tool/sub/b.conf"), "b")
	write(t, f.h(".config/tool/.git/HEAD"), "ref")
	write(t, f.h(".config/tool/.DS_Store"), "junk")
	symlink(t, f.h(".newrc"), f.h(".config/tool/link"))
	r, _ := f.load(t)
	o := New(r, f.home, f.stateHome, time.Unix(0, 0))

	added, err := o.Add(f.h(".newrc"), "new", darwin)
	if err != nil || !reflect.DeepEqual(added, []string{".newrc"}) {
		t.Fatalf("add file: %v, %v", added, err)
	}
	added, err = o.Add(f.h(".config/tool"), "tool_macos", darwin)
	if err != nil {
		t.Fatalf("add dir: %v", err)
	}
	if want := []string{".config/tool/a.conf", ".config/tool/sub/b.conf"}; !reflect.DeepEqual(added, want) {
		t.Errorf("added = %v, want %v", added, want)
	}
	for _, c := range []struct{ group, rel, content string }{
		{"new", ".newrc", "new"}, {"tool_macos", ".config/tool/a.conf", "a"}, {"tool_macos", ".config/tool/sub/b.conf", "b"},
	} {
		if b, _ := os.ReadFile(f.c(filepath.Join(c.group, c.rel))); string(b) != c.content {
			t.Errorf("repo %s/%s = %q", c.group, c.rel, b)
		}
		if lt, _ := os.Readlink(f.h(c.rel)); lt != f.c(filepath.Join(c.group, c.rel)) {
			t.Errorf("%s not linked: %q", c.rel, lt)
		}
		if b, _ := os.ReadFile(filepath.Join(o.BackupRoot, c.rel)); string(b) != c.content {
			t.Errorf("no backup of %s", c.rel)
		}
	}
	if _, err := os.Stat(f.c("tool_macos/.config/tool/.git")); !errors.Is(err, fs.ErrNotExist) {
		t.Error(".git dir was added")
	}
	checkInvariants(t, f)
}

func TestAddRefusals(t *testing.T) {
	f := newFx(t)
	write(t, f.h(".plain"), "p")
	write(t, filepath.Join(f.tmp, "outside"), "o")
	r, _ := f.load(t)
	o := New(r, f.home, f.stateHome, time.Unix(0, 0))
	before := snapshot(t, f.tmp)
	cases := []struct {
		name, path, group string
	}{
		{"inactive group", f.h(".plain"), "x_linux"},
		{"already managed", f.h(".linked"), "other"},
		{"outside target", filepath.Join(f.tmp, "outside"), "g"},
		{"symlink", f.h(".linked"), "g"},
		{"bad group name", f.h(".plain"), "a/b"},
		{"target itself", f.home, "g"},
		{"inside folded dir", f.h(".config/fold/init.lua"), "g"},
	}
	for _, c := range cases {
		if _, err := o.Add(c.path, c.group, darwin); err == nil {
			t.Errorf("%s: add succeeded, want error", c.name)
		}
	}
	if after := snapshot(t, f.tmp); !reflect.DeepEqual(before, after) {
		t.Error("refused add modified the filesystem")
	}
}
