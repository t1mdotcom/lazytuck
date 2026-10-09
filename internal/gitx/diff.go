package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// DiffFiles returns a unified diff from a (repo version) to b (home version).
// Identical files yield "". External diff drivers and textconv are disabled so the
// output is plain unified diff regardless of the user's git config.
func DiffFiles(a, b string) (string, error) {
	cmd := exec.Command("git", "diff", "--no-index", "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=repo:", "--dst-prefix=home:", "--", a, b)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var exit *exec.ExitError
	// Exit 1 means "files differ", but git also exits 1 when a path cannot be read;
	// only the latter writes to stderr.
	if errors.As(err, &exit) && exit.ExitCode() == 1 && stderr.Len() == 0 {
		return string(out), nil
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git diff: %s", msg)
	}
	return string(out), nil
}
