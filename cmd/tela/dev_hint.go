package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// Slow starts on Windows (ADR-125, ADR-126).
//
// Microsoft Defender scans every new executable on its first run, before
// any of its code runs, and every build of tela dev is a new executable:
// measured on a Windows 11 VM, about 690 ms for a 29 MB build (12 ms on a
// second run of the same file). A reload of examples/dashboard took 2.2 s
// there, and 1.1 s with the temporary directory and the Go build cache
// excluded from real-time scanning (the build 1.4 → 0.8 s, start → first
// frame 0.7 → 0.18 s). tela dev cannot avoid the scan and never changes
// the system's settings: when a reload's build takes longer than
// slowStartAfter from its start to its connection, it says once per
// session what the likely cause is and which directories to exclude.
//
// The scan happens before the process runs any code, so the time to the
// connection (the child dials tela dev once the runtime, the package
// initializers and the fonts are ready, before the app mounts) has it
// without the app's first render; and only reloads count, since the first
// start of a session is where an app that is slow to initialize shows
// first, scan or not.
//
// So that a few directories cover all of a build's files, on Windows the
// builds of every session live under one directory, %TEMP%\tela-dev (see
// newBuildDir), and the go command's own temporary files (its WORK
// directory, where it compiles and links before copying the executable
// out) go to the session's directory too, unless GOTMPDIR is set.

// slowStartAfter is the start → connection time above which tela dev
// prints the hint on Windows. A build of the dashboard connects in about
// 60 ms without the scan.
const slowStartAfter = 400 * time.Millisecond

// defaultSlowStart is the supervisor's slowStart: slowStartAfter on
// Windows, zero (no hint) elsewhere.
func defaultSlowStart() time.Duration {
	if runtime.GOOS == "windows" {
		return slowStartAfter
	}
	return 0
}

// checkSlowStart prints the hint, once per session, when p is the build
// of a reload and connected (at) more than slowStart after its start.
func (s *supervisor) checkSlowStart(p *proc, at time.Time) {
	sw := s.swap
	if s.cfg.slowStart <= 0 || s.hinted || !s.started || sw == nil || sw.proc != p || sw.started.IsZero() {
		return
	}
	if d := at.Sub(sw.started); d > s.cfg.slowStart {
		s.hinted = true
		s.log.printf("%s", slowStartHint(p.gen, d, s.cfg.exclusions))
	}
}

// newBuildDir creates the directory of this session's builds in the
// temporary directory and returns it with the directory to name in an
// antivirus exclusion.
func newBuildDir() (dir, exclude string, err error) { return newBuildDirIn(os.TempDir()) }

// newBuildDirIn is newBuildDir in temp. On Windows the sessions share one
// parent, temp\tela-dev, so one exclusion covers every session, but only
// while that directory is the user's own: a temporary directory may be
// shared (C:\Temp on managed machines), and a tela-dev made there by
// someone else, or a link to elsewhere, would let them replace a build
// before it runs (checkPrivateDir). Otherwise, and on other systems (a
// fixed directory in a shared /tmp would be its first user's), each
// session is a fresh tela-dev-* directory of temp, and the exclusion to
// name is temp itself on Windows.
func newBuildDirIn(temp string) (dir, exclude string, err error) {
	if runtime.GOOS == "windows" {
		parent := filepath.Join(temp, "tela-dev")
		if err := os.Mkdir(parent, 0o700); err == nil || errors.Is(err, fs.ErrExist) {
			if checkPrivateDir(parent) == nil {
				if dir, err := os.MkdirTemp(parent, "session-"); err == nil {
					return dir, parent, nil
				}
			}
		}
	}
	dir, err = os.MkdirTemp(temp, "tela-dev-")
	if runtime.GOOS == "windows" {
		return dir, temp, err
	}
	return dir, dir, err
}

// buildTempEnv is what the builds add to their environment: on Windows,
// GOTMPDIR in the session's directory, unless the user set GOTMPDIR
// (gotmpdir, from go env).
func buildTempEnv(dir, gotmpdir string) []string {
	if runtime.GOOS != "windows" || gotmpdir != "" {
		return nil
	}
	return []string{"GOTMPDIR=" + dir}
}

// exclusions lists the directories the hint names: the build directory,
// the Go build cache and the user's GOTMPDIR (each when known).
func exclusions(buildDir string, t *devTarget) []string {
	var paths []string
	for _, p := range []string{buildDir, t.gocache, t.gotmpdir} {
		if p != "" && p != "off" && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	return paths
}

// slowStartHint is the text of the hint: gen is the build that started
// slowly, d its start → connection time, paths the directories to
// exclude.
func slowStartHint(gen int, d time.Duration, paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = psQuote(p)
	}
	return fmt.Sprintf(`build %d took %v to start. On Windows this is usually
Microsoft Defender scanning each new build before it runs (every reload is
a new executable). Excluding tela dev's build directory and the Go build
cache from real-time scanning makes reloads much faster; in PowerShell, run
as administrator:

    Add-MpPreference -ExclusionPath %s

tela dev never changes these settings itself; this note is shown once.`,
		gen, ms(d), strings.Join(quoted, ", "))
}

// psQuote quotes s as a PowerShell literal string.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
