package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/plist"
)

func TestParseBuildArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want buildOptions
	}{
		{nil, buildOptions{Pkg: "."}},
		{[]string{"./cmd/app"}, buildOptions{Pkg: "./cmd/app"}},
		{[]string{"-o", "dist", "-id", "com.example.a", "-name", "A B", "./x"}, buildOptions{Pkg: "./x", Out: "dist", ID: "com.example.a", Name: "A B"}},
		{[]string{"./x", "-tags", "a,b", "-ldflags=-s -w", "-gcflags", "-N", "-trimpath", "-v"}, buildOptions{Pkg: "./x", Tags: "a,b", Ldflags: "-s -w", Gcflags: "-N", Trimpath: true, Verbose: true}},
		{[]string{"--verbose", "--o=a.app"}, buildOptions{Pkg: ".", Out: "a.app", Verbose: true}},
	} {
		got, err := parseBuildArgs(tc.args)
		if err != nil {
			t.Errorf("%v: %v", tc.args, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: got %+v, want %+v", tc.args, got, tc.want)
		}
	}
}

func TestParseBuildArgsErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"a", "b"}, "at most one <pkg>"},
		{[]string{"-nope"}, "-nope"},
		{[]string{"-o"}, "needs an argument"},
		{[]string{""}, "empty <pkg>"},
	} {
		_, err := parseBuildArgs(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: error %v, want it to contain %q", tc.args, err, tc.want)
		}
	}
	if _, err := parseBuildArgs([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: %v", err)
	}
}

func TestParseRunArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want buildOptions
	}{
		{nil, buildOptions{Pkg: "."}},
		{[]string{"./app"}, buildOptions{Pkg: "./app"}},
		// Everything after the package belongs to the program, flags included.
		{[]string{"-id", "a.b", "./app", "-v", "--size=3", "x"}, buildOptions{Pkg: "./app", ID: "a.b", Args: []string{"-v", "--size=3", "x"}}},
		{[]string{"-v", "-tags", "t", ".", "--", "y"}, buildOptions{Pkg: ".", Tags: "t", Verbose: true, Args: []string{"--", "y"}}},
		{[]string{"--", "./app", "z"}, buildOptions{Pkg: "./app", Args: []string{"z"}}},
	} {
		got, err := parseRunArgs(tc.args)
		if err != nil {
			t.Errorf("%v: %v", tc.args, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: got %+v, want %+v", tc.args, got, tc.want)
		}
	}
	if _, err := parseRunArgs([]string{"-o", "x", "."}); err == nil || !strings.Contains(err.Error(), "tela help run") {
		t.Errorf("run has no -o: %v", err)
	}
	if _, err := parseRunArgs([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: %v", err)
	}
}

func TestBuildHelp(t *testing.T) {
	for _, cmd := range []string{"build", "run"} {
		var out, errOut bytes.Buffer
		if code := run([]string{"help", cmd}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "tela "+cmd) {
			t.Errorf("help %s: code %d, %q", cmd, code, out.String())
		}
		out.Reset()
		if code := run([]string{cmd, "-h"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "Usage:") {
			t.Errorf("%s -h: code %d, %q", cmd, code, out.String())
		}
	}
	var out, errOut bytes.Buffer
	run([]string{"help"}, &out, &errOut)
	for _, line := range []string{"\tbuild ", "\trun "} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("the command list lacks %q", line)
		}
	}
}

func TestBundlePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got, err := bundlePath("", "My App")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(evalDir(t, dir), "My App.app"); evalDir(t, filepath.Dir(got)) != filepath.Dir(want) || filepath.Base(got) != "My App.app" {
		t.Errorf("default: %s, want %s", got, want)
	}
	if got, _ := bundlePath("out", "A/B"); filepath.Base(got) != "A-B.app" || filepath.Base(filepath.Dir(got)) != "out" {
		t.Errorf("a folder: %s", got)
	}
	if got, _ := bundlePath("dist/Custom.app", "Ignored"); filepath.Base(got) != "Custom.app" {
		t.Errorf("a .app path: %s", got)
	}
	if err := os.WriteFile("file.app", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := bundlePath("file.app", "x"); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Errorf("a file: %v", err)
	}
}

func evalDir(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPackageBaseName(t *testing.T) {
	for _, tc := range []struct {
		p    listedPackage
		want string
	}{
		{listedPackage{ImportPath: "example.com/me/notes"}, "notes"},
		{listedPackage{ImportPath: "example.com/me/notes/v2"}, "notes"},
		{listedPackage{ImportPath: "notes"}, "notes"},
		{listedPackage{ImportPath: "command-line-arguments", Dir: "/x/y/tool"}, "command-line-arguments"},
		{listedPackage{ImportPath: "", Dir: "/x/y/tool"}, "tool"},
	} {
		if got := packageBaseName(tc.p); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestNumericVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v1.2.3":                                "1.2.3",
		"v0.13.3-0.20261102010101-abcdef123456": "0.13.3",
		"v1.2.3+dirty":                          "1.2.3",
		"v1.2.3-rc.1":                           "1.2.3",
		"(devel)":                               "",
		"":                                      "",
		"v2":                                    "",
	} {
		if got := numericVersion(in); got != want {
			t.Errorf("numericVersion(%q) = %q, want %q", in, got, want)
		}
	}
	if got := moduleVersion(os.Args[0]); got != "" && numericVersion("v"+got) != got {
		t.Errorf("moduleVersion of the test binary = %q", got)
	}
}

func TestRunBundlePath(t *testing.T) {
	cache := t.TempDir()
	defer func(f func() (string, error)) { userCacheDir = f }(userCacheDir)
	userCacheDir = func() (string, error) { return cache, nil }
	got, err := runBundlePath(identity{ID: "dev.example.a", Name: "My/App"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cache, "tela", "run", "dev.example.a", "My-App.app"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	userCacheDir = func() (string, error) { return "", errors.New("no HOME") }
	if _, err := runBundlePath(identity{ID: "a.b", Name: "A"}); err == nil || !strings.Contains(err.Error(), "tela build") {
		t.Errorf("no cache folder: %v", err)
	}
}

func TestRemoveOtherBundles(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"Keep.app", "Old.app", "notes.txt"} {
		if err := os.MkdirAll(filepath.Join(dir, n, "Contents"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	removeOtherBundles(dir, filepath.Join(dir, "Keep.app"))
	var left []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if !reflect.DeepEqual(left, []string{"Keep.app", "notes.txt"}) {
		t.Errorf("left %v", left)
	}
}

func TestExitStatus(t *testing.T) {
	if exitStatus(nil) != 0 {
		t.Error("nil is not 0")
	}
	if runtime.GOOS == "windows" {
		t.Skip("no sh")
	}
	for _, tc := range []struct {
		script string
		want   int
	}{
		{"exit 7", 7},
		{"kill -TERM $$", 128 + 15},
	} {
		err := exec.Command("sh", "-c", tc.script).Run()
		if got := exitStatus(err); got != tc.want {
			t.Errorf("%q: %d, want %d", tc.script, got, tc.want)
		}
	}
}

// bundleModule creates a module outside the repository that requires tela
// (through a replace directive) and has the packages given by files, like
// outsideModule: it skips the test when the dependencies cannot be
// resolved offline.
func bundleModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = fmt.Sprintf(`module example.com/bundles

go 1.27

require github.com/kuronosu/tela v0.0.0

replace github.com/kuronosu/tela => %s
`, filepath.ToSlash(repoRoot(t)))
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Skipf("cannot resolve the dependencies of a module outside the repository offline (go mod tidy: %v):\n%s", err, out)
	}
	return dir
}

// setTarget makes the go command build for goos/goarch.
func setTarget(t *testing.T, goos, goarch string) {
	t.Helper()
	t.Setenv("GOOS", goos)
	t.Setenv("GOARCH", goarch)
}

// TestBuildBundle builds a bundle for darwin whichever OS this is, and
// reads back what tela build wrote.
func TestBuildBundle(t *testing.T) {
	skipShort(t)
	dir := bundleModule(t, map[string]string{
		"notes/main.go": `package main

import "github.com/kuronosu/tela"

const vendor = "dev.example"

var App = tela.App{Title: "Notes Demo", AppID: vendor + ".notes"}

func main() { tela.Main(wrap(App)) }

func wrap(a tela.App) tela.App { return a }
`,
		"noid/main.go": `package main

import "github.com/kuronosu/tela"

func main() { tela.Main(tela.App{Title: "No ID"}) }
`,
		"lib/lib.go": "package lib\n",
	})
	t.Chdir(dir)
	setTarget(t, "darwin", "arm64")

	out := filepath.Join(dir, "dist")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"build", "-o", out, "./notes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("tela build: code %d\n%s", code, stderr.String())
	}
	bundle := filepath.Join(out, "Notes Demo.app")
	if !strings.Contains(stdout.String(), "wrote ") || !strings.Contains(stdout.String(), "Notes Demo.app") {
		t.Errorf("stdout %q", stdout.String())
	}
	exe := filepath.Join(bundle, "Contents", "MacOS", "Notes Demo")
	if st, err := os.Stat(exe); err != nil || st.Size() == 0 {
		t.Fatalf("executable: %v", err)
	}
	head := make([]byte, 4)
	f, _ := os.Open(exe)
	f.Read(head)
	f.Close()
	if string(head) != "\xcf\xfa\xed\xfe" { // 64-bit little-endian Mach-O
		t.Errorf("the executable starts with % x, not a Mach-O", head)
	}
	raw, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := plist.UnmarshalDict(raw)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{
		"CFBundleIdentifier":      "dev.example.notes",
		"CFBundleName":            "Notes Demo",
		"CFBundleDisplayName":     "Notes Demo",
		"CFBundleExecutable":      "Notes Demo",
		"CFBundlePackageType":     "APPL",
		"LSMinimumSystemVersion":  "13.0",
		"NSHighResolutionCapable": true,
	} {
		if info[k] != want {
			t.Errorf("%s = %v, want %v", k, info[k], want)
		}
	}

	// An unchanged rebuild leaves the bundle as it was.
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	infoPath := filepath.Join(bundle, "Contents", "Info.plist")
	for _, p := range []string{infoPath, exe} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if code := run([]string{"build", "-o", out, "./notes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("second tela build: code %d\n%s", code, stderr.String())
	}
	for _, p := range []string{infoPath, exe} {
		if st, _ := os.Stat(p); !st.ModTime().Equal(old) {
			t.Errorf("%s was rewritten by an unchanged rebuild", p)
		}
	}

	// The flags replace the App.
	stdout.Reset()
	if code := run([]string{"build", "-o", filepath.Join(dir, "custom.app"), "-id", "dev.other.id", "-name", "Other", "./notes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("tela build -id: code %d\n%s", code, stderr.String())
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "custom.app", "Contents", "Info.plist"))
	if info, _ = plist.UnmarshalDict(raw); info["CFBundleIdentifier"] != "dev.other.id" || info["CFBundleName"] != "Other" {
		t.Errorf("-id and -name: %v", info)
	}

	// No ID: the teaching message, and nothing written.
	stderr.Reset()
	if code := run([]string{"build", "-o", filepath.Join(dir, "none"), "./noid"}, &stdout, &stderr); code != 1 {
		t.Errorf("no ID: code %d, want 1", code)
	}
	for _, want := range []string{"has no app ID", "set tela.App.AppID", "-id"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("no ID: stderr lacks %q:\n%s", want, stderr.String())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "none")); err == nil {
		t.Errorf("a failed build wrote its output")
	}

	// An ID the Info.plist cannot take.
	stderr.Reset()
	if code := run([]string{"build", "-o", filepath.Join(dir, "none"), "-id", "my app", "./noid"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "CFBundleIdentifier does not allow") {
		t.Errorf("bad ID: code %d\n%s", code, stderr.String())
	}

	// Not a program.
	stderr.Reset()
	if code := run([]string{"build", "./lib"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "not a package main") {
		t.Errorf("a library: code %d\n%s", code, stderr.String())
	}

	// Another target: the plain executable, no bundle, no ID needed.
	setTarget(t, "linux", "amd64")
	plain := filepath.Join(dir, "plain-notes")
	stdout.Reset()
	if code := run([]string{"build", "-o", plain, "./noid"}, &stdout, &stderr); code != 0 {
		t.Fatalf("plain build: code %d\n%s", code, stderr.String())
	}
	if st, err := os.Stat(plain); err != nil || st.IsDir() {
		t.Errorf("plain executable: %v", err)
	}
}

// program is a main package for the run tests: it prints its arguments and
// an environment variable and exits with the code the first argument says.
const programSource = `package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/kuronosu/tela"
)

var App = tela.App{Title: "Runner", AppID: "dev.example.runner"}

func main() {
	fmt.Println("args:", os.Args[1:])
	code, _ := strconv.Atoi(os.Args[1])
	os.Exit(code)
}
`

// TestRunForwardsArgsAndStatus runs a program with tela run on this machine
// (as a bundle on macOS) and checks the arguments and the exit status.
func TestRunForwardsArgsAndStatus(t *testing.T) {
	skipShort(t)
	dir := bundleModule(t, map[string]string{"runner/main.go": programSource})
	t.Chdir(dir)
	cache := t.TempDir()
	defer func(f func() (string, error)) { userCacheDir = f }(userCacheDir)
	userCacheDir = func() (string, error) { return cache, nil }

	var stdout, stderr bytes.Buffer
	if code := run([]string{"run", "-v", "./runner", "3", "-x", "two words"}, &stdout, &stderr); code != 3 {
		t.Fatalf("exit status %d, want 3\nstderr: %s", code, stderr.String())
	}
	if want := "args: [3 -x two words]"; !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout %q, want it to contain %q", stdout.String(), want)
	}
	if runtime.GOOS == "darwin" {
		bundle := filepath.Join(cache, "tela", "run", "dev.example.runner", "Runner.app")
		if _, err := os.Stat(filepath.Join(bundle, "Contents", "Info.plist")); err != nil {
			t.Errorf("no bundle at %s: %v", bundle, err)
		}
		if !strings.Contains(stderr.String(), bundle) {
			t.Errorf("-v did not say where the bundle is:\n%s", stderr.String())
		}
	} else if strings.Contains(stderr.String(), "bundle") {
		t.Errorf("a bundle on %s:\n%s", runtime.GOOS, stderr.String())
	}

	// A second run finds nothing to write.
	stderr.Reset()
	if code := run([]string{"run", "-v", "./runner", "0"}, &stdout, &stderr); code != 0 {
		t.Fatalf("second run: status %d\nstderr: %s", code, stderr.String())
	}
	if runtime.GOOS == "darwin" && !strings.Contains(stderr.String(), "created false, Info.plist written false, executable written false") {
		t.Errorf("the second run rewrote the bundle:\n%s", stderr.String())
	}

	// Not this machine.
	other := "darwin"
	if runtime.GOOS == "darwin" {
		other = "linux"
	}
	setTarget(t, other, "arm64")
	stderr.Reset()
	if code := run([]string{"run", "./runner", "0"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "not this machine") {
		t.Errorf("cross target: code %d\n%s", code, stderr.String())
	}
}
