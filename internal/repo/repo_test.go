package repo

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mkfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocateOrder(t *testing.T) {
	tmp := t.TempDir()
	env := Env{
		Home:      filepath.Join(tmp, "home"),
		ConfigDir: filepath.Join(tmp, "home", ".config"),
		TuckrHome: filepath.Join(tmp, "tuckrhome"),
	}
	homeRepo := filepath.Join(env.Home, ".dotfiles")
	cfgRepo := filepath.Join(env.ConfigDir, "dotfiles")
	envRepo := filepath.Join(env.TuckrHome, "dotfiles")

	steps := []struct {
		create string
		want   string
	}{
		{homeRepo, homeRepo}, // only ~/.dotfiles
		{cfgRepo, cfgRepo},   // config dir beats ~/.dotfiles
		{envRepo, envRepo},   // $TUCKR_HOME/dotfiles beats both
	}
	for _, s := range steps {
		if err := os.MkdirAll(s.create, 0o755); err != nil {
			t.Fatal(err)
		}
		loc, err := Locate("", env)
		if err != nil {
			t.Fatalf("after creating %s: %v", s.create, err)
		}
		if loc.Repo != s.want {
			t.Errorf("after creating %s: repo = %s, want %s", s.create, loc.Repo, s.want)
		}
	}

	flagRepo := filepath.Join(tmp, "explicit")
	if err := os.MkdirAll(flagRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	loc, err := Locate(flagRepo, env)
	if err != nil || loc.Repo != flagRepo {
		t.Errorf("--repo must win: got %q, %v", loc.Repo, err)
	}
}

func TestLocateNotFoundListsCandidates(t *testing.T) {
	tmp := t.TempDir()
	env := Env{Home: tmp, ConfigDir: filepath.Join(tmp, "cfg"), TuckrHome: filepath.Join(tmp, "th")}
	_, err := Locate("", env)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	for _, c := range []string{"th/dotfiles", "cfg/dotfiles", ".dotfiles"} {
		if !strings.Contains(err.Error(), filepath.Join(tmp, c)) {
			t.Errorf("error %q does not list %s", err, c)
		}
	}
	if _, err := Locate(filepath.Join(tmp, "nope"), env); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing --repo dir: want ErrNotFound, got %v", err)
	}
}

func TestLocateTarget(t *testing.T) {
	tmp := t.TempDir()
	mkfile(t, filepath.Join(tmp, ".dotfiles", "x"))
	env := Env{Home: tmp, ConfigDir: filepath.Join(tmp, "cfg")}
	loc, _ := Locate("", env)
	if loc.Target != tmp {
		t.Errorf("target = %s, want $HOME %s", loc.Target, tmp)
	}
	env.TuckrTarget = "/elsewhere"
	loc, _ = Locate("", env)
	if loc.Target != "/elsewhere" {
		t.Errorf("target = %s, want $TUCKR_TARGET", loc.Target)
	}
}

func TestScanLayout(t *testing.T) {
	root := t.TempDir()
	c := filepath.Join(root, "Configs")
	for _, p := range []string{
		"zsh/.zshrc",
		"vscode_macos/Library/Application Support/Code/User/settings.json",
		"scripts/bin/hello",
		"nvim/.config/nvim/init.lua",
		"nvim/.config/nvim/.DS_Store",
		"nvim/.git/HEAD",
		"prog/%PROGRAM_PATH/config.txt",
	} {
		mkfile(t, filepath.Join(c, p))
	}
	mkfile(t, filepath.Join(root, "Hooks", "zsh", "post.sh"))
	mkfile(t, filepath.Join(c, "stray-file-at-configs-root"))

	r, err := Scan(root, Platform{GOOS: "darwin"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]File{}
	var names []string
	for _, g := range r.Groups {
		names = append(names, g.Name)
		got[g.Name] = g.Files
	}
	if want := []string{"nvim", "prog", "scripts", "vscode_macos", "zsh"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("groups = %v, want %v", names, want)
	}
	want := map[string][]File{
		"nvim":         {{Rel: ".config/nvim/init.lua"}},
		"prog":         {{Rel: "%PROGRAM_PATH/config.txt", Unsupported: true}},
		"scripts":      {{Rel: "bin/hello"}},
		"vscode_macos": {{Rel: "Library/Application Support/Code/User/settings.json"}},
		"zsh":          {{Rel: ".zshrc"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("files =\n%v\nwant\n%v", got, want)
	}
	if h := r.Groups[4].Hooks; !reflect.DeepEqual(h, []string{"post.sh"}) {
		t.Errorf("zsh hooks = %v", h)
	}
	if src := r.Source("vscode_macos", "Library/Application Support/x"); src != filepath.Join(c, "vscode_macos", "Library/Application Support/x") {
		t.Errorf("Source = %s", src)
	}
}

func TestGroupTargets(t *testing.T) {
	darwin := Platform{GOOS: "darwin"}
	linux := Platform{GOOS: "linux"}
	wsl := Platform{GOOS: "linux", WSL: true}
	cases := []struct {
		name, base, target string
		prio               int
		active             map[string]bool // platform label → active
	}{
		{"zsh", "zsh", "", 0, map[string]bool{"darwin": true, "linux": true, "wsl": true}},
		{"zsh_unix", "zsh", "unix", 1, map[string]bool{"darwin": true, "linux": true, "wsl": true}},
		{"zsh_macos", "zsh", "macos", 2, map[string]bool{"darwin": true, "linux": false, "wsl": false}},
		{"zsh_linux", "zsh", "linux", 2, map[string]bool{"darwin": false, "linux": true, "wsl": true}},
		{"zsh_wsl", "zsh", "wsl", 3, map[string]bool{"darwin": false, "linux": false, "wsl": true}},
		{"zsh_windows", "zsh", "windows", 1, map[string]bool{"darwin": false, "linux": false, "wsl": false}},
		{"my_tool", "my_tool", "", 0, map[string]bool{"darwin": true, "linux": true, "wsl": true}},
		{"nvim-vscode", "nvim-vscode", "", 0, map[string]bool{"darwin": true, "linux": true, "wsl": true}},
		{"trailing_", "trailing_", "", 0, map[string]bool{"darwin": true, "linux": true, "wsl": true}},
	}
	plats := map[string]Platform{"darwin": darwin, "linux": linux, "wsl": wsl}
	for _, c := range cases {
		base, target := splitTarget(c.name)
		if base != c.base || target != c.target {
			t.Errorf("%s: split = (%q,%q), want (%q,%q)", c.name, base, target, c.base, c.target)
		}
		if p := priority(target); p != c.prio {
			t.Errorf("%s: priority = %d, want %d", c.name, p, c.prio)
		}
		for label, want := range c.active {
			if got := plats[label].Active(target); got != want {
				t.Errorf("%s on %s: active = %v, want %v", c.name, label, got, want)
			}
		}
	}
}
