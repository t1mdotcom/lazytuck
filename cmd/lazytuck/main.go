// Command lazytuck is a lazygit-style TUI for Tuckr dotfile repositories.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// version is set at build time via -ldflags "-X main.version=…".
var version = "dev"

const usage = `lazytuck — lazygit-style TUI for Tuckr dotfile repos

Usage:
  lazytuck [--repo <dir>]                  open the TUI
  lazytuck [--repo <dir>] status [--json]  print per-file state
  lazytuck --version

Exit codes for status: 0 all active files linked, 1 something to fix, 2 error.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lazytuck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	repoFlag := fs.String("repo", "", "dotfiles repo directory (overrides Tuckr lookup)")
	showVersion := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	_ = repoFlag
	fmt.Fprint(stderr, usage)
	return 2
}
