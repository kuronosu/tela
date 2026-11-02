package assoc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// writeFileAtomic replaces path with data: a temporary file in the same
// directory, synced, then renamed over it, so readers (GIO's file
// monitors, another process registering at the same time) never see half
// a file. A symbolic link is followed and its target written (resolveLink),
// so a link into a dotfiles repository stays a link; a link that cannot be
// followed is an error, never replaced by a file. An existing file keeps
// its permissions; a new one gets 0644, and missing directories are
// created with 0700 (XDG Base Directory specification).
func writeFileAtomic(path string, data []byte) error {
	path, err := resolveLink(path)
	if err != nil {
		return err
	}
	perm := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chmod(perm); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// maxLinks bounds the links resolveLink follows (Linux's own limit, 40).
const maxLinks = 40

// resolveLink returns the file that writing path should replace: path
// itself, or, when path is a symbolic link, the end of its chain of links,
// each target taken relative to its link's directory, as GLib's
// g_file_set_contents follows them. The end may not exist yet (a dotfiles
// link whose file is not written): it is created there. A link that cannot
// be read, or a loop, is an error, so the caller never renames a file over
// the user's link.
func resolveLink(path string) (string, error) {
	for range maxLinks {
		fi, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return path, nil
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			return path, nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return "", fmt.Errorf("cannot follow the link %s: %w", path, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
	return "", fmt.Errorf("cannot follow the link %s: more than %d links (a loop?)", path, maxLinks)
}

// removeFile removes path; it reports whether there was one.
func removeFile(path string) (bool, error) {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// runTimeout bounds a database update command.
const runTimeout = 30 * time.Second

// run runs a database update command (LinuxDirs.Run, or os/exec). Its
// failure is ignored: the files written are the registration, and the
// caches are rebuilt by the next update (see the package documentation).
func (l *linuxAssoc) run(name string, args ...string) {
	if l.dirs.Run != nil {
		_ = l.dirs.Run(name, args...)
		return
	}
	_ = runCommand(name, args...)
}

// runCommand runs name when it is installed, with its output discarded.
func runCommand(name string, args ...string) error {
	exe, err := exec.LookPath(name)
	if err != nil {
		return nil // not installed: optional
	}
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	return exec.CommandContext(ctx, exe, args...).Run()
}
