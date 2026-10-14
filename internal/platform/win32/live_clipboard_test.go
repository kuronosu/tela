//go:build windows

package win32

import (
	"image/color"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

var (
	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")

	solidBlack = color.RGBA{0, 0, 0, 0xff}
)

func sendMessage(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

// clipboardSession runs fn on the loop of a backend with one window (a
// clipboard needs an owner window) and returns when fn calls done.
func clipboardSession(t *testing.T, fn func(b *Backend, quit func())) {
	t.Helper()
	b := liveBackend(t)
	started := false
	runLive(t, b, func() {
		if _, err := b.NewWindow(platform.WindowOptions{}); err != nil {
			t.Fatal(err)
		}
	}, func(w platform.Window, e platform.Event) {
		if _, ok := e.(platform.FrameEvent); !ok || started {
			return
		}
		started = true
		presentSolid(t, w, solidBlack)
		fn(b, b.Quit)
	})
}

func TestClipboardRoundTrip(t *testing.T) {
	const text = "tela clipboard ✓\nsecond line 😀\n\nlast"
	var inCall bool
	var got string
	clipboardSession(t, func(b *Backend, quit func()) {
		cb := b.Clipboard()
		if err := cb.WriteText(text); err != nil {
			t.Errorf("WriteText: %v", err)
			quit()
			return
		}
		// The system stores CR LF.
		if raw, err := b.readClipboardRaw(); err != nil || raw != "tela clipboard ✓\r\nsecond line 😀\r\n\r\nlast" {
			t.Errorf("stored text = %q, %v", raw, err)
		}
		inCall = true
		cb.ReadText(func(s string, err error) {
			if inCall {
				t.Error("ReadText called back synchronously")
			}
			if err != nil {
				t.Errorf("ReadText: %v", err)
			}
			got = s
			quit()
		})
		inCall = false
	})
	if got != text {
		t.Errorf("ReadText = %q, want %q", got, text)
	}
}

func TestClipboardReadsOtherApplicationsText(t *testing.T) {
	var got string
	clipboardSession(t, func(b *Backend, quit func()) {
		// What Notepad or PowerShell's Set-Clipboard leave: CR LF.
		if err := b.writeClipboard("one\r\ntwo\rthree\n"); err != nil {
			t.Fatal(err)
		}
		b.Clipboard().ReadText(func(s string, err error) {
			if err != nil {
				t.Errorf("ReadText: %v", err)
			}
			got = s
			quit()
		})
	})
	if got != "one\ntwo\nthree\n" {
		t.Errorf("ReadText = %q, want LF line endings", got)
	}
}

func TestClipboardEmptyAndNoText(t *testing.T) {
	var got = "unset"
	clipboardSession(t, func(b *Backend, quit func()) {
		if err := b.openClipboard(); err != nil {
			t.Fatal(err)
		}
		procEmptyClipboard.Call()
		procCloseClipboard.Call()
		b.Clipboard().ReadText(func(s string, err error) {
			if err != nil {
				t.Errorf("ReadText: %v", err)
			}
			got = s
			quit()
		})
	})
	if got != "" {
		t.Errorf("ReadText of an empty clipboard = %q", got)
	}
}

func TestClipboardRetriesWhileAnotherThreadHoldsIt(t *testing.T) {
	var wg sync.WaitGroup
	var held, release = make(chan struct{}), make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		for {
			if r, _, _ := procOpenClipboard.Call(0); r != 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		close(held)
		<-release
		procCloseClipboard.Call()
	}()
	defer wg.Wait()
	<-held
	time.AfterFunc(40*time.Millisecond, func() { close(release) })
	var err error
	clipboardSession(t, func(b *Backend, quit func()) {
		err = b.Clipboard().WriteText("after the other application")
		quit()
	})
	if err != nil {
		t.Errorf("WriteText while the clipboard was open elsewhere: %v", err)
	}
}

func TestClipboardLargeAndUnicode(t *testing.T) {
	text := ""
	for i := range 20000 {
		text += string(rune('a'+i%26)) + "é😀\n"
	}
	var got string
	clipboardSession(t, func(b *Backend, quit func()) {
		if err := b.Clipboard().WriteText(text); err != nil {
			t.Fatal(err)
		}
		b.Clipboard().ReadText(func(s string, err error) {
			if err != nil {
				t.Errorf("ReadText: %v", err)
			}
			got = s
			quit()
		})
	})
	if got != text {
		t.Errorf("large text round trip differs (%d bytes, want %d)", len(got), len(text))
	}
}

func TestSettings(t *testing.T) {
	b := liveBackend(t)
	s := b.Settings()
	if s.DoubleClickInterval < 100*time.Millisecond || s.DoubleClickInterval > 5*time.Second {
		t.Errorf("DoubleClickInterval = %v", s.DoubleClickInterval)
	}
	light, known := readAppsUseLightTheme()
	if s.DarkMode != (known && light == 0) {
		t.Errorf("DarkMode = %v with AppsUseLightTheme = %d (known %v)", s.DarkMode, light, known)
	}
	t.Logf("settings: %+v (AppsUseLightTheme = %d, known %v, client area animation %v)", s, light, known, clientAreaAnimation())
	if s != readSettings() {
		t.Errorf("Settings() = %+v, read again %+v", s, readSettings())
	}
}

// TestSettingsChange: WM_SETTINGCHANGE re-reads the settings and reports a
// difference once, with a SettingsEvent that has no window.
func TestSettingsChange(t *testing.T) {
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w2, err := b.NewWindow(platform.WindowOptions{}) // the broadcast reaches every window
	if err != nil {
		t.Fatal(err)
	}
	real := b.Settings()
	// Pretend the backend read different values earlier.
	b.settings.DarkMode = !real.DarkMode
	b.settings.ReducedMotion = !real.ReducedMotion
	var events []platform.SettingsEvent
	step := 0
	runLive(t, b, nil, func(w platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.SettingsEvent:
			if w != nil {
				t.Errorf("SettingsEvent has window %v", w)
			}
			events = append(events, e)
		case platform.FrameEvent:
			if step == 0 {
				step = 1
				presentSolid(t, w, solidBlack)
				// "ImmersiveColorSet" is what Windows broadcasts when the
				// color mode changes; the content does not matter here.
				sendMessage(pw.(*Window).hwnd, wmSettingChange, 0, 0)
				sendMessage(w2.(*Window).hwnd, wmSettingChange, 0, 0)
				time.AfterFunc(100*time.Millisecond, b.Wake)
			}
		case platform.WakeEvent:
			b.Quit()
		}
	})
	if len(events) != 1 || events[0].Settings != real {
		t.Errorf("SettingsEvents = %+v, want one with %+v", events, real)
	}
	if b.Settings() != real {
		t.Errorf("Settings() after the change = %+v, want %+v", b.Settings(), real)
	}
}

// TestTitleBarFollowsDarkMode reads back DWMWA_USE_IMMERSIVE_DARK_MODE.
func TestTitleBarFollowsDarkMode(t *testing.T) {
	if !available(procDwmGetWindowAttribute) || !available(procDwmSetWindowAttribute) {
		t.Skip("no DWM window attributes")
	}
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	read := func() (int32, uintptr) {
		var v int32
		hr, _, _ := procDwmGetWindowAttribute.Call(w.hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
		return v, hr
	}
	if _, hr := read(); hr != 0 {
		t.Skipf("DwmGetWindowAttribute(20) = %#x: not supported here", hr)
	}
	for _, dark := range []bool{true, false, b.Settings().DarkMode} {
		w.applyTheme(dark)
		want := int32(0)
		if dark {
			want = 1
		}
		if v, _ := read(); v != want {
			t.Errorf("after applyTheme(%v) the attribute is %d", dark, v)
		}
	}
}

// TestClipboardInterop is driven by a script that runs other programs
// (PowerShell's Set-Clipboard and Get-Clipboard): TELA_CLIPBOARD_WRITE=text
// writes text (Go escapes such as \n and ✓ are decoded, so a batch file
// can stay ASCII) and TELA_CLIPBOARD_READ=file stores what ReadText returns
// in file.
func TestClipboardInterop(t *testing.T) {
	write, read := os.Getenv("TELA_CLIPBOARD_WRITE"), os.Getenv("TELA_CLIPBOARD_READ")
	if write == "" && read == "" {
		t.Skip("TELA_CLIPBOARD_WRITE and TELA_CLIPBOARD_READ are not set")
	}
	clipboardSession(t, func(b *Backend, quit func()) {
		if write != "" {
			text, err := strconv.Unquote(`"` + write + `"`)
			if err != nil {
				t.Fatalf("TELA_CLIPBOARD_WRITE=%q: %v", write, err)
			}
			if err := b.Clipboard().WriteText(text); err != nil {
				t.Errorf("WriteText: %v", err)
			}
		}
		if read == "" {
			quit()
			return
		}
		b.Clipboard().ReadText(func(s string, err error) {
			if err != nil {
				t.Errorf("ReadText: %v", err)
			}
			if err := os.WriteFile(read, []byte(s), 0o644); err != nil {
				t.Errorf("writing %s: %v", read, err)
			}
			t.Logf("ReadText = %q", s)
			quit()
		})
	})
}
