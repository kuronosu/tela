package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// exampleT stands in for the *testing.T that examples do not have.
type exampleT struct {
	testing.TB // nil: the methods telatest uses are below
	cleanups   []func()
}

func (e *exampleT) Helper()                      {}
func (e *exampleT) Name() string                 { return "example" }
func (e *exampleT) Cleanup(f func())             { e.cleanups = append(e.cleanups, f) }
func (e *exampleT) Logf(string, ...any)          {}
func (e *exampleT) Errorf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Fatalf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Failed() bool                 { return false }
func (e *exampleT) close() {
	for i := len(e.cleanups) - 1; i >= 0; i-- {
		e.cleanups[i]()
	}
}

func ExampleButton() {
	t := &exampleT{}
	defer t.close()
	saved := 0
	page := func() tela.Node {
		return h.Div(h.Class("p-6"),
			kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: func() { saved++ }, Children: h.Text("Save")})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Save"))
	fmt.Println("saved", saved)
	// Output: saved 1
}

func ExampleStyleProvider() {
	t := &exampleT{}
	defer t.close()
	// A recipe is data: copy the defaults, add a variant, give the copy to
	// a subtree.
	style := kit.DefaultStyle()
	style.Button.Variants["glass"] = "bg-white/10 text-white backdrop-blur hover:bg-white/20"
	page := func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &style, Children: h.Div(h.Class("bg-slate-900 p-6"),
			kit.Button{Variant: "glass", Children: h.Text("Glass")})})
	}
	app := telatest.MountFunc(t, page)
	fmt.Println(app.Count(telatest.ByClass("backdrop-blur")))
	// Output: 1
}

// emailForm is a field whose input is controlled by the form's state.
type emailForm struct{}

func (emailForm) Build() tela.Node {
	email, setEmail := tela.UseState("")
	return h.Fragment(kit.Field{
		Label:       "Email",
		Description: "We never share it.",
		Children:    kit.Input{Type: "email", Value: &email, OnValueChange: setEmail.Set},
	})
}

func ExampleField() {
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, emailForm{})
	app.Fill(telatest.ByRole("textbox", "Email"), "ana@example.com")
	app.ExpectValue(telatest.ByRole("textbox", "Email"), "ana@example.com")
	fmt.Println("the label names the input")
	// Output: the label names the input
}
