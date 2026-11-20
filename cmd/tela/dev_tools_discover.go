package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Where a tela dev session's tools socket is, and how clients find it
// (ADR-299). The socket lives in a directory of its own, created private
// (0700) in $XDG_RUNTIME_DIR when it is set, else in the temporary
// directory: the placement of the remote host's socket
// (remote.DefaultAddr), in a directory only this user can enter, so the
// socket is owner-only whatever the umask (on Windows the directory
// inherits the per-user temporary directory's ACL, and AF_UNIX sockets are
// files with an ACL like any other). Each session writes a discovery file,
// <user cache>/tela/dev/<pid>.json, removed at its exit; readers skip, and
// remove, the files of processes that no longer run or whose socket no
// longer answers (a session that was killed).

// devSession is a discovery file.
type devSession struct {
	// Addr is the socket path to dial.
	Addr string `json:"addr"`
	// Pkg is the package tela dev runs, as given on its command line.
	Pkg string `json:"pkg"`
	// PID is the process of tela dev.
	PID int `json:"pid"`
	// Started is when the session started.
	Started time.Time `json:"started"`

	path string // the file it was read from
}

// devSessionDir returns the directory of the discovery files.
func devSessionDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("no user cache directory for the discovery files of tela dev: %w", err)
	}
	return filepath.Join(dir, "tela", "dev"), nil
}

// writeDevSession writes s's discovery file in dir (created private if
// missing), atomically: a reader never sees half of it. It returns the
// file's path.
func writeDevSession(dir string, s devSession) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, strconv.Itoa(s.PID)+".json")
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(append(b, '\n'))
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return path, nil
}

// listDevSessions returns the sessions of dir whose tela dev still runs,
// the oldest first. Files of processes that ended, and unreadable ones,
// are removed.
func listDevSessions(dir string) ([]devSession, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []devSession
	for _, e := range ents {
		name := e.Name()
		pid, err := strconv.Atoi(strings.TrimSuffix(name, ".json"))
		if e.IsDir() || !strings.HasSuffix(name, ".json") || err != nil {
			continue
		}
		path := filepath.Join(dir, name)
		b, err := os.ReadFile(path)
		var s devSession
		if err == nil {
			err = json.Unmarshal(b, &s)
		}
		if err != nil || s.PID != pid || s.Addr == "" || !processAlive(pid) {
			os.Remove(path)
			continue
		}
		s.path = path
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b devSession) int { return a.Started.Compare(b.Started) })
	return out, nil
}

// findDevSession returns the session a client attaches to when it names
// none: the only one that runs and answers. Sessions whose socket does not
// answer (their process was killed, and its PID reused) are removed.
func findDevSession(dir string) (devSession, error) {
	all, err := listDevSessions(dir)
	if err != nil {
		return devSession{}, err
	}
	var live []devSession
	for _, s := range all {
		nc, err := net.DialTimeout("unix", s.Addr, time.Second)
		if err != nil {
			os.Remove(s.path)
			continue
		}
		nc.Close()
		live = append(live, s)
	}
	switch len(live) {
	case 0:
		return devSession{}, errors.New("no tela dev session is running; start one with tela dev [pkg]")
	case 1:
		return live[0], nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d tela dev sessions are running; name one by its address:", len(live))
	for _, s := range live {
		fmt.Fprintf(&b, "\n\t%s (%s, pid %d)", s.Addr, s.Pkg, s.PID)
	}
	return devSession{}, errors.New(b.String())
}

// toolsListen listens on a new socket in a private directory (see above).
// cleanup closes nothing: it removes the directory once the listener is
// closed.
func toolsListen() (ln net.Listener, addr string, cleanup func(), err error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	dir, err := os.MkdirTemp(base, "tela-dev-")
	if err != nil {
		return nil, "", nil, err
	}
	addr = filepath.Join(dir, "tools.sock")
	ln, err = net.Listen("unix", addr)
	if err != nil {
		os.RemoveAll(dir)
		return nil, "", nil, fmt.Errorf("tools socket: %w", err)
	}
	if runtime.GOOS != "windows" {
		os.Chmod(addr, 0o600) // the directory already keeps others out
	}
	return ln, addr, func() { os.RemoveAll(dir) }, nil
}
