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
	want := "build -tags teladev -o /tmp/app-3 -overlay /tmp/o.json ./examples/x/zz_tela_dev_main"
	if got != want {
		t.Errorf("args %q, want %q", got, want)
	}
}
