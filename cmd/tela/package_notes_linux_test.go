package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestNotesAppImageUpdate is phase 26's done criterion on Linux: the sample
// app, examples/notes, packaged by tela package and updated from a signed
// manifest by itself. The other two platforms of the criterion are the
// round trips of the Windows zip (TestZipUpdateRoundTrip, run on Windows
// too) and of the macOS bundle (TestPackageAppUpdates); TestNotesPackage
// packages notes for the three systems from one command.
//
// Versions 1.0.0 and 1.1.0 of notes are packaged as AppImages with the
// update URL and key given to notes's own variables by -ldflags -X (the key
// made by tela update keygen), the manifest is made from the folder by
// tela update manifest and signed by tela update sign, and an httptest
// server serves it. Version 1.0.0 is installed and started: notes itself,
// on the headless backend (TELA_BACKEND=headless, the backend's own
// switch, so it opens no window and touches none of the desktop's
// services). Its update.UseUpdate checks, downloads and applies 1.1.0;
// the test then stops it, as a user who quits would. The AppImage must be
// 1.1.0's, with 1.0.0's kept beside it; the next start must run 1.1.0,
// which marks itself healthy, so that more starts than HealthyStarts do not
// roll it back. (An AppImage runs on Linux only: hence the file's name.)
func TestNotesAppImageUpdate(t *testing.T) {
	skipShort(t)
	root := repoRoot(t)
	dir := t.TempDir()

	// The server, first: its URL goes into the programs.
	pub := filepath.Join(dir, "pub")
	os.MkdirAll(pub, 0o755)
	var manifestGets atomic.Int32
	files := http.FileServer(http.Dir(pub))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			manifestGets.Add(1)
		}
		files.ServeHTTP(w, r)
	}))
	defer srv.Close()
	key := keygen(t, dir, "notes.key")

	pack := func(version, out string) string {
		t.Helper()
		t.Chdir(root)
		var stdout, stderr bytes.Buffer
		ldflags := fmt.Sprintf("-X main.updateURL=%s/manifest.json -X main.updateKey=%s", srv.URL, key)
		args := []string{"package", "-os", "linux", "-arch", runtime.GOARCH, "-format", "appimage", "-version", version,
			"-ldflags", ldflags, "-o", out, "./examples/notes"}
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("tela package %s: %d\n%s%s", version, code, stdout.String(), stderr.String())
		}
		return filepath.Join(out, "notes-"+version+"-linux-"+runtime.GOARCH+".AppImage")
	}
	// Both into the published folder, which the manifest is made from
	// as a whole (dist/* in a shell): 1.0.0's file is skipped.
	v1 := pack("1.0.0", pub)
	v2 := pack("1.1.0", pub)
	t.Chdir(dir)
	pubFiles, _ := filepath.Glob(filepath.Join("pub", "*"))
	if code, out := telaIn(t, dir, append([]string{"update", "manifest", "-m", "pub/manifest.json", "-app-id", "dev.telaui.notes", "-version", "1.1.0"}, pubFiles...)...); code != 0 {
		t.Fatalf("tela update manifest: %d\n%s", code, out)
	} else if !strings.Contains(out, "skipped pub/notes-1.0.0-") {
		t.Errorf("tela update manifest did not skip 1.0.0's file:\n%s", out)
	}
	if code, out := telaIn(t, dir, "update", "sign", "-key", "notes.key", "pub/manifest.json"); code != 0 {
		t.Fatalf("tela update sign: %d\n%s", code, out)
	}
	newBytes := readFileBytes(t, v2)
	oldBytes := readFileBytes(t, v1)

	// The install: 1.0.0's AppImage where the user put it.
	inst := filepath.Join(dir, "Applications", "Notes.AppImage")
	os.MkdirAll(filepath.Dir(inst), 0o755)
	if err := os.WriteFile(inst, oldBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	cache := filepath.Join(dir, "cache")
	rt, err := os.MkdirTemp("", "rt") // short: it holds sockets
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(rt)
	os.Chmod(rt, 0o700)
	env := []string{"HOME=" + home, "XDG_CACHE_HOME=" + cache, "XDG_RUNTIME_DIR=" + rt, "TELA_BACKEND=headless",
		"PATH=" + os.Getenv("PATH")}
	stateFile := filepath.Join(dir, "Applications", ".Notes.AppImage.update", "state.json")
	type installState struct {
		Pending *struct {
			Version string
			Starts  int
		}
		Rollbacks []json.RawMessage
	}
	readState := func() (installState, bool) {
		var st installState
		b, err := os.ReadFile(stateFile)
		return st, err == nil && json.Unmarshal(b, &st) == nil
	}

	// start runs the installed AppImage until done says it did what the
	// test waits for, then stops it, and returns the version it ran (from
	// the folder the runtime extracted it to).
	start := func(what string, done func() bool) string {
		t.Helper()
		cmd := exec.Command(inst)
		cmd.Env = env
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		gets := manifestGets.Load()
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		var ran string
		deadline := time.Now().Add(2 * time.Minute)
		for {
			if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", cmd.Process.Pid)); err == nil && strings.HasPrefix(exe, cache) {
				ran = exe
			}
			if ran != "" && manifestGets.Load() > gets && done() {
				break
			}
			select {
			case err := <-exited:
				t.Fatalf("%s: notes exited (%v) before it was done:\n%s", what, err, out.String())
			case <-time.After(50 * time.Millisecond):
			}
			if time.Now().After(deadline) {
				cmd.Process.Kill()
				<-exited
				st, _ := readState()
				t.Fatalf("%s: not done after 2 minutes (manifest fetched %d times, state %+v):\n%s", what, manifestGets.Load(), st, out.String())
			}
		}
		cmd.Process.Kill()
		<-exited
		rel, _ := filepath.Rel(filepath.Join(cache, "tela-appimage", "dev.telaui.notes"), ran)
		v, _, _ := strings.Cut(rel, "-")
		return v
	}

	// 1.0.0 checks, downloads and applies 1.1.0.
	if v := start("version 1.0.0", func() bool {
		st, ok := readState()
		return ok && st.Pending != nil && st.Pending.Version == "1.1.0"
	}); v != "1.0.0" {
		t.Fatalf("the installed AppImage ran version %q, want 1.0.0", v)
	}
	if !bytes.Equal(readFileBytes(t, inst), newBytes) {
		t.Fatal("the AppImage was not replaced by 1.1.0's")
	}
	if st, err := os.Stat(inst); err != nil || st.Mode().Perm()&0o100 == 0 {
		t.Errorf("the new AppImage is not executable: %v %v", st.Mode(), err)
	}
	if !bytes.Equal(readFileBytes(t, filepath.Join(dir, "Applications", ".Notes.AppImage.update", "old")), oldBytes) {
		t.Error("1.0.0's AppImage was not kept")
	}

	// The next start runs 1.1.0, which marks itself healthy.
	if v := start("the first start of 1.1.0", func() bool {
		st, ok := readState()
		return ok && st.Pending == nil
	}); v != "1.1.0" {
		t.Fatalf("the start after the update ran version %q, want 1.1.0", v)
	}
	// Healthy, it stays: more starts than update.HealthyStarts.
	for i := range 3 {
		if v := start(fmt.Sprintf("start %d of 1.1.0", i+2), func() bool { return true }); v != "1.1.0" {
			t.Fatalf("start %d after the update ran version %q, want 1.1.0", i+2, v)
		}
	}
	if st, _ := readState(); st.Pending != nil || len(st.Rollbacks) != 0 {
		t.Errorf("the install's state after the starts: %+v", st)
	}
	if !bytes.Equal(readFileBytes(t, inst), newBytes) {
		t.Error("1.1.0 was rolled back")
	}
}

func readFileBytes(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
