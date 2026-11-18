package update

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// The tests of the order of update's hooks in tela.Run run the test
// binary again as a small app (the child) on the headless backend, with
// its folders in a temporary directory and an AppImage install: tela.Run
// is the code under test. The child replaces startProgram with a report
// of the moment it is called, on its standard output.
const (
	childEnv    = "TELA_UPDATE_TEST_CHILD"
	appImageEnv = "TELA_UPDATE_TEST_APPIMAGE"
)

// runChild is the main of the child: mode says what the app does.
func runChild(mode string) {
	desktop.SetAppID(testAppID)
	p := os.Getenv(appImageEnv)
	detectEnv = func() sysEnv {
		return env("linux", "/tmp/.mount_Notes/usr/bin/notes", map[string]string{"APPIMAGE": p, "APPDIR": "/tmp/.mount_Notes"})
	}
	app := tela.App{AppID: testAppID}
	switch mode {
	case "rollback":
		// The version on trial starts once too often: it is rolled
		// back and the old one started in its place.
		currentVersion = func() string { return "1.1.0" }
		startProgram = func(exe string) error {
			// Is the instance free for the program started?
			b := headless.New(headless.Options{})
			free, err := b.Desktop().ClaimInstance(testAppID, platform.Launch{})
			b.ReleaseInstances()
			fmt.Printf("START %s free=%v err=%v\n", exe, free, err)
			return nil
		}
		app.Root = tela.C(func() tela.Node {
			fmt.Println("RAN")
			tela.UseMount(func() func() { tela.Quit(); return nil })
			return h.Div()
		})
		app = desktop.SingleInstance(app)
	case "restart":
		// Restart while the app runs, with the crash capture on
		// (TELA_DEV=0) and an end function registered after Run
		// started.
		currentVersion = func() string { return "1.0.0" }
		startProgram = func(exe string) error {
			dir, _ := desktop.LogDir()
			fatal, _ := filepath.Glob(filepath.Join(dir, "fatal-*.tmp"))
			fmt.Printf("RESTART %s fatal=%d\n", exe, len(fatal))
			return nil
		}
		app.Root = tela.C(func() tela.Node {
			tela.UseMount(func() func() {
				core.OnRunEnd(func() { fmt.Println("LATE END") })
				if err := Restart(); err != nil {
					fmt.Println("restart:", err)
				}
				return nil
			})
			return h.Div()
		})
	}
	err := tela.Run(app)
	fmt.Printf("RETURNED %v\n", err)
}

// child runs the app in mode with the AppImage at p and the folders in
// dir, and returns its standard output.
func child(t *testing.T, mode, p, dir string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childEnv+"="+mode, appImageEnv+"="+p, desktop.DirEnv+"="+dir, "TELA_DEV=0", "TELA_BACKEND=headless")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the child (%s): %v\nstdout:\n%s\nstderr:\n%s", mode, err, out.String(), errb.String())
		}
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("the child (%s) did not end; stderr:\n%s", mode, errb.String())
	}
	return out.String()
}

// The rollback at the start runs before package desktop claims the single
// instance, so the old version started in place of this process finds it
// free; the run then ends without opening the app.
func TestRollbackBeforeInstanceClaim(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Notes.AppImage")
	writeFiles(t, dir, map[string]string{"Notes.AppImage": "v1.1", ".Notes.AppImage.update/old": "v1.0"})
	in := &install{format: "appimage", path: p}
	if err := in.saveState(&installState{Pending: &pending{Version: "1.1.0", From: "1.0.0", Starts: HealthyStarts}}); err != nil {
		t.Fatal(err)
	}
	out := child(t, "rollback", p, filepath.Join(dir, "home"))
	if want := fmt.Sprintf("START %s free=true err=<nil>\n", p); !strings.Contains(out, want) {
		t.Errorf("the child's output:\n%s\nwant %q", out, want)
	}
	if strings.Contains(out, "RAN") || !strings.Contains(out, "RETURNED <nil>") {
		t.Errorf("the rolled-back run opened the app or failed:\n%s", out)
	}
	if got := readFile(t, p); got != "v1.0" {
		t.Errorf("the install holds %q after the rollback", got)
	}
}

// Restart's start of the new version comes after every other function of
// the end of Run: desktop's crash capture ended (its file removed) and an
// end function registered while the app ran.
func TestRestartAfterRunEnd(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Notes.AppImage")
	writeFiles(t, dir, map[string]string{"Notes.AppImage": "v1.1"})
	out := child(t, "restart", p, filepath.Join(dir, "home"))
	late, restart := strings.Index(out, "LATE END\n"), strings.Index(out, fmt.Sprintf("RESTART %s fatal=0\n", p))
	if late < 0 || restart < 0 || late > restart {
		t.Errorf("the child's output:\n%s\nwant LATE END, then RESTART %s fatal=0", out, p)
	}
}
