//go:build windows

package win32

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Live tests of the Win32 gaps of phase 16 (ADR-234): the menu bar's
// keyboard and dark drawing, the tray icons' DPI, and the global shortcuts
// bound again when the keyboard layout changes. They run on an attended
// Windows desktop with TELA_LIVE=1 (the menu tests type with SendInput).

var procGetGUIThreadInfo = user32.NewProc("GetGUIThreadInfo")

// guiThreadInfo is GUITHREADINFO.
type guiThreadInfo struct {
	size, flags                                        uint32
	active, focus, capture, menuOwner, moveSize, caret uintptr
	caretRect                                          rect
}

const (
	guiInMenuMode = 0x00000004
	scF10         = 0x44
)

// inMenuMode reports whether the thread is in a menu loop.
func inMenuMode(tid uint32) bool {
	info := guiThreadInfo{size: uint32(unsafe.Sizeof(guiThreadInfo{}))}
	r, _, _ := procGetGUIThreadInfo.Call(uintptr(tid), uintptr(unsafe.Pointer(&info)))
	return r != 0 && info.flags&guiInMenuMode != 0
}

// menuModeIs waits up to d for the thread to be in (or out of) menu mode,
// and for "out" for the whole time: the answer is only right if it holds.
func menuModeIs(tid uint32, want bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if inMenuMode(tid) == want {
			if want {
				return true
			}
		} else if !want {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return inMenuMode(tid) == want
}

// leaveMenus taps Escape until the thread is out of its menus.
func leaveMenus(tid uint32) {
	for range 5 {
		if !inMenuMode(tid) {
			return
		}
		sendKeysRaw(ktap(scEsc)...)
		time.Sleep(200 * time.Millisecond)
	}
}

// TestLiveMenuBarKeyboard types Alt, F10 and Alt+key at a window with a
// menu bar: the lone Alt and F10 select the bar, Alt and the access key
// opens the menu, and the keys that are not for the bar (Alt+Shift, Alt and
// another key, Shift+F10) and a window without a bar leave it alone. Every
// key still reaches the app as a KeyEvent.
func TestLiveMenuBarKeyboard(t *testing.T) {
	requireLiveIntegration(t)
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{Title: "tela menu keys", Size: platform.Size{W: 400, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	menu := []platform.MenuItem{
		{Label: "File", Submenu: []platform.MenuItem{{ID: 1, Label: "Open"}, {ID: 2, Label: "Save"}}},
		{Label: "Edit", Submenu: []platform.MenuItem{{ID: 3, Label: "Copy"}}},
		{ID: 4, Label: "Help"},
	}
	var (
		mu       sync.Mutex
		problems []string
		keys     []string
		calls    = make(chan func(), 8)
		started  bool
		finished bool
	)
	fail := func(format string, args ...any) {
		mu.Lock()
		problems = append(problems, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	onUI := func(f func()) {
		done := make(chan struct{})
		calls <- func() { f(); close(done) }
		b.Wake()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			fail("a call to the UI thread did not run")
		}
	}
	check := func(what string, want bool) {
		if !menuModeIs(b.tid, want, 1500*time.Millisecond) {
			fail("%s: in menu mode = %v, want %v", what, !want, want)
		}
		leaveMenus(b.tid)
	}
	script := func() {
		defer func() { finished = true; b.Wake() }()
		time.Sleep(600 * time.Millisecond) // the Alt tap of forceForeground is over
		onUI(func() {
			if err := w.SetMenu(menu); err != nil {
				fail("SetMenu: %v", err)
			}
		})
		time.Sleep(300 * time.Millisecond)

		sendKeysRaw(ktap(scLAltK)...)
		check("a lone Alt", true)
		sendKeysRaw(ktap(scF10)...)
		check("F10", true)
		sendKeysRaw(chord([]uint16{scLAltK}, scF)...)
		if menuModeIs(b.tid, true, 1500*time.Millisecond) && waitWindow("#32768", "") == 0 {
			fail("Alt+F did not open the File menu")
		}
		leaveMenus(b.tid)
		sendKeysRaw(chord([]uint16{scLAltK}, 0x12)...) // Alt+E: Edit
		check("Alt+E (Edit)", true)

		sendKeysRaw(chord([]uint16{scLAltK}, scLShiftK)...)
		check("Alt+Shift", false)
		sendKeysRaw(chord([]uint16{scLAltK}, scX)...)
		check("Alt+X (not an access key)", false)
		sendKeysRaw(chord([]uint16{scLShiftK}, scF10)...)
		check("Shift+F10", false)

		onUI(func() { w.SetMenu(nil) })
		time.Sleep(300 * time.Millisecond)
		sendKeysRaw(ktap(scLAltK)...)
		check("a lone Alt without a bar", false)
		sendKeysRaw(ktap(scF10)...)
		check("F10 without a bar", false)
	}
	runLive(t, b, nil, func(win platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, win, solidBlack)
			if !started {
				forceForeground(w.hwnd)
				time.AfterFunc(3*time.Second, b.Wake)
			}
		case platform.FocusEvent:
			if e.Focused && !started {
				started = true
				go script()
			}
		case platform.KeyEvent:
			if e.Action == platform.KeyPress {
				mu.Lock()
				keys = append(keys, fmt.Sprintf("%v/%d", e.Code, e.Mods))
				mu.Unlock()
			}
		case platform.WakeEvent:
			for {
				select {
				case f := <-calls:
					f()
					continue
				default:
				}
				break
			}
			switch {
			case !started:
				t.Skip("the window did not get the keyboard focus")
			case finished:
				b.Quit()
			}
		}
	})
	mu.Lock()
	defer mu.Unlock()
	for _, p := range problems {
		t.Error(p)
	}
	// The app saw the keys the bar used: Alt, F10, and the Alt+X shortcut.
	got := strings.Join(keys, " ")
	for _, want := range []string{"AltLeft", "F10", fmt.Sprintf("KeyX/%d", platform.ModAlt)} {
		if !strings.Contains(got, want) {
			t.Errorf("the app did not receive %s; key presses: %s", want, got)
		}
	}
}

// TestLiveMenuBarDark draws the bar dark in dark mode and light again after
// it, reading the pixels of the bar and of the line under it from the
// screen. The PNG of the dark bar is written to the temp directory for a
// look.
func TestLiveMenuBarDark(t *testing.T) {
	requireLiveShell(t)
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{Title: "tela dark bar", Size: platform.Size{W: 420, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	menu := []platform.MenuItem{
		{Label: "File", Submenu: []platform.MenuItem{{ID: 1, Label: "Open"}}},
		{Label: "Edit", Submenu: []platform.MenuItem{{ID: 2, Label: "Copy"}}},
		{ID: 3, Label: "Disabled", Disabled: true},
	}
	// sample returns the colors of a point in the empty part of the bar and
	// of the seam under it.
	sample := func() (bar, seam [3]uint8, shot *image.RGBA) {
		win := getWindowRect(w.hwnd)
		r, ok := w.barRect()
		if !ok {
			t.Fatalf("the window has no menu bar")
		}
		origin := point{}
		procClientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&origin)))
		box := image.Rect(int(win.left+r.left), int(win.top+r.top), int(win.left+r.right), int(origin.y)+2)
		shot = captureScreenRect(box)
		px := func(x, y int) [3]uint8 {
			o := shot.PixOffset(x, y)
			return [3]uint8{shot.Pix[o], shot.Pix[o+1], shot.Pix[o+2]}
		}
		x := box.Dx() - 30
		return px(x, 4), px(x, int(origin.y)-box.Min.Y-1), shot
	}
	stage := 0
	runLive(t, b, nil, func(win platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			presentSolid(t, win, solidBlack)
			if stage == 0 {
				stage = 1
				if err := w.SetMenu(menu); err != nil {
					t.Errorf("SetMenu: %v", err)
				}
				b.settings.DarkMode = true
				w.refreshBar()
				time.AfterFunc(900*time.Millisecond, b.Wake)
			}
		case platform.WakeEvent:
			switch stage {
			case 1:
				stage = 2
				bar, seam, shot := sample()
				t.Logf("dark: bar %v, seam %v", bar, seam)
				near := func(c [3]uint8) bool { return c[0] < 0x30 && c[1] < 0x30 && c[2] < 0x30 }
				if !near(bar) {
					t.Errorf("the bar is %v in dark mode, want about #202020", bar)
				}
				if !near(seam) {
					t.Errorf("the line under the bar is %v in dark mode, want about #202020", seam)
				}
				path := filepath.Join(os.TempDir(), "tela-menubar-dark.png")
				if f, err := os.Create(path); err == nil {
					png.Encode(f, shot)
					f.Close()
					t.Logf("the dark bar: %s", path)
				}
				b.settings.DarkMode = false
				w.refreshBar()
				time.AfterFunc(900*time.Millisecond, b.Wake)
			case 2:
				stage = 3
				bar, seam, _ := sample()
				t.Logf("light: bar %v, seam %v", bar, seam)
				if bar[0] < 0x80 {
					t.Errorf("the bar is %v after leaving dark mode, want a light color", bar)
				}
				b.Quit()
			}
		}
	})
}

// TestLiveHotkeyLayoutChange registers a shortcut by character and then
// changes the keyboard layout: the shortcut is bound again under the key
// that types the character, with a new status event. The first part fakes
// a registration made under another layout and posts the
// WM_INPUTLANGCHANGE; the second activates a real layout (German, where the
// semicolon is a different key), when it is installed.
func TestLiveHotkeyLayoutChange(t *testing.T) {
	requireLiveServices(t)
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{Title: "tela hotkey layout", Size: platform.Size{W: 200, H: 100}})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	d := b.Desktop()
	// Ctrl+Alt+Shift+; : a combination no program takes.
	acc := platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt | platform.ModShift, Key: ";"}
	var statuses []platform.ShortcutStatusEvent
	stage := 0
	wantVK, _ := scanKey(';')
	runLive(t, b, nil, func(win platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, win, solidBlack)
			if stage == 0 {
				stage = 1
				if err := d.RegisterShortcut(platform.Shortcut{ID: "live-layout", Accelerator: acc, Description: "Live test"}); err != nil {
					t.Errorf("RegisterShortcut: %v", err)
					b.Quit()
				}
			}
		case platform.ShortcutStatusEvent:
			statuses = append(statuses, e)
			reg := b.app.shortcuts["live-layout"]
			switch stage {
			case 1:
				if e.Status != platform.ShortcutBound || reg == nil || reg.vk != wantVK {
					t.Errorf("first status = %+v, registration %+v; want Bound under vk %#x", e, reg, wantVK)
					b.Quit()
					return
				}
				// Pretend the registration was made under another layout:
				// the key it holds is not the one the layout types ';' with.
				stage = 2
				reg.vk = 0x41
				postMessage(w.hwnd, wmInputLangChange, 0, currentLayout())
			case 2:
				if e.Status != platform.ShortcutBound || reg.vk != wantVK || !reg.registered {
					t.Errorf("status after the layout change = %+v, registration %+v; want Bound under vk %#x", e, reg, wantVK)
				}
				stage = 3
				b.Quit()
			}
		}
	})
	if stage != 3 {
		t.Errorf("stopped at stage %d; statuses: %+v", stage, statuses)
	}

	// A real layout change.
	hkl, restore := layoutSession(t, "00000407") // German
	defer restore()
	b2 := liveBackend(t)
	pw2, err := b2.NewWindow(platform.WindowOptions{Title: "tela hotkey layout 2", Size: platform.Size{W: 200, H: 100}})
	if err != nil {
		t.Fatal(err)
	}
	w2 := pw2.(*Window)
	var before, after uint32
	stage = 0
	runLive(t, b2, nil, func(win platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, win, solidBlack)
			if stage == 0 {
				stage = 1
				forceForeground(w2.hwnd)
				time.AfterFunc(3*time.Second, b2.Wake)
			}
		case platform.FocusEvent:
			if e.Focused && stage == 1 {
				stage = 2
				b2.Desktop().RegisterShortcut(platform.Shortcut{ID: "live-layout", Accelerator: acc, Description: "Live test"})
			}
		case platform.WakeEvent:
			switch {
			case stage < 2:
				t.Skip("the window did not get the keyboard focus")
			case stage == 3:
				b2.Quit() // no status: the key is the same in this layout
			}
		case platform.ShortcutStatusEvent:
			reg := b2.app.shortcuts["live-layout"]
			switch stage {
			case 2:
				before = reg.vk
				stage = 3
				procActivateKeyboardLayout.Call(hkl, 0)
				time.AfterFunc(2*time.Second, b2.Wake)
			case 3:
				after = reg.vk
				stage = 4
				b2.Quit()
			}
		}
	})
	switch {
	case stage < 4 && before != 0:
		t.Logf("no status after activating the German layout: the semicolon is the same key there (vk %#x)", before)
	case stage == 4 && before == after:
		t.Errorf("the shortcut stayed on vk %#x after the layout change", before)
	default:
		t.Logf("the shortcut moved from vk %#x to %#x with the layout", before, after)
	}
}

// TestLiveTrayDPI changes the DPI the tray window is told about and checks
// that the icons are drawn again at the right side, for a tray icon and
// for the icon of a notification, on WM_DPICHANGED, WM_DISPLAYCHANGE and
// WM_SETTINGCHANGE.
func TestLiveTrayDPI(t *testing.T) {
	requireShell(t)
	b := liveBackend(t)
	d := b.Desktop()
	started := false
	runLive(t, b, openWindow(t, b), func(win platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok || started {
			return
		}
		started = true
		defer b.Quit()
		presentSolid(t, win, solidBlack)
		// A balloon notification has a notification-area icon to check;
		// toasts (ADR-235) have none.
		b.desk.toast.off = true
		tray, err := d.NewTray(platform.TrayOptions{Icon: testIcon(), Tooltip: "Tela DPI"})
		if err != nil {
			t.Errorf("NewTray: %v", err)
			return
		}
		icons := []*shellIcon{b.findIcon(func(ic *shellIcon) bool { return ic.tray == tray })}
		if id, err := d.Notify(platform.Notification{Title: "Tela", Body: "DPI", Icon: testIcon()}); err == nil {
			defer d.CloseNotification(id)
			icons = append(icons, b.findIcon(func(ic *shellIcon) bool { return ic.note == id }))
		}
		for i, ic := range icons {
			if ic == nil {
				t.Fatalf("icon %d not found", i)
			}
		}
		base := smallIconSide()
		send := func(msg uint32, wparam uintptr) {
			procSendMessageW.Call(b.desk.hwnd, uintptr(msg), wparam, 0)
		}
		check := func(what string, side int) {
			for i, ic := range icons {
				if ic.side != side {
					t.Errorf("%s: icon %d has side %d, want %d", what, i, ic.side, side)
				}
			}
		}
		check("at creation", base)
		old := icons[0].hicon

		const high = 192
		send(wmDPIChanged, high|high<<16)
		check("after WM_DPICHANGED to 192 dpi", iconSideAt(smallIconMetric, 16, high))
		if icons[0].hicon == old && iconSideAt(smallIconMetric, 16, high) != base {
			t.Errorf("the tray icon was not drawn again")
		}
		send(wmDPIChanged, baseDPI|baseDPI<<16)
		check("after WM_DPICHANGED back to 96 dpi", iconSideAt(smallIconMetric, 16, baseDPI))

		// A settings or display change asks the primary monitor.
		for _, msg := range []uint32{wmSettingChange, wmDisplayChange} {
			for _, ic := range icons {
				ic.side = 1
			}
			send(msg, 0)
			check(fmt.Sprintf("after message %#x", msg), base)
		}
	})
}
