package state

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/t1mdotcom/lazytuck/internal/repo"
)

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

type fixture struct {
	root, home string
	repo       *repo.Repo
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	tmp := t.TempDir()
	f := fixture{root: filepath.Join(tmp, "repo"), home: filepath.Join(tmp, "home")}
	c := func(p string) string { return filepath.Join(f.root, "Configs", p) }
	h := func(p string) string { return filepath.Join(f.home, p) }

	for _, p := range []string{
		"a/.linked_abs", "a/.linked_rel", "a/.missing", "a/.same", "a/.drift",
		"a/.foreign", "a/.dir", "a/.dangling", "a/.inrepo",
		"a/Library/Application Support/x.json",
		"fold/.config/fold/init.lua", "fold/.config/fold/extra.txt", "x/.config/fold/extra.txt",
		"other_linux/.other",
		"zsh/.zshrc", "zsh_macos/.zshrc",
		"alpha/.tie", "beta/.tie",
		"prog/%P/c.txt",
	} {
		write(t, c(p), "repo:"+p)
	}

	symlink(t, c("a/.linked_abs"), h(".linked_abs"))
	rel, _ := filepath.Rel(f.home, c("a/.linked_rel"))
	symlink(t, rel, h(".linked_rel"))
	write(t, h(".same"), "repo:a/.same")
	write(t, h(".drift"), "edited in home")
	write(t, filepath.Join(tmp, "elsewhere"), "x")
	symlink(t, filepath.Join(tmp, "elsewhere"), h(".foreign"))
	if err := os.MkdirAll(h(".dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join(tmp, "nope"), h(".dangling"))
	symlink(t, c("zsh/.zshrc"), h(".inrepo")) // points into the repo, but at another file
	symlink(t, c("a/Library/Application Support/x.json"), h("Library/Application Support/x.json"))
	symlink(t, c("fold/.config/fold"), h(".config/fold")) // Tuckr folding: whole dir linked

	r, err := repo.Scan(f.root, repo.Platform{GOOS: "darwin"})
	if err != nil {
		t.Fatal(err)
	}
	f.repo = r
	return f
}

func TestClassifyAllStates(t *testing.T) {
	f := newFixture(t)
	entries, err := Classify(f.repo, f.home)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Group+"/"+e.Rel] = e
	}
	want := map[string]State{
		"a/.linked_abs": Linked, "a/.linked_rel": Linked, "a/.missing": Missing,
		"a/.same": Same, "a/.drift": Drift, "a/.foreign": Foreign, "a/.dir": Foreign,
		"a/.dangling": Dangling, "a/.inrepo": Foreign,
		"a/Library/Application Support/x.json": Linked,
		"fold/.config/fold/init.lua":           Linked,
		"other_linux/.other":                   Inactive,
		"zsh/.zshrc":                           Shadowed, "zsh_macos/.zshrc": Missing,
		"alpha/.tie": Shadowed, "beta/.tie": Missing,
		"prog/%P/c.txt": Unsupported,
		// x wins the tie for extra.txt, but the target path lies in fold's repo dir.
		"fold/.config/fold/extra.txt": Shadowed, "x/.config/fold/extra.txt": Foreign,
	}
	if len(got) != len(want) {
		t.Errorf("got %d entries, want %d", len(got), len(want))
	}
	for k, s := range want {
		if got[k].State != s {
			t.Errorf("%s: state = %s, want %s", k, got[k].State, s)
		}
	}
	if e := got["fold/.config/fold/init.lua"]; !e.Folded || !e.InRepo {
		t.Errorf("folded entry: Folded=%v InRepo=%v, want both true", e.Folded, e.InRepo)
	}
	if e := got["a/.inrepo"]; e.InRepo {
		t.Error("a symlink in $HOME pointing at another repo file lives outside the repo; replacing it is safe")
	}
	if e := got["x/.config/fold/extra.txt"]; !e.InRepo || e.Folded {
		t.Errorf("file inside folded dir belonging to another group: InRepo=%v Folded=%v, want true/false", e.InRepo, e.Folded)
	}
	if e := got["a/.linked_abs"]; e.InRepo {
		t.Error("plain symlink in $HOME must not be flagged InRepo")
	}
	if e := got["a/.drift"]; e.InRepo || e.Folded {
		t.Error("plain home file must not be flagged InRepo/Folded")
	}
	if w := got["zsh/.zshrc"].Winner; w != "zsh_macos" {
		t.Errorf("zsh winner = %q, want zsh_macos (OS-specific beats base)", w)
	}
	if w := got["alpha/.tie"].Winner; w != "beta" {
		t.Errorf("tie winner = %q, want beta (later name wins)", w)
	}
	if lt := got["a/.dangling"].LinkTo; lt == "" {
		t.Error("dangling entry should carry LinkTo")
	}
}

// snapshot records every path under dir with mode, size, mtime and link target.
func snapshot(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		lt, _ := os.Readlink(p)
		out = append(out, fmt.Sprintf("%s %v %d %d %s", p, fi.Mode(), fi.Size(), fi.ModTime().UnixNano(), lt))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestClassifyIsReadOnlyAndDeterministic(t *testing.T) {
	f := newFixture(t)
	parent := filepath.Dir(f.root)
	before := snapshot(t, parent)
	a, err := Classify(f.repo, f.home)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Classify(f.repo, f.home)
	if err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, parent); !reflect.DeepEqual(before, after) {
		t.Error("Classify modified the filesystem")
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("Classify is not deterministic")
	}
	if !sort.SliceIsSorted(a, func(i, j int) bool {
		if a[i].Group != a[j].Group {
			return a[i].Group < a[j].Group
		}
		return a[i].Rel < a[j].Rel
	}) {
		t.Error("entries not sorted by group, path")
	}
}

func TestNeedsAction(t *testing.T) {
	for s, want := range map[State]bool{
		Linked: false, Missing: true, Same: true, Drift: true, Foreign: true,
		Dangling: true, Inactive: false, Shadowed: false, Unsupported: false,
	} {
		if s.NeedsAction() != want {
			t.Errorf("%s.NeedsAction() = %v, want %v", s, !want, want)
		}
	}
}
