package telatest_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func TestGoldenUpdatePassFail(t *testing.T) {
	t.Chdir(t.TempDir()) // goldens live in testdata/ of the working directory

	mount := func(tb testing.TB) *telatest.App {
		return telatest.Mount(tb, tela.C(Counter), telatest.Size(200, 120))
	}

	// TELA_UPDATE=1 writes the golden and does not fail.
	t.Setenv("TELA_UPDATE", "1")
	mustPass(t, func(tb testing.TB) { mount(tb).Golden("counter.png") })
	if _, err := os.Stat(filepath.Join("testdata", "counter.png")); err != nil {
		t.Fatalf("TELA_UPDATE=1 did not write the golden: %v", err)
	}
	t.Setenv("TELA_UPDATE", "")

	// The same picture passes; the ".png" extension is optional.
	mustPass(t, func(tb testing.TB) { mount(tb).Golden("counter.png") })
	mustPass(t, func(tb testing.TB) { mount(tb).Golden("counter") })

	// A different picture fails, writing the actual and diff images.
	changed := func(tb testing.TB) *telatest.App {
		app := mount(tb)
		app.Click(telatest.ByRole("button", "Increment"))
		return app
	}
	mustFail(t, func(tb testing.TB) { changed(tb).Golden("counter.png") },
		"golden: counter", "pixel(s) differ", "TELA_UPDATE=1")
	for _, f := range []string{"counter_actual.png", "counter_diff.png"} {
		if _, err := os.Stat(filepath.Join("testdata", f)); err != nil {
			t.Errorf("a failed golden did not write %s: %v", f, err)
		}
	}
	// A tolerance accepts it.
	mustPass(t, func(tb testing.TB) { changed(tb).Golden("counter.png", telatest.Tolerance(255, 1<<30)) })

	// A missing golden says how to create it.
	mustFail(t, func(tb testing.TB) { mount(tb).Golden("missing.png") }, "does not exist", "TELA_UPDATE=1")

	// A different scale is a different size, not a pixel difference.
	mustFail(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(Counter), telatest.Size(200, 120), telatest.Scale(2)).Golden("counter.png")
	}, "size 400x240")
}

func TestAriaGolden(t *testing.T) {
	t.Chdir(t.TempDir())
	mount := func(tb testing.TB) *telatest.App {
		return telatest.Mount(tb, tela.C(Counter), telatest.Size(200, 120))
	}
	t.Setenv("TELA_UPDATE", "1")
	mustPass(t, func(tb testing.TB) { mount(tb).AriaGolden("counter.aria.yml") })
	t.Setenv("TELA_UPDATE", "")
	got, err := os.ReadFile(filepath.Join("testdata", "counter.aria.yml"))
	if err != nil || string(got) != "- 'heading \"Count: 0\" [level=1]'\n- button \"Increment\"\n" {
		t.Fatalf("the aria golden is %q (%v)", got, err)
	}
	mustPass(t, func(tb testing.TB) { mount(tb).AriaGolden("counter.aria.yml") })
	mustFail(t, func(tb testing.TB) {
		app := mount(tb)
		app.Click(telatest.ByRole("button", "Increment"))
		app.AriaGolden("counter.aria.yml")
	}, "output differs")
}

// The pixels of a mounted window are those of the headless entry point, so
// goldens written with either agree.
func TestRenderMatchesTheWindow(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		opts := []telatest.Option{telatest.Size(300, 200), telatest.Scale(scale)}
		img, err := telatest.Render(tela.C(Counter), opts...)
		if err != nil {
			t.Fatal(err)
		}
		app := telatest.Mount(t, tela.C(Counter), opts...)
		win := app.Screenshot()
		if img.Rect != win.Rect || !bytes.Equal(img.Pix, win.Pix) {
			t.Errorf("scale %g: Render and the window's frame differ", scale)
		}
	}
}

func TestHeadlessEntryPoints(t *testing.T) {
	opts := []telatest.Option{telatest.Size(300, 200)}
	yaml, err := telatest.AriaSnapshot(tela.C(Counter), opts...)
	if err != nil {
		t.Fatal(err)
	}
	if want := "- 'heading \"Count: 0\" [level=1]'\n- button \"Increment\""; yaml != want {
		t.Errorf("AriaSnapshot = %q, want %q", yaml, want)
	}
	app := telatest.Mount(t, tela.C(Counter), opts...)
	if yaml != app.AriaSnapshot() {
		t.Error("AriaSnapshot differs from App.AriaSnapshot")
	}

	js, err := telatest.LayoutJSON(tela.C(Counter), opts...)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(js, []byte("}\n")) || !bytes.HasPrefix(js, []byte("{\n  \"kind\": \"root\"")) {
		t.Errorf("LayoutJSON is not indented JSON of the root:\n%.200s", js)
	}
	var tree map[string]any
	if err := json.Unmarshal(js, &tree); err != nil {
		t.Fatalf("LayoutJSON is not JSON: %v", err)
	}
	if !strings.Contains(string(js), "Increment") || !strings.Contains(string(js), "\"text\"") {
		t.Errorf("LayoutJSON lacks the texts:\n%.400s", js)
	}

	// Dark and RootClass reach the render.
	img, err := telatest.Render(tela.C(func() tela.Node { return Div(Class("size-4")) }),
		telatest.Size(20, 20), telatest.Dark(), telatest.RootClass("bg-white dark:bg-black"))
	if err != nil {
		t.Fatal(err)
	}
	if p := img.RGBAAt(10, 10); p.R != 0 || p.G != 0 || p.B != 0 {
		t.Errorf("pixel = %v, want black: Dark and RootClass did not apply", p)
	}
}

func TestHeadlessEntryPointErrors(t *testing.T) {
	boom := tela.C(func() tela.Node {
		tela.UseState(0)
		panic("boom")
	})
	for name, f := range map[string]func() error{
		"Render":       func() error { _, err := telatest.Render(boom); return err },
		"AriaSnapshot": func() error { _, err := telatest.AriaSnapshot(boom); return err },
		"LayoutJSON":   func() error { _, err := telatest.LayoutJSON(boom); return err },
	} {
		if err := f(); err == nil || !strings.Contains(err.Error(), "the UI panicked: boom") {
			t.Errorf("%s: error = %v, want the recovered panic", name, err)
		}
	}
	if _, err := telatest.Render(boom, telatest.Size(0, 10)); err == nil || !strings.Contains(err.Error(), "telatest.Size(0, 10)") {
		t.Errorf("a half-zero size: error = %v", err)
	}
	if _, err := telatest.Render(boom, telatest.Scale(-1)); err == nil || !strings.Contains(err.Error(), "telatest.Scale(-1)") {
		t.Errorf("a negative scale: error = %v", err)
	}
}
