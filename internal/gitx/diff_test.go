package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffFiles(t *testing.T) {
	dir := t.TempDir()
	// A user config with an external diff driver must not change the output.
	cfg := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(cfg, []byte("[diff]\n\texternal = /bin/false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)

	a, b, c := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c")
	for p, s := range map[string]string{a: "keep\nrepo line\n", b: "keep\nhome line\n", c: "keep\nrepo line\n"} {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	d, err := DiffFiles(a, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-repo line", "+home line", " keep"} {
		if !strings.Contains(d, want) {
			t.Errorf("diff lacks %q:\n%s", want, d)
		}
	}

	if d, err := DiffFiles(a, c); err != nil || d != "" {
		t.Errorf("identical files: %q, %v", d, err)
	}
	if _, err := DiffFiles(a, filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file should be an error")
	}
}
