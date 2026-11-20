package kit_test

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// TestMarkdownLinkCallsOnLink: a press and Enter on a focused link call
// OnLink with the destination as an HTML renderer writes it.
func TestMarkdownLinkCallsOnLink(t *testing.T) {
	t.Parallel()
	var got []string
	a := mdMount(t, kit.Markdown{
		Source: "[docs](https://tela.dev/ü) and [home](/home) and <https://x.org/?q=1&r=2> and [anchor](#top)",
		OnLink: func(href string) { got = append(got, href) },
	})
	a.Click(telatest.ByRole("link", "docs"))
	a.Click(telatest.ByRole("link", "home"))
	a.Click(telatest.ByRole("link", "https://x.org/?q=1&r=2"))
	a.Focus(telatest.ByRole("link", "anchor"))
	a.Press("Enter")
	want := []string{"https://tela.dev/%C3%BC", "/home", "https://x.org/?q=1&r=2", "#top"}
	if !slices.Equal(got, want) {
		t.Errorf("OnLink got %q, want %q", got, want)
	}
}

// TestMarkdownLinkOpensTheURL: without OnLink a link opens with the
// desktop's OpenURI (recorded by the headless backend); a destination the
// desktop cannot open (no scheme) does nothing.
func TestMarkdownLinkOpensTheURL(t *testing.T) {
	t.Parallel()
	b, host, win := rawwin.Mount(t, kit.Markdown{Source: "[site](https://tela.dev/docs?a=1) and [rel](notes/1) and [mail](mailto:me@x.org)"}, 480, 200, 0, core.Config{})
	defer host.Tree().ReceivePosts()()
	click := func(name string) {
		n := rawwin.FindNode(host.Tree().Semantics(), "link", name)
		if n == nil {
			t.Fatalf("no link %q", name)
		}
		x, y := (n.Bounds.X0+n.Bounds.X1)/2, (n.Bounds.Y0+n.Bounds.Y1)/2
		rawwin.Pointer(win, platform.PointerMouse, platform.PointerMove, x, y, platform.ButtonPrimary, false)
		rawwin.Pointer(win, platform.PointerMouse, platform.PointerDown, x, y, platform.ButtonPrimary, true)
		rawwin.Pointer(win, platform.PointerMouse, platform.PointerUp, x, y, platform.ButtonPrimary, false)
		rawwin.Settle(b, host)
	}
	click("site")
	click("rel")
	click("mail")
	var targets []string
	for _, o := range b.Opened() {
		if o.Path {
			t.Errorf("%q opened as a path", o.Target)
		}
		targets = append(targets, o.Target)
	}
	if want := []string{"https://tela.dev/docs?a=1", "mailto:me@x.org"}; !slices.Equal(targets, want) {
		t.Errorf("opened %q, want %q", targets, want)
	}
}
