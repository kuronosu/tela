package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// download checks and downloads, failing the test on an error.
func download(t *testing.T, f *fixture) *Update {
	t.Helper()
	u, err := Check(context.Background(), f.config())
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Download(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	return u
}

// starts plays n starts of the running version and returns the program
// the last one would start instead ("" for none).
func starts(t *testing.T, n int) string {
	t.Helper()
	exe := ""
	for range n {
		exe = startCheck()
	}
	return exe
}

func TestApplyAppImageAndRollback(t *testing.T) {
	f := newFixture(t, "1.0.0")
	p := appImage(t, "v1.0")
	f.publish(manifest(release("1.1.0", f.artifact("a", "appimage", []byte("v1.1")))))
	u := download(t, f)
	if err := u.Apply(); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(filepath.Dir(p), ".Notes.AppImage.update", "old")
	if readFile(t, p) != "v1.1" || readFile(t, old) != "v1.0" {
		t.Fatalf("after Apply: %q at the install, %q kept", readFile(t, p), readFile(t, old))
	}
	if st, _ := os.Stat(p); runtime.GOOS != "windows" && st.Mode().Perm()&0o100 == 0 {
		t.Errorf("the new AppImage is not executable: %v", st.Mode())
	}
	// Starts of the old version (not restarted yet) count nothing.
	if exe := starts(t, 3); exe != "" {
		t.Fatalf("the old version's starts rolled back")
	}
	if _, err := Check(context.Background(), f.config()); !errors.Is(err, ErrUpToDate) {
		t.Errorf("Check after Apply, before a restart: %v, want ErrUpToDate (1.1.0 is installed)", err)
	}

	f.setVersion("1.1.0")
	if exe := starts(t, HealthyStarts); exe != "" {
		t.Fatalf("rolled back within %d starts", HealthyStarts)
	}
	if exe := starts(t, 1); exe != p {
		t.Fatalf("start %d: program %q, want the restored %s", HealthyStarts+1, exe, p)
	}
	if readFile(t, p) != "v1.0" {
		t.Errorf("after the rollback the install holds %q", readFile(t, p))
	}
	rb, ok := RolledBack()
	if !ok || rb.Version != "1.1.0" || rb.From != "1.0.0" || !strings.Contains(rb.Reason, "not marked healthy within 2 starts") {
		t.Errorf("RolledBack = %+v, %v", rb, ok)
	}
	f.setVersion("1.0.0")
	if _, err := Check(context.Background(), f.config()); !errors.Is(err, ErrUpToDate) {
		t.Errorf("Check offers the rolled-back version: %v", err)
	}
}

func TestMarkHealthy(t *testing.T) {
	f := newFixture(t, "1.0.0")
	p := appImage(t, "v1.0")
	f.publish(manifest(release("1.1.0", f.artifact("a", "appimage", []byte("v1.1")))))
	if err := download(t, f).Apply(); err != nil {
		t.Fatal(err)
	}
	f.setVersion("1.1.0")
	starts(t, 1)
	if err := MarkHealthy(); err != nil {
		t.Fatal(err)
	}
	if exe := starts(t, 5); exe != "" || readFile(t, p) != "v1.1" {
		t.Errorf("a healthy version was rolled back")
	}
	if _, ok := RolledBack(); ok {
		t.Errorf("RolledBack reports a rollback")
	}
}

// Applying twice before a restart replaces the version that never ran and
// keeps the old copy.
func TestApplyTwiceBeforeRestart(t *testing.T) {
	f := newFixture(t, "1.0.0")
	p := appImage(t, "v1.0")
	f.publish(manifest(release("1.1.0", f.artifact("a", "appimage", []byte("v1.1")))))
	if err := download(t, f).Apply(); err != nil {
		t.Fatal(err)
	}
	m := manifest(release("1.1.0", f.artifact("a", "appimage", []byte("v1.1"))), release("1.2.0", f.artifact("b", "appimage", []byte("v1.2"))))
	m.Published = t0.Add(-30 * time.Minute)
	f.publish(m)
	u := download(t, f)
	if u.Version != "1.2.0" {
		t.Fatalf("offered %s", u.Version)
	}
	if err := u.Apply(); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(filepath.Dir(p), ".Notes.AppImage.update", "old")
	if readFile(t, p) != "v1.2" || readFile(t, old) != "v1.0" {
		t.Errorf("install %q, old %q; want v1.2 and v1.0", readFile(t, p), readFile(t, old))
	}
	f.setVersion("1.2.0")
	if exe := starts(t, HealthyStarts+1); exe != p || readFile(t, p) != "v1.0" {
		t.Errorf("the rollback of 1.2.0 restored %q", readFile(t, p))
	}
}

func TestApplyBundle(t *testing.T) {
	f := newFixture(t, "1.0.0")
	dir := t.TempDir()
	app := filepath.Join(dir, "Notes.app")
	writeFiles(t, app, map[string]string{"Contents/MacOS/notes": "v1.0", "Contents/Info.plist": "plist 1.0"})
	exe := filepath.Join(app, "Contents", "MacOS", "notes")
	useEnv(t, env("darwin", exe, nil))
	files := map[string]string{
		"Notes.app/":                      "",
		"Notes.app/Contents/MacOS/notes":  "v1.1",
		"Notes.app/Contents/Info.plist":   "plist 1.1",
		"Notes.app/Contents/Frameworks/A": "lib",
	}
	// Bundles are macOS's; Windows makes symbolic links only with a
	// privilege, and has no executable bits.
	unix := runtime.GOOS != "windows"
	if unix {
		files["Notes.app/Contents/Frameworks/Link"] = "->A"
	}
	art := zipOfFiles(t, files)
	f.publish(manifest(release("1.1.0", f.artifact("notes.zip", "app", art))))
	if err := download(t, f).Apply(); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, ".Notes.app.update", "old")
	if readFile(t, exe) != "v1.1" || readFile(t, filepath.Join(old, "Contents", "MacOS", "notes")) != "v1.0" {
		t.Fatalf("after Apply: %q installed, old %q", readFile(t, exe), readFile(t, filepath.Join(old, "Contents/MacOS/notes")))
	}
	if unix {
		if st, _ := os.Stat(exe); st.Mode().Perm()&0o100 == 0 {
			t.Errorf("the program lost its executable bit: %v", st.Mode())
		}
		if l, err := os.Readlink(filepath.Join(app, "Contents", "Frameworks", "Link")); err != nil || l != "A" {
			t.Errorf("link %q, %v", l, err)
		}
	}
	f.setVersion("1.1.0")
	if got := starts(t, HealthyStarts+1); got != exe {
		t.Fatalf("rollback starts %q, want %s", got, exe)
	}
	if readFile(t, exe) != "v1.0" || readFile(t, filepath.Join(app, "Contents", "Info.plist")) != "plist 1.0" {
		t.Errorf("after the rollback: %q", readFile(t, exe))
	}
}

// A rollback starts the restored bundle's program, also when the new
// version's program had another name.
func TestRollbackBundleRenamedProgram(t *testing.T) {
	f := newFixture(t, "1.0.0")
	dir := t.TempDir()
	app := filepath.Join(dir, "Notes.app")
	writeFiles(t, app, map[string]string{"Contents/MacOS/notes": "v1.0"})
	useEnv(t, env("darwin", filepath.Join(app, "Contents", "MacOS", "notes"), nil))
	art := zipOfFiles(t, map[string]string{"Notes.app/Contents/MacOS/notes-next": "v1.1"})
	f.publish(manifest(release("1.1.0", f.artifact("notes.zip", "app", art))))
	if err := download(t, f).Apply(); err != nil {
		t.Fatal(err)
	}
	// The new version runs, from its own program.
	f.setVersion("1.1.0")
	useEnv(t, env("darwin", filepath.Join(app, "Contents", "MacOS", "notes-next"), nil))
	want := filepath.Join(app, "Contents", "MacOS", "notes")
	if got := starts(t, HealthyStarts+1); got != want {
		t.Fatalf("rollback starts %q, want %s", got, want)
	}
	if readFile(t, want) != "v1.0" {
		t.Errorf("after the rollback: %q", readFile(t, want))
	}
}

func TestApplyZipFolder(t *testing.T) {
	f := newFixture(t, "1.0.0")
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		MarkerName:  marker("1.0.0", "notes.exe", "old.dll", MarkerName),
		"notes.exe": "v1.0",
		"old.dll":   "old",
		"data.txt":  "the user's", // not the package's: stays
	})
	exe := filepath.Join(dir, "notes.exe")
	useEnv(t, env("windows", exe, nil))
	// The running executable, played by this inode.
	running := inode(t, exe)
	art := zipOfFiles(t, map[string]string{
		"Notes/" + MarkerName: marker("1.1.0", "notes.exe", "new.dll", MarkerName),
		"Notes/notes.exe":     "v1.1",
		"Notes/new.dll":       "new",
	})
	f.publish(manifest(release("1.1.0", f.artifact("notes.zip", "zip", art))))
	if err := download(t, f).Apply(); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, ".update", "old")
	if readFile(t, exe) != "v1.1" || readFile(t, filepath.Join(dir, "new.dll")) != "new" || readFile(t, filepath.Join(dir, "data.txt")) != "the user's" {
		t.Errorf("after Apply the folder holds the wrong files")
	}
	if _, err := os.Stat(filepath.Join(dir, "old.dll")); err == nil {
		t.Errorf("old.dll is still in the folder")
	}
	// The running executable was renamed, not copied or overwritten.
	if !os.SameFile(inode(t, filepath.Join(old, "notes.exe")), running) || readFile(t, filepath.Join(old, "old.dll")) != "old" {
		t.Errorf("the old copy is not the renamed files")
	}
	if got := programOf(&install{format: "zip", path: dir, exe: exe}); got != exe {
		t.Errorf("Restart would start %s", got)
	}

	f.setVersion("1.1.0")
	if got := starts(t, HealthyStarts+1); got != exe {
		t.Fatalf("rollback starts %q", got)
	}
	if readFile(t, exe) != "v1.0" || readFile(t, filepath.Join(dir, "old.dll")) != "old" || readFile(t, filepath.Join(dir, "data.txt")) != "the user's" {
		t.Errorf("after the rollback the folder holds the wrong files")
	}
	if _, err := os.Stat(filepath.Join(dir, "new.dll")); err == nil {
		t.Errorf("new.dll survived the rollback")
	}
	if !os.SameFile(inode(t, exe), running) {
		t.Errorf("the rollback did not put the old executable back")
	}
}

// zipInstall makes a zip folder install of version 1.0.0 (notes.exe,
// old.dll, the marker, and the user's data.txt) and makes detection see
// it; it returns the folder and the program.
func zipInstall(t *testing.T) (dir, exe string) {
	t.Helper()
	dir = t.TempDir()
	writeFiles(t, dir, map[string]string{
		MarkerName:  marker("1.0.0", "notes.exe", "old.dll", MarkerName),
		"notes.exe": "v1.0",
		"old.dll":   "old",
		"data.txt":  "the user's",
	})
	exe = filepath.Join(dir, "notes.exe")
	useEnv(t, env("windows", exe, nil))
	return dir, exe
}

// zipRelease serves a zip artifact of version v (notes.exe holding
// "v"+v, new.dll, the marker) and returns its release.
func zipRelease(f *fixture, v string) Release {
	art := zipOfFiles(f.t, map[string]string{
		"Notes/" + MarkerName: marker(v, "notes.exe", "new.dll", MarkerName),
		"Notes/notes.exe":     "v" + v,
		"Notes/new.dll":       "new " + v,
	})
	return release(v, f.artifact("notes-"+v+".zip", "zip", art))
}

// failRenames makes the renames of moveEntries fail for the sources and
// destinations that fail reports, as a sharing violation on Windows does.
func failRenames(t *testing.T, fail func(src, dst string) bool) {
	setVar(t, &rename, func(src, dst string) error {
		if fail(src, dst) {
			return &os.LinkError{Op: "rename", Old: src, New: dst, Err: errors.New("injected sharing violation")}
		}
		return os.Rename(src, dst)
	})
}

// isEntry reports whether p is the entry name of the folder dir.
func isEntry(p, dir, name string) bool { return p == filepath.Join(dir, name) }

func pendingOf(t *testing.T, dir string) *pending {
	t.Helper()
	st, err := (&install{format: "zip", path: dir}).loadState()
	if err != nil {
		t.Fatal(err)
	}
	return st.Pending
}

// A rename that fails while the new entries move in puts the folder back
// as it was: the old version stays installed, nothing is recorded, and a
// later Apply works.
func TestApplyZipFolderInterrupted(t *testing.T) {
	f := newFixture(t, "1.0.0")
	dir, exe := zipInstall(t)
	running := inode(t, exe)
	f.publish(manifest(zipRelease(f, "1.1.0")))
	u := download(t, f)
	next := filepath.Join(dir, ".update", "new")
	failRenames(t, func(src, _ string) bool { return isEntry(src, next, "new.dll") })
	if err := u.Apply(); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("Apply = %v, want the injected failure", err)
	}
	if readFile(t, exe) != "v1.0" || !os.SameFile(inode(t, exe), running) || readFile(t, filepath.Join(dir, "old.dll")) != "old" ||
		!strings.Contains(readFile(t, filepath.Join(dir, MarkerName)), `"1.0.0"`) || readFile(t, filepath.Join(dir, "data.txt")) != "the user's" {
		t.Errorf("the folder was not put back as it was")
	}
	if _, err := os.Stat(filepath.Join(dir, "new.dll")); err == nil {
		t.Errorf("new.dll is in the folder")
	}
	if p := pendingOf(t, dir); p != nil {
		t.Errorf("a failed Apply recorded %+v", p)
	}
	setVar(t, &rename, os.Rename)
	if err := u.Apply(); err != nil {
		t.Fatalf("Apply after the failure: %v", err)
	}
	if readFile(t, exe) != "v1.1.0" {
		t.Errorf("the second Apply installed %q", readFile(t, exe))
	}
}

// Replacing a version on trial that never ran: a failed move puts the
// trial's entries back, and the old copy stays.
func TestApplyZipFolderInterruptedReplacing(t *testing.T) {
	f := newFixture(t, "1.0.0")
	dir, exe := zipInstall(t)
	f.publish(manifest(zipRelease(f, "1.1.0")))
	if err := download(t, f).Apply(); err != nil {
		t.Fatal(err)
	}
	m := manifest(zipRelease(f, "1.1.0"), zipRelease(f, "1.2.0"))
	m.Published = t0.Add(-30 * time.Minute)
	f.publish(m)
	u := download(t, f)
	next := filepath.Join(dir, ".update", "new")
	failRenames(t, func(src, _ string) bool { return isEntry(src, next, "new.dll") })
	if err := u.Apply(); err == nil {
		t.Fatal("Apply succeeded")
	}
	if readFile(t, exe) != "v1.1.0" || readFile(t, filepath.Join(dir, "new.dll")) != "new 1.1.0" {
		t.Errorf("the trial's entries were not put back: %q", readFile(t, exe))
	}
	if p := pendingOf(t, dir); p == nil || p.Version != "1.1.0" {
		t.Errorf("pending %+v, want 1.1.0's", p)
	}
	if readFile(t, filepath.Join(dir, ".update", "old", "notes.exe")) != "v1.0" {
		t.Errorf("the old copy is gone")
	}
}

// When the folder cannot be put back either, Apply records what it holds,
// so the rollback at the new version's starts restores the old version.
func TestApplyZipFolderInterruptedUndoFails(t *testing.T) {
	f := newFixture(t, "1.0.0")
	dir, exe := zipInstall(t)
	f.publish(manifest(zipRelease(f, "1.1.0")))
	u := download(t, f)
	next := filepath.Join(dir, ".update", "new")
	failRenames(t, func(src, dst string) bool {
		return isEntry(src, next, "new.dll") || isEntry(dst, next, "notes.exe")
	})
	err := u.Apply()
	if err == nil || !strings.Contains(err.Error(), "putting the folder back failed too") {
		t.Fatalf("Apply = %v, want the failed undo reported", err)
	}
	p := pendingOf(t, dir)
	if p == nil || p.Version != "1.1.0" || !slices.Equal(p.Files, []string{MarkerName, "notes.exe"}) || len(p.OldFiles) != 3 {
		t.Fatalf("pending %+v, want 1.1.0 holding its marker and notes.exe", p)
	}
	setVar(t, &rename, os.Rename)
	f.setVersion("1.1.0")
	if got := starts(t, HealthyStarts+1); got != exe {
		t.Fatalf("rollback starts %q", got)
	}
	if readFile(t, exe) != "v1.0" || readFile(t, filepath.Join(dir, "old.dll")) != "old" || !strings.Contains(readFile(t, filepath.Join(dir, MarkerName)), `"1.0.0"`) {
		t.Errorf("the rollback did not restore the old version")
	}
}

// inode returns what identifies the file at p now, compared with
// os.SameFile.
func inode(t *testing.T, p string) os.FileInfo {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// On Windows the file ID is read from the path on the first SameFile
	// and kept: read it before a rename puts another file at the path.
	os.SameFile(st, st)
	return st
}

func TestApplyRefusesBadZips(t *testing.T) {
	f := newFixture(t, "1.0.0")
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{MarkerName: marker("1.0.0", "notes.exe", MarkerName), "notes.exe": "v1.0"})
	useEnv(t, env("windows", filepath.Join(dir, "notes.exe"), nil))
	for _, tc := range []struct {
		name, want string
		files      map[string]string
	}{
		{"no marker", "has no " + MarkerName, map[string]string{"notes.exe": "v1.1"}},
		{"an escaping entry", "entry outside its folder", map[string]string{MarkerName: marker("1.1.0", "notes.exe", MarkerName), "notes.exe": "x", "../evil": "x"}},
		{"an escaping link", "points outside its folder", map[string]string{MarkerName: marker("1.1.0", "notes.exe", MarkerName), "notes.exe": "x", "l": "->../../etc/passwd"}},
		{"a missing file", "which the artifact lacks", map[string]string{MarkerName: marker("1.1.0", "notes.exe", "lib.dll", MarkerName), "notes.exe": "x"}},
	} {
		f.publish(manifest(release("1.1.0", f.artifact("z", "zip", zipOfFiles(t, tc.files)))))
		if err := download(t, f).Apply(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Apply = %v, want an error with %q", tc.name, err, tc.want)
		}
		if readFile(t, filepath.Join(dir, "notes.exe")) != "v1.0" {
			t.Fatalf("%s: the install changed", tc.name)
		}
	}
}

func TestRestartOutsideRun(t *testing.T) {
	newFixture(t, "1.0.0")
	p := appImage(t, "v1")
	var started []string
	exited := -1
	setVar(t, &startProgram, func(exe string) error { started = append(started, exe); return nil })
	setVar(t, &exitProcess, func(code int) { exited = code })
	if err := Restart(); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0] != p || exited != 0 {
		t.Errorf("started %v, exit %d", started, exited)
	}
}
