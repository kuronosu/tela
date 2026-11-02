package headless

import (
	"errors"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestAppMenu(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()
	menu := []platform.MenuItem{
		{Label: "File", Submenu: []platform.MenuItem{{ID: 1, Label: "New", Accelerator: "Cmd+N"}, {ID: 2, Label: "Gone", Disabled: true}}},
		{Label: "Edit", Role: platform.RoleEditMenu, Submenu: []platform.MenuItem{{ID: 3, Label: "Copy", Role: platform.RoleCopy}}},
	}
	items := []platform.MenuItem{{ID: 4, Label: "Settings…", Accelerator: "Cmd+,"}}
	if m, it := b.AppMenu(); m != nil || it != nil || b.AppMenuChanges() != 0 {
		t.Fatalf("a new backend has an app menu: %v %v", m, it)
	}
	if err := d.SetAppMenu(menu, items); err != nil {
		t.Fatal(err)
	}
	menu[0].Submenu[0].Label, items[0].Label = "changed", "changed" // SetAppMenu copied them
	gotMenu, gotItems := b.AppMenu()
	if gotMenu[0].Submenu[0].Label != "New" || gotMenu[1].Role != platform.RoleEditMenu || gotMenu[1].Submenu[0].Role != platform.RoleCopy ||
		len(gotItems) != 1 || gotItems[0].Label != "Settings…" || b.AppMenuChanges() != 1 {
		t.Errorf("AppMenu = %+v, %+v (changes %d)", gotMenu, gotItems, b.AppMenuChanges())
	}

	r := &recorder{}
	b.ChooseAppMenu(1)
	b.InjectApp(platform.AppMenuEvent{Item: 4}) // an app item
	b.ChooseAppMenu(3)                          // a role item: headless has no native action
	b.Drain(r)
	if got := r.take(); got != "app:appmenu(1) app:appmenu(4) app:appmenu(3)" {
		t.Errorf("choices = %s", got)
	}
	expectPanic(t, "disabled", func() { b.ChooseAppMenu(2) })
	expectPanic(t, "no item 9", func() { b.ChooseAppMenu(9) })
	expectPanic(t, "reuses ID 1", func() { _ = d.SetAppMenu(menu, []platform.MenuItem{{ID: 1, Label: "x"}}) })

	// SetAppMenu(nil, nil) restores the default bar.
	if err := d.SetAppMenu(nil, nil); err != nil {
		t.Fatal(err)
	}
	if m, it := b.AppMenu(); m != nil || it != nil || b.AppMenuChanges() != 2 {
		t.Errorf("after SetAppMenu(nil, nil): %v %v", m, it)
	}
	expectPanic(t, "SetAppMenu first", func() { b.ChooseAppMenu(1) })

	b.SetUnsupported(ServiceAppMenu)
	if err := d.SetAppMenu(menu, nil); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SetAppMenu with ServiceAppMenu unsupported = %v", err)
	}
	if b.AppMenuChanges() != 2 {
		t.Errorf("an unsupported SetAppMenu was recorded")
	}
}

func TestRequestQuit(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	r := &recorder{}
	b.Drain(r)
	r.take()
	b.RequestQuit()
	b.Drain(r)
	if got := r.take(); got != "app:quit-requested" {
		t.Errorf("RequestQuit delivered %s", got)
	}
	if w.Closed() {
		t.Errorf("a quit request closed a window; quitting is the app's answer")
	}
}

// TestTitleBarOverlay: an overlay window is honored like a frameless one
// (client-side decorations, no window menu bar), with the controls'
// rectangle from Options.TitleBarControls and SetTitleBarControls.
func TestTitleBarOverlay(t *testing.T) {
	controls := platform.Rect{X: 12, Y: 10, W: 54, H: 16}
	b := New(Options{TitleBarControls: controls})
	o := newWindow(t, b, platform.WindowOptions{Title: "o", TitleBarOverlay: true})
	plain := newWindow(t, b, platform.WindowOptions{Title: "p"})
	frameless := newWindow(t, b, platform.WindowOptions{Title: "f", Frameless: true})
	r := &recorder{}
	b.Drain(r)
	want := platform.WindowState{ClientSideDecorations: true, TitleBarControls: controls}
	if o.State() != want {
		t.Errorf("overlay window state = %v, want %v", o.State(), want)
	}
	if plain.State() != (platform.WindowState{}) || frameless.State() != (platform.WindowState{ClientSideDecorations: true}) {
		t.Errorf("other windows: %v, %v; the controls are an overlay window's only", plain.State(), frameless.State())
	}
	if err := o.SetMenu([]platform.MenuItem{{ID: 1, Label: "x"}}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SetMenu on an overlay window = %v, want ErrUnsupported (as frameless)", err)
	}

	r.take()
	moved := platform.Rect{X: 20, Y: 18, W: 54, H: 16}
	o.SetTitleBarControls(moved)
	o.SetTitleBarControls(moved) // unchanged: no event
	b.Drain(r)
	if got := r.take(); got != "o:state{csd controls 20,18 54x16}" {
		t.Errorf("SetTitleBarControls delivered %s", got)
	}
	o.SetTitleBarControls(platform.Rect{})
	b.Drain(r)
	if got := o.State(); got.TitleBarControls != (platform.Rect{}) || !got.ClientSideDecorations {
		t.Errorf("after removing the controls: %v", got)
	}
	expectPanic(t, "without WindowOptions.TitleBarOverlay", func() { plain.SetTitleBarControls(moved) })
	expectPanic(t, "negative size", func() { o.SetTitleBarControls(platform.Rect{W: -1}) })
	expectPanic(t, "not finite", func() { New(Options{TitleBarControls: platform.Rect{X: float32(1e9)}}) })

	// The overlay keeps its controls across state changes the OS makes.
	o.SetTitleBarControls(controls)
	o.SetMaximized(true)
	b.Drain(r)
	if got := o.State(); !got.Maximized || got.TitleBarControls != controls {
		t.Errorf("maximized overlay window: %v", got)
	}
	expectPanic(t, "both Frameless and TitleBarOverlay", func() { _, _ = b.NewWindow(platform.WindowOptions{Frameless: true, TitleBarOverlay: true}) })
}
