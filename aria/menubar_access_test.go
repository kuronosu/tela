package aria_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/telatest"
)

// Typeahead, access keys and the keyboard's cues of a menu bar (ADR-260).

// accessItems are the items of accessMenu, with their access keys: two
// items share the O.
var (
	accessItems = map[string][]string{
		"file": {"new", "open", "options", "save"},
		"edit": {"undo", "redo"},
		"view": {"zoom", "fullscreen"},
	}
	accessKeys = map[string]rune{"new": 'n', "open": 'o', "options": 'o', "save": 's', "undo": 'u', "redo": 'r', "zoom": 'z', "fullscreen": 'f'}
)

// accessMenu is barMenu with access keys on its items.
type accessMenu struct {
	bar aria.MenuBarResult[string]
	ev  *events
}

func (m accessMenu) Build() tela.Node {
	k := m.bar.OpenKey
	items := accessItems[k]
	pop := aria.UsePopover(m.bar.PopoverOptions(k))
	menu := aria.UseMenu(aria.MenuOptions[string]{
		Keys: items, TextValue: capital, AccessKey: func(i string) rune { return accessKeys[i] },
		OnAction: func(i string) { m.ev.add("action:" + i) },
		OnClose:  m.bar.Close, AutoFocus: true, FocusStrategy: m.bar.FocusStrategy, Ref: m.bar.MenuRef(k),
	})
	return Div(pop.Props, m.bar.MenuProps(k), menu.Props, Class("w-40 bg-white"),
		Range(len(items), func(i int) tela.Node {
			return Div(menu.ItemProps(items[i]), Key(items[i]), Class("h-8 px-2"), Text(capital(items[i])))
		}))
}

// accessPage is a bar of File, Edit, View and Export with typeahead and
// access keys F, E, V and X, a button before it, and the cues shown as a
// text; got is the bar's last result.
func accessPage(o aria.MenuBarOptions[string], ev *events, got *aria.MenuBarResult[string]) func() tela.Node {
	keys := []string{"file", "edit", "view", "export"}
	access := map[string]rune{"file": 'f', "edit": 'e', "view": 'v', "export": 'x'}
	return func() tela.Node {
		o.Keys = keys
		o.AriaLabel = "Application"
		o.TextValue = capital
		o.AccessKey = func(k string) rune { return access[k] }
		bar := aria.UseMenuBar(o)
		*got = bar
		cues := "hidden"
		if bar.ShowAccessKeys {
			cues = "shown"
		}
		return Div(Class("h-full"),
			Button(TestID("before"), Class("h-8 w-20"), Text("Before")),
			Div(bar.Props, Class("flex h-8 gap-1"),
				Range(len(keys), func(i int) tela.Node {
					k := keys[i]
					return Button(bar.TriggerProps(k), Key(k), Class("h-8 w-20"), Text(capital(k)))
				})),
			Div(TestID("cues"), AriaLabel("cues "+cues), Role("status"), Text(cues)),
			Portal(If(bar.Open && accessItems[bar.OpenKey] != nil, tela.WithKey(accessMenu{bar, ev}, bar.OpenKey))))
	}
}

// A character typed on a trigger moves the focus to the next one whose
// text starts with it (the APG menubar's typeahead), without opening a
// menu, in a bar without access keys too; typed again it goes on.
func TestMenuBarTypeahead(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, barPage(aria.MenuBarOptions[string]{TextValue: func(k string) string {
		return map[string]string{"file": "File", "edit": "Edit", "view": "Export"}[k]
	}}, &ev), telatest.Size(400, 300))
	app.Focus(trigger("File"))
	app.Press("e")
	app.ExpectFocused(trigger("Edit")) // the trigger's text is its name's
	app.Press("e")
	app.ExpectFocused(telatest.ByRole("menuitem", "View")) // "Export"
	app.Press("e")
	app.ExpectFocused(trigger("Edit")) // around the end
	app.ExpectCount(telatest.ByRole("menu", ""), 0)
	// A longer search stays on a trigger that still matches.
	app.Advance(1100 * time.Millisecond)
	app.Press("e")
	app.Press("x")
	app.ExpectFocused(telatest.ByRole("menuitem", "View"))
}

// With ActivationKeys Alt and an access key open that menu from anywhere
// in the window, on its first item; the key alone does so on a focused
// trigger, before typeahead; an item's key in the open menu chooses it,
// and Alt and another menu's key moves to that menu.
func TestMenuBarAccessKeys(t *testing.T) {
	var ev events
	var bar aria.MenuBarResult[string]
	app := telatest.MountFunc(t, accessPage(aria.MenuBarOptions[string]{ActivationKeys: true}, &ev, &bar),
		telatest.Size(480, 320), telatest.PlatformKeys("linux"))
	before := telatest.ByTestID("before")
	app.Focus(before)
	app.Press("Alt+e")
	if !bar.IsOpen("edit") {
		t.Fatalf("Alt+E: open %q", bar.OpenKey)
	}
	app.ExpectFocused(telatest.ByRole("menuitem", "Undo"))
	if _, ok := bar.FocusedBefore(); !ok {
		t.Errorf("no FocusedBefore after Alt+E from the Before button")
	}
	// Another menu's key with Alt, from inside the open menu.
	app.Press("Alt+v")
	if !bar.IsOpen("view") {
		t.Fatalf("Alt+V in the Edit menu: open %q", bar.OpenKey)
	}
	app.ExpectFocused(telatest.ByRole("menuitem", "Zoom"))
	// An item's key alone chooses it, and closes the menu.
	app.Press("f")
	if got := ev.take(); got != "action:fullscreen" {
		t.Errorf("F in the View menu: %q", got)
	}
	// Two items with the key take turns getting the focus.
	app.Press("Alt+f")
	app.ExpectFocused(telatest.ByRole("menuitem", "New"))
	app.Press("o")
	app.ExpectFocused(telatest.ByRole("menuitem", "Open"))
	app.Press("o")
	app.ExpectFocused(telatest.ByRole("menuitem", "Options"))
	app.Press("o")
	app.ExpectFocused(telatest.ByRole("menuitem", "Open"))
	if got := ev.take(); got != "" {
		t.Errorf("a shared key chose %q", got)
	}
	// Alt and a key of no item and no menu does nothing in the menu (no
	// typeahead to Options).
	app.Press("Alt+p")
	app.ExpectFocused(telatest.ByRole("menuitem", "Open"))
	app.Press("Escape")
	app.ExpectFocused(trigger("File"))
	// On a focused trigger, the key alone opens its menu.
	app.Press("x")
	if !bar.IsOpen("export") {
		t.Errorf("X on the File trigger: open %q", bar.OpenKey)
	}
}

// Without ActivationKeys Alt and a key are not the bar's, and under the
// Mac flavor there are no access keys at all: the key on a trigger is
// typeahead.
func TestMenuBarAccessKeysScope(t *testing.T) {
	for _, c := range []struct {
		name string
		keys bool
		goos string
	}{{"no ActivationKeys", false, "linux"}, {"mac", true, "darwin"}} {
		t.Run(c.name, func(t *testing.T) {
			var ev events
			var bar aria.MenuBarResult[string]
			app := telatest.MountFunc(t, accessPage(aria.MenuBarOptions[string]{ActivationKeys: c.keys}, &ev, &bar),
				telatest.Size(480, 320), telatest.PlatformKeys(c.goos))
			app.Focus(telatest.ByTestID("before"))
			app.Press("Alt+e")
			if bar.Open {
				t.Errorf("Alt+E opened %q", bar.OpenKey)
			}
			app.Focus(trigger("File"))
			app.Press("x")
			if c.keys && bar.Open {
				t.Errorf("X opened %q under the Mac flavor", bar.OpenKey)
			}
			if !c.keys && !bar.IsOpen("export") {
				// The key alone on a focused trigger works without
				// ActivationKeys: only the Alt combinations need them.
				t.Errorf("X on a trigger without ActivationKeys: open %q", bar.OpenKey)
			}
		})
	}
}

// The cues are shown while Alt is held, and while the keyboard is in the
// bar since F10, a lone Alt or an access key brought it there; never
// under the Mac flavor.
func TestMenuBarShowAccessKeys(t *testing.T) {
	for _, fl := range []platform.Flavor{platform.FlavorLinux, platform.FlavorMac} {
		t.Run(fl.String(), func(t *testing.T) {
			var ev events
			var bar aria.MenuBarResult[string]
			b, host, win := rawwin.Mount(t, tela.C(accessPage(aria.MenuBarOptions[string]{ActivationKeys: true}, &ev, &bar)), 480, 320, 0, core.Config{Flavor: fl})
			app := &rawApp{t: t, b: b, host: host, win: win, tree: host.Tree()}
			want := func(what string, shown bool) {
				t.Helper()
				if fl == platform.FlavorMac {
					shown = false
				}
				if bar.ShowAccessKeys != shown {
					t.Errorf("%s: ShowAccessKeys %v, want %v", what, bar.ShowAccessKeys, shown)
				}
			}
			altDown := platform.KeyEvent{Action: platform.KeyPress, Key: platform.KeyAlt, Code: platform.CodeAltLeft, Mods: platform.ModAlt}
			altUp := platform.KeyEvent{Action: platform.KeyRelease, Key: platform.KeyAlt, Code: platform.CodeAltLeft}
			send := func(e platform.KeyEvent) { win.Inject(e); app.settle() }
			want("at rest", false)
			send(altDown)
			want("Alt held", true)
			// A key that is no access key cancels the lone Alt, not the cues.
			send(platform.KeyEvent{Action: platform.KeyPress, Key: "q", Code: platform.CodeKeyQ, Mods: platform.ModAlt})
			send(platform.KeyEvent{Action: platform.KeyRelease, Key: "q", Code: platform.CodeKeyQ, Mods: platform.ModAlt})
			want("Alt+Q held", true)
			send(altUp)
			want("Alt released after Alt+Q", false)
			// A lone Alt brings the keyboard to the bar: the cues stay.
			send(altDown)
			send(altUp)
			want("after a lone Alt", true)
			app.key(platform.KeyPress, platform.KeyEscape, platform.CodeEscape, false)
			want("after Escape", false)
			// Alt held, then the window loses the focus (Alt+Tab).
			send(altDown)
			win.SetFocused(false)
			app.settle()
			want("Alt+Tab away", false)
			win.SetFocused(true)
			app.settle()
		})
	}
}

// The focus that comes to the bar from nowhere (nothing had it) leaves no
// origin, while the focus a closing menu gives back to its trigger keeps
// the one there was.
func TestMenuBarFocusedBeforeFromNowhere(t *testing.T) {
	var ev events
	var bar aria.MenuBarResult[string]
	var setOff tela.Setter[bool]
	page := accessPage(aria.MenuBarOptions[string]{}, &ev, &bar)
	app := telatest.MountFunc(t, func() tela.Node {
		off, so := tela.UseState(false)
		setOff = so
		return Div(Class("h-full"), Inert(off), tela.C(page))
	}, telatest.Size(480, 320))
	before := telatest.ByTestID("before")
	app.Focus(before)
	app.Press("Tab") // File
	app.ExpectFocused(trigger("File"))
	if _, ok := bar.FocusedBefore(); !ok {
		t.Fatalf("no origin after Tab from Before")
	}
	// The page goes inert for a moment: File is blurred to nothing, and
	// the focus then comes back to the bar from nowhere.
	setOff.Set(true)
	app.ExpectCount(trigger("File"), 0) // hidden from the semantics tree too
	if f := app.AriaSnapshot(); f != "" {
		t.Logf("inert page: %q", f)
	}
	setOff.Set(false)
	app.ExpectCount(trigger("File"), 1)
	app.Expect(trigger("File"), telatest.Focused(false))
	app.Focus(trigger("File"))
	if f, ok := bar.FocusedBefore(); ok {
		t.Errorf("origin %v kept for a focus that came from nowhere", f)
	}
	// The focus a closing menu gives back keeps the origin.
	app.Focus(before)
	app.Press("Tab")
	app.Press("ArrowDown")
	app.ExpectFocused(telatest.ByRole("menuitem", "New"))
	app.Press("Escape") // the menu unmounts; its scope focuses File
	app.ExpectFocused(trigger("File"))
	if _, ok := bar.FocusedBefore(); !ok {
		t.Errorf("the origin went with the menu's restoration")
	}
}

// The keys of a context menu with nothing focused in the window open it
// at the window's top-left corner plus one pixel (Chromium's non-located
// menu); a controlled menu the app opens itself appears at Position, else
// where the keys open it.
func TestContextMenuNonLocated(t *testing.T) {
	var ev events
	var cm aria.ContextMenuTriggerResult
	var setOpen tela.Setter[bool]
	var setPos tela.Setter[*tela.Point]
	app := telatest.MountFunc(t, func() tela.Node {
		open, so := tela.UseState(false)
		pos, sp := tela.UseState[*tela.Point](nil)
		setOpen, setPos = so, sp
		cm = aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{Open: &open, OnOpenChange: so.Set, Position: pos})
		return Div(cm.TriggerProps, Class("relative h-full p-10"),
			Button(TestID("inner"), Class("h-8 w-20"), Text("Inner")),
			Portal(If(cm.Open, ctxMenu{cm, &ev})))
	}, telatest.Size(400, 300))
	app.Press("Shift+F10")
	app.ExpectCount(ctxMenuLoc, 1)
	if a := cm.Anchor(); a.X != 1 || a.Y != 1 {
		t.Errorf("anchor with nothing focused %+v, want (1, 1)", a)
	}
	app.Press("Escape")
	app.ExpectCount(ctxMenuLoc, 0)

	// The app opens it: at the focused element, as the keys do.
	app.Focus(telatest.ByTestID("inner"))
	b := app.Bounds(telatest.ByTestID("inner"))
	setOpen.Set(true)
	app.ExpectCount(ctxMenuLoc, 1)
	if a := cm.Anchor(); a.X != b.X+b.Width/2 || a.Y != b.Y+b.Height/2 {
		t.Errorf("app opening at %+v, want the center of %+v", a, b)
	}
	setOpen.Set(false)
	app.ExpectCount(ctxMenuLoc, 0)
	// At Position, and moved with it.
	setPos.Set(&tela.Point{X: 120, Y: 90})
	setOpen.Set(true)
	app.ExpectCount(ctxMenuLoc, 1)
	if m := app.Bounds(ctxMenuLoc); m.X != 120 || m.Y != 90 {
		t.Errorf("menu at (%v, %v), want Position (120, 90)", m.X, m.Y)
	}
	setPos.Set(&tela.Point{X: 60, Y: 40})
	app.Advance(time.Millisecond)
	if m := app.Bounds(ctxMenuLoc); m.X != 60 || m.Y != 40 {
		t.Errorf("menu at (%v, %v), want the new Position (60, 40)", m.X, m.Y)
	}
	// SetOpen(true) is an opening with no point too.
	setOpen.Set(false)
	app.ExpectCount(ctxMenuLoc, 0)
	cm.SetOpen(true)
	app.ExpectCount(ctxMenuLoc, 1)
	if m := app.Bounds(ctxMenuLoc); m.X != 60 || m.Y != 40 {
		t.Errorf("SetOpen(true): menu at (%v, %v), want Position (60, 40)", m.X, m.Y)
	}
	// A right click still opens it at the pointer.
	setOpen.Set(false)
	app.ExpectCount(ctxMenuLoc, 0)
	app.ClickAt(200, 150, telatest.Right())
	app.ExpectCount(ctxMenuLoc, 1)
	if m := app.Bounds(ctxMenuLoc); m.X != 200 || m.Y != 150 {
		t.Errorf("right click: menu at (%v, %v), want (200, 150)", m.X, m.Y)
	}
}

// AccessKeys: the bar's rule (Windows' native bars) and a menu's own keys.
func TestAccessKeys(t *testing.T) {
	items := []aria.AccessKeyItem{{Label: "File"}, {Label: "Format"}, {Label: "Fit", Disabled: true}, {Label: "ファイル"}, {Label: "編集", Key: 'e'}, {Label: "Window", Key: 'd'}}
	var got []string
	for i, ak := range aria.AccessKeys(items, true) {
		b, k, a := ak.Split(items[i].Label)
		got = append(got, b+"["+k+"]"+a)
	}
	// File takes F, Format its first free letter (o), a disabled item none,
	// a Japanese label an appended free letter, an own key where the label
	// has it or appended.
	if s := strings.Join(got, " "); s != "[F]ile F[o]rmat Fit[] ファイル([A]) 編集([E]) Win[d]ow" {
		t.Errorf("auto: %s", s)
	}
	got = got[:0]
	for i, ak := range aria.AccessKeys(items, false) {
		b, k, a := ak.Split(items[i].Label)
		got = append(got, b+"["+k+"]"+a)
	}
	if s := strings.Join(got, " "); s != "File[] Format[] Fit[] ファイル[] 編集([E]) Win[d]ow" {
		t.Errorf("own keys only: %s", s)
	}
}

// Only a typed non-ASCII character falls back to the physical key: a
// dead key, an IME's Process or an Unidentified key on the key of F do
// not choose the F item; a Cyrillic "а" on the key of F does (review of
// ADR-260).
func TestAccessKeyPhysicalFallback(t *testing.T) {
	var ev events
	var bar aria.MenuBarResult[string]
	b, host, win := rawwin.Mount(t, tela.C(accessPage(aria.MenuBarOptions[string]{ActivationKeys: true}, &ev, &bar)), 480, 320, 0, core.Config{Flavor: platform.FlavorLinux})
	app := &rawApp{t: t, b: b, host: host, win: win, tree: host.Tree()}
	send := func(k platform.Key, c platform.KeyCode) {
		e := platform.KeyEvent{Action: platform.KeyPress, Key: k, Code: c}
		if !k.IsNamed() {
			e.Text = string(k)
		}
		win.Inject(e)
		app.settle()
		win.Inject(platform.KeyEvent{Action: platform.KeyRelease, Key: k, Code: c})
		app.settle()
	}
	bar.SetOpen("view", true) // Zoom (Z), Fullscreen (F)
	app.settle()
	for _, k := range []platform.Key{platform.KeyDead, platform.KeyProcess, platform.KeyUnidentified} {
		send(k, platform.CodeKeyF)
		if got := ev.take(); got != "" {
			t.Errorf("%s on the key of F chose %q", k, got)
		}
	}
	send("а", platform.CodeKeyF)
	if got := ev.take(); got != "action:fullscreen" {
		t.Errorf("a Cyrillic а on the key of F: %q, want the F item", got)
	}
}
