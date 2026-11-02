//go:build darwin

package cocoa

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The desktop services tests. The dialogs, alerts and trays are shown and
// answered programmatically (cancel:, performClick:, the menu's
// performActionForItemAtIndex:), so they need no user; every run has a
// deadline and fails instead of hanging. The keychain test skips when the
// keychain asks the user (locked, or an access prompt), and the trash test
// moves only files it creates under the home folder, then deletes them
// from the Trash.

// testSel holds the selectors only the tests send.
var testSel = struct {
	attachedSheet, accessoryView, numberOfItems, indexOfSelected, allowedTypes    darwin.SEL
	sendAction, shouldEnable, buttons, performClick, title, image, size, reps     darwin.SEL
	toolTip, itemAtIndex, state, isEnabled, hasSubmenu, submenu, performAtIndex   darwin.SEL
	nameField, canChooseDirectories, canChooseFiles, allowsMultiple, alertStyleOf darwin.SEL
	message, informative, keyEquivalent, postEventAtStart                         darwin.SEL
}{
	attachedSheet:        darwin.Sel("attachedSheet"),
	accessoryView:        darwin.Sel("accessoryView"),
	numberOfItems:        darwin.Sel("numberOfItems"),
	indexOfSelected:      darwin.Sel("indexOfSelectedItem"),
	allowedTypes:         darwin.Sel("allowedContentTypes"),
	sendAction:           darwin.Sel("sendAction:to:from:"),
	shouldEnable:         darwin.Sel("panel:shouldEnableURL:"),
	buttons:              darwin.Sel("buttons"),
	performClick:         darwin.Sel("performClick:"),
	title:                darwin.Sel("title"),
	image:                darwin.Sel("image"),
	size:                 darwin.Sel("size"),
	reps:                 darwin.Sel("representations"),
	toolTip:              darwin.Sel("toolTip"),
	itemAtIndex:          darwin.Sel("itemAtIndex:"),
	state:                darwin.Sel("state"),
	isEnabled:            darwin.Sel("isEnabled"),
	hasSubmenu:           darwin.Sel("hasSubmenu"),
	submenu:              darwin.Sel("submenu"),
	performAtIndex:       darwin.Sel("performActionForItemAtIndex:"),
	nameField:            darwin.Sel("nameFieldStringValue"),
	canChooseDirectories: darwin.Sel("canChooseDirectories"),
	canChooseFiles:       darwin.Sel("canChooseFiles"),
	allowsMultiple:       darwin.Sel("allowsMultipleSelection"),
	alertStyleOf:         darwin.Sel("alertStyle"),
	message:              darwin.Sel("messageText"),
	informative:          darwin.Sel("informativeText"),
	keyEquivalent:        darwin.Sel("keyEquivalent"),
	postEventAtStart:     darwin.Sel("postEvent:atStart:"),
}

// eventWindow is the window of the event desktopRun's step is called
// with (nil for an app-scoped event); step's w is always the test's window.
var eventWindow platform.Window

// desktopRun runs b with one window. start is called on the UI thread
// once the window presented its first frame; step for every event after
// that, until it returns true or the run is past its deadline. Every
// window's first frame is presented (a sheet needs a window on screen),
// and a ticker wakes the loop every 50 ms so steps can poll. It reports
// whether the deadline ended the run.
func desktopRun(t *testing.T, b *Backend, deadline time.Duration, start func(w *Window), step func(w *Window, e platform.Event) bool) bool {
	t.Helper()
	var timedOut atomic.Bool
	onMain(func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "tela " + t.Name(), Size: platform.Size{W: 480, H: 360}})
		if err != nil {
			t.Errorf("NewWindow: %v", err)
			return
		}
		w := pw.(*Window)
		timer := time.AfterFunc(deadline, func() {
			timedOut.Store(true)
			b.Quit()
			// Should the loop's source not fire inside a modal session,
			// Quit would never be seen there: abortModal is the call
			// AppKit allows from another thread.
			time.Sleep(time.Second)
			if darwin.Send(app.ns, darwin.Sel("modalWindow")) != 0 {
				darwin.Send(app.ns, darwin.Sel("abortModal"))
			}
		})
		defer timer.Stop()
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			tick := time.NewTicker(50 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-tick.C:
					b.Wake()
				case <-stop:
					return
				}
			}
		}()
		presented := map[platform.Window]bool{}
		started, finished := false, false
		_ = b.Run(platform.HandlerFunc(func(win platform.Window, e platform.Event) {
			if _, ok := e.(platform.FrameEvent); ok && win != nil && !presented[win] {
				presented[win] = true
				presentSolid(t, win, color.RGBA{0x30, 0x40, 0x60, 0xff})
			}
			eventWindow = win
			switch {
			case finished:
			case !started:
				if win == w && presented[w] {
					started = true
					start(w)
				}
			case step(w, e):
				finished = true
				b.Quit()
			}
		}))
	})
	if timedOut.Load() {
		t.Errorf("the run did not finish within %v", deadline)
	}
	return timedOut.Load()
}

func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s did not panic", what)
		} else {
			t.Logf("%s: %v", what, r)
		}
	}()
	fn()
}

func count(arr darwin.ID) int {
	if arr == 0 {
		return 0
	}
	return darwin.SendInt(arr, darwin.Sel("count"))
}

// TestDesktopArguments checks the argument checks and the synchronous
// errors, without showing anything.
func TestDesktopArguments(t *testing.T) {
	b := newBackend(t)
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}
	desktopRun(t, b, 20*time.Second, func(w *Window) {
		d := b.Desktop()
		mustPanic(t, "OpenFiles with a nil callback", func() { _ = d.OpenFiles(platform.FileDialogOptions{}, nil) })
		mustPanic(t, "SaveFile with a pattern holding ';'", func() {
			_ = d.SaveFile(platform.FileDialogOptions{Filters: []platform.FileFilter{{Patterns: []string{"*.a;*.b"}}}}, func(string, error) {})
		})
		mustPanic(t, "PickFolder with a nil callback", func() { _ = d.PickFolder(platform.FileDialogOptions{}, nil) })
		mustPanic(t, "MessageBox with invalid buttons", func() {
			_ = d.MessageBox(platform.MessageBoxOptions{Buttons: 9}, func(platform.MessageAnswer, error) {})
		})
		mustPanic(t, "NewTray without an icon", func() { _, _ = d.NewTray(platform.TrayOptions{}) })
		mustPanic(t, "NewTray with an item without ID", func() {
			_, _ = d.NewTray(platform.TrayOptions{Icon: icon, Menu: []platform.MenuItem{{Label: "x"}}})
		})
		mustPanic(t, "Notify with the action ID default", func() {
			_, _ = d.Notify(platform.Notification{Actions: []platform.NotificationAction{{ID: "default"}}})
		})
		mustPanic(t, "Secret without a name", func() { _, _ = d.Secret("dev.telaui.test", "") })
		if len(b.desk.dialogs) != 0 || len(b.desk.trays) != 0 {
			t.Errorf("an invalid request left %d dialogs and %d trays", len(b.desk.dialogs), len(b.desk.trays))
		}
		for _, uri := range []string{"no scheme", "file:///etc/hosts", "%zz"} {
			if err := d.OpenURI(nil, uri); err == nil {
				t.Errorf("OpenURI(%q) = nil, want an error", uri)
			}
		}
		if err := d.OpenPath(nil, ""); err == nil {
			t.Errorf("OpenPath(\"\") = nil, want an error")
		}
		if err := d.OpenPath(nil, filepath.Join(t.TempDir(), "missing")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("OpenPath of a missing file = %v, want fs.ErrNotExist", err)
		}
		if err := d.UpdateTray(77, platform.TrayOptions{Icon: icon}); err == nil {
			t.Errorf("UpdateTray of an unknown tray = nil, want an error")
		}
		if err := d.RemoveTray(77); err != nil {
			t.Errorf("RemoveTray of an unknown tray = %v, want nil", err)
		}
		if err := d.CloseNotification(77); err != nil {
			t.Errorf("CloseNotification of an unknown notification = %v, want nil", err)
		}
		if err := d.SetSecret("dev.telaui.test", "x", make([]byte, platform.MaxSecretSize+1)); !errors.Is(err, platform.ErrSecretTooLarge) {
			t.Errorf("SetSecret of %d bytes = %v, want ErrSecretTooLarge", platform.MaxSecretSize+1, err)
		}
		if err := d.MoveToTrash("relative/path"); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("MoveToTrash of a relative path = %v, want fs.ErrInvalid", err)
		}
		// Shortcuts and single instance: shortcut_darwin_test.go and
		// launch_darwin_test.go.
	}, func(*Window, platform.Event) bool { return true })
}

// TestNotifyWithoutBundle: a test binary is not an app bundle, so Notify
// refuses with ErrUnsupported and names tela run, without touching the
// notification center (which would raise an exception).
func TestNotifyWithoutBundle(t *testing.T) {
	b := newBackend(t)
	var bundle, ident string
	desktopRun(t, b, 20*time.Second, func(w *Window) {
		pool := darwin.PoolPush()
		mb := darwin.SendClass(darwin.GetClass("NSBundle"), nsel.mainBundle)
		bundle = darwin.GoString(darwin.Send(mb, nsel.bundlePath))
		ident = darwin.GoString(darwin.Send(mb, nsel.bundleIdentifier))
		darwin.PoolPop(pool)
		id, err := b.Desktop().Notify(platform.Notification{Title: "tela", Body: "test"})
		t.Logf("main bundle %q, identifier %q: Notify = %d, %v", bundle, ident, id, err)
		if !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), "tela run") {
			t.Errorf("Notify without a bundle = %d, %v; want an error wrapping ErrUnsupported that names tela run", id, err)
		}
		if b.desk.notes.center != 0 {
			t.Errorf("Notify without a bundle got the notification center")
		}
		if err := b.Desktop().CloseNotification(1); err != nil {
			t.Errorf("CloseNotification without a bundle = %v, want nil", err)
		}
	}, func(*Window, platform.Event) bool { return true })
}

// dialogRound is one dialog of a script: opened by open, inspected and
// ended by act once it is on screen, answered once through the loop.
type dialogRound struct {
	name  string
	open  func(w *Window, done func(any)) error
	check func(r *dialogRequest) // once visible, before act
	act   func(r *dialogRequest) // ends it (cancel:, a button)
	want  any                    // what done receives
}

// runDialogs runs the rounds in order and checks that each dialog shows,
// that the loop keeps delivering while it is open (wake ticks), and that
// done runs exactly once, later, through the loop, with the wanted value.
func runDialogs(t *testing.T, rounds ...dialogRound) {
	b := newBackend(t)
	i := 0
	calls := make([]int, len(rounds))
	got := make([]any, len(rounds))
	var opened time.Time
	var wakes int
	acted := false
	begin := func(w *Window) bool {
		r := rounds[i]
		wakes, acted, opened = 0, false, time.Now()
		k := i
		inCall := true
		err := r.open(w, func(v any) {
			if inCall {
				t.Errorf("%s: done ran inside the call", r.name)
			}
			calls[k]++
			got[k] = v
		})
		inCall = false
		if err != nil {
			t.Errorf("%s: %v", r.name, err)
			return false
		}
		return true
	}
	desktopRun(t, b, 60*time.Second, func(w *Window) {
		if !begin(w) {
			i = len(rounds)
		}
	}, func(w *Window, e platform.Event) bool {
		if i >= len(rounds) {
			return true
		}
		r := rounds[i]
		if calls[i] > 0 {
			if !acted {
				t.Errorf("%s: done ran before the dialog was answered", r.name)
			}
			t.Logf("%s: done(%v) after %v, %d wake passes while it was open", r.name, got[i], time.Since(opened).Round(time.Millisecond), wakes)
			i++
			if i == len(rounds) {
				return true
			}
			if !begin(w) {
				return true
			}
			return false
		}
		if _, ok := e.(platform.WakeEvent); !ok || acted {
			return false
		}
		if len(b.desk.dialogs) == 0 {
			t.Errorf("%s: no open dialog and no answer", r.name)
			return true
		}
		req := b.desk.dialogs[len(b.desk.dialogs)-1]
		if !darwin.SendBool(req.window(), psel.isVisible) {
			if time.Since(opened) > 10*time.Second {
				t.Errorf("%s: the dialog is not visible after 10 s", r.name)
				return true
			}
			return false
		}
		wakes++
		if wakes < 3 {
			return false // the loop delivers while the dialog is open
		}
		if r.check != nil {
			r.check(req)
		}
		acted = true
		r.act(req)
		return false
	})
	for k, r := range rounds {
		if k >= i && calls[k] == 0 {
			continue // reported above
		}
		if calls[k] != 1 {
			t.Errorf("%s: done ran %d times, want once", r.name, calls[k])
		}
		if fmt.Sprint(got[k]) != fmt.Sprint(r.want) {
			t.Errorf("%s: done(%v), want done(%v)", r.name, got[k], r.want)
		}
	}
}

func cancelPanel(r *dialogRequest) { darwin.Send(r.obj, psel.cancel, darwin.ID(0)) }

// clickAlert presses button i of an alert, and posts an event so that a
// modal session ended outside an event (here, a wake pass) returns.
func clickAlert(i int) func(r *dialogRequest) {
	return func(r *dialogRequest) {
		btns := darwin.ArrayItems(darwin.Send(r.obj, testSel.buttons))
		if i >= len(btns) {
			return
		}
		darwin.Send(btns[i], testSel.performClick, darwin.ID(0))
		ev := darwin.SendClass(app.event, sel.otherEvent,
			uintptr(darwin.NSEventTypeApplicationDefined), darwin.CGPoint{}, uintptr(0), float64(0),
			uintptr(0), uintptr(0), uintptr(0), uintptr(0), uintptr(0))
		darwin.Send(app.ns, testSel.postEventAtStart, ev, true)
	}
}

func pathsAnswer(v []string, err error) any { return fmt.Sprint(v, err) }
func pathAnswer(v string, err error) any    { return fmt.Sprintf("%q %v", v, err) }

var testFilters = []platform.FileFilter{
	{Name: "Images", Patterns: []string{"*.png", "*.jpg"}},
	{Name: "Read me", Patterns: []string{"README*", "*.txt"}},
}

// checkOpenFilters checks the format popup of an open panel with
// testFilters, and that choosing the second filter changes the types and
// the delegate's filter.
func checkOpenFilters(t *testing.T) func(r *dialogRequest) {
	return func(r *dialogRequest) {
		panel := r.obj
		t.Logf("open panel: accessory view %v, popup %v, delegate %v, %d allowed types",
			darwin.Send(panel, testSel.accessoryView) != 0, r.popup != 0, r.delegate, count(darwin.Send(panel, testSel.allowedTypes)))
		if r.popup == 0 || darwin.SendInt(r.popup, testSel.numberOfItems) != 2 || darwin.SendInt(r.popup, testSel.indexOfSelected) != 0 {
			t.Errorf("the format popup is missing, or does not hold the 2 filters with the first selected")
			return
		}
		if n := count(darwin.Send(panel, testSel.allowedTypes)); n != 2 {
			t.Errorf("the first filter allows %d content types, want 2 (png, jpg)", n)
		}
		if !darwin.SendBool(panel, testSel.canChooseFiles) || darwin.SendBool(panel, testSel.canChooseDirectories) || !darwin.SendBool(panel, testSel.allowsMultiple) {
			t.Errorf("the open panel does not choose several files only")
		}
		darwin.Send(r.popup, psel.selectItemAtIndex, 1)
		darwin.SendBool(app.ns, testSel.sendAction, deskClasses.selFilterChanged, r.helper, r.popup)
		if r.active != 1 {
			t.Errorf("choosing the second format left filter %d active", r.active)
		}
		if n := count(darwin.Send(panel, testSel.allowedTypes)); n != 0 {
			t.Errorf("the second filter (with a glob) allows %d content types, want 0 (the delegate filters)", n)
		}
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		dir := os.TempDir()
		for name, want := range map[string]bool{"README": true, "notes.TXT": true, "photo.png": false} {
			url := darwin.FileURL(filepath.Join(dir, name), false)
			if got := darwin.SendBool(r.helper, testSel.shouldEnable, panel, url); got != want {
				t.Errorf("panel:shouldEnableURL: %s = %v, want %v", name, got, want)
			}
		}
	}
}

// TestFileDialogs shows each file dialog as a separate panel and cancels
// it programmatically.
func TestFileDialogs(t *testing.T) {
	var d platform.Desktop
	runDialogs(t,
		dialogRound{
			name: "OpenFiles",
			open: func(w *Window, done func(any)) error {
				d = w.b.Desktop()
				return d.OpenFiles(platform.FileDialogOptions{Title: "tela test: open", Directory: os.TempDir(), Multiple: true, Filters: testFilters},
					func(p []string, err error) { done(pathsAnswer(p, err)) })
			},
			check: checkOpenFilters(t),
			act:   cancelPanel,
			want:  pathsAnswer(nil, nil),
		},
		dialogRound{
			name: "SaveFile",
			open: func(w *Window, done func(any)) error {
				return d.SaveFile(platform.FileDialogOptions{Title: "tela test: save", Name: "report", Filters: testFilters[1:]},
					func(p string, err error) { done(pathAnswer(p, err)) })
			},
			check: func(r *dialogRequest) {
				name := darwin.GoString(darwin.Send(r.obj, testSel.nameField))
				n := count(darwin.Send(r.obj, testSel.allowedTypes))
				t.Logf("save panel: name field %q, %d allowed types, popup %v", name, n, r.popup != 0)
				if !strings.HasPrefix(name, "report") || n != 1 || r.popup != 0 {
					t.Errorf("save panel: name %q, %d types, popup %v; want report, 1 type (txt), no popup", name, n, r.popup != 0)
				}
			},
			act:  cancelPanel,
			want: pathAnswer("", nil),
		},
		dialogRound{
			name: "PickFolder",
			open: func(w *Window, done func(any)) error {
				return d.PickFolder(platform.FileDialogOptions{Directory: os.TempDir()}, func(p string, err error) { done(pathAnswer(p, err)) })
			},
			check: func(r *dialogRequest) {
				if darwin.SendBool(r.obj, testSel.canChooseFiles) || !darwin.SendBool(r.obj, testSel.canChooseDirectories) {
					t.Errorf("the folder panel does not choose folders only")
				}
			},
			act:  cancelPanel,
			want: pathAnswer("", nil),
		},
	)
}

// TestFileDialogSheets shows file dialogs as sheets: one canceled, one
// ended by closing its window (Window.Close), which answers canceled.
func TestFileDialogSheets(t *testing.T) {
	runDialogs(t,
		dialogRound{
			name: "OpenFiles sheet",
			open: func(w *Window, done func(any)) error {
				return w.b.Desktop().OpenFiles(platform.FileDialogOptions{Parent: w, Filters: testFilters},
					func(p []string, err error) { done(pathsAnswer(p, err)) })
			},
			check: func(r *dialogRequest) {
				var attached darwin.ID
				if r.sheet != nil {
					attached = darwin.Send(r.sheet.ns, testSel.attachedSheet)
				}
				t.Logf("sheet: parent %v, attached sheet is the panel: %v", r.sheet != nil, attached == r.obj)
				if r.sheet == nil || attached != r.obj {
					t.Errorf("the panel is not a sheet on its parent")
				}
			},
			act:  cancelPanel,
			want: pathsAnswer(nil, nil),
		},
		dialogRound{
			name: "SaveFile sheet on a closing window",
			open: func(w *Window, done func(any)) error {
				return w.b.Desktop().SaveFile(platform.FileDialogOptions{Parent: w, Name: "x.txt"},
					func(p string, err error) { done(pathAnswer(p, err)) })
			},
			act: func(r *dialogRequest) {
				if r.sheet == nil {
					t.Errorf("the save panel is not a sheet")
					cancelPanel(r)
					return
				}
				r.sheet.Close()
			},
			want: pathAnswer("", nil),
		},
	)
}

// TestMessageBoxes shows an alert as a sheet answered with a button, and
// an app-modal alert answered from a wake pass, which only arrives when
// the run loop source fires inside the modal session.
func TestMessageBoxes(t *testing.T) {
	answer := func(a platform.MessageAnswer, err error) any { return fmt.Sprint(a, err) }
	runDialogs(t,
		dialogRound{
			name: "MessageBox sheet",
			open: func(w *Window, done func(any)) error {
				return w.b.Desktop().MessageBox(platform.MessageBoxOptions{Parent: w, Kind: platform.MessageQuestion,
					Title: "Save changes?", Text: "tela test", Buttons: platform.ButtonsYesNoCancel},
					func(a platform.MessageAnswer, err error) { done(answer(a, err)) })
			},
			check: func(r *dialogRequest) {
				var titles []string
				for _, btn := range darwin.ArrayItems(darwin.Send(r.obj, testSel.buttons)) {
					titles = append(titles, darwin.GoString(darwin.Send(btn, testSel.title))+
						fmt.Sprintf("(%q)", darwin.GoString(darwin.Send(btn, testSel.keyEquivalent))))
				}
				msg := darwin.GoString(darwin.Send(r.obj, testSel.message))
				info := darwin.GoString(darwin.Send(r.obj, testSel.informative))
				t.Logf("alert sheet %v: %q / %q, style %d, buttons %v", r.sheet != nil, msg, info, darwin.SendInt(r.obj, testSel.alertStyleOf), titles)
				if r.sheet == nil || len(titles) != 3 || !strings.HasPrefix(titles[0], "Yes") || !strings.HasPrefix(titles[2], "No") {
					t.Errorf("want a sheet with Yes, Cancel, No")
				}
				if msg != "Save changes?" || info != "tela test" {
					t.Errorf("message %q, informative %q", msg, info)
				}
			},
			act:  clickAlert(2),
			want: answer(platform.AnswerNo, nil),
		},
		dialogRound{
			name: "MessageBox app-modal",
			open: func(w *Window, done func(any)) error {
				return w.b.Desktop().MessageBox(platform.MessageBoxOptions{Kind: platform.MessageWarning,
					Title: "tela test", Text: "app-modal", Buttons: platform.ButtonsOKCancel},
					func(a platform.MessageAnswer, err error) { done(answer(a, err)) })
			},
			check: func(r *dialogRequest) {
				t.Logf("app-modal alert: sheet %v, runModal running %v", r.sheet != nil, r.b.desk.modalRunning)
				if r.sheet != nil || !r.b.desk.modalRunning {
					t.Errorf("want an alert in runModal, delivering wake passes")
				}
			},
			act:  clickAlert(1),
			want: answer(platform.AnswerCancel, nil),
		},
	)
}

// TestMessageBoxQuit: Quit while an app-modal alert is open ends the
// alert's modal session and then Run, without calling done.
func TestMessageBoxQuit(t *testing.T) {
	b := newBackend(t)
	calls := 0
	quitAt := time.Time{}
	desktopRun(t, b, 30*time.Second, func(w *Window) {
		if err := b.Desktop().MessageBox(platform.MessageBoxOptions{Text: "tela test: quit", Buttons: platform.ButtonsOK},
			func(platform.MessageAnswer, error) { calls++ }); err != nil {
			t.Errorf("MessageBox: %v", err)
		}
	}, func(w *Window, e platform.Event) bool {
		if _, ok := e.(platform.WakeEvent); ok && quitAt.IsZero() && b.desk.modalRunning && len(b.desk.dialogs) == 1 &&
			darwin.SendBool(b.desk.dialogs[0].window(), psel.isVisible) {
			quitAt = time.Now()
			b.Quit()
		}
		return false
	})
	if quitAt.IsZero() {
		t.Errorf("the alert never showed")
	}
	t.Logf("Run returned %v after Quit; done ran %d times", time.Since(quitAt).Round(time.Millisecond), calls)
	if calls != 0 {
		t.Errorf("done ran %d times after Quit, want 0 (Run returned first)", calls)
	}
}

// TestTray adds a status item, chooses its menu items, clicks it without
// a menu, removes it, and checks that Run removes the trays left.
func TestTray(t *testing.T) {
	b := newBackend(t)
	img := func(side int) image.Image {
		m := image.NewNRGBA(image.Rect(0, 0, side, side))
		for i := range m.Pix {
			m.Pix[i] = 0xc0
		}
		return m
	}
	icon := []image.Image{img(16), img(32), img(64)}
	menu := []platform.MenuItem{
		{ID: 1, Label: "Open"},
		{Separator: true},
		{ID: 2, Label: "Pinned", Checkable: true, Checked: true},
		{ID: 3, Label: "Off", Disabled: true},
		{Label: "More", Submenu: []platform.MenuItem{{ID: 4, Label: "Deep"}}},
	}
	var id platform.TrayID
	var events []string
	step := 0
	var sawLeftover bool
	desktopRun(t, b, 30*time.Second, func(w *Window) {
		var err error
		id, err = b.Desktop().NewTray(platform.TrayOptions{Icon: icon, Tooltip: "tela test", Menu: menu})
		if err != nil {
			t.Errorf("NewTray: %v", err)
			step = 99
			return
		}
		tr := b.desk.trays[id]
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		button := darwin.Send(tr.item, trsel.button)
		nsImage := darwin.Send(button, testSel.image)
		size := darwin.SendPoint(nsImage, testSel.size) // NSSize: two doubles, like a point
		reps := count(darwin.Send(nsImage, testSel.reps))
		tip := darwin.GoString(darwin.Send(button, testSel.toolTip))
		nsMenu := darwin.Send(tr.item, darwin.Sel("menu"))
		t.Logf("tray %d: image %vx%v points, %d representations, tooltip %q, menu of %d items, bar %v points",
			id, size.X, size.Y, reps, tip, darwin.SendInt(nsMenu, testSel.numberOfItems), darwin.SendFloat(statusBar(), trsel.thickness))
		if size.Y < 16 || reps != 2 || tip != "tela test" || darwin.SendInt(nsMenu, testSel.numberOfItems) != 5 {
			t.Errorf("want an image at least 16 points tall with 2 representations, the tooltip and 5 menu items")
		}
		item := func(i int) darwin.ID { return darwin.Send(nsMenu, testSel.itemAtIndex, i) }
		if darwin.SendInt(item(2), testSel.state) != darwin.NSControlStateValueOn || darwin.SendBool(item(3), testSel.isEnabled) ||
			!darwin.SendBool(item(4), testSel.hasSubmenu) {
			t.Errorf("menu items: want a check mark on 2, 3 disabled, a submenu on 4")
		}
		darwin.Send(nsMenu, testSel.performAtIndex, 0)
		darwin.Send(nsMenu, testSel.performAtIndex, 3) // disabled: no event
		darwin.Send(darwin.Send(item(4), testSel.submenu), testSel.performAtIndex, 0)
	}, func(w *Window, e platform.Event) bool {
		if te, ok := e.(platform.TrayEvent); ok {
			events = append(events, fmt.Sprintf("%d %v %d", te.Tray, te.Kind, te.Item))
		}
		if _, ok := e.(platform.WakeEvent); !ok {
			return false
		}
		d := b.Desktop()
		switch step {
		case 0: // the menu's events arrived
			if len(events) < 2 {
				return false
			}
			want := []string{fmt.Sprintf("%d menu-item 1", id), fmt.Sprintf("%d menu-item 4", id)}
			if fmt.Sprint(events) != fmt.Sprint(want) {
				t.Errorf("menu events %q, want %q", events, want)
			}
			events = nil
			if err := d.UpdateTray(id, platform.TrayOptions{Icon: icon[:1]}); err != nil {
				t.Errorf("UpdateTray: %v", err)
			}
			tr := b.desk.trays[id]
			if darwin.Send(tr.item, darwin.Sel("menu")) != 0 {
				t.Errorf("UpdateTray without a menu left the menu")
			}
			darwin.Send(darwin.Send(tr.item, trsel.button), testSel.performClick, darwin.ID(0))
			step++
		case 1: // the click arrived
			if len(events) == 0 {
				return false
			}
			if want := fmt.Sprintf("%d activated 0", id); fmt.Sprint(events) != fmt.Sprint([]string{want}) {
				t.Errorf("click events %q, want [%q]", events, want)
			}
			if err := d.RemoveTray(id); err != nil {
				t.Errorf("RemoveTray: %v", err)
			}
			if err := d.UpdateTray(id, platform.TrayOptions{Icon: icon}); err == nil {
				t.Errorf("UpdateTray after RemoveTray = nil, want an error")
			}
			if _, err := d.NewTray(platform.TrayOptions{Icon: icon, Tooltip: "left for Run"}); err != nil {
				t.Errorf("NewTray: %v", err)
			}
			sawLeftover = len(b.desk.trays) == 1
			return true
		default:
			return true
		}
		return false
	})
	if !sawLeftover {
		t.Errorf("the second tray was not added")
	}
	if len(b.desk.trays) != 0 {
		t.Errorf("%d trays left after Run returned", len(b.desk.trays))
	}
}

// --- secrets and the trash (any goroutine, no loop) ---

// errBlocked is blocking's error for a call that did not return in time.
var errBlocked = errors.New("the call did not return in time")

// blocking runs fn on its own goroutine and waits at most d; it returns
// errBlocked when fn did not return (it is left running: a keychain
// prompt).
func blocking[T any](d time.Duration, fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() { v, err := fn(); ch <- result{v, err} }()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(d):
		var zero T
		return zero, errBlocked
	}
}

func loadDarwin(t *testing.T) {
	t.Helper()
	if err := darwin.Load(); err != nil {
		t.Skipf("darwin bindings unavailable: %v", err)
	}
}

// TestKeychain round-trips a secret with a unique name off the main
// thread. It skips when the keychain is locked or asks for access (a
// call that does not return within 15 s, or a canceled or unsupported
// keychain), since nobody answers a prompt on CI.
func TestKeychain(t *testing.T) {
	loadDarwin(t)
	var d platform.Desktop = desktop{}
	const app = "dev.telaui.test"
	name := fmt.Sprintf("tela-test-%d", time.Now().UnixNano())
	call := func(what string, fn func() ([]byte, error)) ([]byte, error) {
		t.Helper()
		start := time.Now()
		v, err := blocking(15*time.Second, fn)
		if err == errBlocked {
			t.Skipf("%s did not return within 15 s: the keychain is showing a prompt (locked, or asking for access); skipped", what)
		}
		t.Logf("%s: %d bytes, %v (%v)", what, len(v), err, time.Since(start).Round(time.Millisecond))
		if errors.Is(err, platform.ErrUnsupported) || errors.Is(err, platform.ErrSecretCanceled) {
			t.Skipf("%s: %v; skipped", what, err)
		}
		return v, err
	}
	t.Cleanup(func() { blocking(15*time.Second, func() (int, error) { return 0, d.DeleteSecret(app, name) }) })

	secret := []byte("tela \x00 secret \xff")
	if _, err := call("SetSecret", func() ([]byte, error) { return nil, d.SetSecret(app, name, secret) }); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	if v, err := call("Secret", func() ([]byte, error) { return d.Secret(app, name) }); err != nil || string(v) != string(secret) {
		t.Fatalf("Secret = %q, %v; want %q", v, err, secret)
	}
	if _, err := call("SetSecret again", func() ([]byte, error) { return nil, d.SetSecret(app, name, []byte("v2")) }); err != nil {
		t.Fatalf("SetSecret replacing: %v", err)
	}
	if v, err := call("Secret again", func() ([]byte, error) { return d.Secret(app, name) }); err != nil || string(v) != "v2" {
		t.Fatalf("Secret after replacing = %q, %v; want v2", v, err)
	}
	if _, err := call("DeleteSecret", func() ([]byte, error) { return nil, d.DeleteSecret(app, name) }); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if v, err := call("Secret deleted", func() ([]byte, error) { return d.Secret(app, name) }); !errors.Is(err, platform.ErrSecretNotFound) {
		t.Errorf("Secret after DeleteSecret = %q, %v; want ErrSecretNotFound", v, err)
	}
	if _, err := call("DeleteSecret again", func() ([]byte, error) { return nil, d.DeleteSecret(app, name) }); err != nil {
		t.Errorf("DeleteSecret of a missing secret = %v, want nil", err)
	}
}

// TestTrash moves a file, a symbolic link and a folder made under the
// home folder (the startup disk's ~/.Trash takes them) to the Trash,
// checks that they left their place (and, for the link, that its target
// stayed), and deletes them from the Trash.
func TestTrash(t *testing.T) {
	loadDarwin(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home folder: %v", err)
	}
	dir := filepath.Join(home, fmt.Sprintf(".tela-trash-test-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Skipf("cannot write under the home folder: %v", err)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "tela trash test.txt")
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link")
	folder := filepath.Join(dir, "folder")
	for _, p := range []string{file, target} {
		if err := os.WriteFile(p, []byte("tela"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(folder, "inner"), 0o700); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{file, link, folder} {
		res, err := blocking(30*time.Second, func() (string, error) { return moveToTrash(p) })
		if err == errBlocked {
			t.Fatalf("MoveToTrash(%s) did not return within 30 s", p)
		}
		t.Logf("MoveToTrash(%s) = %q, %v", filepath.Base(p), res, err)
		if err != nil {
			t.Errorf("MoveToTrash(%s): %v", p, err)
			continue
		}
		if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s is still there after MoveToTrash (%v)", p, err)
		}
		if filepath.Base(filepath.Dir(res)) != ".Trash" {
			t.Errorf("MoveToTrash(%s) moved it to %q, want a file in ~/.Trash", p, res)
		}
		fi, err := os.Lstat(res)
		switch {
		case errors.Is(err, fs.ErrPermission):
			t.Logf("cannot look into the Trash (%v): the test binary lacks Full Disk Access", err)
		case err != nil:
			t.Errorf("the trashed item %q: %v", res, err)
		case p == link && fi.Mode()&fs.ModeSymlink == 0:
			t.Errorf("the trashed link %q is a %v, want the symbolic link itself", res, fi.Mode())
		}
		if p == link {
			if _, err := os.Stat(target); err != nil {
				t.Errorf("trashing the link moved its target: %v", err)
			}
		}
		if err := os.RemoveAll(res); err != nil {
			t.Logf("could not delete %q from the Trash: %v", res, err)
		}
	}

	if err := (desktop{}).MoveToTrash(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("MoveToTrash of a missing file = %v, want fs.ErrNotExist", err)
	}
}
