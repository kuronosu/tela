package kit_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func TestDefaultStyleIsACopy(t *testing.T) {
	a := kit.DefaultStyle()
	a.Button.Variants["glass"] = "bg-white/10"
	a.Button.Sizes[kit.Sm] = "h-1"
	a.Card.Root += " backdrop-blur-xl"
	a.Separator.Orientations[kit.Vertical] = "w-2"

	b := kit.DefaultStyle()
	if _, ok := b.Button.Variants["glass"]; ok {
		t.Error("a variant added to one copy shows in the next")
	}
	if b.Button.Sizes[kit.Sm] == "h-1" || strings.Contains(b.Card.Root, "backdrop") || b.Separator.Orientations[kit.Vertical] == "w-2" {
		t.Error("an edit of one copy changed the defaults")
	}
	if b.Button.Root == "" || b.Card.Root == "" || b.Input.Root == "" || b.Field.Root == "" {
		t.Errorf("a recipe is empty: %+v", b)
	}
}

func TestTheme(t *testing.T) {
	th := kit.Theme()
	if th.Radius != 10 {
		t.Errorf("radius %v, want shadcn's 10", th.Radius)
	}
	if _, ok := th.Colors["primary"]; !ok {
		t.Error("the theme has no primary color")
	}
}

func TestStyleProviderChangesTheLook(t *testing.T) {
	s := kit.DefaultStyle()
	s.Button.Variants["glass"] = "bg-red-500 text-white"
	s.Card.Root += " ring-4"
	page := func() tela.Node {
		return h.Div(
			kit.Button{Variant: "glass", Children: h.Text("Plain")},
			kit.StyleProvider{Style: &s, Children: h.Div(
				kit.Button{Variant: "glass", Children: h.Text("Glass")},
				kit.Card{Children: h.Text("Body")})},
		)
	}
	// "glass" is only defined inside the provider: outside it, dev mode
	// reports it with the variants there are.
	_, err := telatest.AriaSnapshot(tela.C(page))
	if err == nil || !strings.Contains(err.Error(), `Button.Variant "glass" is not defined`) {
		t.Fatalf("error %v, want the undefined variant reported", err)
	}
	inside := func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &s, Children: h.Div(
			kit.Button{Variant: "glass", Children: h.Text("Glass")},
			kit.Card{Children: h.Text("Body")})})
	}
	app := telatest.MountFunc(t, inside)
	app.Expect(telatest.ByClass("bg-red-500"), telatest.Disabled(false))
	if n := app.Count(telatest.ByClass("ring-4")); n != 1 {
		t.Errorf("the card has the provider's classes %d times, want 1", n)
	}
}

func TestUndefinedVariantAndSizePanicWithTheClosest(t *testing.T) {
	for _, tc := range []struct {
		root tela.Component
		want string
	}{
		{kit.Button{Variant: "ouline"}, `Button.Variant "ouline" is not defined by the active style, which defines Default, "destructive", "ghost", "link", "outline", "secondary"; did you mean "outline"?`},
		{kit.Button{Size: "sml"}, `did you mean "sm"?`},
		{kit.Badge{Variant: "primary"}, `Badge.Variant "primary"`},
		{kit.Avatar{Size: "huge"}, `Avatar.Size "huge"`},
		{kit.Field{Orientation: "diagonal", Label: "x"}, `Field.Orientation "diagonal"`},
	} {
		_, err := telatest.Render(tc.root)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%T: error %v, want it to contain %q", tc.root, err, tc.want)
		}
		// Outside dev mode the default look is used.
		if _, err := telatest.Render(tc.root, telatest.NoDev()); err != nil {
			t.Errorf("%T without dev mode: %v", tc.root, err)
		}
	}
}

func TestButtonBehavior(t *testing.T) {
	var clicks int
	submitted := 0
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Form(h.OnSubmit(func() { submitted++ }),
			kit.Button{Variant: kit.Outline, OnClick: func() { clicks++ }, Type: "button", Children: h.Text("Save")},
			kit.Button{Disabled: true, OnClick: func() { clicks += 100 }, Children: h.Text("Locked")},
			kit.Button{Type: "submit", Icon: iconCheck, Children: h.Text("Send")},
			kit.Button{Size: kit.Icon, Icon: iconX, AriaLabel: "Close"},
		)
	})
	app.Click(telatest.ByRole("button", "Save"))
	app.Focus(telatest.ByRole("button", "Save"))
	app.Press("Enter")
	app.Press("Space")
	if clicks != 3 {
		t.Errorf("%d clicks, want 3 (pointer, Enter, Space)", clicks)
	}
	app.Expect(telatest.ByRole("button", "Locked"), telatest.Disabled(true))
	app.Click(telatest.ByRole("button", "Send"))
	if submitted != 1 {
		t.Errorf("the submit button submitted the form %d times, want 1", submitted)
	}
	app.Expect(telatest.ByRole("button", "Close"), telatest.Disabled(false))
}

// form is a controlled Input and Textarea and an uncontrolled Input.
type form struct{ seen *[]string }

func (f form) Build() tela.Node {
	name, setName := tela.UseState("")
	bio, setBio := tela.UseState("")
	return h.Div(
		kit.Field{Label: "Name", Children: kit.Input{Value: &name, OnValueChange: func(s string) { setName.Set(strings.ToUpper(s)) }}},
		kit.Field{Label: "Bio", Children: kit.Textarea{Value: &bio, OnValueChange: setBio.Set}},
		kit.Field{Label: "Nick", Children: kit.Input{DefaultValue: "ana", OnValueChange: func(s string) { *f.seen = append(*f.seen, s) }}},
	)
}

func TestInputsFollowTheValueRule(t *testing.T) {
	var seen []string
	app := telatest.Mount(t, form{&seen})
	name := telatest.ByRole("textbox", "Name")
	app.Fill(name, "mei")
	// Controlled: the field shows what the owner of the state made of it.
	app.ExpectValue(name, "MEI")
	app.Fill(telatest.ByRole("textbox", "Bio"), "line one")
	app.ExpectValue(telatest.ByRole("textbox", "Bio"), "line one")
	// Uncontrolled: it starts with DefaultValue and tells of each change.
	nick := telatest.ByRole("textbox", "Nick")
	app.ExpectValue(nick, "ana")
	app.Fill(nick, "anita")
	app.ExpectValue(nick, "anita")
	if len(seen) == 0 || seen[len(seen)-1] != "anita" {
		t.Errorf("OnValueChange saw %v, want it to end with anita", seen)
	}
}

func TestFieldWiresLabelErrorAndDescription(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(
			kit.Field{Label: "Email", Description: "We never share it.", Children: kit.Input{Type: "email"}},
			kit.Field{Label: "Password", Description: "At least 8 characters.", Error: "Too short.", Children: kit.Input{Type: "password"}},
		)
	})
	// The label names the control and focuses it.
	app.Expect(telatest.ByRole("textbox", "Email"), telatest.HasName("Email"))
	app.Click(telatest.ByText("Email", telatest.Exact()))
	app.Expect(telatest.ByRole("textbox", "Email"), telatest.Focused(true))
	app.ExpectCount(telatest.ByRole("alert", ""), 1)
	app.ExpectText("Too short.")
	app.ExpectText("We never share it.")
	// The error describes the field that has one, and the description the
	// other.
	snap := app.AriaSnapshot()
	for _, want := range []string{"We never share it.", "Too short."} {
		if !strings.Contains(snap, want) {
			t.Errorf("the aria snapshot lacks %q:\n%s", want, snap)
		}
	}
}

func TestSeparatorSemantics(t *testing.T) {
	snap, err := telatest.AriaSnapshot(h.Div(kit.Separator{}, kit.Separator{Semantic: true, Orientation: kit.Vertical}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(snap, "separator") != 1 {
		t.Errorf("want only the semantic separator in the tree:\n%s", snap)
	}
}

func TestCardSlotsAndAvatarFallback(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(
			kit.Card{Title: h.Text("Title"), Description: h.Text("Desc"), Children: h.Text("Content"), Footer: h.Text("Footer")},
			kit.Card{Children: h.Text("Only content")},
			kit.Avatar{Fallback: "MT"},
		)
	})
	for _, s := range []string{"Title", "Desc", "Content", "Footer", "Only content", "MT"} {
		app.ExpectText(s)
	}
}
