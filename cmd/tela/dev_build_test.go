package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// tinyModule writes a module with a package main whose source is src.
func tinyModule(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/tiny\n\ngo 1.27\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644)
	return dir
}

func tinyBuilder(t *testing.T, goBin, dir string) *goBuilder {
	t.Helper()
	b, err := newGoBuilder(goBin, &devTarget{modDir: dir, arg: "."}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestGoBuilderSuccess(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip(err)
	}
	dir := tinyModule(t, `//go:build teladev

package main

import "fmt"

func main() { fmt.Println("built with teladev") }
`)
	b := tinyBuilder(t, goBin, dir)
	res := b.build(context.Background(), 1)
	if res.err != nil {
		t.Fatalf("build: %v\n%s", res.err, res.output)
	}
	if !res.owned || filepath.Base(res.exe) != exeName(1) {
		t.Errorf("result %+v", res)
	}
	out, err := exec.Command(res.exe).Output()
	if err != nil || strings.TrimSpace(string(out)) != "built with teladev" {
		t.Errorf("running the build: %q, %v (the teladev tag must be set)", out, err)
	}
	res.remove()
	if _, err := os.Stat(res.exe); !os.IsNotExist(err) {
		t.Errorf("remove left %s", res.exe)
	}
}

func TestGoBuilderFailure(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip(err)
	}
	dir := tinyModule(t, "package main\n\nfunc main() { undefinedThing() }\n")
	res := tinyBuilder(t, goBin, dir).build(context.Background(), 2)
	if res.err == nil {
		t.Fatal("a broken package built")
	}
	if res.isCancelled() || !strings.Contains(res.output, "undefined: undefinedThing") || !strings.Contains(res.output, "main.go:3") {
		t.Errorf("compiler output %q", res.output)
	}
	if _, err := os.Stat(res.exe); !os.IsNotExist(err) {
		t.Errorf("a failed build left %s", res.exe)
	}
}

// TestGoBuilderCancel cancels a build that does not finish on its own: the
// go command is a script that sleeps, and the build returns once it is
// interrupted.
func TestGoBuilderCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the go command")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip(err)
	}
	fake := filepath.Join(t.TempDir(), "go")
	os.WriteFile(fake, []byte("#!"+sh+"\nexec sleep 30\n"), 0o755)
	b := tinyBuilder(t, fake, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan buildResult, 1)
	go func() { done <- b.build(ctx, 1) }()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	cancel()
	select {
	case res := <-done:
		if !res.isCancelled() {
			t.Errorf("err %v, want context.Canceled", res.err)
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Errorf("cancel took %v", d)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a cancelled build did not return")
	}
}

func TestBuildArgs(t *testing.T) {
	b := &goBuilder{arg: "./examples/x/zz_tela_dev_main", overlay: "/tmp/o.json"}
	got := strings.Join(b.args("/tmp/app-3"), " ")
	want := "build -tags teladev -buildvcs=false -ldflags=-s -w -gcflags=-l -o /tmp/app-3 -overlay /tmp/o.json ./examples/x/zz_tela_dev_main"
	if got != want {
		t.Errorf("args %q, want %q", got, want)
	}
}

// TestDevBuildFlagsGOFLAGS leaves the flags the user sets in GOFLAGS to
// them: the command line would replace them.
func TestDevBuildFlagsGOFLAGS(t *testing.T) {
	for _, c := range []struct{ goflags, want string }{
		{"", "-tags teladev -buildvcs=false -ldflags=-s -w -gcflags=-l"},
		{"-mod=mod", "-tags teladev -buildvcs=false -ldflags=-s -w -gcflags=-l"},
		{"-ldflags=-X=main.version=1", "-tags teladev -buildvcs=false -gcflags=-l"},
		{"-mod=mod --gcflags=all=-N -buildvcs=true", "-tags teladev -ldflags=-s -w"},
		{"-ldflagsx=1", "-tags teladev -buildvcs=false -ldflags=-s -w -gcflags=-l"},
		// An entry quoted whole holds spaces, as the go command splits it.
		{`"-ldflags=-s -w -X=main.v=1" '-gcflags=all=-N -l'`, "-tags teladev -buildvcs=false"},
		// The user's tags follow teladev (the command line replaces them).
		{"-mod=mod -tags=a,b", "-tags teladev,a,b -buildvcs=false -ldflags=-s -w -gcflags=-l"},
	} {
		if got := strings.Join(devBuildFlags(c.goflags), " "); got != c.want {
			t.Errorf("GOFLAGS=%q: %q, want %q", c.goflags, got, c.want)
		}
	}
}

// TestDevTags merges the tags of GOFLAGS into teladev's -tags, reading
// GOFLAGS as the go command does.
func TestDevTags(t *testing.T) {
	for _, c := range []struct{ goflags, want string }{
		{"", "teladev"},
		{"-mod=mod", "teladev"},
		{"-tags=a,b", "teladev,a,b"},
		{"--tags=a", "teladev,a"},
		{"-tags=a,,b,", "teladev,a,b"},
		// One entry is one flag: the go command rejects "a,b" as a
		// non-flag, and a "-tags" without a value.
		{"-tags a,b -mod=mod", "teladev"},
		{"-tags", "teladev"},
		{"tags=a", "teladev"},
		{"---tags=a", "teladev"},
		// The legacy space-separated list, quoted whole in GOFLAGS: a
		// comma is part of a word there, and quotes at the start of a
		// word group it as cmd/internal/quoted does.
		{`"-tags=a b"`, "teladev,a,b"},
		{`'-tags=a b.c'`, "teladev,a,b.c"},
		{`"-tags='a' b"`, "teladev,a,b"},
		{`"-tags=a,b c"`, "teladev,c"},
		{`"-tags='a b' c"`, "teladev,c"},
		// Without a space or a single quote the value is split on commas
		// only: a double quote is part of a tag, which no constraint can
		// name.
		{`-tags="a",b`, "teladev,b"},
		{"-tags=teladev,a,a", "teladev,a"},
		{"-tags=a -tags=b", "teladev,b"}, // the last one wins, as in the go command
		{"-tags=a -tags=", "teladev"},
		{"-tags=", "teladev"},
		{"-tagsx=a", "teladev"},
		{"-tags=ñ_1,go1.25", "teladev,ñ_1,go1.25"},
	} {
		if got := devTags(c.goflags); got != c.want {
			t.Errorf("GOFLAGS=%q: -tags %q, want %q", c.goflags, got, c.want)
		}
	}
}

// TestGoListKeepsGOFLAGSTags lists a package that builds only with the
// user's tags besides teladev.
func TestGoListKeepsGOFLAGSTags(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip(err)
	}
	dir := tinyModule(t, "//go:build teladev && usertag\n\npackage main\n\nfunc main() {}\n")
	pkgs, err := goList(goBin, dir, "-tags=usertag", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "main" || len(pkgs[0].GoFiles) != 1 {
		t.Errorf("go list: %+v, want package main with main.go", pkgs)
	}
}

// TestDevBuildKeepsFileLines checks what -ldflags=-s -w and -gcflags=-l
// must keep: a panic's stack has the file:line of the app's frames, and
// the function names component identity uses (runtime.FuncForPC).
func TestDevBuildKeepsFileLines(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip(err)
	}
	dir := tinyModule(t, `package main

import (
	"fmt"
	"reflect"
	"runtime"
)

func Sidebar() int { return 1 }

func main() {
	fmt.Println(runtime.FuncForPC(reflect.ValueOf(Sidebar).Pointer()).Name())
	var m map[string]int
	m["x"] = 1
}
`)
	res := tinyBuilder(t, goBin, dir).build(context.Background(), 1)
	if res.err != nil {
		t.Fatalf("build: %v\n%s", res.err, res.output)
	}
	defer res.remove()
	out, _ := exec.Command(res.exe).CombinedOutput()
	if !strings.Contains(string(out), "main.Sidebar\n") {
		t.Errorf("FuncForPC lost the name:\n%s", out)
	}
	if !strings.Contains(string(out), "main.go:14") {
		t.Errorf("the panic has no file:line:\n%s", out)
	}
}

// TestGoBuilderKeepsGOFLAGSTags builds with -tags on the command line, which
// replaces the -tags of GOFLAGS: the user's tags must be merged into it.
func TestGoBuilderKeepsGOFLAGSTags(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip(err)
	}
	dir := tinyModule(t, `//go:build teladev && usertag && other

package main

import "fmt"

func main() { fmt.Println("built with both tags") }
`)
	b := tinyBuilder(t, goBin, dir)
	b.env = append(b.env, "GOFLAGS=-mod=mod -tags=usertag,other")
	b.goflags = goEnvFlags(goBin, dir, b.env)
	res := b.build(context.Background(), 1)
	if res.err != nil {
		t.Fatalf("build: %v\n%s", res.err, res.output)
	}
	defer res.remove()
	out, err := exec.Command(res.exe).Output()
	if err != nil || strings.TrimSpace(string(out)) != "built with both tags" {
		t.Errorf("running the build: %q, %v", out, err)
	}
}
