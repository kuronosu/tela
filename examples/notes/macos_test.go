package main

import (
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The macOS integration of phase 15 (ADR-224, ADR-225, ADR-226): the OS's
// traffic lights over the app's title bar, the app's menu bar and Cmd+Q.
// The tests play a Mac on any OS: the look is named (kit.MacOS), the keys
// are the Mac's (telatest.PlatformKeys) and the headless window manager
// keeps the buttons where macOS puts them.

// mountMac opens the app as it runs on a Mac.
func mountMac(t *testing.T, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return mountLook(t, kit.MacOS, append([]telatest.Option{telatest.PlatformKeys("darwin")}, opts...)...)
}

var windowControls = telatest.ByRole("group", "Window controls")

// The OS keeps its buttons over the title bar: the bar draws no controls of
// its own, leaves their room free and has the height that centers them.
func TestMacTitleBarLeavesRoomForTheTrafficLights(t *testing.T) {
	sandbox(t)
	a := mountMac(t)
	r := telatest.MacTitleBarControls
	if got := a.WindowState().TitleBarControls; got != r {
		t.Fatalf("the window reports the OS's buttons at %v, want %v", got, r)
	}
	a.ExpectWindowState(telatest.ClientDecorations(true))
	a.ExpectCount(windowControls, 0)
	a.ExpectCount(telatest.ByRole("button", "Close"), 0)
	a.ExpectCount(telatest.ByRole("button", "Minimize"), 0)
	// The bar's own content (the badge, the title, Open and Save) starts
	// after the buttons.
	open := a.Bounds(openBtn)
	if open.X < r.X+r.Width {
		t.Errorf("Open is at x=%v, inside the traffic lights that end at %v", open.X, r.X+r.Width)
	}
	if title := a.Bounds(telatest.ByText("Notes").Nth(0)); title.X < r.X+r.Width {
		t.Errorf("the title is at x=%v, inside the traffic lights that end at %v", title.X, r.X+r.Width)
	}
	// The bar's height centers the lights: the buttons sit at its middle.
	if mid, want := open.Y+open.Height/2, r.Y+r.Height/2; math.Abs(float64(mid-want)) > 1 {
		t.Errorf("the bar's content is centered at y=%v, the lights at y=%v", mid, want)
	}
	// The bar is still a drag area.
	hasCaption := false
	for _, h := range a.HitRegions() {
		hasCaption = hasCaption || h.Kind == telatest.HitCaption
	}
	if !hasCaption {
		t.Errorf("the title bar has no drag region: %v", a.HitRegions())
	}
	a.MouseMove(windowW-60, windowH-120)
	a.Golden("notes_macos.png")
}

// A Mac window that the OS gives no buttons (fullscreen) draws its own
// controls, and the buttons coming back take their room again.
func TestMacTitleBarFullscreenDrawsItsOwnControls(t *testing.T) {
	sandbox(t)
	a := mountMac(t)
	before := a.Bounds(openBtn).X
	a.SetTitleBarControls(tela.Rect{})
	a.ExpectCount(windowControls, 1)
	a.SetTitleBarControls(telatest.MacTitleBarControls)
	a.ExpectCount(windowControls, 0)
	if x := a.Bounds(openBtn).X; x != before {
		t.Errorf("Open is at x=%v after the buttons came back, was at %v", x, before)
	}
}

// Where the OS draws no buttons over the content, the window is a Frameless
// one: the controls are drawn (the goldens of the other platforms).
func TestOverlayIsFramelessElsewhere(t *testing.T) {
	sandbox(t)
	a := mount(t)
	if got := a.WindowState().TitleBarControls; got.Width != 0 || got.Height != 0 {
		t.Errorf("the window reports buttons at %v on a platform that cannot keep them", got)
	}
	a.ExpectRole("group", "Window controls")
}

// The menu bar is the app's, with Settings… in the app menu.
func TestMacAppMenuBar(t *testing.T) {
	sandbox(t)
	a := mountMac(t)

	bar := a.AppMenuBar()
	var tops []string
	for _, m := range bar {
		tops = append(tops, m.Label)
	}
	if got := strings.Join(tops, ","); got != "File,Edit,View,Window" {
		t.Fatalf("menu bar = %s, want File,Edit,View,Window", got)
	}
	items := a.AppMenuItems()
	if len(items) != 1 || items[0].Label != "Settings…" || items[0].Accelerator != "CmdOrCtrl+," {
		t.Errorf("app items = %+v, want Settings… on Cmd+,", items)
	}
	file := bar[0].Submenu
	want := [][2]string{{"New Note", "CmdOrCtrl+N"}, {"Open…", "CmdOrCtrl+O"}, {"Save", "CmdOrCtrl+S"}, {"Delete Note", "CmdOrCtrl+Shift+D"}, {"", ""}, {"Close Window", "CmdOrCtrl+W"}}
	if len(file) != len(want) {
		t.Fatalf("File menu = %+v", file)
	}
	for i, w := range want {
		if file[i].Label != w[0] || file[i].Accelerator != w[1] {
			t.Errorf("File item %d = %q %q, want %q %q", i, file[i].Label, file[i].Accelerator, w[0], w[1])
		}
	}
	if bar[1].Role != desktop.RoleEditMenu || bar[3].Role != desktop.RoleWindowMenu {
		t.Errorf("Edit and Window are %v and %v, want the standard menus", bar[1].Role, bar[3].Role)
	}
	// Notes draws no menu bar of its own.
	a.ExpectNoText("File")
}

// Every item does what the button or key of the window does.
func TestMacAppMenuItems(t *testing.T) {
	sandbox(t)
	a := mountMac(t)

	a.ChooseAppMenuItem("File", "New Note")
	a.ExpectRole("button", "Untitled note")
	a.ExpectFocused(titleBox)
	a.Type("From the menu")
	a.Click(editor)
	a.Type("body")
	a.ExpectRole("button", "From the menu, unsaved changes")

	// Save asks where the note goes, as the button and Cmd+S do.
	a.ChooseAppMenuItem("File", "Save")
	if d := a.FileDialogs(); len(d) != 1 || d[0].Kind != telatest.SaveFileDialog {
		t.Fatalf("File > Save opened %+v, want a save dialog", d)
	}
	target := filepath.Join(t.TempDir(), "menu.txt")
	a.AnswerFileDialog(target)
	a.ExpectCount(unsaved, 0)

	a.ChooseAppMenuItem("File", "Open…")
	if d := a.FileDialogs(); len(d) != 1 || d[0].Kind != telatest.OpenFileDialog {
		t.Fatalf("File > Open… opened %+v, want an open dialog", d)
	}
	a.AnswerFileDialog()

	// View shows the overview of a view of the rail.
	a.ChooseAppMenuItem("View", "Pinned")
	a.Expect(railItem("Pinned"), telatest.Current("page"))
	a.ExpectText("Pinned")
	a.ChooseAppMenuItem("View", "All Notes")
	a.Expect(railItem("All notes"), telatest.Current("page"))

	// Delete Note acts on the open note.
	a.Click(card("From the menu"))
	a.ChooseAppMenuItem("File", "Delete Note")
	a.ExpectCount(card("From the menu"), 0)

	// Settings… in the app menu opens Preferences.
	prefs := a.WaitForWindow(func() { a.ChooseAppMenuItem("Settings…") })
	prefs.ExpectTitle("Preferences")
}

// Save and Delete need a note: the overview, with none open, has them off.
func TestMacAppMenuNeedsANote(t *testing.T) {
	sandbox(t)
	a := mountMac(t)
	disabled := func() (save, del bool) {
		for _, it := range a.AppMenuBar()[0].Submenu {
			switch it.Label {
			case "Save":
				save = it.Disabled
			case "Delete Note":
				del = it.Disabled
			}
		}
		return
	}
	if s, d := disabled(); s || d {
		t.Errorf("with a note open Save and Delete are disabled: %v %v", s, d)
	}
	a.Click(railItem("All notes"))
	if s, d := disabled(); !s || !d {
		t.Errorf("with the overview shown Save and Delete are enabled: %v %v", s, d)
	}
}

// Close Window closes the window as its button does; the Edit menu acts on
// the focused field.
func TestMacAppMenuRoles(t *testing.T) {
	sandbox(t)
	a := mountMac(t)
	typeNote(a, "hello world")
	a.ChooseAppMenuItem("Edit", "Select All")
	a.Type("x")
	a.ExpectValue(editor, "x")
	a.ChooseAppMenuItem("Edit", "Undo")
	a.ExpectValue(editor, "hello world")

	// With the tray setting on, Close Window hides the window, as the
	// close button does.
	setSetting(t, keyKeepInTray, true)
	b := mountMac(t)
	b.ChooseAppMenuItem("File", "Close Window")
	b.ExpectWindowState(telatest.Hidden(true))
	b.ExpectRunning()
}

// The keys of the menu reach the window only where there is no native bar:
// there the keyboard handler does what the items do.
func TestShortcutsWhereTheMenuIsNotNative(t *testing.T) {
	sandbox(t)
	a := mount(t)
	if bar := a.AppMenuBar(); len(bar) != 0 {
		t.Fatalf("a platform without a native menu bar got one: %+v", bar)
	}
	a.Click(editor)
	a.Press("ControlOrMeta+2")
	a.Expect(railItem("Pinned"), telatest.Current("page"))
	a.Press("ControlOrMeta+3")
	a.Expect(railItem("Unsaved"), telatest.Current("page"))
	a.Press("ControlOrMeta+1")
	a.Expect(railItem("All notes"), telatest.Current("page"))
	a.Click(card("Getting started"))
	a.WaitForWindow(func() { a.Press("ControlOrMeta+,") })
}

// Windows and Linux get no menu bar: not in the main window and not in the
// framed Preferences window, which desktop.AppMenuBar would give one.
func TestNoMenuBarOnWindows(t *testing.T) {
	sandbox(t)
	a := mountLook(t, kit.Windows, telatest.WithUnsupported(telatest.AppMenu))
	if bar := a.AppMenuBar(); len(bar) != 0 {
		t.Fatalf("the main window got a menu bar on Windows: %+v", bar)
	}
	prefs := openPreferences(a)
	if bar := prefs.AppMenuBar(); len(bar) != 0 {
		t.Fatalf("Preferences got a menu bar on Windows: %+v", bar)
	}
}

// Cmd+Q, the app menu's Quit and the Dock's Quit really quit, also with
// "keep running in the tray" on, after the question that closing asks.
func TestMacQuitRequest(t *testing.T) {
	for _, tray := range []bool{false, true} {
		name := "tray off"
		if tray {
			name = "tray on"
		}
		t.Run(name+"/clean", func(t *testing.T) {
			sandbox(t)
			setSetting(t, keyKeepInTray, tray)
			a := mountMac(t)
			a.RequestQuit()
			a.ExpectQuit()
		})
		t.Run(name+"/unsaved", func(t *testing.T) {
			sandbox(t)
			setSetting(t, keyKeepInTray, tray)
			a := mountMac(t)
			typeNote(a, "draft")
			a.RequestQuit()
			boxes := a.MessageBoxes()
			if len(boxes) != 1 {
				t.Fatalf("message boxes = %+v, want the question", boxes)
			}
			question := boxes[0]
			if question.Kind != desktop.MessageQuestion || question.Buttons != desktop.ButtonsYesNoCancel || !strings.Contains(question.Text, `"draft"`) {
				t.Errorf("message box = %+v, want a Yes/No/Cancel question about draft", question)
			}
			// It is the question that closing the window asks with the tray
			// setting off.
			if !tray {
				a.AnswerMessage(desktop.AnswerCancel)
				a.RequestClose()
				if b := a.MessageBoxes(); len(b) != 1 || b[0].Text != question.Text {
					t.Errorf("closing asks %+v, quitting asked %q", b, question.Text)
				}
				a.AnswerMessage(desktop.AnswerCancel)
				a.RequestQuit()
			}
			// Cancel keeps the app running and its window up.
			a.AnswerMessage(desktop.AnswerCancel)
			a.ExpectRunning()
			a.ExpectWindowState(telatest.Hidden(false))

			// No quits and leaves the draft in the library.
			a.RequestQuit()
			a.AnswerMessage(desktop.AnswerNo)
			a.ExpectQuit()
		})
	}
}

// A window hidden in the tray is shown to ask, and Yes saves the notes
// before it quits.
func TestMacQuitFromTheTraySavesFirst(t *testing.T) {
	sandbox(t)
	setSetting(t, keyKeepInTray, true)
	a := mountMac(t)
	typeNote(a, "draft")
	a.RequestClose()
	a.ExpectWindowState(telatest.Hidden(true))
	a.ExpectRunning()

	a.RequestQuit()
	a.ExpectWindowState(telatest.Hidden(false))
	a.AnswerMessage(desktop.AnswerYes)
	target := filepath.Join(t.TempDir(), "draft.txt")
	a.AnswerFileDialog(target)
	a.ExpectQuit()
	if got := read(t, target); got != "draft" {
		t.Errorf("the file has %q, want draft", got)
	}
}

// A hidden window with nothing to save quits at once, with no question.
func TestMacQuitFromTheTrayAsksNothingWhenClean(t *testing.T) {
	sandbox(t)
	setSetting(t, keyKeepInTray, true)
	a := mountMac(t)
	a.RequestClose()
	a.ExpectWindowState(telatest.Hidden(true))
	a.RequestQuit()
	a.ExpectQuit()
}
