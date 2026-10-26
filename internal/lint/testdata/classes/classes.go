// Package classes is a fixture of the class rule: every line that ends in a
// "want" comment must be reported, with the rule and a part of the message,
// and no other line may be.
package classes

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

const (
	cardTypo = "rounded-xxl p-4"
	base     = "flex items-center "
	good     = base + "gap-2"
	bad      = base + "gapp-2"
)

func Sinks(dynamic string, n int) []tela.Node {
	return []tela.Node{
		h.Div(h.Class("flex items-center gap-2 hover:bg-white/10 md:p-4 dark:bg-[#1e1e2e]")),
		h.Div(h.Class("flex rounded-xxl")),         // want class `unknown class "rounded-xxl"; did you mean "rounded-2xl"?`
		h.Div(h.Class("flex", "bg-whte", dynamic)), // want class `unknown class "bg-whte"`
		h.Div(h.Class(cardTypo)),                   // want class `unknown class "rounded-xxl"`
		h.Div(h.Class(good)),
		h.Div(h.Class(bad)),                              // want class `unknown class "gapp-2"; did you mean "gap-2"?`
		h.Div(h.Class("flex " + "gapp-3")),               // want class `unknown class "gapp-3"`
		h.Div(h.Class("hover:bg-reed-500")),              // want class `unknown class "hover:bg-reed-500"`
		h.Div(h.Class("hovr:bg-red-500")),                // want class `unknown variant "hovr:"`
		h.Div(h.Class("w-[calc(100%-" + dynamic + ")]")), // computed: skipped
		h.Div(h.Class("p-"+itoa(n), "flexx")),            // want class `unknown class "flexx"`
		h.Div(h.Class("flex"), h.Class("itms-center")),   // want class `unknown class "itms-center"`
		h.Div(h.Exit("opacity-0 scal-95")),               // want class `unknown class "scal-95"`
		h.Div(h.Class("bg-[oops]")),                      // want class `bg-[oops]`
		h.Div(h.Class("two-typos-a two-typos-b")),        // want class `unknown class "two-typos-a"` class `unknown class "two-typos-b"`
		h.Div(h.Class(dynamicList()...)),
	}
}

func dynamicList() []string { return nil }

func itoa(n int) string { return "" }

func Kit() []tela.Node {
	return []tela.Node{
		h.Div(kit.Button{Class: "px-4 rounded-md"}),
		h.Div(kit.Button{Class: "px-4 rouded-md"}),                   // want class `unknown class "rouded-md"; did you mean "rounded-md"?`
		h.Div(kit.Card{HeaderClass: "pb-2", TitleClass: "text-lgg"}), // want class `unknown class "text-lgg"`
		h.Div(kit.Card{Class: cardTypo}),                             // want class `unknown class "rounded-xxl"`
		h.Div(kit.Input{Class: "w-full", Type: "text"}),
	}
}

// Recipes are checked field by field, in literals and in assignments.
func Recipes() {
	s := kit.DefaultStyle()
	s.Button.Root = "inline-flex gapp-2"                                       // want class `unknown class "gapp-2"`
	s.Button.Root += " items-center"                                           // fine
	s.Button.Variants[kit.Ghost] = "hover:bg-accentt"                          // want class `unknown class "hover:bg-accentt"`
	s.Button.Sizes = map[kit.Size]string{"": "h-9 px-4", kit.Sm: "h-8 pxx-3"}  // want class `unknown class "pxx-3"`
	s.Card = kit.CardStyle{Root: "rounded-xl border", Title: "font-semiboldd"} // want class `unknown class "font-semiboldd"`
	var c kit.Card
	c.Class = "p-44x" // want class `unknown class "p-44x"`
	_, _ = s, c
}

func App() tela.App {
	return tela.App{
		Title:     "x",
		RootClass: "bg-background text-foregroundd", // want class `unknown class "text-foregroundd"`
	}
}

func Test(t telatest.Option) []telatest.Option {
	return []telatest.Option{telatest.RootClass("bg-red-50"), telatest.RootClass("bg-rd-50")} // want class `unknown class "bg-rd-50"`
}

// Ignored findings.
func Ignored() []tela.Node {
	return []tela.Node{
		h.Div(h.Class("rounded-xxl")), //tela:ignore class
		//tela:ignore class -- the reason may follow
		h.Div(h.Class("rounded-xxl")),
		//tela:ignore role,class
		h.Div(h.Class("rounded-xxl")),
		h.Div(h.Class("rounded-xxl")), // want class `unknown class "rounded-xxl"`
		//tela:ignore hooks
		h.Div(h.Class("rounded-xxl")), // want class `unknown class "rounded-xxl"`
		h.Div(h.Class("ok")),          //tela:ignore class
		h.Div(h.Class("rounded-xxl")), // want class `unknown class "rounded-xxl"`
		//tela:ignore
		h.Div(h.Class("rounded-xxl")),
	}
}
