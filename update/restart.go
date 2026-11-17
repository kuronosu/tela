package update

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// The app's run, as the hooks of tela.Run see it.
var run struct {
	mu      sync.Mutex
	running bool
	restart string // the program to start when Run ends
}

// Replaced in tests: start the program at exe with the process's
// arguments in place of this one. On Unix it is an exec, so it returns
// only on failure; on Windows it starts the program and the caller exits.
var (
	startProgram = startProgramOS
	exitProcess  = os.Exit
)

func init() {
	core.OnRunStart(func(rs core.RunStart) bool {
		if rs.DevChild || rs.Reload {
			return true
		}
		if exe := startCheck(); exe != "" {
			// The new version was rolled back: start the old one in
			// place of this process, before any window opens.
			if startProgram(exe) == nil {
				return false
			}
		}
		run.mu.Lock()
		run.running = true
		run.mu.Unlock()
		return true
	})
	core.OnRunEnd(func() {
		run.mu.Lock()
		exe := run.restart
		run.running, run.restart = false, ""
		run.mu.Unlock()
		if exe != "" {
			// After the windows closed and package desktop wrote its
			// settings (its hooks run first). On Windows, Run then
			// returns and the process ends.
			startProgram(exe)
		}
	})
}

// Restart starts the installed version of the app in place of the running
// one: after Apply, the new version. When the app runs (tela.Run), it
// quits it as tela.Quit does, and the new version starts once the windows
// closed and the settings were written; otherwise it starts it at once.
// On Unix the new version replaces the process (exec); on Windows it is
// started and this process exits when Run returns (at once outside Run).
//
// It returns only on failure, or after asking the running app to quit.
// A single-instance app (desktop.SingleInstance) restarted on Windows may
// see the exiting process still holding the instance for a moment.
func Restart() error {
	in, err := findSelf()
	if err != nil {
		return err
	}
	exe := programOf(in)
	run.mu.Lock()
	if run.running {
		run.restart = exe
		run.mu.Unlock()
		tela.Quit()
		return nil
	}
	run.mu.Unlock()
	if err := startProgram(exe); err != nil {
		return err
	}
	exitProcess(0) // Windows: the new version runs; Unix exec'd already
	return nil
}

// programOf returns the program of the installed copy: the marker's exe
// for a zip folder, the bundle's program (the running one's name, else
// the one file in Contents/MacOS).
func programOf(in *install) string {
	switch in.format {
	case "zip":
		if m, err := readMarker(filepath.Join(in.path, MarkerName)); err == nil {
			return filepath.Join(in.path, m.Exe)
		}
	case "app":
		if _, err := os.Stat(in.exe); err != nil {
			dir := filepath.Join(in.path, "Contents", "MacOS")
			if ents, err := os.ReadDir(dir); err == nil && len(ents) == 1 {
				return filepath.Join(dir, ents[0].Name())
			}
		}
	}
	return in.exe
}
