// Package gitx runs the git CLI against the dotfiles repo.
package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Change is one entry of `git status`.
type Change struct {
	Code string // two-letter XY code, "??" for untracked
	Path string
}

// Status is a snapshot of the repo's git state.
type Status struct {
	IsRepo   bool
	Branch   string
	Upstream string
	Ahead    int
	Behind   int
	Changes  []Change
}

func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("git %s: %s", args[0], msg)
	}
	return out, nil
}

// Read returns the git status of dir. A directory that is not a git repo yields IsRepo=false.
func Read(dir string) (Status, error) {
	if _, err := git(dir, "rev-parse", "--git-dir"); err != nil {
		return Status{}, nil
	}
	out, err := git(dir, "status", "--porcelain=v2", "--branch", "-z")
	if err != nil {
		return Status{IsRepo: true}, err
	}
	return parseStatus(out), nil
}

// parseStatus parses `git status --porcelain=v2 --branch -z`.
func parseStatus(out []byte) Status {
	s := Status{IsRepo: true}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		switch {
		case strings.HasPrefix(f, "# branch.head "):
			s.Branch = strings.TrimPrefix(f, "# branch.head ")
		case strings.HasPrefix(f, "# branch.upstream "):
			s.Upstream = strings.TrimPrefix(f, "# branch.upstream ")
		case strings.HasPrefix(f, "# branch.ab "):
			ab := strings.Fields(strings.TrimPrefix(f, "# branch.ab "))
			if len(ab) == 2 {
				s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(ab[0], "+"))
				s.Behind, _ = strconv.Atoi(strings.TrimPrefix(ab[1], "-"))
			}
		case strings.HasPrefix(f, "1 "):
			// 1 XY sub mH mI mW hH hI path
			if p := strings.SplitN(f, " ", 9); len(p) == 9 {
				s.Changes = append(s.Changes, Change{Code: p[1], Path: p[8]})
			}
		case strings.HasPrefix(f, "2 "):
			// 2 XY sub mH mI mW hH hI Xscore path, followed by origPath as its own field
			if p := strings.SplitN(f, " ", 10); len(p) == 10 {
				s.Changes = append(s.Changes, Change{Code: p[1], Path: p[9]})
			}
			i++ // skip origPath
		case strings.HasPrefix(f, "u "):
			// u XY sub m1 m2 m3 mW h1 h2 h3 path
			if p := strings.SplitN(f, " ", 11); len(p) == 11 {
				s.Changes = append(s.Changes, Change{Code: p[1], Path: p[10]})
			}
		case strings.HasPrefix(f, "? "):
			s.Changes = append(s.Changes, Change{Code: "??", Path: strings.TrimPrefix(f, "? ")})
		}
	}
	return s
}
