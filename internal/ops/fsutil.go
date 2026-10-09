package ops

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func tmpName(dir string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return filepath.Join(dir, ".lazytuck-"+hex.EncodeToString(b)+".tmp")
}

// placeLink makes target a symlink to source without a window where target is absent:
// the link is created under a temp name next to target and renamed over it (V3, V8).
// Parent directories are created as real directories.
func placeLink(source, target string) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp := tmpName(dir)
	if err := os.Symlink(source, tmp); err != nil {
		return fmt.Errorf("create link: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", target, err)
	}
	return nil
}

// writeFileAtomic copies src's bytes and permission bits to dst via temp file + rename.
func writeFileAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := tmpName(filepath.Dir(dst))
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, fi.Mode().Perm()); err != nil { // umask may have narrowed it
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// copyTree copies a directory recursively, recreating symlinks as symlinks.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch {
		case fi.IsDir():
			return os.MkdirAll(out, fi.Mode().Perm()|0o700)
		case fi.Mode()&fs.ModeSymlink != 0:
			to, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(to, out)
		case fi.Mode().IsRegular():
			return writeFileAtomic(p, out)
		default:
			return nil // sockets, fifos: nothing sensible to back up
		}
	})
}

// moveDir renames a directory, falling back to copy + remove across filesystems.
func moveDir(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := copyTree(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// within reports whether path equals root or lies below it.
func within(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}
