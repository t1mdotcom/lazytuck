package gitx

import (
	"errors"
	"strings"
)

// ErrNothingToCommit is returned by Commit when the working tree is clean.
var ErrNothingToCommit = errors.New("nothing to commit")

// Commit stages every change in dir and commits it with message.
func Commit(dir, message string) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("commit message is empty")
	}
	if _, err := git(dir, "add", "-A"); err != nil {
		return err
	}
	if _, err := git(dir, "diff", "--cached", "--quiet"); err == nil {
		return ErrNothingToCommit
	}
	_, err := git(dir, "commit", "-q", "-m", message)
	return err
}

// Pull fast-forwards the current branch; it never creates a merge commit.
func Pull(dir string) (string, error) {
	out, err := git(dir, "pull", "--ff-only", "-q")
	return strings.TrimSpace(string(out)), err
}

// Push pushes the current branch to its upstream.
func Push(dir string) (string, error) {
	out, err := git(dir, "push", "-q")
	return strings.TrimSpace(string(out)), err
}
