// Command devapp is the app of TestDevEndToEnd: a small screen whose state
// the test follows through reloads. Every component prints its state on
// standard output each time it builds ("devapp: counter n=2 pkg=2"), which
// is how the test sees what a reload kept.
//
// The layout has fixed heights so the test knows where to click:
//
//	  0- 48  header (the class and the text the test edits)
//	 48- 88  Counter: a button and the count
//	 88-128  Mixed: a button, a kept slot and a slot that cannot be kept
//	128-168  Sidebar: a button (the test adds a hook to it)
//	168-208  an uncontrolled Input
//	208-304  a scroll container of 30 rows of 32 px, each a color of its own
package main

import (
	"fmt"
	"os"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// pkgCount lives outside the component tree: App.DevState keeps it.
var pkgCount int

// devState is assigned by devstate_dev.go, built only with the teladev tag.
var devState tela.DevState

// The assets directory is watched by tela dev: the child reports it.
func init() { tela.Assets(os.DirFS("assets")) }

func main() {
	tela.Main(tela.App{
		Title:         "devapp",
		Width:         480,
		Height:        400,
		InstantScroll: true,
		Root:          tela.C(Screen),
		DevState:      devState,
	})
}

func Screen() tela.Node {
	fmt.Println("devapp: screen")
	return Div(Class("flex flex-col"),
		Div(Class("h-12 w-full bg-blue-500 text-white"), Text("Version A")),
		tela.C(Counter),
		tela.C(Mixed),
		tela.C(Sidebar),
		tela.C(Fields),
	)
}

func Counter() tela.Node {
	n, setN := tela.UseState(0)
	fmt.Printf("devapp: counter n=%d pkg=%d\n", n, pkgCount)
	return Div(Class("flex h-10 items-center"),
		Button(Class("h-10 w-32 bg-slate-800 text-white"), OnClick(func() {
			setN.Set(n + 1)
			pkgCount++
		}), Text("Increment")),
		Textf("Count: %d", n),
	)
}

// Mixed has a slot that can be kept and one that cannot (a func): the
// second resets, the first does not.
func Mixed() tela.Node {
	n, setN := tela.UseState(0)
	f, _ := tela.UseState(func() int { return 7 })
	fmt.Printf("devapp: mixed n=%d f=%d\n", n, f())
	return Div(Class("flex h-10 items-center"),
		Button(Class("h-10 w-32 bg-slate-700 text-white"), OnClick(func() { setN.Set(n + 1) }), Text("Mixed")),
		Textf("Mixed: %d", n),
	)
}

func Sidebar() tela.Node {
	n, setN := tela.UseState(0)
	// HOOK
	fmt.Printf("devapp: sidebar n=%d\n", n)
	return Div(Class("flex h-10 items-center"),
		Button(Class("h-10 w-32 bg-slate-600 text-white"), OnClick(func() { setN.Set(n + 1) }), Text("Sidebar")),
		Textf("Sidebar: %d", n),
	)
}

func Fields() tela.Node {
	rows := make([]tela.Child, 30)
	for i := range rows {
		rows[i] = Div(Class(fmt.Sprintf("h-8 w-full bg-[#%02x%02x%02x]", 40+i*7, 200-i*5, 100+i*3)))
	}
	return Div(
		Div(Class("flex h-10"),
			Input(Class("h-10 w-64 border px-2"), Placeholder("name"),
				OnFocus(func() { fmt.Println("devapp: focus") }),
				OnBlur(func() { fmt.Println("devapp: blur") }), OnChange(func(e tela.ChangeEvent) {
					fmt.Printf("devapp: input %q\n", e.Value)
				})),
		),
		Div(append([]tela.Child{Class("h-24 w-64 overflow-y-auto")}, rows...)...),
	)
}
