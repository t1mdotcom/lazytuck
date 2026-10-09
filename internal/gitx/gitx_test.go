package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/t1mdotcom/lazytuck/internal/testutil"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setup returns (repo, other): two clones of one bare remote, seeded with Configs/zsh/.zshrc.
func setup(t *testing.T) (string, string) {
	t.Helper()
	testutil.IsolateGit(t)
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	testutil.Remote(t, remote)
	repo, other := filepath.Join(tmp, "repo"), filepath.Join(tmp, "other")
	testutil.Clone(t, remote, repo)
	writeFile(t, filepath.Join(repo, "Configs", "zsh", ".zshrc"), "v1\n")
	writeFile(t, filepath.Join(repo, "Configs", "old name"), "x\n")
	testutil.Seed(t, repo)
	testutil.Clone(t, remote, other)
	return repo, other
}

func TestReadStatus(t *testing.T) {
	repo, _ := setup(t)
	if s, _ := Read(t.TempDir()); s.IsRepo {
		t.Error("plain dir reported as repo")
	}

	writeFile(t, filepath.Join(repo, "Configs", "zsh", ".zshrc"), "v2\n")
	writeFile(t, filepath.Join(repo, "new file.txt"), "n\n")
	testutil.Git(t, repo, "mv", "Configs/old name", "Configs/new name")
	s, err := Read(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !s.IsRepo || s.Branch != "main" || s.Upstream != "origin/main" || s.Ahead != 0 || s.Behind != 0 {
		t.Errorf("header = %+v", s)
	}
	want := []Change{
		{Code: "R.", Path: "Configs/new name"},
		{Code: ".M", Path: "Configs/zsh/.zshrc"},
		{Code: "??", Path: "new file.txt"},
	}
	if !reflect.DeepEqual(s.Changes, want) {
		t.Errorf("changes = %+v\nwant %+v", s.Changes, want)
	}

	if err := Commit(repo, "edit"); err != nil {
		t.Fatal(err)
	}
	if s, _ := Read(repo); s.Ahead != 1 || len(s.Changes) != 0 {
		t.Errorf("after commit: ahead %d, changes %v", s.Ahead, s.Changes)
	}
}

func TestCommit(t *testing.T) {
	repo, _ := setup(t)
	if err := Commit(repo, "  "); err == nil {
		t.Error("empty message accepted")
	}
	if err := Commit(repo, "noop"); !errors.Is(err, ErrNothingToCommit) {
		t.Errorf("clean tree: %v, want ErrNothingToCommit", err)
	}
	writeFile(t, filepath.Join(repo, "Configs", "vim", ".vimrc"), "set nu\n")
	if err := Commit(repo, "add vim"); err != nil {
		t.Fatal(err)
	}
	if msg := testutil.Git(t, repo, "log", "-1", "--format=%s"); msg != "add vim" {
		t.Errorf("last commit = %q", msg)
	}
}

func TestPushPullFastForwardOnly(t *testing.T) {
	repo, other := setup(t)
	writeFile(t, filepath.Join(repo, "Configs", "zsh", ".zshrc"), "v2\n")
	if err := Commit(repo, "v2"); err != nil {
		t.Fatal(err)
	}
	if _, err := Push(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(other); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(other, "Configs", "zsh", ".zshrc")); string(b) != "v2\n" {
		t.Errorf("pulled content = %q", b)
	}

	// Diverge: both sides commit. Pull must refuse instead of creating a merge commit.
	writeFile(t, filepath.Join(repo, "a"), "a\n")
	testutil.CommitAll(t, repo, "a")
	testutil.Git(t, repo, "push", "-q")
	writeFile(t, filepath.Join(other, "b"), "b\n")
	testutil.CommitAll(t, other, "b")
	head := testutil.Git(t, other, "rev-parse", "HEAD")
	if _, err := Pull(other); err == nil {
		t.Error("diverged pull succeeded, want --ff-only failure")
	}
	if got := testutil.Git(t, other, "rev-parse", "HEAD"); got != head {
		t.Error("failed pull moved HEAD")
	}
}
