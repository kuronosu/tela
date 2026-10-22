package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// Slow starts on Windows (ADR-125).
//
// Microsoft Defender scans every new executable on its first run, before
// any of its code runs, and every build of tela dev is a new executable:
// measured on a Windows 11 VM, about 690 ms for a 29 MB build (12 ms on a
// second run of the same file). A reload of examples/dashboard took 2.2 s
// there, and 1.1 s with the temporary directory and the Go build cache
// excluded from real-time scanning (the build 1.4 → 0.8 s, start → first
// frame 0.7 → 0.18 s). tela dev cannot avoid the scan and never changes
// the system's settings: when a build takes longer than slowStartAfter
// from its start to its first frame, it says once per session what the
// likely cause is and which directories to exclude.
//
// So that a few directories cover all of a build's files, on Windows the
// builds of every session live under one directory, %TEMP%\tela-dev (see
// newBuildDir), and the go command's own temporary files (its WORK
// directory, where it compiles and links before copying the executable
// out) go to the session's directory too, unless GOTMPDIR is set.

// slowStartAfter is the start → first frame time above which tela dev
// prints the hint on Windows. A build of the dashboard starts in 90–180 ms
// without the scan.
const slowStartAfter = 400 * time.Millisecond

// defaultSlowStart is the supervisor's slowStart: slowStartAfter on
// Windows, zero (no hint) elsewhere.
func defaultSlowStart() time.Duration {
	if runtime.GOOS == "windows" {
		return slowStartAfter
	}
	return 0
}

// newBuildDir creates the directory of this session's builds and returns
// it with the directory to name in an antivirus exclusion. On Windows the
// sessions share one parent, %TEMP%\tela-dev (the temporary directory is
// the user's own there), so one exclusion covers every session; elsewhere
// each session is a tela-dev-* directory of the temporary directory, since
// a fixed directory in a shared /tmp would belong to the first user who
// made it.
func newBuildDir() (dir, exclude string, err error) {
	if runtime.GOOS != "windows" {
		dir, err = os.MkdirTemp("", "tela-dev-")
		return dir, dir, err
	}
	parent := filepath.Join(os.TempDir(), "tela-dev")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", "", err
	}
	dir, err = os.MkdirTemp(parent, "session-")
	return dir, parent, err
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
// slowly, d its start → first frame time, paths the directories to
// exclude.
func slowStartHint(gen int, d time.Duration, paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = psQuote(p)
	}
	return fmt.Sprintf(`build %d took %v from its start to its first frame. On Windows this is
usually Microsoft Defender scanning each new build before it runs (every
reload is a new executable). Excluding tela dev's build directory and the
Go build cache from real-time scanning makes reloads much faster; in
PowerShell, run as administrator:

    Add-MpPreference -ExclusionPath %s

tela dev never changes these settings itself; this note is shown once.`,
		gen, ms(d), strings.Join(quoted, ", "))
}

// psQuote quotes s as a PowerShell literal string.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
