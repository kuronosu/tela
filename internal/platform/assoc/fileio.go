package assoc

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// writeFileAtomic replaces path with data: a temporary file in the same
// directory, synced, then renamed over it, so readers (GIO's file
// monitors, another process registering at the same time) never see half
// a file. A symbolic link is followed and its target replaced, so a link
// into a dotfiles repository stays a link. An existing file keeps its
// permissions; a new one gets 0644, and missing directories are created
// with 0700 (XDG Base Directory specification).
func writeFileAtomic(path string, data []byte) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
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
