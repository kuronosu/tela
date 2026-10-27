package desktop_test

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The file dialogs answer on the UI thread with the chosen paths, cancel
// is "" and a nil error, and the options reach the dialog.
func TestFileDialogs(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, func() tela.Node {
		path, setPath := tela.UseState("none")
		pick := func() {
			err := desktop.OpenFile(func(p string, err error) {
				switch {
				case err != nil:
					setPath.Set("error")
				case p == "":
					setPath.Set("canceled")
				default:
					setPath.Set(p)
				}
			}, desktop.Title("Open a note"), desktop.Directory("/notes"), desktop.Filter("Notes", "*.md", "*.txt"))
			if err != nil {
				log = append(log, err.Error())
			}
		}
		save := func() {
			_ = desktop.SaveFile(func(p string, err error) { setPath.Set("saved " + p) }, desktop.DefaultName("Untitled.md"))
		}
		many := func() {
			_ = desktop.OpenFiles(func(ps []string, err error) { setPath.Set(strings.Join(ps, "+")) })
		}
		folder := func() {
			_ = desktop.PickFolder(func(p string, err error) { setPath.Set("folder " + p) })
		}
		return Div(Text("path: "+path),
			Button(OnClick(pick), Text("Open")), Button(OnClick(save), Text("Save")),
			Button(OnClick(many), Text("Many")), Button(OnClick(folder), Text("Folder")))
	})
	app.Click(telatest.ByRole("button", "Open"))
	ds := app.FileDialogs()
	if len(ds) != 1 || ds[0].Kind != telatest.OpenFileDialog || ds[0].Title != "Open a note" || ds[0].Directory != "/notes" ||
		len(ds[0].Filters) != 1 || ds[0].Filters[0].Name != "Notes" || strings.Join(ds[0].Filters[0].Patterns, ",") != "*.md,*.txt" {
		t.Fatalf("dialogs %+v", ds)
	}
	app.AnswerFileDialog("/notes/groceries.md")
	app.ExpectText("path: /notes/groceries.md")

	app.Click(telatest.ByRole("button", "Open"))
	app.AnswerFileDialog()
	app.ExpectText("path: canceled")

	app.Click(telatest.ByRole("button", "Save"))
	if ds := app.FileDialogs(); len(ds) != 1 || ds[0].Kind != telatest.SaveFileDialog || ds[0].DefaultName != "Untitled.md" {
		t.Fatalf("save dialog %+v", ds)
	}
	app.AnswerFileDialog("/notes/new.md")
	app.ExpectText("path: saved /notes/new.md")

	app.Click(telatest.ByRole("button", "Many"))
	app.AnswerFileDialog("/a", "/b")
	app.ExpectText("path: /a+/b")

	app.Click(telatest.ByRole("button", "Folder"))
	app.AnswerFileDialog("/notes")
	app.ExpectText("path: folder /notes")

	app.Unsupported(telatest.FileDialogs)
	app.Click(telatest.ByRole("button", "Open"))
	if len(log) != 1 || !strings.Contains(log[0], "not supported") && !strings.Contains(log[0], "unsupported") {
		t.Errorf("an unsupported dialog returned %v", log)
	}
}

// The errors of a missing service wrap ErrUnsupported and
// errors.ErrUnsupported; without a running app, the error says so.
func TestUnsupportedErrors(t *testing.T) {
	if err := desktop.OpenFile(func(string, error) {}); err == nil || !strings.Contains(err.Error(), "running app") {
		t.Errorf("OpenFile without an app: %v", err)
	}
	var errs []error
	app := telatest.MountFunc(t, func() tela.Node {
		return Button(OnClick(func() {
			errs = append(errs,
				desktop.PickFolder(func(string, error) {}),
				func() error { _, err := desktop.Notify(desktop.Notification{Title: "x"}); return err }())
		}), Text("Go"))
	})
	app.Unsupported(telatest.FileDialogs, telatest.Notifications)
	app.Click(telatest.ByRole("button", "Go"))
	if len(errs) != 2 {
		t.Fatalf("errors %v", errs)
	}
	for _, err := range errs {
		if !errors.Is(err, desktop.ErrUnsupported) || !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("%v does not wrap desktop.ErrUnsupported and errors.ErrUnsupported", err)
		}
	}
}

// A message box is the backend's when it has one (Windows; the headless
// fake), and drawn in a modal window otherwise (Linux).
func TestShowMessage(t *testing.T) {
	for _, drawn := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "drawn"}[drawn], func(t *testing.T) {
			app := telatest.MountFunc(t, func() tela.Node {
				answer, setAnswer := tela.UseState("none")
				return Div(Text("answer: "+answer), Button(OnClick(func() {
					err := desktop.ShowMessage(desktop.Message{
						Kind: desktop.MessageQuestion, Title: "Unsaved changes", Text: "Save before closing?",
						Buttons: desktop.ButtonsYesNoCancel,
					}, func(a desktop.Answer) { setAnswer.Set(a.String()) })
					if err != nil {
						setAnswer.Set("error " + err.Error())
					}
				}), Text("Close")))
			})
			if drawn {
				app.Unsupported(telatest.MessageBoxes)
			}
			app.Click(telatest.ByRole("button", "Close"))
			boxes := app.MessageBoxes()
			want := telatest.MessageBox{Kind: desktop.MessageQuestion, Title: "Unsaved changes", Text: "Save before closing?",
				Buttons: desktop.ButtonsYesNoCancel, Drawn: drawn}
			if len(boxes) != 1 || boxes[0].Kind != want.Kind || boxes[0].Title != want.Title || boxes[0].Text != want.Text ||
				boxes[0].Buttons != want.Buttons || boxes[0].Drawn != want.Drawn {
				t.Fatalf("message boxes %+v, want %+v", boxes, want)
			}
			app.AnswerMessage(desktop.AnswerNo)
			app.ExpectText("answer: no")
			if n := len(app.MessageBoxes()); n != 0 {
				t.Errorf("%d boxes open after the answer", n)
			}
		})
	}
}

// Notifications run their callbacks on the UI thread: a click, an action,
// a dismissal.
func TestNotify(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		event, setEvent := tela.UseState("none")
		return Div(Text("event: "+event), Button(OnClick(func() {
			_, err := desktop.Notify(desktop.Notification{
				Title: "Export finished", Body: "notes.pdf was saved.",
				OnClick: func() { setEvent.Set("click") },
				OnClose: func() { setEvent.Set("close") },
				Actions: []desktop.NotificationAction{{Label: "Open", OnClick: func() { setEvent.Set("open") }}},
			})
			if err != nil {
				setEvent.Set(err.Error())
			}
		}), Text("Export")))
	})
	app.Click(telatest.ByRole("button", "Export"))
	ns := app.Notifications()
	if len(ns) != 1 || ns[0].Title != "Export finished" || ns[0].Body != "notes.pdf was saved." || strings.Join(ns[0].Actions, ",") != "Open" || ns[0].Closed {
		t.Fatalf("notifications %+v", ns)
	}
	app.ClickNotification("Export finished")
	app.ExpectText("event: click")

	app.Click(telatest.ByRole("button", "Export"))
	app.ClickNotificationAction("Export finished", "Open")
	app.ExpectText("event: open")

	app.Click(telatest.ByRole("button", "Export"))
	app.DismissNotification("Export finished")
	app.ExpectText("event: close")
}

// A shown notification can be withdrawn; its callbacks no longer run.
func TestNotificationClose(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		shown := tela.UseRef(desktop.ShownNotification{})
		return Div(
			Button(OnClick(func() {
				shown.Current, _ = desktop.Notify(desktop.Notification{Title: "Syncing"})
			}), Text("Start")),
			Button(OnClick(func() { _ = shown.Current.Close() }), Text("Stop")),
		)
	})
	app.Click(telatest.ByRole("button", "Start"))
	app.Click(telatest.ByRole("button", "Stop"))
	if ns := app.Notifications(); len(ns) != 1 || !ns[0].Closed {
		t.Errorf("notifications %+v; want one, closed", ns)
	}
}

var trayIcon = []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16)), image.NewRGBA(image.Rect(0, 0, 32, 32))}

// A Tray is shown while mounted, follows its fields, runs the callbacks of
// its items and goes away on unmount.
func TestTray(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		shown, setShown := tela.UseState(true)
		muted, setMuted := tela.UseState(false)
		clicks, setClicks := tela.UseState(0)
		return Div(
			Textf("muted: %v, clicks: %d", muted, clicks),
			Button(OnClick(func() { setShown.Set(false) }), Text("Hide tray")),
			If(shown, tela.Component(desktop.Tray{
				Icon:    trayIcon,
				Tooltip: "Notes",
				OnClick: func() { setClicks.Update(func(n int) int { return n + 1 }) },
				Menu: []desktop.MenuItem{
					{Label: "Mute", Checkbox: true, Checked: muted, OnSelect: func() { setMuted.Set(!muted) }},
					{Separator: true},
					{Label: "Theme", Submenu: []desktop.MenuItem{{Label: "Light"}, {Label: "Dark", OnSelect: func() { setClicks.Set(100) }}}},
					{Label: "Disabled", Disabled: true},
				},
			})),
		)
	})
	tr := app.Tray()
	if tr.Tooltip != "Notes" || len(tr.Menu) != 4 || tr.Menu[0].Label != "Mute" || !tr.Menu[0].Checkbox || tr.Menu[0].Checked ||
		!tr.Menu[1].Separator || len(tr.Menu[2].Submenu) != 2 || !tr.Menu[3].Disabled {
		t.Fatalf("tray %+v", tr)
	}
	app.ClickTrayItem("Mute")
	app.ExpectText("muted: true, clicks: 0")
	if tr := app.Tray(); !tr.Menu[0].Checked || tr.Updates != 1 {
		t.Errorf("after Mute: checked %v, %d updates; want true and 1", tr.Menu[0].Checked, tr.Updates)
	}
	app.ClickTray()
	app.ExpectText("muted: true, clicks: 1")
	app.ClickTrayItem("Theme", "Dark")
	app.ExpectText("muted: true, clicks: 100")
	if tr := app.Tray(); tr.Updates != 1 {
		t.Errorf("%d updates; a build with the same menu must not update the tray", tr.Updates)
	}
	app.Click(telatest.ByRole("button", "Hide tray"))
	if ts := app.Trays(); len(ts) != 0 {
		t.Errorf("trays after unmount: %+v", ts)
	}
}

// Without a tray, OnError gets an error wrapping ErrUnsupported.
func TestTrayUnsupported(t *testing.T) {
	var got error
	app := telatest.MountFunc(t, func() tela.Node {
		show, setShow := tela.UseState(false)
		return Div(Button(OnClick(func() { setShow.Set(true) }), Text("Show")),
			If(show, tela.Component(desktop.Tray{Icon: trayIcon, OnError: func(err error) { got = err }})))
	})
	app.Unsupported(telatest.TrayIcons)
	app.Click(telatest.ByRole("button", "Show"))
	if !errors.Is(got, desktop.ErrUnsupported) {
		t.Errorf("OnError got %v, want desktop.ErrUnsupported", got)
	}
}

// UseSetting reads the settings store, writes it, and builds every reader
// again when the value changes, from a component or from the store.
func TestUseSetting(t *testing.T) {
	telatest.TempDesktopDirs(t)
	desktop.SetAppID("com.example.settingstest")
	reader := func(name string) tela.Node {
		return tela.CP(func(n string) tela.Node {
			size, setSize := desktop.UseSetting("editor.fontSize", 14)
			return Div(Textf("%s: %d", n, size),
				Button(OnClick(func() { setSize.Update(func(v int) int { return v + 1 }) }), Text(n+" bigger")))
		}, name, Key(name))
	}
	app := telatest.MountFunc(t, func() tela.Node { return Div(reader("a"), reader("b")) })
	app.ExpectText("a: 14")
	app.Click(telatest.ByRole("button", "a bigger"))
	app.ExpectText("a: 15")
	app.ExpectText("b: 15")
	_ = desktop.Settings().Set("editor.fontSize", 20)
	app.ExpectText("a: 20")
	app.ExpectText("b: 20")
	if err := desktop.Settings().Flush(); err != nil {
		t.Fatal(err)
	}
	dir, _ := desktop.ConfigDir()
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil || !strings.Contains(string(data), `"editor.fontSize": 20`) {
		t.Errorf("settings.json: %q, %v", data, err)
	}
}

// UseSavePlacement saves the window's placement; LoadPlacement reads it
// back; PersistApp does both for the main window.
func TestPlacement(t *testing.T) {
	telatest.TempDesktopDirs(t)
	desktop.SetAppID("com.example.placementtest")
	if p := desktop.LoadPlacement("main"); p != (tela.WindowPlacement{}) {
		t.Fatalf("a placement before any was saved: %+v", p)
	}
	app := telatest.MountApp(t, desktop.PersistApp(tela.App{Width: 420, Height: 310, Root: tela.C(func() tela.Node { return Text("notes") })}))
	app.ExpectText("notes")
	if p := desktop.LoadPlacement(desktop.MainWindowKey); p.Width != 420 || p.Height != 310 {
		t.Errorf("saved placement %+v, want 420x310", p)
	}
	app.Resize(500, 400)
	app.Close()
	if p := desktop.LoadPlacement(desktop.MainWindowKey); p.Width != 500 || p.Height != 400 {
		t.Errorf("placement after a resize and close %+v, want 500x400", p)
	}
	if err := desktop.FlushAll(); err != nil {
		t.Fatal(err)
	}
	dir, _ := desktop.StateDir()
	if data, err := os.ReadFile(filepath.Join(dir, "windows.json")); err != nil || !strings.Contains(string(data), `"main"`) {
		t.Errorf("windows.json: %q, %v", data, err)
	}
	next := desktop.PersistApp(tela.App{Root: tela.C(func() tela.Node { return Text("x") })})
	if next.Placement.Width != 500 || next.Placement.Height != 400 {
		t.Errorf("PersistApp's placement %+v, want the saved 500x400", next.Placement)
	}
}
