package telatest

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
)

// The fallback title bar (ADR-238) of a framed window on a desktop whose
// compositor draws no title bars (GNOME on Wayland), through the fake
// window manager's NoServerDecorations: the look in every state, at x1
// and x2. The test is internal to the package because the desktop's
// button layout has no public API: it sets the headless backend's
// settings directly.

// chromeContent is the content under the bar: a column that fills the
// rest of the window (h-full is the content area, not the window).
func chromeContent() tela.Node {
	return h.Div(h.Class("flex h-full flex-col gap-2 bg-muted/40 p-4 text-sm text-foreground"),
		h.Div(h.Class("font-semibold"), h.Text("Inbox")),
		h.Div(h.Class("text-muted-foreground"), h.Text("Three new messages")),
	)
}

func mountChrome(t *testing.T, opts ...Option) *App {
	t.Helper()
	base := []Option{Size(480, 120), Screen(480, 120), NoServerDecorations(), Title("Notes"), RootClass("bg-background")}
	return Mount(t, tela.C(chromeContent), append(base, opts...)...)
}

func TestFallbackTitleBarGoldens(t *testing.T) {
	t.Run("light", func(t *testing.T) {
		app := mountChrome(t)
		app.ExpectRole("button", "Close")
		app.ExpectRole("button", "Maximize")
		app.ExpectRole("button", "Minimize")
		app.GoldenScales("fallback_titlebar_light", 1, 2)
	})
	t.Run("dark", func(t *testing.T) {
		app := mountChrome(t, Dark())
		app.GoldenScales("fallback_titlebar_dark", 1, 2)
	})
	t.Run("unfocused", func(t *testing.T) {
		app := mountChrome(t)
		app.win.Inject(platform.FocusEvent{Focused: false})
		app.settleAfter()
		app.ExpectWindowState(Active(false))
		app.GoldenScales("fallback_titlebar_unfocused", 1, 2)
	})
	t.Run("maximized", func(t *testing.T) {
		app := mountChrome(t)
		app.Maximize()
		app.ExpectRole("button", "Restore")
		app.GoldenScales("fallback_titlebar_maximized", 1, 2)
	})
	t.Run("rtl", func(t *testing.T) {
		app := Mount(t, tela.C(func() tela.Node { return h.Div(h.Dir("rtl"), h.Class("h-full"), chromeContent()) }),
			Size(480, 120), NoServerDecorations(), Title("ملاحظات"), RootClass("bg-background"))
		app.GoldenScales("fallback_titlebar_rtl", 1, 2)
	})
	t.Run("close only", func(t *testing.T) {
		app := mountChrome(t)
		app.backend.SetSettings(platform.SystemSettings{TitleBarButtons: "appmenu:close"})
		app.settleAfter()
		app.ExpectCount(ByRole("button", "Minimize"), 0)
		app.GoldenScales("fallback_titlebar_close_only", 1, 2)
	})
	t.Run("keyboard focus", func(t *testing.T) {
		app := mountChrome(t)
		app.Press("Tab") // the content has nothing to focus: the first button
		app.ExpectFocused(ByRole("button", "Minimize"))
		app.GoldenScales("fallback_titlebar_focus", 1, 2)
	})
	t.Run("hover close", func(t *testing.T) {
		app := mountChrome(t)
		app.Hover(ByRole("button", "Close"))
		app.GoldenScales("fallback_titlebar_hover_close", 1, 2)
	})
}

// Without client-side decorations, or with the app's own title bar, the
// window shows no fallback bar.
func TestFallbackTitleBarAbsent(t *testing.T) {
	app := Mount(t, tela.C(chromeContent), Size(480, 120), Title("Notes"))
	app.ExpectCount(ByRole("button", "Close"), 0)

	own := func() tela.Node {
		return h.Div(h.Class("flex h-full flex-col"),
			h.Div(h.Class("h-8"), h.WindowDrag(), h.Text("My bar")),
			chromeContent())
	}
	app = Mount(t, tela.C(own), Size(480, 120), Title("Notes"), NoServerDecorations())
	app.ExpectWindowState(ClientDecorations(true))
	app.ExpectCount(ByRole("button", "Close"), 0)
	if b := app.Bounds(ByText("My bar")); b.Y != 0 {
		t.Errorf("the app's bar at y=%v, want 0", b.Y)
	}
}
