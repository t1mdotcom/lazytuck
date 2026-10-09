package repo

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File is one dotfile inside a group, addressed by its path relative to the group dir.
type File struct {
	Rel         string // e.g. ".config/nvim/init.lua", maps 1:1 to <target>/<Rel>
	Unsupported bool   // a path segment starts with '%' (Tuckr env-var path); never touched
}

// Group is one directory under Configs/.
type Group struct {
	Name     string // directory name, e.g. "zsh_macos"
	Base     string // name without target suffix, e.g. "zsh"
	Target   string // "", "macos", "linux", "unix", "wsl", …
	Active   bool   // target applies on the current platform
	Priority int    // Tuckr precedence, higher wins
	Files    []File // sorted by Rel
	Hooks    []string
}

// Repo is a scanned Tuckr dotfiles repository.
type Repo struct {
	Root   string
	Groups []Group // sorted by Name
}

// ConfigsDir is the directory holding the groups.
func (r *Repo) ConfigsDir() string { return filepath.Join(r.Root, "Configs") }

// Source returns the absolute repo path of a file in a group.
func (r *Repo) Source(group, rel string) string {
	return filepath.Join(r.Root, "Configs", group, rel)
}

// skipFile lists names that are never dotfiles: Finder metadata appears as soon as
// someone opens Configs/ in Finder and would otherwise be linked into $HOME.
var skipFile = map[string]bool{".DS_Store": true}

// Scan reads Configs/ and Hooks/ under root. It never writes.
func Scan(root string, p Platform) (*Repo, error) {
	r := &Repo{Root: root}
	entries, err := os.ReadDir(r.ConfigsDir())
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", r.ConfigsDir(), err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		base, target := SplitTarget(e.Name())
		g := Group{
			Name:     e.Name(),
			Base:     base,
			Target:   target,
			Active:   p.Active(target),
			Priority: priority(target),
		}
		if g.Files, err = scanFiles(filepath.Join(r.ConfigsDir(), e.Name())); err != nil {
			return nil, err
		}
		g.Hooks = scanHooks(filepath.Join(root, "Hooks", e.Name()))
		r.Groups = append(r.Groups, g)
	}
	sort.Slice(r.Groups, func(i, j int) bool { return r.Groups[i].Name < r.Groups[j].Name })
	return r, nil
}

func scanFiles(dir string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if skipFile[d.Name()] {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, File{Rel: rel, Unsupported: hasEnvSegment(rel)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	return files, nil
}

func hasEnvSegment(rel string) bool {
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(seg, "%") {
			return true
		}
	}
	return false
}

func scanHooks(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var hooks []string
	for _, e := range entries {
		if !e.IsDir() {
			hooks = append(hooks, e.Name())
		}
	}
	sort.Strings(hooks)
	return hooks
}
