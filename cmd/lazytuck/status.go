package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/t1mdotcom/lazytuck/internal/state"
	"github.com/t1mdotcom/lazytuck/internal/workspace"
)

func runStatus(repoFlag string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "status: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	ws, err := workspace.Open(repoFlag)
	if err != nil {
		fmt.Fprintln(stderr, "lazytuck:", err)
		return 2
	}

	if *asJSON {
		entries := ws.Entries
		if entries == nil {
			entries = []state.Entry{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries); err != nil {
			fmt.Fprintln(stderr, "lazytuck:", err)
			return 2
		}
	} else {
		printTable(stdout, ws)
	}
	if ws.NeedsAction() {
		return 1
	}
	return 0
}

func printTable(w io.Writer, ws *workspace.Workspace) {
	fmt.Fprintf(w, "repo:   %s\ntarget: %s\n\n", ws.Loc.Repo, ws.Loc.Target)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATE\tGROUP\tPATH\tNOTE")
	counts := map[state.State]int{}
	for _, e := range ws.Entries {
		counts[e.State]++
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.State, e.Group, e.Rel, note(e))
	}
	tw.Flush()

	keys := make([]string, 0, len(counts))
	for s := range counts {
		keys = append(keys, string(s))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", counts[state.State(k)], k))
	}
	fmt.Fprintf(w, "\n%d files: %s\n", len(ws.Entries), strings.Join(parts, ", "))
}

func note(e state.Entry) string {
	switch {
	case e.State == state.Shadowed:
		return "provided by " + e.Winner
	case e.Folded:
		return "via directory symlink"
	case e.InRepo:
		return "target resolves into repo"
	case e.State == state.Foreign || e.State == state.Dangling:
		if e.LinkTo != "" {
			return "-> " + e.LinkTo
		}
	}
	return ""
}
