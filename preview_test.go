package tela_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// session is the state a screen reads from context.
type session struct{ User string }

var sessionCtx = tela.CreateContext(session{User: "guest"})

// inbox is a screen with local state (the open tab, seeded by a field) and
// state from a context (the user, seeded by a Wrap).
type inbox struct{ InitialTab string }

func (m inbox) Build() tela.Node {
	tab, _ := tela.UseState(m.InitialTab)
	s := tela.UseContext(sessionCtx)
	return Div(Textf("%s sees the %s tab", s.User, tab))
}

var inboxPreviews = tela.Previews{
	"open":   {Root: inbox{InitialTab: "open"}},
	"closed": {Root: inbox{InitialTab: "closed"}},
	"mocked": {
		Root: inbox{InitialTab: "open"},
		Wrap: func(n tela.Node) tela.Node { return sessionCtx.Provider(session{User: "Ana"}, n) },
	},
}

func ExamplePreview() {
	// A Preview is a Component: mount it as `tela snapshot` does.
	for _, name := range inboxPreviews.Names() {
		yaml, _ := telatest.AriaSnapshot(inboxPreviews[name])
		fmt.Printf("%s: %s\n", name, strings.TrimPrefix(yaml, "- text: "))
	}
	// Output:
	// closed: guest sees the closed tab
	// mocked: Ana sees the open tab
	// open: guest sees the open tab
}

// A Root may be a node, such as tela.C(fn), as well as a struct component.
func TestPreviewRootKinds(t *testing.T) {
	fn := func() tela.Node {
		n, _ := tela.UseState(4)
		return P(Textf("n=%d", n))
	}
	for name, p := range map[string]tela.Preview{
		"C":      {Root: tela.C(fn)},
		"CP":     {Root: tela.CP(func(n int) tela.Node { return P(Textf("n=%d", n)) }, 4)},
		"struct": {Root: inbox{InitialTab: "t"}},
		"node":   {Root: P(Text("n=4"))},
		"wrapped": {Root: tela.C(fn), Wrap: func(n tela.Node) tela.Node {
			return Div(n)
		}},
	} {
		yaml, err := telatest.AriaSnapshot(p)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if name != "struct" && !strings.Contains(yaml, "n=4") {
			t.Errorf("%s: snapshot %q lacks the text", name, yaml)
		}
	}
}

// Wrap receives the root as a node and its result is what is mounted.
func TestPreviewWrap(t *testing.T) {
	calls := 0
	p := tela.Preview{
		Root: inbox{InitialTab: "x"},
		Wrap: func(n tela.Node) tela.Node {
			calls++
			return Div(Text("wrapped"), n)
		},
	}
	yaml, err := telatest.AriaSnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Error("Wrap was not called")
	}
	if !strings.Contains(yaml, "wrapped") || !strings.Contains(yaml, "guest sees the x tab") {
		t.Errorf("snapshot:\n%s", yaml)
	}
}

// A preview without a Root is a mistake with a fix.
func TestPreviewNeedsRoot(t *testing.T) {
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, "needs a Root") || !strings.Contains(msg, "tela.C(Settings)") {
			t.Errorf("panic message %q", msg)
		}
	}()
	tela.Preview{}.Build()
}
