// Package repo finds a Tuckr dotfiles repository and reads its layout.
package repo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Env holds the inputs of the Tuckr lookup. Tests construct it directly;
// EnvFromOS reads the real process environment.
type Env struct {
	Home        string // $HOME
	ConfigDir   string // macOS ~/Library/Application Support, Linux $XDG_CONFIG_HOME or ~/.config
	TuckrHome   string // $TUCKR_HOME
	TuckrTarget string // $TUCKR_TARGET
}

// EnvFromOS reads Env from the running process.
func EnvFromOS() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, fmt.Errorf("cannot determine home directory: %w", err)
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return Env{}, fmt.Errorf("cannot determine config directory: %w", err)
	}
	return Env{
		Home:        home,
		ConfigDir:   cfg,
		TuckrHome:   os.Getenv("TUCKR_HOME"),
		TuckrTarget: os.Getenv("TUCKR_TARGET"),
	}, nil
}

// Location is a resolved repo plus the directory its files are linked into.
type Location struct {
	Repo   string
	Target string
}

// ErrNotFound is returned when no candidate directory exists.
var ErrNotFound = errors.New("no dotfiles repo found")

// Locate resolves the repo the same way Tuckr's get_dotfiles_path does:
// --repo flag, then $TUCKR_HOME/dotfiles, then <config dir>/dotfiles, then ~/.dotfiles.
// The first candidate that exists as a directory wins.
func Locate(flagRepo string, env Env) (Location, error) {
	target := env.Home
	if env.TuckrTarget != "" {
		target = env.TuckrTarget
	}

	if flagRepo != "" {
		abs, err := filepath.Abs(flagRepo)
		if err != nil {
			return Location{}, err
		}
		if !isDir(abs) {
			return Location{}, fmt.Errorf("%w: --repo %s is not a directory", ErrNotFound, abs)
		}
		return Location{Repo: abs, Target: target}, nil
	}

	var tried []string
	if env.TuckrHome != "" {
		tried = append(tried, filepath.Join(env.TuckrHome, "dotfiles"))
	}
	tried = append(tried,
		filepath.Join(env.ConfigDir, "dotfiles"),
		filepath.Join(env.Home, ".dotfiles"),
	)
	for _, c := range tried {
		if isDir(c) {
			return Location{Repo: c, Target: target}, nil
		}
	}
	return Location{}, fmt.Errorf("%w; tried: %s", ErrNotFound, strings.Join(tried, ", "))
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
