package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var diffFlags = []string{"-U0", "--no-color", "--no-ext-diff", "--no-textconv"}

// PendingDiff returns the diff that `Commit` would record (`git add -A` against HEAD),
// built in a throwaway index so the real index is never touched.
func PendingDiff(dir string) (string, error) {
	tmp, err := os.MkdirTemp("", "lazytuck-index-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	env := append(os.Environ(), "GIT_INDEX_FILE="+filepath.Join(tmp, "index"))

	_, headErr := git(dir, "rev-parse", "--verify", "-q", "HEAD")
	if headErr == nil {
		if _, err := gitEnv(dir, env, "read-tree", "HEAD"); err != nil {
			return "", err
		}
	}
	if _, err := gitEnv(dir, env, "add", "-A"); err != nil {
		return "", err
	}
	args := append([]string{"diff", "--cached"}, diffFlags...)
	if headErr == nil {
		args = append(args, "HEAD")
	}
	out, err := gitEnv(dir, env, args...)
	return string(out), err
}

// OutgoingDiff returns the patch of every commit not yet on the upstream, one after
// another. A secret added and removed again in later commits still shows up.
func OutgoingDiff(dir string) (string, error) {
	args := append([]string{"log", "-p", "--format="}, diffFlags...)
	out, err := git(dir, append(args, "@{u}..HEAD")...)
	return string(out), err
}

func gitEnv(dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
