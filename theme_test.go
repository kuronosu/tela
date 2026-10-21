package tela_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// themeAliases are the class aliases of the theme tests.
var themeAliases = map[string]string{
	"text-muted": "text-sm text-muted-foreground",
	"card":       "flex w-[300px] flex-col gap-4 rounded-xl border border-border bg-card p-5 text-card-foreground shadow-sm",
	"btn":        "inline-flex h-9 items-center justify-center rounded-md px-4 text-sm font-medium shadow-xs",
}

// themeCard is a shadcn/ui-styled card written with tokens and aliases.
func themeCard() tela.Node {
	return Div(Class("card"),
		Div(Class("flex flex-col gap-1.5"),
			H3(Class("text-base font-semibold leading-none"), Text("Create project")),
			P(Class("text-muted"), Text("Deploy your new project in one click.")),
		),
		Input(AriaLabel("Name"), Placeholder("Name of your project"),
			Class("h-9 rounded-md border border-input px-3 text-sm shadow-xs placeholder:text-muted-foreground dark:bg-input/30")),
		Div(Class("flex justify-end gap-2"),
			Button(Class("btn border border-border bg-background dark:border-input dark:bg-input/30"), Text("Cancel")),
			Button(Class("btn bg-primary text-primary-foreground hover:bg-primary/90"), Text("Deploy")),
		),
	)
}

func themeOpts(extra ...telatest.Option) []telatest.Option {
	th := tela.DefaultTheme()
	th.Radius = 10
	return append([]telatest.Option{telatest.Size(340, 300), telatest.Theme(th), telatest.ClassAliases(themeAliases),
		telatest.RootClass("flex items-center justify-center bg-background text-foreground")}, extra...)
}

// TestThemeGoldens renders the card in a light app, in a dark app and in a
// dark Scheme panel inside a light app, at scales 1 and 2.
func TestThemeGoldens(t *testing.T) {
	app := telatest.MountFunc(t, themeCard, themeOpts()...)
	app.GoldenScales("theme_light", 1, 2)

	app = telatest.MountFunc(t, themeCard, themeOpts(telatest.ColorScheme(tela.Dark))...)
	app.GoldenScales("theme_dark", 1, 2)

	panel := func() tela.Node {
		return Div(Class("flex w-full flex-col"),
			P(Class("py-3 text-center text-muted"), Text("A light app")),
			Div(Scheme(tela.Dark), Class("flex justify-center bg-background py-5 text-foreground"), themeCard()),
		)
	}
	app = telatest.MountFunc(t, panel, themeOpts(telatest.Size(340, 360))...)
	app.GoldenScales("theme_scheme_panel", 1, 2)
}

// TestUseThemeAndSetTheme: UseTheme follows the scheme at the component's
// position and SetTheme replaces the theme of the running app.
func TestUseThemeAndSetTheme(t *testing.T) {
	var seen []tela.Color
	reader := func() tela.Node {
		seen = append(seen, tela.UseTheme().Color("brand"))
		return Div()
	}
	th := tela.DefaultTheme()
	th.Colors["brand"] = tela.SchemeColor{Light: tela.Hex("#7c3aed"), Dark: tela.Hex("#a78bfa")}
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(tela.C(reader), Div(Scheme(tela.Dark), tela.C(reader)))
	}, telatest.Theme(th))
	if len(seen) != 2 || seen[0] != tela.Hex("#7c3aed") || seen[1] != tela.Hex("#a78bfa") {
		t.Fatalf("UseTheme reads %v", seen)
	}
	th.Colors["brand"] = tela.SchemeColor{Light: tela.Hex("#000000"), Dark: tela.Hex("#ffffff")}
	tela.SetTheme(th)
	app.WaitFor("the readers to build again", func() bool { return len(seen) == 4 })
	if len(seen) != 4 || seen[2] != tela.Black || seen[3] != tela.White {
		t.Fatalf("after SetTheme: %v", seen)
	}
}
