package headless

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// Tests of the phase-13 fakes (ADR-210).

func TestClipboardData(t *testing.T) {
	b := New(Options{})
	c := b.Clipboard()
	data := platform.Data{{Type: platform.MIMEHTML, Bytes: []byte("<b>hi</b>")}, {Type: platform.MIMEText, Bytes: []byte("hi")}}
	if err := c.Write(data); err != nil {
		t.Fatal(err)
	}
	data[1].Bytes[0] = 'X' // Write copied it
	var got []string
	if err := c.ReadTypes(func(types []string, err error) { got = append(got, fmt.Sprint(types, err)) }); err != nil {
		t.Fatal(err)
	}
	if err := c.Read(platform.MIMEHTML, func(b []byte, err error) { got = append(got, fmt.Sprintf("%s %v", b, err)) }); err != nil {
		t.Fatal(err)
	}
	if err := c.Read(platform.MIMEPNG, func(b []byte, err error) { got = append(got, fmt.Sprintf("png=%v %v", b == nil, err)) }); err != nil {
		t.Fatal(err)
	}
	c.ReadText(func(s string, err error) { got = append(got, s) })
	if len(got) != 0 {
		t.Fatalf("a read answered inside its call: %v", got)
	}
	b.Drain(&recorder{})
	if want := "[[text/html text/plain;charset=utf-8] <nil> <b>hi</b> <nil> png=true <nil> hi]"; fmt.Sprint(got) != want {
		t.Errorf("reads = %v, want %s", got, want)
	}
	// Another application copies files; WriteText replaces everything.
	a := filepath.Join(t.TempDir(), "a") // a native absolute path on every OS
	files, err := platform.FileData(a)
	if err != nil {
		t.Fatal(err)
	}
	b.SetClipboardData(files)
	if p, ok := b.ClipboardData().Paths(); !ok || p[0] != a {
		t.Errorf("ClipboardData = %v", b.ClipboardData())
	}
	_ = c.WriteText("plain")
	if d := b.ClipboardData(); len(d) != 1 || d[0].Type != platform.MIMEText {
		t.Errorf("after WriteText: %v", d)
	}
	b.SetUnsupported(ServiceClipboard)
	if err := c.Write(platform.TextData("x")); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Write without the service = %v", err)
	}
	if err := c.WriteText("still works"); err != nil {
		t.Errorf("WriteText without the data service = %v", err)
	}
	expectPanic(t, "not a MIME type", func() { _ = c.Read("png", func([]byte, error) {}) })
	expectPanic(t, "twice", func() { _ = c.Write(platform.Data{{Type: "a/b"}, {Type: "a/b"}}) })
}

func TestExternalDrag(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	r := &recorder{}
	b.Drain(r)
	r.take()
	data := platform.Data{{Type: "application/x-card", Bytes: []byte("card")}, {Type: platform.MIMEHTML, Bytes: []byte("<i>x</i>")},
		{Type: platform.MIMEText, Bytes: []byte("x")}}
	b.StartExternalDrag(data, platform.DropCopy|platform.DropMove)
	var seen []platform.DragEvent
	r.react = func(pw platform.Window, e platform.Event) {
		if d, ok := e.(platform.DragEvent); ok {
			seen = append(seen, d)
			if d.Action == platform.DragOver && d.Pos.X >= 20 {
				pw.SetDropEffect(platform.DefaultDropEffect(d.Allowed, d.Mods))
			}
		}
	}
	w.Inject(platform.PointerEvent{Action: platform.PointerEnter, Pos: platform.Point{X: 10, Y: 10}})
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 10, Y: 10}}) // unchanged: nothing
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 15, Y: 10}})
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 20, Y: 10}, Mods: platform.ModShift}) // other mods: not coalesced
	b.Drain(r)
	if got, want := r.take(), "w:drag(enter 10,10) w:drag(over 15,10) w:drag(over 20,10)"; got != want {
		t.Errorf("events = %s\nwant     %s", got, want)
	}
	if e := seen[0]; !slices.Equal(e.Types, data.Types()) || e.Allowed != platform.DropCopy|platform.DropMove || e.FromApp {
		t.Errorf("DragEnter = %+v", e)
	}
	if eff, _ := w.DropAnswer(); eff != platform.DropMove {
		t.Errorf("DropAnswer = %v, want move (Shift)", eff)
	}
	w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 20, Y: 10}, Mods: platform.ModShift})
	b.Drain(r)
	if got, want := r.take(), `w:drag(drop 20,10 ["text/html" "text/plain;charset=utf-8"])`; got != want {
		t.Errorf("drop = %s, want %s (the well-known types only, in the source's order)", got, want)
	}
	if d := seen[len(seen)-1].Data; string(d[0].Bytes) != "<i>x</i>" {
		t.Errorf("drop data = %v", d)
	}
	if got := w.Drops(); !slices.Equal(got, []platform.DropEffect{platform.DropMove}) {
		t.Errorf("Drops = %v", got)
	}
	if _, ok := b.Drag(); ok {
		t.Errorf("the drag is still in progress after the drop")
	}
	// Pointer events reach the app again.
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 1, Y: 1}})
	b.Drain(r)
	if got := r.take(); got != "w:pointer(move 1,1)" {
		t.Errorf("after the drag: %s", got)
	}

	// A drag the window refuses ends with a leave at the release; one that
	// asks for a custom type gets it.
	b.StartExternalDrag(data, platform.DropCopy)
	r.react = nil
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 5, Y: 5}})
	w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 5, Y: 5}})
	b.Drain(r)
	if got := r.take(); got != "w:drag(enter 5,5) w:drag(leave 5,5)" {
		t.Errorf("refused drop = %s", got)
	}
	b.StartExternalDrag(data, platform.DropCopy)
	r.react = func(pw platform.Window, e platform.Event) {
		if d, ok := e.(platform.DragEvent); ok && d.Action == platform.DragEnter {
			pw.SetDropEffect(platform.DropCopy, "application/x-card", "image/png")
		}
	}
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 5, Y: 5}})
	b.Drain(r)
	w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 5, Y: 5}})
	b.Drain(r)
	if got := r.take(); got != `w:drag(enter 5,5) w:drag(drop 5,5 ["application/x-card"])` {
		t.Errorf("custom drop = %s", got)
	}
	// An effect the drag does not allow refuses.
	b.StartExternalDrag(data, platform.DropCopy)
	r.react = func(pw platform.Window, e platform.Event) { pw.SetDropEffect(platform.DropLink) }
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 5, Y: 5}})
	b.Drain(r)
	w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: 5, Y: 5}})
	b.Drain(r)
	if got := r.take(); got != "w:drag(enter 5,5) w:drag(leave 5,5)" {
		t.Errorf("drop with a disallowed effect = %s", got)
	}
	b.StartExternalDrag(data, platform.DropCopy)
	expectPanic(t, "in progress", func() { b.StartExternalDrag(data, platform.DropCopy) })
	expectPanic(t, "CancelDrag", func() { b.DropOutside(platform.DropCopy) })
	b.CancelDrag()
	expectPanic(t, "without a drag", func() { b.CancelDrag() })
	expectPanic(t, "does not accept DragEvent", func() { w.Inject(platform.DragEvent{}) })
}

func TestAppDrag(t *testing.T) {
	b := New(Options{})
	src := newWindow(t, b, platform.WindowOptions{Title: "src"})
	dst := newWindow(t, b, platform.WindowOptions{Title: "dst"})
	r := &recorder{}
	b.Drain(r)
	r.take()
	opts := platform.DragOptions{Data: platform.Data{{Type: "application/x-card", Bytes: []byte("7")}, {Type: platform.MIMEText, Bytes: []byte("Card 7")}},
		Allowed: platform.DropCopy | platform.DropMove, Image: image.NewRGBA(image.Rect(3, 3, 13, 9)), Hotspot: image.Pt(5, 3)}
	if err := src.StartDrag(opts); err != platform.ErrNoGrab {
		t.Errorf("StartDrag without a held button = %v, want ErrNoGrab", err)
	}
	var started error
	r.react = func(w platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.PointerEvent:
			if e.Action == platform.PointerMove && w == src {
				started = src.StartDrag(opts)
				if again := src.StartDrag(opts); again != platform.ErrNoGrab {
					t.Errorf("a second StartDrag = %v, want ErrNoGrab", again)
				}
			}
		case platform.DragEvent:
			if w == dst && e.Action == platform.DragEnter {
				w.SetDropEffect(platform.DropMove, "application/x-card")
			}
			if w == dst && e.Action == platform.DragDrop && string(e.Data[0].Bytes) != "7" {
				t.Errorf("drop data = %v", e.Data)
			}
		}
	}
	src.Inject(platform.PointerEvent{Action: platform.PointerDown, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask(), Pos: platform.Point{X: 4, Y: 4}})
	src.Inject(platform.PointerEvent{Action: platform.PointerMove, Buttons: platform.ButtonPrimary.Mask(), Pos: platform.Point{X: 9, Y: 4}})
	b.Drain(r)
	if started != nil {
		t.Fatalf("StartDrag = %v", started)
	}
	if got, want := r.take(), "src:pointer(down 4,4) src:pointer(move 9,4) src:pointer(cancel 9,4) src:drag(enter 9,4 app)"; got != want {
		t.Errorf("start = %s\nwant    %s", got, want)
	}
	info, ok := b.Drag()
	if !ok || info.Source != src || info.Image.Rect != image.Rect(0, 0, 10, 6) || info.Hotspot != image.Pt(5, 3) {
		t.Errorf("Drag = %+v, %v", info, ok)
	}
	// The release goes to the drag, over the other window.
	dst.Inject(platform.PointerEvent{Action: platform.PointerMove, Buttons: platform.ButtonPrimary.Mask(), Pos: platform.Point{X: 30, Y: 30}})
	b.Drain(r)
	dst.Inject(platform.PointerEvent{Action: platform.PointerUp, Button: platform.ButtonPrimary, Pos: platform.Point{X: 30, Y: 30}})
	b.Drain(r)
	if got, want := r.take(), `src:drag(leave 9,4 app) dst:drag(enter 30,30 app) dst:drag(drop 30,30 app ["application/x-card"]) src:dragdone(move)`; got != want {
		t.Errorf("drop = %s\nwant   %s", got, want)
	}

	// The target refuses at the drop: the source keeps its data.
	r.react = func(w platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.PointerEvent:
			if e.Action == platform.PointerDown {
				started = src.StartDrag(opts)
			}
		case platform.DragEvent:
			if w == src && e.Action == platform.DragEnter {
				w.SetDropEffect(platform.DropCopy)
			}
			if e.Action == platform.DragDrop {
				w.SetDropEffect(platform.DropNone)
			}
		}
	}
	src.Inject(platform.PointerEvent{Action: platform.PointerDown, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask(), Pos: platform.Point{X: 1, Y: 1}})
	b.Drain(r)
	src.Inject(platform.PointerEvent{Action: platform.PointerUp, Button: platform.ButtonPrimary, Pos: platform.Point{X: 1, Y: 1}})
	b.Drain(r)
	if got, want := r.take(), `src:pointer(down 1,1) src:pointer(cancel 1,1) src:drag(enter 1,1 app) src:drag(drop 1,1 app ["text/plain;charset=utf-8"]) src:dragdone(none)`; got != want {
		t.Errorf("refused at the drop = %s\nwant                   %s", got, want)
	}
	if got := src.Drops(); !slices.Equal(got, []platform.DropEffect{platform.DropNone}) {
		t.Errorf("src Drops = %v", got)
	}

	// Dropped on another application, or cancelled.
	for _, c := range []struct {
		end  func()
		want string
	}{
		{func() { b.DropOutside(platform.DropCopy) }, "src:drag(leave 1,1 app) src:dragdone(copy)"},
		{func() { b.CancelDrag() }, "src:drag(leave 1,1 app) src:dragdone(none)"},
	} {
		r.react = func(w platform.Window, e platform.Event) {
			if e, ok := e.(platform.PointerEvent); ok && e.Action == platform.PointerDown {
				_ = src.StartDrag(opts)
			}
		}
		src.Inject(platform.PointerEvent{Action: platform.PointerDown, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask(), Pos: platform.Point{X: 1, Y: 1}})
		b.Drain(r)
		r.take()
		c.end()
		b.Drain(r)
		if got := r.take(); got != c.want {
			t.Errorf("end = %s, want %s", got, c.want)
		}
	}
	b.SetUnsupported(ServiceDragDrop)
	if err := src.StartDrag(opts); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("StartDrag without the service = %v", err)
	}
	expectPanic(t, "Data is empty", func() { _ = src.StartDrag(platform.DragOptions{Allowed: platform.DropCopy}) })
	expectPanic(t, "set of effects", func() { src.SetDropEffect(platform.DropAll) })
}

func TestAppDragWithTheReleaseQueued(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	r := &recorder{react: func(pw platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.PointerEvent:
			if e.Action == platform.PointerMove && e.Buttons != 0 && e.Pos.X == 10 {
				if err := pw.StartDrag(platform.DragOptions{Data: platform.TextData("x"), Allowed: platform.DropCopy}); err != nil {
					t.Errorf("StartDrag = %v", err)
				}
			}
		case platform.DragEvent:
			pw.SetDropEffect(platform.DropCopy)
		}
	}}
	b.Drain(r)
	r.take()
	// The whole gesture is injected at once, as telatest's drags do.
	held := platform.ButtonPrimary.Mask()
	w.Inject(platform.PointerEvent{Action: platform.PointerDown, Button: platform.ButtonPrimary, Buttons: held, Pos: platform.Point{X: 1, Y: 1}})
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Buttons: held, Pos: platform.Point{X: 10, Y: 1}})
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Buttons: held, Pos: platform.Point{X: 20, Y: 1}})
	w.Inject(platform.PointerEvent{Action: platform.PointerUp, Button: platform.ButtonPrimary, Pos: platform.Point{X: 20, Y: 1}})
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 30, Y: 1}}) // after the drop: the app's again
	b.Drain(r)
	want := `w:pointer(down 1,1) w:pointer(move 10,1) w:pointer(cancel 10,1) w:drag(enter 10,1 app) w:drag(over 20,1 app) w:drag(drop 20,1 app ["text/plain;charset=utf-8"]) w:dragdone(copy) w:pointer(move 30,1)`
	if got := r.take(); got != want {
		t.Errorf("events = %s\nwant     %s", got, want)
	}
}

func TestMenus(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	frameless := newWindow(t, b, platform.WindowOptions{Title: "f", Frameless: true})
	menu := []platform.MenuItem{{Label: "File", Submenu: []platform.MenuItem{{ID: 1, Label: "Save", Accelerator: "Ctrl+S"}, {ID: 2, Label: "Gone", Disabled: true}}}}
	if err := w.SetMenu(menu); err != nil {
		t.Fatal(err)
	}
	menu[0].Submenu[0].Label = "changed" // SetMenu copied it
	if got := w.Menu(); got[0].Submenu[0].Label != "Save" || got[0].Submenu[0].Accelerator != "Ctrl+S" || w.MenuChanges() != 1 {
		t.Errorf("Menu = %+v", got)
	}
	if err := frameless.SetMenu(menu); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SetMenu on a frameless window = %v", err)
	}
	r := &recorder{}
	b.Drain(r)
	r.take()
	w.Inject(platform.MenuEvent{Item: 1})
	b.Drain(r)
	if got := r.take(); got != "w:menu(1)" {
		t.Errorf("menu choice = %s", got)
	}
	expectPanic(t, "disabled", func() { w.Inject(platform.MenuEvent{Item: 2}) })
	expectPanic(t, "no item 9", func() { w.Inject(platform.MenuEvent{Item: 9}) })

	var got []string
	done := func(id platform.MenuItemID, err error) { got = append(got, fmt.Sprint(id, err)) }
	if err := w.PopupMenu(platform.Point{X: 5, Y: 6}, menu[0].Submenu, done); err != nil {
		t.Fatal(err)
	}
	open := b.OpenDialogs()
	if len(open) != 1 || open[0].Kind != DialogPopupMenu || open[0].Popup.Window != w || open[0].Popup.At != (platform.Point{X: 5, Y: 6}) {
		t.Fatalf("OpenDialogs = %+v", open)
	}
	expectPanic(t, "disabled", func() { b.AnswerDialog(open[0].ID, DialogResponse{Item: 2}) })
	b.AnswerDialog(open[0].ID, DialogResponse{Item: 1})
	b.SetDialogResponder(func(Dialog) DialogResponse { return DialogResponse{} }) // dismissed
	_ = w.PopupMenu(platform.Point{}, menu, done)
	if len(got) != 0 {
		t.Fatalf("a popup answered inside its call")
	}
	b.Drain(r)
	if fmt.Sprint(got) != "[1 <nil> 0 <nil>]" {
		t.Errorf("popup answers = %v", got)
	}
	b.SetUnsupported(ServiceMenus)
	if err := w.PopupMenu(platform.Point{}, menu, done); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("PopupMenu without the service = %v", err)
	}
	if err := w.SetMenu(nil); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SetMenu without the service = %v", err)
	}
	expectPanic(t, "empty menu", func() { _ = w.PopupMenu(platform.Point{}, nil, done) })
}

func TestShortcuts(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()
	show := platform.Shortcut{ID: "show", Accelerator: platform.Accelerator{Mods: platform.ModSuper | platform.ModShift, Key: "n"}, Description: "Show notes"}
	if err := d.RegisterShortcut(show); err != nil {
		t.Fatal(err)
	}
	r := &recorder{}
	b.Drain(r)
	if got := r.take(); got != `app:shortcut-status(show bound "Shift+Super+N")` {
		t.Errorf("status = %s", got)
	}
	b.InjectApp(platform.ShortcutEvent{ID: "show"})
	b.InjectApp(platform.ShortcutEvent{ID: "show", Action: platform.ShortcutReleased})
	b.InjectApp(platform.ShortcutStatusEvent{ID: "show", Status: platform.ShortcutBound, Trigger: "Ctrl+Alt+N"})
	b.Drain(r)
	if got := r.take(); got != `app:shortcut(show pressed) app:shortcut(show released) app:shortcut-status(show bound "Ctrl+Alt+N")` {
		t.Errorf("events = %s", got)
	}
	if recs := b.Shortcuts(); len(recs) != 1 || recs[0].Trigger != "Ctrl+Alt+N" {
		t.Errorf("Shortcuts = %+v", recs)
	}
	b.SetShortcutResponder(func(s platform.Shortcut) platform.ShortcutStatusEvent {
		return platform.ShortcutStatusEvent{Status: platform.ShortcutTaken, Reason: "held by another app"}
	})
	hide := show
	hide.ID = "hide"
	_ = d.RegisterShortcut(hide)
	b.Drain(r)
	if got := r.take(); got != `app:shortcut-status(hide taken "")` {
		t.Errorf("taken = %s", got)
	}
	expectPanic(t, "not bound", func() { b.InjectApp(platform.ShortcutEvent{ID: "hide"}) })
	expectPanic(t, "no shortcut", func() { b.InjectApp(platform.ShortcutEvent{ID: "missing"}) })
	_ = d.UnregisterShortcut("show")
	_ = d.UnregisterShortcut("unknown")
	if recs := b.Shortcuts(); len(recs) != 1 || recs[0].Shortcut.ID != "hide" {
		t.Errorf("after Unregister: %+v", recs)
	}
	b.SetUnsupported(ServiceShortcuts)
	if err := d.RegisterShortcut(show); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("RegisterShortcut without the service = %v", err)
	}
	expectPanic(t, "upper case", func() {
		_ = d.RegisterShortcut(platform.Shortcut{ID: "x", Accelerator: platform.Accelerator{Key: "N"}, Description: "x"})
	})
}

func TestSecrets(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()
	const app = "com.example.Notes"
	if _, err := d.Secret(app, "key"); !errors.Is(err, platform.ErrSecretNotFound) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Secret before SetSecret = %v", err)
	}
	// Blocking and safe from any goroutine.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := fmt.Sprint("k", i)
			if err := d.SetSecret(app, name, []byte(name)); err != nil {
				t.Error(err)
			}
			if v, err := d.Secret(app, name); err != nil || string(v) != name {
				t.Errorf("Secret(%s) = %q, %v", name, v, err)
			}
		}()
	}
	wg.Wait()
	secret := []byte{1, 2, 3}
	_ = d.SetSecret(app, "key", secret)
	secret[0] = 9
	if v, _ := d.Secret(app, "key"); v[0] != 1 {
		t.Errorf("SetSecret kept the caller's slice")
	}
	if err := d.SetSecret(app, "big", make([]byte, platform.MaxSecretSize+1)); !errors.Is(err, platform.ErrSecretTooLarge) {
		t.Errorf("oversized SetSecret = %v", err)
	}
	if err := d.DeleteSecret(app, "key"); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.StoredSecret(app, "key"); ok {
		t.Errorf("DeleteSecret left the secret")
	}
	if err := d.DeleteSecret(app, "key"); err != nil {
		t.Errorf("deleting a missing secret = %v", err)
	}
	locked := errors.New("the keyring is locked")
	b.SetSecretError(locked)
	if _, err := d.Secret(app, "k1"); err != locked {
		t.Errorf("Secret with an injected error = %v", err)
	}
	b.SetSecretError(nil)
	b.SetUnsupported(ServiceSecrets)
	if err := d.SetSecret(app, "k", nil); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SetSecret without the service = %v", err)
	}
	expectPanic(t, "invalid secret name", func() { _, _ = d.Secret(app, "") })
}

func TestSingleInstance(t *testing.T) {
	first, second := New(Options{}), New(Options{})
	t.Cleanup(first.ReleaseInstances)
	t.Cleanup(second.ReleaseInstances)
	const id = "dev.telaui.test.SingleInstance"
	if primary, err := first.Desktop().ClaimInstance(id, platform.Launch{}); !primary || err != nil {
		t.Fatalf("first claim = %v, %v", primary, err)
	}
	if primary, _ := first.Desktop().ClaimInstance(id, platform.Launch{}); !primary {
		t.Errorf("claiming again in the primary returned false")
	}
	launch := platform.Launch{Args: []string{"notes:open?id=3"}, Dir: "/home/u", Token: "tok"}
	if primary, err := second.Desktop().ClaimInstance(id, launch); primary || err != nil {
		t.Fatalf("second claim = %v, %v", primary, err)
	}
	launch.Args[0] = "changed" // forwarded as a copy
	w := newWindow(t, first, platform.WindowOptions{Title: "w"})
	r := &recorder{react: func(_ platform.Window, e platform.Event) {
		if l, ok := e.(platform.LaunchEvent); ok {
			w.ActivateWithToken(l.Launch.Token)
		}
	}}
	first.Drain(r)
	if got := r.take(); !containsEvent(got, `app:launch(["notes:open?id=3"] /home/u "tok")`) {
		t.Errorf("events = %s", got)
	}
	if got := w.ActivationTokens(); !slices.Equal(got, []string{"tok"}) || w.Activations() != 1 {
		t.Errorf("ActivationTokens = %q, Activations = %d", got, w.Activations())
	}
	if ls := first.Launches(); len(ls) != 1 || ls[0].Args[0] != "notes:open?id=3" {
		t.Errorf("Launches = %+v", ls)
	}
	first.InjectApp(platform.LaunchEvent{Launch: platform.Launch{Args: []string{"x"}}})
	expectPanic(t, "claimed no instance", func() { second.InjectApp(platform.LaunchEvent{}) })
	// When the first instance exits, the next process becomes the primary.
	first.ReleaseInstances()
	if primary, _ := second.Desktop().ClaimInstance(id, launch); !primary || !slices.Equal(second.Instances(), []string{id}) {
		t.Errorf("claim after the first released = %v, %v", primary, second.Instances())
	}
	second.SetUnsupported(ServiceInstance)
	if _, err := second.Desktop().ClaimInstance("other", platform.Launch{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("ClaimInstance without the service = %v", err)
	}
}

func containsEvent(events, want string) bool {
	return slices.Contains(splitEvents(events), want)
}

func splitEvents(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ' ':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

func TestRunReleasesInstance(t *testing.T) {
	b := New(Options{})
	const id = "dev.telaui.test.RunReleases"
	if primary, _ := b.Desktop().ClaimInstance(id, platform.Launch{}); !primary {
		t.Fatal("not primary")
	}
	b.Quit()
	_ = b.Run(platform.HandlerFunc(func(platform.Window, platform.Event) {}))
	other := New(Options{})
	t.Cleanup(other.ReleaseInstances)
	if primary, _ := other.Desktop().ClaimInstance(id, platform.Launch{}); !primary {
		t.Errorf("the claim outlived Run")
	}
}

func TestAssociationsThroughTheBackend(t *testing.T) {
	b := New(Options{})
	app := platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: "/opt/notes"}
	a := b.Desktop().Associations()
	if changed, err := a.Register(app, platform.Association{Scheme: "notes"}, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	if regs := b.Associations().Registrations(); len(regs) != 1 || regs[0].Association != "notes:" {
		t.Errorf("Registrations = %+v", regs)
	}
	b.SetUnsupported(ServiceAssociations)
	if _, err := b.Desktop().Associations().Status(app, platform.Association{Scheme: "notes"}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Status without the service = %v", err)
	}
}

func TestSettingsV2(t *testing.T) {
	b := New(Options{Settings: platform.SystemSettings{TextScale: 1.25}})
	s := platform.SystemSettings{DarkMode: true, AccentColor: color.NRGBA{R: 0x35, G: 0x84, B: 0xe4, A: 0xff}, HasAccentColor: true, HighContrast: true, TextScale: 1.5}
	var got platform.SystemSettings
	b.SetSettings(s)
	b.Drain(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		if se, ok := e.(platform.SettingsEvent); ok {
			got = se.Settings
		}
	}))
	if got != s || b.Settings() != s {
		t.Errorf("SettingsEvent = %+v, want %+v", got, s)
	}
	expectPanic(t, "TextScale", func() { b.SetSettings(platform.SystemSettings{TextScale: -2}) })
	expectPanic(t, "TextScale", func() { New(Options{Settings: platform.SystemSettings{TextScale: 100}}) })
}
