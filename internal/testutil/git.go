// Package testutil holds helpers shared by tests. It is not used by the binary.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// IsolateGit points git at a temp global config (identity, default branch) and ignores
// the system config, so tests behave the same on every machine.
func IsolateGit(t *testing.T) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	content := "[user]\n\tname = Test\n\temail = test@example.com\n[init]\n\tdefaultBranch = main\n[commit]\n\tgpgsign = false\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// Git runs git in dir and fails the test on error.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// Remote creates a bare repository at path.
func Remote(t *testing.T, path string) {
	t.Helper()
	Git(t, filepath.Dir(path), "init", "-q", "--bare", path)
}

// Clone clones remote into dir.
func Clone(t *testing.T, remote, dir string) {
	t.Helper()
	Git(t, filepath.Dir(dir), "clone", "-q", remote, dir)
}

// Seed commits everything in dir and pushes it as main with upstream tracking.
func Seed(t *testing.T, dir string) {
	t.Helper()
	CommitAll(t, dir, "seed")
	Git(t, dir, "push", "-q", "-u", "origin", "main")
}

// CommitAll stages everything in dir and commits it.
func CommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	Git(t, dir, "add", "-A")
	Git(t, dir, "commit", "-q", "-m", msg)
}
