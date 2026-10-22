package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDevArgs(t *testing.T) {
	o, err := parseDevArgs(nil)
	if err != nil || o != (devOptions{Pkg: "."}) {
		t.Errorf("defaults: %+v, %v", o, err)
	}
	o, err = parseDevArgs([]string{"--size", "300x200", "./x", "--dark", "--preview", "A", "--no-state", "-v", "--scale", "1.5"})
	want := devOptions{Pkg: "./x", W: 300, H: 200, SizeSet: true, Scale: 1.5, Dark: true, Preview: "A", NoState: true, Verbose: true}
	if err != nil || o != want {
		t.Errorf("flags: %+v, %v; want %+v", o, err, want)
	}
	if _, err := parseDevArgs([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: %v", err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"a", "b"}, "expected at most one <pkg>, got 2"},
		{[]string{"--size", "big"}, "invalid --size"},
		{[]string{"--scale", "0"}, ""}, // 0 is the default: the display's scale
		{[]string{"--scale", "-1"}, "invalid --scale"},
		{[]string{"--scale", "100"}, "invalid --scale"},
		{[]string{"--size", "30000x30000", "--scale", "4"}, "too large"},
		{[]string{"--bogus"}, "flag provided but not defined: -bogus (see \"tela help dev\")"},
	} {
		_, err := parseDevArgs(c.args)
		if c.want == "" {
			if err != nil {
				t.Errorf("%v: %v", c.args, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: error %v, want %q", c.args, err, c.want)
		}
	}
}

func TestDevDispatch(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"help", "dev"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "tela dev [--size WxH]") {
		t.Errorf("help dev: %d %q", code, out.String())
	}
	out.Reset()
	if code := run([]string{"dev", "-h"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "--no-state") {
		t.Errorf("dev -h: %d %q", code, out.String())
	}
	errb.Reset()
	if code := run([]string{"dev", "--bogus"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "tela dev: flag provided but not defined") {
		t.Errorf("dev --bogus: %d %q", code, errb.String())
	}
	out.Reset()
	run([]string{"help"}, &out, &errb)
	if !strings.Contains(out.String(), "dev         run an app") {
		t.Errorf("the command list lacks dev:\n%s", out.String())
	}
}

func TestResolveDevTarget(t *testing.T) {
	skipShort(t)
	goBin, _ := exec.LookPath("go")
	root := repoRoot(t)
	resolve := func(pkg, preview string) (*devTarget, error) {
		return resolveDevTarget(goBin, devOptions{Pkg: filepath.Join(root, filepath.FromSlash(pkg)), Preview: preview})
	}

	// A package main is built as is.
	tg, err := resolve("examples/counter", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tg.overlay) != 0 || tg.arg != "./examples/counter" || tg.modDir != root || tg.describe() != "package main" {
		t.Errorf("counter: %+v", tg)
	}

	// A package main with --preview gets an init file in the package.
	tg, err = resolve("examples/dashboard", "2 Dialog")
	if err != nil {
		t.Fatal(err)
	}
	if src := tg.overlay[filepath.Join(root, "examples", "dashboard", devInitFile)]; !bytes.Contains(src, []byte("func init() {")) ||
		!bytes.Contains(src, []byte(`name := "2 Dialog"`)) || !bytes.Contains(src, []byte("telaDevAppOf(App)")) || tg.arg != "./examples/dashboard" {
		t.Errorf("dashboard --preview: arg %q, overlay %v", tg.arg, tg.overlay)
	}

	// A non-main package gets a main package that imports it.
	tg, err = resolve("cmd/tela/testdata/previews", "")
	if err != nil {
		t.Fatal(err)
	}
	src := tg.overlay[filepath.Join(root, "cmd", "tela", "testdata", "previews", devRunnerDir, "main.go")]
	if !bytes.Contains(src, []byte(`telatarget "github.com/kuronosu/tela/cmd/tela/testdata/previews"`)) || tg.arg != "./cmd/tela/testdata/previews/"+devRunnerDir || tg.describe() != "its Previews" {
		t.Errorf("previews: arg %q, source:\n%s", tg.arg, src)
	}

	for _, c := range []struct{ pkg, preview, want string }{
		{"cmd/tela/testdata/empty", "", "cannot be run: it is not a package main"},
		{"cmd/tela/testdata/scene", "", "only defines Scene"},
		{"cmd/tela/testdata/preview", "X", `--preview "X": package preview defines Preview(), a single screen`},
		{"cmd/tela/testdata/apponly", "X", "defines App, whose Root is a single screen"},
		{"examples/counter", "X", `defines Preview(), a single screen`},
		{"examples/nonexistent", "", "cannot find package"},
	} {
		_, err := resolve(c.pkg, c.preview)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s --preview %q: %v, want %q", c.pkg, c.preview, err, c.want)
		}
	}
}

// TestDevRunnersCompile builds the generated runners with -tags teladev and
// checks the one error they report themselves (an unknown preview), which
// exits before tela.Main.
func TestDevRunnersCompile(t *testing.T) {
	skipShort(t)
	goBin, _ := exec.LookPath("go")
	root := repoRoot(t)
	for _, c := range []struct{ pkg, preview string }{
		{"cmd/tela/testdata/preview", ""},
		{"cmd/tela/testdata/previews", "Nope"},
		{"cmd/tela/testdata/apponly", ""},
		{"cmd/tela/testdata/appstruct", ""},
		{"examples/dashboard", "Nope"},
	} {
		t.Run(filepath.Base(c.pkg), func(t *testing.T) {
			tg, err := resolveDevTarget(goBin, devOptions{Pkg: filepath.Join(root, filepath.FromSlash(c.pkg)), Preview: c.preview})
			if err != nil {
				t.Fatal(err)
			}
			b, err := newGoBuilder(goBin, tg, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			res := b.build(context.Background(), 1)
			if res.err != nil {
				t.Fatalf("build: %v\n%s", res.err, res.output)
			}
			if c.preview != "Nope" {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, res.exe).CombinedOutput()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != 1 || !strings.Contains(string(out), `tela dev: unknown preview "Nope"; the package defines: `) {
				t.Errorf("unknown preview: %v, output %q", err, out)
			}
		})
	}
}

func TestListDeps(t *testing.T) {
	skipShort(t)
	goBin, _ := exec.LookPath("go")
	root := repoRoot(t)
	tg, err := resolveDevTarget(goBin, devOptions{Pkg: filepath.Join(root, "examples", "counter")})
	if err != nil {
		t.Fatal(err)
	}
	d, err := listDeps(goBin, tg)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.modules) != 1 || d.modules[0] != root {
		t.Errorf("modules %v, want the tela module only", d.modules)
	}
	// internal/assets/fonts embeds the fonts.
	found := false
	for _, f := range d.embeds {
		found = found || strings.Contains(filepath.ToSlash(f), "internal/assets/fonts/")
	}
	if !found {
		t.Errorf("embedded files %v lack the fonts", d.embeds)
	}
}

// TestDevMainHeadless runs the whole command (devMain) on the headless
// backend with a real go build of testdata/devfake, an app that speaks the
// dev-loop protocol itself and exits with status 0 after a moment: tela dev
// starts it, forwards its output and quits with it.
func TestDevMainHeadless(t *testing.T) {
	skipShort(t)
	goBin, _ := exec.LookPath("go")
	t.Setenv("TELA_BACKEND", "headless")
	t.Setenv("DEVFAKE_EXIT_AFTER", "1s")
	var out, errb syncBuffer
	pkg := filepath.Join(repoRoot(t), "cmd", "tela", "testdata", "devfake")
	done := make(chan int, 1)
	go func() {
		code, err := devMain(goBin, devOptions{Pkg: pkg, Verbose: true, noTerminal: true}, &out, &errb)
		if err != nil {
			t.Errorf("devMain: %v", err)
		}
		done <- code
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code %d", code)
		}
	case <-time.After(2 * time.Minute):
		t.Fatalf("tela dev did not quit; output:\n%s\n%s", out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "devfake: running with count 0") || !strings.Contains(errb.String(), "tela dev: started in") {
		t.Errorf("output:\n%s\nstderr:\n%s", out.String(), errb.String())
	}
	t.Logf("stderr:\n%s", errb.String())
}
