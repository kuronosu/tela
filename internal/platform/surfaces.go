package platform

import (
	"fmt"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"unicode"
)

// The desktop surfaces of platform contract v16 (ADR-298): start at login
// (Desktop.SetStartAtLogin, StartsAtLogin), the app's tasks
// (Desktop.SetTasks: Windows jump list tasks, the macOS Dock menu, Linux
// desktop entry actions) and recent documents (Desktop.AddRecentDocument,
// ClearRecentDocuments). See "Desktop surfaces" in the package
// documentation.

// AtLoginFlag is the argument the entries of start at login add to the
// app's command line on Windows (the Run value) and Linux (the autostart
// entry's Exec), so that a start at login can be told from any other.
// This package's init removes it from os.Args before the app's main runs
// (every Tela program imports it), so an app that parses its flags never
// sees it; [StartedAtLogin] reports it.
const AtLoginFlag = "-tela-at-login"

// startedAtLogin is set by init (the flag) and by MarkStartedAtLogin.
var startedAtLogin atomic.Bool

func init() {
	args, at := StripAtLoginFlag(os.Args)
	if at {
		os.Args = args
		startedAtLogin.Store(true)
	}
}

// StripAtLoginFlag returns args (a whole command line, the program's name
// first) without AtLoginFlag, and whether it held it. The program's name
// is never taken for the flag, and arguments after a "--" are left alone
// (they are the app's operands). args itself is not modified.
func StripAtLoginFlag(args []string) ([]string, bool) {
	at := false
	var out []string
	for i, a := range args {
		if i > 0 && a == "--" {
			if out != nil {
				out = append(out, args[i:]...)
			}
			break
		}
		if i > 0 && a == AtLoginFlag {
			if out == nil {
				out = slices.Clone(args[:i])
			}
			at = true
			continue
		}
		if out != nil {
			out = append(out, a)
		}
	}
	if !at {
		return args, false
	}
	return out, true
}

// StartedAtLogin reports whether the desktop started this process because
// the user logged in (Desktop.SetStartAtLogin): its command line had
// AtLoginFlag (Windows, Linux), or the Cocoa backend found the launch
// Apple event marked as a login item's (keyAELaunchedAsLogInItem, known
// once AppKit finished launching the app). Safe from any goroutine.
func StartedAtLogin() bool { return startedAtLogin.Load() }

// MarkStartedAtLogin records that the process was started at login, for a
// backend that learns it from the OS rather than from the command line
// (Cocoa). Safe from any goroutine.
func MarkStartedAtLogin() { startedAtLogin.Store(true) }

// Task is one of the app's tasks ([Desktop.SetTasks]): an entry of the
// Windows taskbar's jump list, the macOS Dock menu, or a Linux launcher's
// menu (the desktop entry's actions), which starts the app with Args.
type Task struct {
	// Title is the entry's label, one line; required.
	Title string
	// Description is a longer text: the jump list item's tooltip and the
	// Dock menu item's; Linux actions have none.
	Description string
	// Args are the command-line arguments the task starts the app with
	// (Windows, Linux), or the Args of the LaunchEvent the Dock menu
	// delivers (macOS).
	Args []string
	// Icon is the entry's picture at one or more sizes ([CheckIcon]);
	// empty shows none (Windows: the app's own icon).
	Icon []image.Image
}

// MaxTasks bounds the tasks of SetTasks: more than Windows shows in a
// jump list (10 by default) or anyone reads in a menu.
const MaxTasks = 32

// CheckTasks panics with an actionable message if tasks cannot be given
// to SetTasks: more than MaxTasks, a Title that is empty or holds control
// characters, a Description with control characters other than a newline,
// Args holding NUL bytes or the AtLoginFlag, or an invalid icon
// ([CheckIcon]). what names the call.
func CheckTasks(what string, tasks []Task) {
	if len(tasks) > MaxTasks {
		panicf("platform: %s: %d tasks; give at most %d (Windows shows about 10)", what, len(tasks), MaxTasks)
	}
	for i, t := range tasks {
		if strings.TrimSpace(t.Title) == "" {
			panicf("platform: %s: tasks[%d] has no Title; it is the label of the jump list or menu entry", what, i)
		}
		if strings.IndexFunc(t.Title, unicode.IsControl) >= 0 {
			panicf("platform: %s: tasks[%d].Title %q has control characters; a title is one line", what, i, t.Title)
		}
		if strings.IndexFunc(t.Description, func(r rune) bool { return r != '\n' && unicode.IsControl(r) }) >= 0 {
			panicf("platform: %s: tasks[%d].Description has control characters", what, i)
		}
		for j, a := range t.Args {
			if strings.ContainsRune(a, 0) {
				panicf("platform: %s: tasks[%d].Args[%d] holds a NUL byte", what, i, j)
			}
			if a == AtLoginFlag {
				panicf("platform: %s: tasks[%d].Args[%d] is %s, which marks a start at login and is removed from the command line", what, i, j, AtLoginFlag)
			}
		}
		CheckIcon(fmt.Sprintf("%s: tasks[%d].Icon", what, i), t.Icon)
	}
}

// CloneTasks returns a copy of tasks whose Args and Icon slices the
// caller may modify (the images themselves are shared).
func CloneTasks(tasks []Task) []Task {
	if tasks == nil {
		return nil
	}
	out := make([]Task, len(tasks))
	for i, t := range tasks {
		t.Args = slices.Clone(t.Args)
		t.Icon = slices.Clone(t.Icon)
		out[i] = t
	}
	return out
}

// CheckRecentPath returns the cleaned path, or an error wrapping
// fs.ErrInvalid when path cannot be given to AddRecentDocument: empty,
// relative or holding a NUL byte. Like a trashed file's, the path is data
// (a file the user opened), so a bad one is an error, not a panic.
func CheckRecentPath(path string) (string, error) {
	switch {
	case path == "" || strings.ContainsRune(path, 0):
		return "", fmt.Errorf("platform: AddRecentDocument(%q): not a path: %w", path, fs.ErrInvalid)
	case !filepath.IsAbs(path):
		return "", fmt.Errorf("platform: AddRecentDocument(%q): the path must be absolute: %w", path, fs.ErrInvalid)
	}
	return filepath.Clean(path), nil
}

// CheckRecent checks path with CheckRecentPath and then that it exists
// (os.Stat). It returns the cleaned path and its information, or an
// error: an invalid path wraps fs.ErrInvalid and a missing one
// fs.ErrNotExist. Backends that have the service call it first.
func CheckRecent(path string) (string, fs.FileInfo, error) {
	clean, err := CheckRecentPath(path)
	if err != nil {
		return "", nil, err
	}
	fi, err := os.Stat(clean)
	if err != nil {
		return "", nil, fmt.Errorf("platform: AddRecentDocument: %w", err)
	}
	return clean, fi, nil
}
