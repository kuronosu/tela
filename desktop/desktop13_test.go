package desktop_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// A GlobalShortcut is registered while mounted, presses and releases run
// its callbacks, statuses reach OnStatus, an equal build does not
// register again, a change of Keys does, two IDs coexist, and unmounting
// removes it.
func TestGlobalShortcut(t *testing.T) {
	var statuses []desktop.ShortcutStatusEvent
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(true)
		keys, setKeys := tela.UseState("ctrl+alt+n")
		presses, setPresses := tela.UseState(0)
		releases, setReleases := tela.UseState(0)
		other, setOther := tela.UseState(0)
		bump := func(s tela.Setter[int]) func() { return func() { s.Update(func(n int) int { return n + 1 }) } }
		return Div(
			Textf("presses %d, releases %d, other %d", presses, releases, other),
			Button(OnClick(func() { setKeys.Set("Ctrl+Shift+N") }), Text("Rebind")),
			Button(OnClick(func() { setOn.Set(false) }), Text("Off")),
			If(on, tela.Component(desktop.GlobalShortcut{
				ID: "new-note", Keys: keys, Description: "Create a note",
				OnPress: bump(setPresses), OnRelease: bump(setReleases),
				OnStatus: func(e desktop.ShortcutStatusEvent) { statuses = append(statuses, e) },
			})),
			tela.Component(desktop.GlobalShortcut{ID: "show", Keys: "Super+F5", Description: "Show Notes", OnPress: bump(setOther)}),
		)
	})
	got := app.GlobalShortcuts()
	if len(got) != 2 || got[0].ID != "new-note" || got[0].Keys != "Ctrl+Alt+N" || got[0].Status != desktop.ShortcutBound ||
		got[1].ID != "show" || got[1].Keys != "Super+F5" || got[0].Description != "Create a note" {
		t.Fatalf("shortcuts %+v", got)
	}
	if len(statuses) != 1 || statuses[0].Status != desktop.ShortcutBound || statuses[0].Trigger != "Ctrl+Alt+N" || statuses[0].Err != nil {
		t.Fatalf("statuses %+v, want one Bound with the trigger Ctrl+Alt+N", statuses)
	}
	app.PressShortcut("new-note")
	app.ReleaseShortcut("new-note")
	app.PressShortcut("show")
	app.ExpectText("presses 1, releases 1, other 1")

	app.SetShortcutStatus("new-note", desktop.ShortcutTaken, "")
	if e := statuses[len(statuses)-1]; e.Status != desktop.ShortcutTaken || e.Err == nil ||
		!strings.Contains(e.Err.Error(), "Ctrl+Alt+N is taken by another application") {
		t.Errorf("Taken status %+v", e)
	}
	// A build with the same fields does not register again (which would
	// reset the status to Bound).
	app.PressShortcut("show")
	app.ExpectText("other 2")
	if s := app.GlobalShortcuts()[0]; s.Status != desktop.ShortcutTaken {
		t.Errorf("after a build with the same fields: %+v; want it left Taken", s)
	}
	n := len(statuses)
	app.Click(telatest.ByRole("button", "Rebind"))
	if s := app.GlobalShortcuts()[0]; s.Keys != "Ctrl+Shift+N" || s.Status != desktop.ShortcutBound {
		t.Errorf("after Rebind: %+v", s)
	}
	if len(statuses) != n+1 || statuses[n].Trigger != "Ctrl+Shift+N" {
		t.Errorf("statuses after Rebind %+v", statuses[n:])
	}
	app.SetShortcutStatus("new-note", desktop.ShortcutBound, "Ctrl+Alt+M")
	if e := statuses[len(statuses)-1]; e.Status != desktop.ShortcutBound || e.Trigger != "Ctrl+Alt+M" {
		t.Errorf("rebound by the user: %+v", e)
	}
	app.Click(telatest.ByRole("button", "Off"))
	if got := app.GlobalShortcuts(); len(got) != 1 || got[0].ID != "show" {
		t.Errorf("after unmount: %+v", got)
	}
}

// Without global shortcuts OnStatus gets Failed wrapping ErrUnsupported;
// invalid Keys panic in dev mode with the fix.
func TestGlobalShortcutErrors(t *testing.T) {
	var got desktop.ShortcutStatusEvent
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(false)
		return Div(Button(OnClick(func() { setOn.Set(true) }), Text("On")),
			If(on, tela.Component(desktop.GlobalShortcut{ID: "x", Keys: "Ctrl+K", Description: "X",
				OnStatus: func(e desktop.ShortcutStatusEvent) { got = e }})))
	})
	app.Unsupported(telatest.GlobalShortcuts)
	app.Click(telatest.ByRole("button", "On"))
	if got.Status != desktop.ShortcutFailed || !errors.Is(got.Err, desktop.ErrUnsupported) {
		t.Errorf("status %+v, want Failed wrapping ErrUnsupported", got)
	}

	defer func() {
		r := recover()
		if s, _ := r.(string); !strings.Contains(s, `"Hyper" is not a modifier`) {
			t.Errorf("panic %v, want the parse error", r)
		}
	}()
	desktop.GlobalShortcut{ID: "y", Keys: "Hyper+K", Description: "Y"}.Build()
}

func TestParseKeys(t *testing.T) {
	if s, err := desktop.ParseKeys(" ctrl + alt + n "); s != "Ctrl+Alt+N" || err != nil {
		t.Errorf("ParseKeys = %q, %v", s, err)
	}
	if _, err := desktop.ParseKeys("Ctrl+"); err == nil {
		t.Error("ParseKeys(\"Ctrl+\") gave no error")
	}
}

// A single-instance app raises its hidden main window for a forwarded
// launch and gives it to UseLaunch, with URLs, file: URIs and relative
// paths sorted out.
func TestUseLaunch(t *testing.T) {
	var got []desktop.Launch
	app := telatest.MountApp(t, desktop.SingleInstance(tela.App{
		AppID:  "com.example.launchtest",
		Hidden: true,
		Root: tela.C(func() tela.Node {
			n, setN := tela.UseState(0)
			desktop.UseLaunch(func(l desktop.Launch) {
				got = append(got, l)
				setN.Update(func(n int) int { return n + 1 })
			})
			return Textf("launches: %d", n)
		}),
	}))
	app.ExpectWindowState(telatest.Hidden(true))
	if len(got) != 0 {
		t.Fatalf("telatest gave a start launch: %+v", got)
	}
	app.InjectLaunch(desktop.Launch{Dir: "/home/ana", Args: []string{
		"--verbose", "notes:open?id=3", "file:///tmp/a%20b.note", "docs/c.note", "/abs/d.note", "https://example.com/x", "--", "-dash.note"}})
	app.ExpectText("launches: 1")
	app.ExpectWindowState(telatest.Hidden(false))
	l := got[0]
	if !l.Forwarded {
		t.Error("a forwarded launch has Forwarded false")
	}
	if want := []string{"notes:open?id=3", "https://example.com/x"}; !slices.Equal(l.URLs(), want) {
		t.Errorf("URLs = %q, want %q", l.URLs(), want)
	}
	if want := []string{"/tmp/a b.note", "/home/ana/docs/c.note", "/abs/d.note", "/home/ana/-dash.note"}; !slices.Equal(l.Files(), want) {
		t.Errorf("Files = %q, want %q", l.Files(), want)
	}
}

// The process's own start reaches every UseLaunch mounted with the first
// one, once: not those that mount later, nor one that mounts again.
func TestUseLaunchStart(t *testing.T) {
	desktop.SetStartLaunchForTest(t, "/home/ana", "list.note")
	var got []string
	listener := func(name string) tela.Component {
		return tela.C(func() tela.Node {
			desktop.UseLaunch(func(l desktop.Launch) {
				got = append(got, name+":"+strings.Join(l.Files(), ",")+":"+strconv.FormatBool(l.Forwarded))
			})
			return nil
		})
	}
	app := telatest.MountFunc(t, func() tela.Node {
		later, setLater := tela.UseState(false)
		return Div(
			listener("a"), listener("b"),
			Button(OnClick(func() { setLater.Set(!later) }), Text("Later")),
			If(later, listener("c")),
		)
	})
	app.Click(telatest.ByRole("button", "Later"))
	app.Click(telatest.ByRole("button", "Later"))
	app.Click(telatest.ByRole("button", "Later"))
	want := []string{"a:/home/ana/list.note:false", "b:/home/ana/list.note:false"}
	if !slices.Equal(got, want) {
		t.Errorf("launches %q, want %q", got, want)
	}
	got = nil
	app.InjectLaunch(desktop.Launch{Dir: "/x", Args: []string{"y"}})
	want = []string{"a:/x/y:true", "b:/x/y:true", "c:/x/y:true"}
	if slices.Sort(got); !slices.Equal(got, want) {
		t.Errorf("forwarded launch %q, want %q", got, want)
	}
}

// The secret functions keep, read and delete secrets from a goroutine;
// a missing one, a canceled prompt and a desktop without a keyring are
// told apart with errors.Is.
func TestSecrets(t *testing.T) {
	type result struct {
		val string
		err error
	}
	results := make(chan result, 1)
	app := telatest.MountFunc(t, func() tela.Node {
		op, setOp := tela.UseState("")
		tela.UseGo(func(ctx context.Context) {
			var r result
			switch op {
			case "":
				return
			case "read":
				b, err := desktop.Secret("token")
				r = result{string(b), err}
			case "write":
				r.err = desktop.SetSecret("token", []byte("s3cret"))
			case "delete":
				r.err = desktop.DeleteSecret("token")
			}
			results <- r
		}, op)
		return Div(
			Button(OnClick(func() { setOp.Set("read") }), Text("read")),
			Button(OnClick(func() { setOp.Set("write") }), Text("write")),
			Button(OnClick(func() { setOp.Set("delete") }), Text("delete")),
			Button(OnClick(func() { setOp.Set("") }), Text("reset")),
		)
	})
	do := func(op string) result {
		t.Helper()
		app.Click(telatest.ByRole("button", "reset"))
		app.Click(telatest.ByRole("button", op))
		return <-results
	}
	if r := do("read"); !errors.Is(r.err, desktop.ErrSecretNotFound) {
		t.Errorf("read before any write: %+v, want ErrSecretNotFound", r)
	}
	app.SetStoredSecret("token", []byte("preset"))
	if r := do("read"); r.val != "preset" || r.err != nil {
		t.Errorf("read of the preset: %+v", r)
	}
	if r := do("write"); r.err != nil {
		t.Errorf("write: %v", r.err)
	}
	if b, ok := app.StoredSecret("token"); !ok || string(b) != "s3cret" {
		t.Errorf("StoredSecret = %q, %v", b, ok)
	}
	app.FailSecrets(desktop.ErrSecretCanceled)
	if r := do("read"); !errors.Is(r.err, desktop.ErrSecretCanceled) {
		t.Errorf("read with a canceled prompt: %+v", r)
	}
	app.FailSecrets(nil)
	if r := do("delete"); r.err != nil {
		t.Errorf("delete: %v", r.err)
	}
	if _, ok := app.StoredSecret("token"); ok {
		t.Error("the secret is still stored after DeleteSecret")
	}
	app.Unsupported(telatest.Secrets)
	if r := do("read"); !errors.Is(r.err, desktop.ErrUnsupported) {
		t.Errorf("read without a keyring: %+v", r)
	}
}

// The clipboard carries text, HTML, PNG images and files both ways.
func TestClipboard(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(1, 1, color.NRGBA{R: 255, A: 255})
	var read *tela.DataTransfer
	var readErr error
	var text string
	app := telatest.MountFunc(t, func() tela.Node {
		copyAll := func() {
			var dt tela.DataTransfer
			dt.SetHTML("<b>milk</b>")
			dt.SetText("milk")
			if err := dt.SetImage(img); err != nil {
				t.Error(err)
			}
			if err := dt.SetFiles("/home/ana/list.md"); err != nil {
				t.Error(err)
			}
			dt.SetData("application/x-notes-item", "42")
			if err := desktop.WriteClipboard(&dt); err != nil {
				t.Error(err)
			}
		}
		paste := func(types ...string) func() {
			return func() {
				if err := desktop.ReadClipboard(func(dt *tela.DataTransfer, err error) { read, readErr = dt, err }, types...); err != nil {
					readErr = err
				}
			}
		}
		return Div(
			Button(OnClick(copyAll), Text("Copy")),
			Button(OnClick(paste()), Text("Paste")),
			Button(OnClick(paste("text", "application/x-notes-item")), Text("Paste item")),
			Button(OnClick(func() { _ = desktop.WriteClipboardText("plain") }), Text("Copy text")),
			Button(OnClick(func() {
				_ = desktop.ReadClipboardText(func(s string, err error) { text = s })
			}), Text("Paste text")),
		)
	})
	app.Click(telatest.ByRole("button", "Copy"))
	clip := app.Clipboard()
	if clip.Text() != "milk" || clip.HTML() != "<b>milk</b>" || !slices.Equal(clip.Files(), []string{"/home/ana/list.md"}) ||
		clip.GetData("application/x-notes-item") != "42" {
		t.Fatalf("clipboard types %q text %q html %q files %q", clip.Types(), clip.Text(), clip.HTML(), clip.Files())
	}
	if got, ok := clip.Image(); !ok || got.Bounds().Dx() != 2 {
		t.Errorf("clipboard image %v, %v", got, ok)
	}

	// Another application copies; the app pastes the well-known types, or
	// the ones it names.
	var other tela.DataTransfer
	other.SetText("eggs")
	other.SetHTML("<i>eggs</i>")
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)
	other.Set("image/png", pngBuf.Bytes())
	other.SetData("application/x-notes-item", "7")
	app.SetClipboard(&other)
	app.Click(telatest.ByRole("button", "Paste"))
	if readErr != nil || read == nil {
		t.Fatalf("ReadClipboard: %v", readErr)
	}
	if want := []string{"text/plain;charset=utf-8", "text/html", "image/png"}; !slices.Equal(read.Types(), want) {
		t.Errorf("pasted types %q, want the well-known ones %q", read.Types(), want)
	}
	if read.Text() != "eggs" || read.HTML() != "<i>eggs</i>" {
		t.Errorf("pasted %q %q", read.Text(), read.HTML())
	}
	if _, ok := read.Image(); !ok {
		t.Error("pasted no image")
	}
	app.Click(telatest.ByRole("button", "Paste item"))
	if want := []string{"text/plain;charset=utf-8", "application/x-notes-item"}; !slices.Equal(read.Types(), want) || read.GetData("application/x-notes-item") != "7" {
		t.Errorf("pasted types %q, want %q", read.Types(), want)
	}
	app.Click(telatest.ByRole("button", "Copy text"))
	if got := app.Clipboard().Text(); got != "plain" {
		t.Errorf("clipboard text %q", got)
	}
	app.Click(telatest.ByRole("button", "Paste text"))
	if text != "plain" {
		t.Errorf("ReadClipboardText = %q", text)
	}

	app.Unsupported(telatest.ClipboardData)
	readErr = nil
	app.Click(telatest.ByRole("button", "Paste"))
	if !errors.Is(readErr, desktop.ErrUnsupported) {
		t.Errorf("ReadClipboard on a text-only clipboard: %v", readErr)
	}
	app.Click(telatest.ByRole("button", "Paste text"))
	if text != "plain" {
		t.Errorf("ReadClipboardText on a text-only clipboard = %q", text)
	}
}

// A native context menu runs the chosen item's OnSelect and done; a
// dismissed one only done; without native menus the call says
// ErrUnsupported at once, so the app draws its own.
func TestPopupMenu(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, func() tela.Node {
		drawn, setDrawn := tela.UseState(false)
		open := func() {
			err := desktop.PopupMenu(tela.WindowHandle{}, tela.Point{X: 10, Y: 20}, []desktop.MenuItem{
				{Label: "Copy", Accelerator: "Ctrl+C", OnSelect: func() { log = append(log, "copy") }},
				{Separator: true},
				{Label: "More", Submenu: []desktop.MenuItem{{Label: "Rename", OnSelect: func() { log = append(log, "rename") }}}},
				{Label: "Delete", Disabled: true},
			}, func(chosen bool, err error) { log = append(log, "done", strings.ToLower(strings.TrimSpace(boolText(chosen)))) })
			if errors.Is(err, desktop.ErrUnsupported) {
				setDrawn.Set(true)
			}
		}
		return Div(Button(OnClick(open), Text("Menu")), If(drawn, Text("drawn menu")))
	})
	app.Click(telatest.ByRole("button", "Menu"))
	ms := app.PopupMenus()
	if len(ms) != 1 || ms[0].At != (tela.Point{X: 10, Y: 20}) || ms[0].Items[0].Accelerator != "Ctrl+C" || !ms[0].Items[1].Separator ||
		ms[0].Items[2].Submenu[0].Label != "Rename" || !ms[0].Items[3].Disabled {
		t.Fatalf("popup menus %+v", ms)
	}
	app.ChoosePopupItem("More", "Rename")
	app.Click(telatest.ByRole("button", "Menu"))
	app.DismissPopupMenu()
	if want := []string{"rename", "done", "chosen", "done", "dismissed"}; !slices.Equal(log, want) {
		t.Errorf("log %q, want %q", log, want)
	}
	if len(app.PopupMenus()) != 0 {
		t.Error("a menu is still open")
	}
	app.Unsupported(telatest.NativeMenus)
	app.Click(telatest.ByRole("button", "Menu"))
	app.ExpectText("drawn menu")
}

func boolText(chosen bool) string {
	if chosen {
		return "chosen"
	}
	return "dismissed"
}

// A MenuBar sets the window's menu while mounted, follows its fields,
// runs the chosen item, and is removed at unmount; without native menu
// bars OnError gets ErrUnsupported.
func TestMenuBar(t *testing.T) {
	var errs []error
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(true)
		wrap, setWrap := tela.UseState(false)
		return Div(
			Textf("wrap: %v", wrap),
			Button(OnClick(func() { setOn.Set(!on) }), Text("Toggle")),
			If(on, tela.Component(desktop.MenuBar{
				Menus: []desktop.MenuItem{
					{Label: "File", Submenu: []desktop.MenuItem{{Label: "Quit", Accelerator: "Ctrl+Q"}}},
					{Label: "View", Submenu: []desktop.MenuItem{
						{Label: "Word wrap", Checkbox: true, Checked: wrap, OnSelect: func() { setWrap.Set(!wrap) }},
					}},
				},
				OnError: func(err error) { errs = append(errs, err) },
			})),
		)
	})
	bar := app.MenuBar()
	if len(bar) != 2 || bar[0].Submenu[0].Accelerator != "Ctrl+Q" || bar[1].Submenu[0].Checked {
		t.Fatalf("menu bar %+v", bar)
	}
	app.ChooseMenuItem("View", "Word wrap")
	app.ExpectText("wrap: true")
	if bar := app.MenuBar(); !bar[1].Submenu[0].Checked {
		t.Errorf("the menu bar did not follow the state: %+v", bar)
	}
	app.Click(telatest.ByRole("button", "Toggle"))
	if bar := app.MenuBar(); bar != nil {
		t.Errorf("menu bar after unmount: %+v", bar)
	}
	app.Unsupported(telatest.NativeMenus)
	app.Click(telatest.ByRole("button", "Toggle"))
	if len(errs) != 1 || !errors.Is(errs[0], desktop.ErrUnsupported) {
		t.Errorf("errors %v, want one ErrUnsupported", errs)
	}

	// A frameless window has no native menu bar either.
	var ferr error
	telatest.MountFunc(t, func() tela.Node {
		return tela.Component(desktop.MenuBar{Menus: []desktop.MenuItem{{Label: "File"}}, OnError: func(err error) { ferr = err }})
	}, telatest.Frameless())
	if !errors.Is(ferr, desktop.ErrUnsupported) {
		t.Errorf("frameless window: %v", ferr)
	}
}

// While an app runs, registration goes to its backend: looking before
// writing, the other application's association kept without Replace, the
// default only when asked.
func TestRegistrationInApp(t *testing.T) {
	notes := desktop.URLScheme("notes")
	doc := desktop.FileType(".note", "application/x-notes-note", "Notes document")
	app := telatest.MountApp(t, tela.App{AppID: "com.example.Notes", Root: tela.C(func() tela.Node { return Text("notes") })})
	forApp := desktop.ForApp(desktop.HandlerApp{Exec: "/opt/notes/notes"})
	if changed, err := desktop.Register(notes, forApp); !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	if changed, err := desktop.Register(notes, forApp); changed || err != nil {
		t.Errorf("Register again = %v, %v; want nothing written", changed, err)
	}
	regs := app.Registrations()
	if len(regs) != 1 || regs[0].Association != "notes:" || regs[0].App.ID != "com.example.Notes" || regs[0].App.Name != "Notes" ||
		regs[0].App.Exec != "/opt/notes/notes" || !regs[0].Default {
		t.Errorf("registrations %+v", regs)
	}
	app.HoldAssociation(doc, "org.other.Editor")
	if _, err := desktop.Register(doc, forApp); !errors.Is(err, desktop.ErrAssociationTaken) || !strings.Contains(err.Error(), "org.other.Editor") {
		t.Errorf("Register of a held type: %v", err)
	}
	if changed, err := desktop.Register(doc, forApp, desktop.Replace()); !changed || err != nil {
		t.Errorf("Register with Replace = %v, %v", changed, err)
	}
	app.SetDefaultHandler(doc, "org.other.Editor")
	if s, err := desktop.RegistrationStatus(doc); err != nil || !s.Registered || s.Default || s.Other != "org.other.Editor" {
		t.Errorf("status %+v, %v", s, err)
	}
	if r, err := desktop.MakeDefault(doc, forApp); r != desktop.DefaultSet || err != nil {
		t.Errorf("MakeDefault = %v, %v", r, err)
	}
	if s, _ := desktop.RegistrationStatus(doc); !s.Default {
		t.Errorf("status after MakeDefault %+v", s)
	}
	if changed, err := desktop.Unregister(notes); !changed || err != nil {
		t.Errorf("Unregister = %v, %v", changed, err)
	}
	if regs := app.Registrations(); len(regs) != 1 || regs[0].Association != ".note" {
		t.Errorf("after Unregister %+v", regs)
	}
	app.Unsupported(telatest.Associations)
	if _, err := desktop.Register(notes, forApp); !errors.Is(err, desktop.ErrUnsupported) {
		t.Errorf("Register without registration: %v", err)
	}
}

// Where there is no window of a running app (tela snapshot), a MenuBar
// reports ErrUnsupported too, so whoever draws a menu instead does.
func TestMenuBarWithoutWindow(t *testing.T) {
	var ferr error
	_, err := telatest.Render(tela.C(func() tela.Node {
		return tela.Component(desktop.MenuBar{Menus: []desktop.MenuItem{{Label: "File"}}, OnError: func(err error) { ferr = err }})
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(ferr, desktop.ErrUnsupported) {
		t.Errorf("OnError got %v, want ErrUnsupported", ferr)
	}
}
