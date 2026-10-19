package tela_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// t stands in for the *testing.T of a real test in the examples that
// mount an app, so that they read like test bodies.
var t testing.TB = exampleTB{}

type exampleTB struct{ testing.TB }

func (exampleTB) Helper()                      {}
func (exampleTB) Name() string                 { return "example" }
func (exampleTB) Failed() bool                 { return false }
func (exampleTB) Cleanup(func())               {}
func (exampleTB) Logf(string, ...any)          {}
func (exampleTB) Errorf(f string, args ...any) { fmt.Printf("error: "+f+"\n", args...) }
func (exampleTB) Fatalf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (exampleTB) Error(args ...any)            { fmt.Println(append([]any{"error:"}, args...)...) }
func (exampleTB) Fatal(args ...any)            { panic(fmt.Sprint(args...)) }
func (exampleTB) Log(...any)                   {}

func ExampleUseColorScheme() {
	theme := func() tela.Node {
		name := "light"
		if tela.UseColorScheme() == tela.Dark {
			name = "dark"
		}
		return P(Textf("Theme: %s", name))
	}
	// The system prefers dark (telatest.Dark), the app follows it.
	yaml, _ := telatest.AriaSnapshot(tela.C(theme), telatest.Dark())
	fmt.Println(yaml)
	// The app overrides the system (tela.App.ColorScheme).
	yaml, _ = telatest.AriaSnapshot(tela.C(theme), telatest.Dark(), telatest.ColorScheme(tela.Light))
	fmt.Println(yaml)
	// Output:
	// - paragraph: "Theme: dark"
	// - paragraph: "Theme: light"
}

func ExampleSetColorScheme() {
	toggle := func() tela.Node {
		dark := tela.UseColorScheme() == tela.Dark
		next, label := tela.Dark, "Dark mode"
		if dark {
			next, label = tela.Light, "Light mode"
		}
		return Button(Class("bg-white text-black dark:bg-black dark:text-white"),
			OnClick(func(tela.MouseEvent) { tela.SetColorScheme(next) }), Text(label))
	}
	app := telatest.Mount(t, tela.C(toggle))
	defer app.Close()
	app.Click(telatest.ByRole("button", "Dark mode"))
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - button "Light mode"
}

func ExampleUseReducedMotion() {
	spinner := func() tela.Node {
		if tela.UseReducedMotion() {
			return P(Text("Loading…"))
		}
		return Div(Class("size-6 animate-spin rounded-full border-2 border-t-transparent"), AriaLabel("Loading"), Role("status"))
	}
	yaml, _ := telatest.AriaSnapshot(tela.C(spinner), telatest.ReducedMotion())
	fmt.Println(yaml)
	// Output:
	// - paragraph: Loading…
}
