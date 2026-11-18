package main

import (
	"bufio"
	"bytes"
	"debug/elf"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/appimage"
	"github.com/kuronosu/tela/cmd/tela/internal/appimage/layout"
	"github.com/kuronosu/tela/cmd/tela/internal/appimage/squashfs"
)

// The AppImage runtime (ADR-295), run on this machine: its start,
// concurrent starts, its integrity checks, the cleanup of old versions and
// its options.

var aiEpoch = time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)

// aiFixtureSource is the payload's program: it prints its environment and
// arguments, and what the argument asks.
const aiFixtureSource = `package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	dir := os.Getenv("APPDIR")
	var st syscall.Stat_t
	syscall.Stat(dir, &st)
	data, _ := os.ReadFile(filepath.Join(dir, "usr/share/data.txt"))
	fmt.Printf("APPIMAGE=%s\nAPPDIR=%s\nARGV0=%s\nOWD=%s\nINO=%d\nDATA=%s\nARGS=%q\n",
		os.Getenv("APPIMAGE"), dir, os.Getenv("ARGV0"), os.Getenv("OWD"), st.Ino, data, os.Args[1:])
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "exit3":
			os.Exit(3)
		case "sleep":
			fmt.Println("READY")
			time.Sleep(time.Minute)
		case "exec":
			err := syscall.Exec(os.Args[2], []string{os.Args[2], "done"}, os.Environ())
			fmt.Println("exec:", err)
			os.Exit(1)
		}
	}
}
`

// buildProgram builds src for linux/goarch.
func aiBuildProgram(t *testing.T, src, goarch string) []byte {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo "+appimage.GoDirective+"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", "prog", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+goarch, "GOFLAGS=", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the fixture: %v\n%s", err, out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "prog"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// makeAppImage writes an AppImage of the fixture with the data file's
// content data, as version v.
func aiMakeAppImage(t *testing.T, rt, prog []byte, dir, v, data string) string {
	t.Helper()
	b, err := appimage.Assemble(rt, appimage.Spec{
		AppID: "dev.example.fixture", Version: v, ModTime: aiEpoch,
		Entries: []squashfs.Entry{
			{Path: "AppRun", Mode: fs.ModeSymlink | 0o777, Target: "usr/bin/fixture"},
			{Path: "usr/bin/fixture", Mode: 0o755, Data: prog},
			{Path: "usr/share/data.txt", Mode: 0o644, Data: []byte(data)},
			{Path: "dev.example.fixture.desktop", Mode: 0o644, Data: []byte("[Desktop Entry]\n")},
		},
		UpdateInfo: "zsync|https://example.com/fixture.zsync",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "fixture-"+v+".AppImage")
	if err := os.WriteFile(p, b, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// result is the fixture's output.
type aiResult struct {
	code   int
	out    map[string]string
	stdout string
	stderr string
}

func aiStart(path string, cache string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cache)
	return cmd
}

func aiRun(t *testing.T, path, cache string, args ...string) aiResult {
	t.Helper()
	cmd := aiStart(path, cache, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	r := aiResult{out: map[string]string{}, stdout: stdout.String(), stderr: stderr.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		r.code = ee.ExitCode()
	} else if err != nil {
		t.Errorf("running %s: %v", path, err) // not Fatal: called from goroutines
		r.code = -1
	}
	for _, l := range strings.Split(r.stdout, "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			r.out[k] = v
		}
	}
	return r
}

// versions lists the version folders of the fixture in cache.
func aiVersions(t *testing.T, cache string) []string {
	t.Helper()
	ents, _ := os.ReadDir(filepath.Join(cache, "tela-appimage", "dev.example.fixture"))
	var out []string
	for _, e := range ents {
		if e.Name() != ".lock" {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func TestRuntime(t *testing.T) {
	skipShort(t)
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("runs the AppImage on this machine: linux/amd64 or linux/arm64 only")
	}
	rt, err := appimage.BuildRuntime("go", runtime.GOARCH, "test", os.Environ(), os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := appimage.BuildRuntime("go", runtime.GOARCH, "test", os.Environ(), os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rt, rt2) {
		t.Errorf("two builds of the runtime differ")
	}
	prog := aiBuildProgram(t, aiFixtureSource, runtime.GOARCH)
	dir := t.TempDir()
	v1 := aiMakeAppImage(t, rt, prog, dir, "1.0.0", "one")
	if b := aiMakeAppImage(t, rt, prog, t.TempDir(), "1.0.0", "one"); true {
		x, _ := os.ReadFile(v1)
		y, _ := os.ReadFile(b)
		if !bytes.Equal(x, y) {
			t.Errorf("two AppImages of the same inputs differ")
		}
	}
	cache := t.TempDir()

	t.Run("start", func(t *testing.T) {
		r := aiRun(t, v1, cache, "exit3", "two words")
		if r.code != 3 {
			t.Fatalf("exit status %d, want 3\n%s%s", r.code, r.stdout, r.stderr)
		}
		wantDir := filepath.Join(cache, "tela-appimage", "dev.example.fixture", "1.0.0-")
		if r.out["APPIMAGE"] != v1 || !strings.HasPrefix(r.out["APPDIR"], wantDir) || r.out["ARGV0"] != v1 || r.out["DATA"] != "one" {
			t.Errorf("environment: %v", r.out)
		}
		if wd, _ := os.Getwd(); r.out["OWD"] != wd {
			t.Errorf("OWD %q, want %q", r.out["OWD"], wd)
		}
		if r.out["ARGS"] != `["exit3" "two words"]` {
			t.Errorf("arguments %s", r.out["ARGS"])
		}
		if vs := aiVersions(t, cache); len(vs) != 1 {
			t.Errorf("cache: %v", vs)
		}
		// A warm start uses the same folder.
		r2 := aiRun(t, v1, cache)
		if r2.code != 0 || r2.out["INO"] != r.out["INO"] {
			t.Errorf("warm start: code %d, folder %s then %s", r2.code, r.out["INO"], r2.out["INO"])
		}
		r3 := aiRun(t, v1, cache, "--appimage-extract-and-run", "x")
		if r3.code != 0 || r3.out["ARGS"] != `["x"]` {
			t.Errorf("--appimage-extract-and-run: %d %v", r3.code, r3.out["ARGS"])
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		c := t.TempDir()
		const n = 6
		var wg sync.WaitGroup
		res := make([]aiResult, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res[i] = aiRun(t, v1, c)
			}()
		}
		wg.Wait()
		for i, r := range res {
			if r.code != 0 || r.out["INO"] != res[0].out["INO"] {
				t.Errorf("start %d: code %d, folder %s (first: %s)\n%s", i, r.code, r.out["INO"], res[0].out["INO"], r.stderr)
			}
		}
		if vs := aiVersions(t, c); len(vs) != 1 {
			t.Errorf("cache after concurrent starts: %v", vs)
		}
	})

	t.Run("crashed extraction", func(t *testing.T) {
		c := t.TempDir()
		stale := filepath.Join(c, "tela-appimage", "dev.example.fixture", ".tmp-12345")
		os.MkdirAll(filepath.Join(stale, "usr"), 0o755)
		if r := aiRun(t, v1, c); r.code != 0 {
			t.Fatalf("code %d\n%s", r.code, r.stderr)
		}
		if _, err := os.Stat(stale); err == nil {
			t.Errorf("the leftover of a crashed extraction is still there")
		}
	})

	t.Run("damaged", func(t *testing.T) {
		b, _ := os.ReadFile(v1)
		lf, err := layout.Read(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatal(err)
		}
		bad := bytes.Clone(b)
		bad[lf.End+5000] ^= 0x40
		p := filepath.Join(t.TempDir(), "bad.AppImage")
		os.WriteFile(p, bad, 0o755)
		c := t.TempDir()
		r := aiRun(t, p, c)
		if r.code == 0 || !strings.Contains(r.stderr, "damaged") || r.out["APPDIR"] != "" {
			t.Errorf("a damaged AppImage: code %d, stderr %q", r.code, r.stderr)
		}
		if vs := aiVersions(t, c); len(vs) != 0 {
			t.Errorf("a damaged AppImage left %v", vs)
		}
		os.WriteFile(p, b[:len(b)-4096], 0o755)
		if r := aiRun(t, p, c); r.code == 0 || !strings.Contains(r.stderr, "incomplete") {
			t.Errorf("a truncated AppImage: code %d, stderr %q", r.code, r.stderr)
		}
		bad = bytes.Clone(b)
		bad[lf.End+5000] ^= 0x40
		os.WriteFile(p, bad, 0o755)
		cwd := t.TempDir()
		cmd := aiStart(p, c, "--appimage-extract")
		cmd.Dir = cwd
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "damaged") {
			t.Errorf("--appimage-extract of a damaged AppImage: %v %s", err, out)
		}
	})

	t.Run("altered tree", func(t *testing.T) {
		c := t.TempDir()
		r := aiRun(t, v1, c)
		appdir := r.out["APPDIR"]
		data := filepath.Join(appdir, "usr/share/data.txt")
		for _, alter := range []struct {
			name string
			do   func() error
		}{
			{"same size, new content", func() error { return os.WriteFile(data, []byte("eno"), 0o644) }},
			{"deleted", func() error { return os.Remove(data) }},
			{"mode", func() error { return os.Chmod(data, 0o600) }},
			{"program deleted", func() error { return os.Remove(filepath.Join(appdir, "usr/bin/fixture")) }},
			{"marker", func() error {
				m := filepath.Join(appdir, ".tela-extracted")
				os.Chmod(m, 0o644)
				b, _ := os.ReadFile(m)
				return os.WriteFile(m, bytes.Replace(b, []byte("data.txt"), []byte("data.tx_"), 1), 0o644)
			}},
		} {
			if err := alter.do(); err != nil {
				t.Fatal(err)
			}
			r = aiRun(t, v1, c)
			if r.code != 0 || r.out["DATA"] != "one" {
				t.Errorf("%s: code %d, data %q\n%s", alter.name, r.code, r.out["DATA"], r.stderr)
			}
			appdir = r.out["APPDIR"]
			data = filepath.Join(appdir, "usr/share/data.txt")
		}
		// The time changed alone: the content is the same, no new
		// extraction.
		before := r.out["INO"]
		later := time.Now()
		os.Chtimes(data, later, later)
		if r = aiRun(t, v1, c); r.out["INO"] != before {
			t.Errorf("a new time alone made a new extraction")
		}
		if vs := aiVersions(t, c); len(vs) != 1 {
			t.Errorf("the altered copies were not removed: %v", vs)
		}
	})

	t.Run("old versions", func(t *testing.T) {
		c := t.TempDir()
		v2 := aiMakeAppImage(t, rt, prog, dir, "2.0.0", "two")
		// v1 runs (it holds its folder) while v2 starts: v1 stays.
		cmd := aiStart(v1, c, "sleep")
		stdout, _ := cmd.StdoutPipe()
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer cmd.Process.Kill()
		sc := bufio.NewScanner(stdout)
		for sc.Scan() && sc.Text() != "READY" {
		}
		if r := aiRun(t, v2, c); r.code != 0 || r.out["DATA"] != "two" {
			t.Fatalf("v2: %d %v %s", r.code, r.out, r.stderr)
		}
		if vs := aiVersions(t, c); len(vs) != 2 {
			t.Errorf("while v1 runs: %v, want both versions", vs)
		}
		cmd.Process.Kill()
		cmd.Wait()
		if r := aiRun(t, v2, c); r.code != 0 {
			t.Fatal(r.stderr)
		}
		vs := aiVersions(t, c)
		if len(vs) != 1 || !strings.HasPrefix(vs[0], "2.0.0-") {
			t.Errorf("after v1 ended: %v, want v2 only", vs)
		}

		// v1 replaces itself with v2 by exec (an update's restart): v2
		// closes the lock it inherited and removes v1.
		if r := aiRun(t, v1, c); r.code != 0 {
			t.Fatal(r.stderr)
		}
		r := aiRun(t, v1, c, "exec", v2)
		if r.code != 0 || !strings.Contains(r.stdout, "DATA=two") {
			t.Fatalf("exec: %d\n%s%s", r.code, r.stdout, r.stderr)
		}
		vs = aiVersions(t, c)
		if len(vs) != 1 || !strings.HasPrefix(vs[0], "2.0.0-") {
			t.Errorf("after v1 exec'd v2: %v, want v2 only", vs)
		}
	})

	t.Run("options", func(t *testing.T) {
		cwd := t.TempDir()
		cmd := aiStart(v1, t.TempDir(), "--appimage-extract")
		cmd.Dir = cwd
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("--appimage-extract: %v\n%s", err, out)
		}
		if b, _ := os.ReadFile(filepath.Join(cwd, "squashfs-root/usr/share/data.txt")); string(b) != "one" {
			t.Errorf("extracted data %q", b)
		}
		if l, _ := os.Readlink(filepath.Join(cwd, "squashfs-root/AppRun")); l != "usr/bin/fixture" {
			t.Errorf("AppRun -> %q", l)
		}
		if st, err := os.Stat(filepath.Join(cwd, "squashfs-root/usr/bin/fixture")); err != nil || st.Mode().Perm() != 0o755 {
			t.Errorf("the program: %v %v", st, err)
		}
		cwd2 := t.TempDir()
		cmd = aiStart(v1, t.TempDir(), "--appimage-extract", "usr/share")
		cmd.Dir = cwd2
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("--appimage-extract usr/share: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(cwd2, "squashfs-root/usr/share/data.txt")); err != nil {
			t.Errorf("the pattern's tree: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(cwd2, "squashfs-root/AppRun")); err == nil {
			t.Errorf("the pattern extracted AppRun")
		}

		r := aiRun(t, v1, t.TempDir(), "--appimage-offset")
		b, _ := os.ReadFile(v1)
		lf, _ := layout.Read(bytes.NewReader(b), int64(len(b)))
		if strings.TrimSpace(r.stdout) != fmt.Sprint(lf.End) || string(b[lf.End:lf.End+4]) != "hsqs" {
			t.Errorf("--appimage-offset %q, ELF end %d, magic there %q", r.stdout, lf.End, b[lf.End:lf.End+4])
		}
		if r := aiRun(t, v1, t.TempDir(), "--appimage-updateinformation"); strings.TrimSpace(r.stdout) != "zsync|https://example.com/fixture.zsync" {
			t.Errorf("--appimage-updateinformation %q", r.stdout)
		}
		if r := aiRun(t, v1, t.TempDir(), "--appimage-version"); !strings.Contains(r.stdout, "Tela AppImage runtime test") {
			t.Errorf("--appimage-version %q", r.stdout)
		}
		if r := aiRun(t, v1, t.TempDir(), "--appimage-help"); r.code != 0 || !strings.Contains(r.stdout, "--appimage-extract") {
			t.Errorf("--appimage-help %d %q", r.code, r.stdout)
		}
		if r := aiRun(t, v1, t.TempDir(), "--appimage-nope"); r.code == 0 || !strings.Contains(r.stderr, "unknown option") {
			t.Errorf("--appimage-nope %d %q", r.code, r.stderr)
		}
	})

	t.Run("warm start", func(t *testing.T) {
		c := t.TempDir()
		aiRun(t, v1, c)
		direct := filepath.Join(t.TempDir(), "fixture")
		os.WriteFile(direct, prog, 0o755)
		median := func(f func()) time.Duration {
			var ds []time.Duration
			for range 25 {
				t0 := time.Now()
				f()
				ds = append(ds, time.Since(t0))
			}
			slices.Sort(ds)
			return ds[len(ds)/2]
		}
		quiet := func(p string, env ...string) func() {
			return func() {
				cmd := exec.Command(p)
				cmd.Env = append(os.Environ(), env...)
				cmd.Run()
			}
		}
		a := median(quiet(direct, "APPDIR="+t.TempDir()))
		b := median(quiet(v1, "XDG_CACHE_HOME="+c))
		t.Logf("warm start: the program alone %v, through the AppImage %v (overhead %v)", a, b, b-a)
	})
}

// TestRuntimeArm64 cross-builds the runtime for arm64 and runs it under
// qemu-aarch64-static when that is installed.
func TestRuntimeArm64(t *testing.T) {
	skipShort(t)
	rt, err := appimage.BuildRuntime("go", "arm64", "test", os.Environ(), os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	prog := aiBuildProgram(t, aiFixtureSource, "arm64")
	p := aiMakeAppImage(t, rt, prog, t.TempDir(), "1.0.0", "one")
	f, err := elf.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Machine != elf.EM_AARCH64 || f.Class != elf.ELFCLASS64 || f.Section(layout.UpdateInfo) == nil || f.Section(layout.Payload) == nil {
		t.Errorf("machine %v, class %v, sections %v", f.Machine, f.Class, f.Sections)
	}
	f.Close()
	if _, err := appimage.BuildRuntime("go", "386", "test", os.Environ(), os.Stderr); err == nil || !strings.Contains(err.Error(), "64-bit") {
		t.Errorf("386: %v", err)
	}
	// Run it under qemu-user when installed. The payload's program is a
	// plain arm64 ELF: its exec needs the kernel to run it through qemu
	// (binfmt_misc). The AppImage itself does not match a binfmt_misc
	// entry that wants zeros at offset 8, so it is started through qemu
	// directly.
	qemu, err := exec.LookPath("qemu-aarch64-static")
	_, errBinfmt := os.Stat("/proc/sys/fs/binfmt_misc/qemu-aarch64")
	if err != nil || errBinfmt != nil || runtime.GOOS != "linux" {
		t.Skip("runs the arm64 AppImage only with qemu-aarch64-static and its binfmt_misc entry")
	}
	out, err := exec.Command(qemu, p, "--appimage-version").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Tela AppImage runtime") {
		t.Errorf("--appimage-version under qemu: %v %s", err, out)
	}
	cmd := exec.Command(qemu, p, "arg")
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+t.TempDir())
	out, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), `ARGS=["arg"]`) || !strings.Contains(string(out), "DATA=one") {
		t.Errorf("under qemu: %v\n%s", err, out)
	}
}
