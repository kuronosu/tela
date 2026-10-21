package telatest_test

import (
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// SignIn is a small form with controlled fields.
func SignIn(submitted *[]string) func() tela.Node {
	return func() tela.Node {
		email, setEmail := tela.UseState("")
		pass, setPass := tela.UseState("")
		return Form(Class("flex flex-col gap-2 p-4"),
			OnSubmit(func(tela.SubmitEvent) { *submitted = append(*submitted, email+"|"+pass) }),
			Label(For("email"), Class("text-sm font-medium"), Text("Email")),
			Input(ID("email"), Type("email"), Placeholder("you@example.com"), Bind(email, setEmail),
				Class("w-full rounded-md border border-slate-300 px-3 py-2 placeholder:text-slate-400 focus:outline-2 invalid:border-red-500")),
			Label(Class("text-sm font-medium"), Text("Password "),
				Input(Type("password"), Bind(pass, setPass), Class("rounded-md border border-slate-300 px-3 py-2"))),
			Button(Class("rounded-md bg-indigo-600 px-4 py-2 text-white"), Text("Sign in")),
			P(Textf("chars: %d", len(email))),
		)
	}
}

func TestFieldsTypeAndSubmit(t *testing.T) {
	var submitted []string
	app := telatest.Mount(t, tela.C(SignIn(&submitted)), telatest.Size(400, 300))
	app.ExpectAria(`
		- text: Email
		- textbox "Email"
		- text: Password
		- textbox "Password"
		- button "Sign in"
		- paragraph: "chars: 0"
	`)
	email := telatest.ByRole("textbox", "Email")
	app.Click(email)
	app.Type("ana@example.com")
	app.ExpectValue(email, "ana@example.com")
	app.ExpectText("chars: 15")
	app.Press("Tab")
	app.Type("secret")
	app.ExpectValue(telatest.ByRole("textbox", "Password"), "secret")
	app.Press("Enter")
	if len(submitted) != 1 || submitted[0] != "ana@example.com|secret" {
		t.Fatalf("submitted %q", submitted)
	}
	snap := app.AriaSnapshot()
	if !strings.Contains(snap, `textbox "Email": ana@example.com`) || !strings.Contains(snap, `textbox "Password": ••••••`) {
		t.Errorf("snapshot:\n%s", snap)
	}
	if dir := os.Getenv("TELA_FIELDS_DUMP"); dir != "" {
		f, _ := os.Create(dir + "/signin.png")
		png.Encode(f, app.Screenshot())
		f.Close()
	}
}

func TestFieldsFill(t *testing.T) {
	var changes []string
	app := telatest.Mount(t, tela.C(func() tela.Node {
		v, set := tela.UseState("old")
		return Div(Class("p-4"),
			Input(AriaLabel("Name"), Value(v), OnChange(func(e tela.ChangeEvent) {
				changes = append(changes, e.InputType+":"+e.Value)
				set.Set(e.Value)
			})),
			P(Text("v="+v)))
	}), telatest.Size(300, 120))
	app.Fill(telatest.ByRole("textbox", "Name"), "Ada Lovelace")
	app.ExpectText("v=Ada Lovelace")
	if len(changes) != 1 || changes[0] != "insertText:Ada Lovelace" {
		t.Errorf("changes %q", changes)
	}
	app.Fill(telatest.ByRole("textbox", "Name"), "")
	app.ExpectValue(telatest.ByRole("textbox", "Name"), "")
}
