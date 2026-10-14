//go:build windows

package win32

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	utf16pkg "unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform"
)

// These tests type on the real keyboard path: SendInput puts scan codes into
// the system input queue, Windows maps them with the active layout, makes
// WM_KEYDOWN and (through TranslateMessage) the character messages, and the
// window turns them into KeyEvents. They need the keyboard focus, so they
// skip where the window cannot get it (a session without a desktop), and they
// load other keyboard layouts for the thread (they are restored afterwards).
//
// TELA_KEYLOG=1 logs what every case produced instead of failing on a
// difference, to record new expectations.

var (
	procSendInput              = user32.NewProc("SendInput")
	procLoadKeyboardLayoutW    = user32.NewProc("LoadKeyboardLayoutW")
	procUnloadKeyboardLayout   = user32.NewProc("UnloadKeyboardLayout")
	procActivateKeyboardLayout = user32.NewProc("ActivateKeyboardLayout")
	procGetKeyboardLayoutList  = user32.NewProc("GetKeyboardLayoutList")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procSetFocus               = user32.NewProc("SetFocus")
	procBringWindowToTop       = user32.NewProc("BringWindowToTop")
	procAttachThreadInput      = user32.NewProc("AttachThreadInput")
	procGetWindowThreadProcID  = user32.NewProc("GetWindowThreadProcessId")
	procGetCurrentThreadID     = kernel32.NewProc("GetCurrentThreadId")
	procWineVersion            = windows.NewLazySystemDLL("ntdll.dll").NewProc("wine_get_version")
)

const (
	inputKeyboard     = 1
	keyeventfExtended = 0x0001
	keyeventfKeyUp    = 0x0002
	keyeventfScancode = 0x0008
	klfActivate       = 0x0001
	klfNoTellShell    = 0x0080
)

// keybdInput and input are KEYBDINPUT and INPUT. INPUT is the size of its
// largest union member, MOUSEINPUT, hence the padding.
type keybdInput struct {
	vk, scan uint16
	flags    uint32
	time     uint32
	extra    uintptr
}

type input struct {
	typ uint32
	ki  keybdInput
	_   [8]byte
}

// sk is a key transition by scan code (0xE000 set for extended keys).
type sk struct {
	sc uint16
	up bool
}

func kdn(sc uint16) []sk  { return []sk{{sc, false}} }
func kup(sc uint16) []sk  { return []sk{{sc, true}} }
func ktap(sc uint16) []sk { return []sk{{sc, false}, {sc, true}} }

// seq concatenates key sequences.
func seq(parts ...[]sk) []sk {
	var out []sk
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// chord presses mods in order, taps the keys, and releases the mods in
// reverse order.
func chord(mods []uint16, keys ...uint16) []sk {
	var out []sk
	for _, m := range mods {
		out = append(out, kdn(m)...)
	}
	for _, k := range keys {
		out = append(out, ktap(k)...)
	}
	for i := len(mods) - 1; i >= 0; i-- {
		out = append(out, kup(mods[i])...)
	}
	return out
}

func sendKeys(t *testing.T, keys []sk) {
	t.Helper()
	ins := make([]input, len(keys))
	for i, k := range keys {
		flags := uint32(keyeventfScancode)
		if k.sc&0xFF00 != 0 {
			flags |= keyeventfExtended
		}
		if k.up {
			flags |= keyeventfKeyUp
		}
		ins[i] = input{typ: inputKeyboard, ki: keybdInput{scan: k.sc & 0xFF, flags: flags}}
	}
	n, _, e := procSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
	if int(n) != len(ins) {
		t.Fatalf("SendInput injected %d of %d events: %v", n, len(ins), e)
	}
}

// Scan codes of the keys the cases use (the US names; the layouts decide
// what they type).
const (
	scEsc, scTab, scEnterK, scBackspace, scSpaceK                 = 0x01, 0x0F, 0x1C, 0x0E, 0x39
	scLShiftK, scRShiftK, scLCtrlK, scLAltK                       = 0x2A, 0x36, 0x1D, 0x38
	scRAltK, scRCtrlK, scLWinK                                    = 0xE038, 0xE01D, 0xE05B
	scF1, scF4, scF24                                             = 0x3B, 0x3E, 0x76
	scCapsLock, scNumLockK                                        = 0x3A, 0xE045
	scArrowLeft, scHome, scDelete                                 = 0xE04B, 0xE047, 0xE053
	scNumpad1, scNumpadDot, scNumpadDiv, scNumpadEnter            = 0x4F, 0x53, 0xE035, 0xE01C
	scNumpadMul, scNumpadAdd                                      = 0x37, 0x4E
	scNumpad0, scNumpad2, scNumpad3                               = 0x52, 0x50, 0x51
	scBackquote, scDigit1, scDigit2, scDigit4, scDigit7, scDigit0 = 0x29, 0x02, 0x03, 0x05, 0x08, 0x0B
	scMinus, scEqual                                              = 0x0C, 0x0D
	scQ, scW, scE, scR, scT, scY, scU, scI, scO, scP              = 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19
	scBracketL, scBracketR                                        = 0x1A, 0x1B
	scA, scS, scD, scF, scG, scH, scJ, scK, scL                   = 0x1E, 0x1F, 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26
	scSemicolon, scQuoteK, scBackslash                            = 0x27, 0x28, 0x2B
	scZ, scX, scC, scV, scB, scN, scM                             = 0x2C, 0x2D, 0x2E, 0x2F, 0x30, 0x31, 0x32
	scComma, scPeriod, scSlash, scIntlBackslash                   = 0x33, 0x34, 0x35, 0x56
)

// fmtKey renders a KeyEvent the way the cases write expectations:
// "dKeyA a|a S" is a press of KeyA with Key "a", Text "a" and Shift; "u" a
// release, "r" a repeat, "x" a synthetic release.
func fmtKey(e platform.KeyEvent) string {
	dir := "d"
	switch {
	case e.Action == platform.KeyRelease && e.Synthetic:
		dir = "x"
	case e.Action == platform.KeyRelease:
		dir = "u"
	case e.Repeat:
		dir = "r"
	}
	var mods strings.Builder
	for _, m := range []struct {
		m platform.Modifiers
		c string
	}{{platform.ModShift, "S"}, {platform.ModCtrl, "C"}, {platform.ModAlt, "A"}, {platform.ModSuper, "M"}} {
		if e.Mods&m.m != 0 {
			mods.WriteString(m.c)
		}
	}
	if mods.Len() == 0 {
		mods.WriteString("-")
	}
	code := e.Code.String()
	if n, ok := e.Code.Native(); ok {
		code = fmt.Sprintf("?%#x", n)
	}
	loc := ""
	if e.Location != platform.KeyLocationStandard {
		loc = "/" + e.Location.String()[:1]
	}
	return fmt.Sprintf("%s %s%s %q|%q %s", dir, code, loc, string(e.Key), e.Text, mods.String())
}

// kbdCase is a sequence of key transitions and the events it must produce
// (fmtKey form, without the sentinel).
type kbdCase struct {
	name    string
	warmup  bool
	slow    bool   // inject the keys 40 ms apart
	numlock bool   // needs Num Lock on (a remote desktop client may not let it change)
	notWine string // why Wine gives another result
	keys    []sk
	want    []string
}

// loadedLayouts returns the layouts loaded in the session.
func loadedLayouts() map[uintptr]bool {
	var buf [64]uintptr
	n, _, _ := procGetKeyboardLayoutList.Call(uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	m := map[uintptr]bool{}
	for _, h := range buf[:n] {
		m[h] = true
	}
	return m
}

// origLayout is the layout the thread had when layoutSession started.
var origLayout uintptr

// layoutSession loads a keyboard layout and keeps the test on one OS thread
// until the returned func restores the previous layout. The layout becomes
// the window's once it has the focus (runCases activates it then: Windows
// takes the layout of a new window's input queue from the process, whatever
// the thread activated before).
func layoutSession(t *testing.T, klid string) (hkl uintptr, restore func()) {
	t.Helper()
	if procWineVersion.Find() == nil {
		if klid == "00000409" {
			return 0, func() {} // Wine's layout is US already
		}
		t.Skip("Wine does not implement keyboard layouts; this runs on Windows")
	}
	// The layout belongs to the thread: stay on this one until restored.
	runtime.LockOSThread()
	orig, _, _ := procGetKeyboardLayout.Call(0)
	origLayout = orig
	before := loadedLayouts()
	h, _, e := procLoadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(utf16(klid))), klfNoTellShell)
	if h == 0 {
		runtime.UnlockOSThread()
		t.Skipf("keyboard layout %s is not available: %v", klid, e)
	}
	restore = func() {
		procActivateKeyboardLayout.Call(orig, 0)
		resetForegroundLayout(orig)
		if !before[h] {
			procUnloadKeyboardLayout.Call(h)
		}
		runtime.UnlockOSThread()
	}
	t.Logf("layout %s is HKL %#x, AltGr %v", klid, h, layoutHasAltGr(h))
	return h, restore
}

// forceForeground gives hwnd the keyboard focus, which Windows only grants
// to the process that last received input: a tap of the Alt key (harmless to
// whatever has the focus now) makes this process that one. The input queue
// is not shared with the old foreground thread (AttachThreadInput would
// share it, and a layout activated later would stay on that thread).
func forceForeground(hwnd uintptr) {
	sendKeysRaw(sk{scLAltK, false}, sk{scLAltK, true})
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
	procSetFocus.Call(hwnd)
}

// resetForegroundLayout gives the layout orig back to the thread of the
// foreground window if a test left it with another one (a layout activated
// while input queues were attached by an earlier version of these tests, or
// by Windows itself).
func resetForegroundLayout(orig uintptr) {
	fg, _, _ := procGetForegroundWindow.Call()
	if fg == 0 {
		return
	}
	th, _, _ := procGetWindowThreadProcID.Call(fg, 0)
	cur, _, _ := procGetCurrentThreadID.Call()
	if l, _, _ := procGetKeyboardLayout.Call(th); th == 0 || th == cur || l == orig {
		return
	}
	procAttachThreadInput.Call(cur, th, 1)
	procActivateKeyboardLayout.Call(orig, 0)
	procAttachThreadInput.Call(cur, th, 0)
}

// toggleKey reads a toggle key (Caps Lock, Num Lock) and returns whether it
// is on.
func toggleOn(vk int) bool {
	r, _, _ := procGetKeyState.Call(uintptr(vk))
	return r&1 != 0
}

// runCases opens a window, gets the focus, activates the layout hkl (if not
// zero) and types every case.
//
// setup runs once the window has the focus (to put toggle keys in a known
// state), and its result runs after the cases.
func runCases(t *testing.T, hkl uintptr, cases []kbdCase, setup func(t *testing.T) (cleanup func())) {
	t.Helper()
	// The first case only absorbs the events of setup (toggle keys).
	cases = append([]kbdCase{{name: "(setup)", warmup: true}}, cases...)
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 320, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	record := os.Getenv("TELA_KEYLOG") != ""
	var trace []string
	if os.Getenv("TELA_KEYLOG") == "2" {
		keyTrace = func(format string, args ...any) { trace = append(trace, fmt.Sprintf(format, args...)) }
		defer func() { keyTrace = nil }()
	}

	// The run is a sequence of steps, each with a deadline that a Wake
	// (from a timer) checks: waiting for the frame and the focus, letting
	// the layout switch, and one step per case.
	const (
		stepFocus  = iota
		stepLayout // waiting for the layout to switch
		stepProbe  // a probe key checks that it did
		stepCases
		stepRestore
	)
	var (
		step           = stepFocus
		frame, focused bool
		finished       bool
		deadline       time.Time
		cur            int
		got            []string
		cleanup        func()
	)
	arm := func(d time.Duration) {
		deadline = time.Now().Add(d)
		time.AfterFunc(d+10*time.Millisecond, b.Wake)
	}
	finish := func() {
		if hkl != 0 && origLayout != 0 && step != stepRestore {
			// The layout is global to the session: give the old one back
			// while the window still has the focus, or it stays.
			step = stepRestore
			procActivateKeyboardLayout.Call(origLayout, 0)
			arm(300 * time.Millisecond)
			return
		}
		finished = true
		b.Quit()
	}
	var next func()
	next = func() {
		if cur == len(cases) {
			finish()
			return
		}
		got, trace = nil, nil
		if why := cases[cur].notWine; why != "" && procWineVersion.Find() == nil {
			t.Logf("%s: skipped under Wine, which %s", cases[cur].name, why)
			cur++
			next()
			return
		}
		if cases[cur].numlock && !toggleOn(vkNumLock) {
			t.Logf("%s: skipped, Num Lock is off", cases[cur].name)
			cur++
			next()
			return
		}
		keys := seq(cases[cur].keys, ktap(scF24))
		if cases[cur].slow {
			// Windows merges the key-downs that wait in the queue into one
			// message with a repeat count; typematic repeats come 30 ms
			// apart. Inject from another thread, pausing between events.
			go func() {
				for _, k := range keys {
					sendKeysRaw(k)
					time.Sleep(40 * time.Millisecond)
				}
			}()
		} else {
			sendKeys(t, keys)
		}
		arm(3 * time.Second)
	}
	check := func() {
		c := cases[cur]
		line := strings.Join(got, " ; ")
		switch {
		case c.warmup:
		case record:
			t.Logf("%-28s %s [numlock %v caps %v]", c.name, line, toggleOn(vkNumLock), toggleOn(vkCapital))
			for _, l := range trace {
				t.Logf("        %s", l)
			}
		case strings.Join(c.want, " ; ") != line:
			t.Errorf("%s:\n got  %s\n want %s", c.name, line, strings.Join(c.want, " ; "))
		}
		cur++
		next()
	}
	attempts := 0
	var probeKey platform.Key
	var probeCtrl bool
	var probeSc uint16
	var probeWant platform.Key
	if hkl != 0 {
		probeSc, probeWant = layoutProbe(hkl)
	}
	activate := func() {
		step = stepLayout
		if attempts > 0 {
			// Windows sometimes keeps the old layout for the window's input;
			// going through another layout makes it notice.
			us, _, _ := procLoadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(utf16("00000409"))), klfNoTellShell)
			procActivateKeyboardLayout.Call(us, 0)
			time.Sleep(150 * time.Millisecond)
		}
		procActivateKeyboardLayout.Call(hkl, 0)
		arm(300 * time.Millisecond)
	}
	start := func() {
		step = stepCases
		if setup != nil {
			cleanup = setup(t)
		}
		next()
	}
	runLive(t, b, nil, func(win platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			if !frame {
				frame = true
				presentSolid(t, win, solidBlack)
				forceForeground(w.hwnd)
				arm(2 * time.Second)
			}
		case platform.FocusEvent:
			focused = e.Focused
		case platform.CloseRequestedEvent:
			got = append(got, "close")
		case platform.WakeEvent:
			if finished || time.Now().Before(deadline) {
				return
			}
			switch step {
			case stepFocus:
				t.Skip("the window did not get the keyboard focus")
			case stepLayout:
				// Windows switches the input layout of a window
				// asynchronously, and not always: type a key that the
				// layout types differently from US and see.
				if probeSc == 0 {
					start()
					return
				}
				step = stepProbe
				probeKey = ""
				// The space ends a dead key; the right Alt shows whether the system
				// believes in AltGr (it sends a fake Ctrl with it) as the layout does.
				probeCtrl = false
				sendKeys(t, seq(ktap(probeSc), ktap(scSpaceK), ktap(scRAltK), ktap(scF24)))
				arm(2 * time.Second)
			case stepProbe:
				t.Fatalf("the layout probe key did not arrive")
			case stepCases:
				t.Errorf("%s: timed out; got %s", cases[cur].name, strings.Join(got, " ; "))
				finish()
			case stepRestore:
				finish()
			}
		case platform.KeyEvent:
			if e.Code == platform.CodeF24 {
				if e.Action == platform.KeyRelease && step == stepCases {
					check()
				} else if e.Action == platform.KeyRelease && step == stepProbe {
					if probeKey == probeWant && !probeCtrl {
						start()
					} else if attempts++; attempts < 5 {
						t.Logf("the input layout is not the layout yet (probe typed %q, want %q, Ctrl %v); activating again", probeKey, probeWant, probeCtrl)
						activate()
					} else {
						t.Fatalf("the layout %#x did not become the window's input layout (probe typed %q, want %q, Ctrl %v)", hkl, probeKey, probeWant, probeCtrl)
					}
				}
				return
			}
			if step != stepCases && e.Code == platform.CodeAltLeft {
				return // the tap of forceForeground
			}
			if step == stepProbe && e.Action == platform.KeyPress && probeKey == "" {
				probeKey = e.Key
			}
			if step == stepProbe && e.Code == platform.CodeControlLeft {
				probeCtrl = true // a fake Ctrl that was not recognized: the layout has no AltGr for the system
			}
			got = append(got, fmtKey(e))
		}
		if step == stepFocus && frame && focused {
			if hkl == 0 {
				start()
				return
			}
			// Now that the window has the focus the layout applies to its
			// input; give Windows a moment to switch.
			activate()
		}
	})
	if cleanup != nil {
		cleanup()
	}
}

// layoutProbe finds a key that the layout hkl types differently from another
// layout (US, or German for US itself) and the Key it must report, to check
// that hkl is really the layout Windows uses for the window's input: sc is 0
// when no key differs.
func layoutProbe(hkl uintptr) (sc uint16, want platform.Key) {
	var other uintptr
	before := loadedLayouts()
	for _, klid := range []string{"00000409", "00000407"} {
		h, _, _ := procLoadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(utf16(klid))), klfNoTellShell)
		if h != 0 && h != hkl {
			other = h
			if !before[h] {
				defer procUnloadKeyboardLayout.Call(h)
			}
			break
		}
	}
	if other == 0 {
		return 0, ""
	}
	ask := func(l uintptr, scan uint16) (platform.Key, bool) {
		vk := mapVirtualKey(uint32(scan), 1, l) // MAPVK_VSC_TO_VK
		if vk == 0 {
			return "", false
		}
		var state [256]byte
		text, kind := toUnicode(vk, uint32(scan), &state, l)
		switch {
		case kind == charDead:
			return platform.KeyDead, true
		case text == "":
			return "", false
		}
		return platform.Key(text), true
	}
	for _, scan := range []uint16{scY, scQ, scA, scSemicolon, scQuoteK, scBackquote, scBracketL, scBracketR, scMinus, scEqual, scBackslash, scSlash} {
		a, okA := ask(other, scan)
		b, okB := ask(hkl, scan)
		if okA && okB && a != b {
			return scan, b
		}
	}
	return 0, ""
}

// ensureToggle puts a toggle key (Caps Lock, Num Lock) in a state and
// returns the func that restores the previous one.
func ensureToggle(vk, sc int, on bool) (cleanup func()) {
	orig := toggleOn(vk)
	if orig != on {
		sendKeysRaw(sk{uint16(sc), false}, sk{uint16(sc), true})
		time.Sleep(50 * time.Millisecond)
	}
	return func() {
		if toggleOn(vk) != orig {
			sendKeysRaw(sk{uint16(sc), false}, sk{uint16(sc), true})
		}
	}
}

func sendKeysRaw(keys ...sk) {
	ins := make([]input, len(keys))
	for i, k := range keys {
		flags := uint32(keyeventfScancode)
		if k.sc&0xFF00 != 0 {
			flags |= keyeventfExtended
		}
		if k.up {
			flags |= keyeventfKeyUp
		}
		ins[i] = input{typ: inputKeyboard, ki: keybdInput{scan: k.sc & 0xFF, flags: flags}}
	}
	procSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
}

// standardToggles puts Caps Lock off and Num Lock on, the state the cases
// assume, and returns the func that restores the previous one.
func standardToggles(t *testing.T) func() {
	c1 := ensureToggle(vkCapital, scCapsLock, false)
	c2 := ensureToggle(vkNumLock, scNumLockK, true)
	return func() { c2(); c1() }
}

// TestKeyboardFocus: losing the focus releases the held key before the
// FocusEvent, a window without focus reports no keys (SetFocus(NULL) sends
// them to the active window as system keys), and a key that is already down
// when the focus returns presses nothing and its release is dropped.
func TestKeyboardFocus(t *testing.T) {
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{Size: platform.Size{W: 320, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	var log []string
	var frame, started, finished bool
	phase := 0
	runLive(t, b, nil, func(win platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			if !frame {
				frame = true
				presentSolid(t, win, solidBlack)
				forceForeground(w.hwnd)
				time.AfterFunc(3*time.Second, b.Wake)
			}
		case platform.WakeEvent:
			switch {
			case !started:
				t.Skip("the window did not get the keyboard focus")
			case phase == 3:
				// The keys sent without focus have been dispatched by now.
				phase = 4
				procSetFocus.Call(w.hwnd)
				// B is still down (the repeat bit is set); its release is
				// dropped; C is a normal key.
				sendKeys(t, seq(kdn(scB), kup(scB), ktap(scC), ktap(scF24)))
			case !finished:
				t.Errorf("timed out; got %s", strings.Join(log, " ; "))
				b.Quit()
			}
		case platform.FocusEvent:
			log = append(log, fmt.Sprintf("focus %v", e.Focused))
			switch {
			case e.Focused && !started:
				started, phase = true, 1
				sendKeys(t, kdn(scA))
			case !e.Focused && phase == 2:
				// The keys sent without focus find the window active and
				// arrive as system keys; give them time to be dispatched.
				phase = 3
				time.AfterFunc(300*time.Millisecond, b.Wake)
			}
		case platform.KeyEvent:
			if e.Code == platform.CodeF24 {
				if e.Action == platform.KeyRelease {
					finished = true
					b.Quit()
				}
				return
			}
			if e.Code == platform.CodeAltLeft {
				return // the tap of forceForeground
			}
			dir := map[bool]string{false: "d", true: "u"}[e.Action == platform.KeyRelease]
			if e.Synthetic {
				dir = "x"
			}
			log = append(log, fmt.Sprintf("%s %v mods=%v", dir, e.Code, e.Mods))
			if phase == 1 && e.Code == platform.CodeKeyA && e.Action == platform.KeyPress {
				phase = 2
				procSetFocus.Call(0)
				sendKeys(t, seq(kup(scA), kdn(scB)))
			}
		}
	})
	want := "focus true ; d KeyA mods=0 ; x KeyA mods=0 ; focus false ; focus true ; d KeyC mods=0 ; u KeyC mods=0"
	if got := strings.Join(log, " ; "); got != want {
		t.Errorf("events:\n got  %s\n want %s", got, want)
	}
}

func TestKeyboardUS(t *testing.T) {
	hkl, restore := layoutSession(t, "00000409")
	defer restore()
	runCases(t, hkl, usCases(), standardToggles)
}

func TestKeyboardUSInternational(t *testing.T) {
	hkl, restore := layoutSession(t, "00020409")
	defer restore()
	runCases(t, hkl, usIntlCases(), standardToggles)
}

func TestKeyboardGerman(t *testing.T) {
	hkl, restore := layoutSession(t, "00000407")
	defer restore()
	runCases(t, hkl, germanCases(), standardToggles)
}

func TestKeyboardFrench(t *testing.T) {
	hkl, restore := layoutSession(t, "0000040C")
	defer restore()
	runCases(t, hkl, frenchCases(), standardToggles)
}

func TestKeyboardSpanish(t *testing.T) {
	hkl, restore := layoutSession(t, "0000040A")
	defer restore()
	runCases(t, hkl, spanishCases(), standardToggles)
}

// TestToUnicodeDeadKeyState checks what the translation relies on: ToUnicodeEx
// with the keep-state flag never starts or finishes a dead-key sequence
// (so asking what a dead key types is harmless), but it does see a sequence
// that is pending. German: ^ is a dead key.
func TestToUnicodeDeadKeyState(t *testing.T) {
	if procWineVersion.Find() == nil {
		t.Skip("Wine does not implement keyboard layouts")
	}
	hkl, restore := layoutSession(t, "00000407")
	defer restore()
	const vkCircumflex, vkE = 0xDC, 'E' // VK_OEM_5 is the dead ^ of the German layout
	var state [256]byte
	call := func(flags, vk uintptr) string {
		var buf [8]uint16
		sc := mapVirtualKey(uint32(vk), 0, hkl)
		n, _, _ := procToUnicodeEx.Call(vk, uintptr(sc), uintptr(unsafe.Pointer(&state[0])), uintptr(unsafe.Pointer(&buf[0])), 8, flags, hkl)
		return fmt.Sprintf("%d %q", int32(n), string(utf16pkg.Decode(buf[:max(0, min(int(int32(n)), 8))])))
	}
	for _, step := range []struct {
		what      string
		flags, vk uintptr
		want      string
	}{
		{"a letter clears any pending sequence", 0, vkE, `1 "e"`},
		{"a dead key with flag 4 (clean state)", toUnicodeKeepState, vkCircumflex, `-1 ""`},
		{"and again", toUnicodeKeepState, vkCircumflex, `-1 ""`},
		{"flag 4 left no sequence pending", 0, vkE, `1 "e"`},
		{"a dead key starts a sequence", 0, vkCircumflex, `-1 ""`},
		{"flag 4 sees the pending sequence", toUnicodeKeepState, vkE, `1 "ê"`},
		{"and the dead key itself composes with it", toUnicodeKeepState, vkCircumflex, `2 "^^"`},
		{"flag 4 did not finish the sequence", 0, vkE, `1 "ê"`},
		{"the call without the flag did", 0, vkE, `1 "e"`},
	} {
		if got := call(step.flags, step.vk); got != step.want {
			t.Errorf("%s: %s, want %s", step.what, got, step.want)
		}
	}
}

// TestKeyboardLayoutList logs the layouts of the session, to know what a
// failing run started from.
func TestKeyboardLayoutList(t *testing.T) {
	var buf [64]uintptr
	n, _, _ := procGetKeyboardLayoutList.Call(uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	for _, h := range buf[:n] {
		t.Logf("installed layout %#x, AltGr %v", h, layoutHasAltGr(h))
	}
	t.Logf("current layout of the thread %#x", currentLayout())
	fg, _, _ := procGetForegroundWindow.Call()
	if fg != 0 {
		th, _, _ := procGetWindowThreadProcID.Call(fg, 0)
		l, _, _ := procGetKeyboardLayout.Call(th)
		t.Logf("foreground window %#x, its thread's layout %#x", fg, l)
	}
}

// TestKeyboardResetLayouts puts the session back in the state it should be
// in: US for the foreground thread, and no layout loaded that a test added.
// It only runs when TELA_RESET_LAYOUTS is set.
func TestKeyboardResetLayouts(t *testing.T) {
	if os.Getenv("TELA_RESET_LAYOUTS") == "" {
		t.Skip("TELA_RESET_LAYOUTS is not set")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	us, _, _ := procLoadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(utf16("00000409"))), klfNoTellShell)
	procActivateKeyboardLayout.Call(us, 0)
	resetForegroundLayout(us)
	for _, klid := range []string{"00000407", "0000040C", "0000040A", "00020409"} {
		h, _, _ := procLoadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(utf16(klid))), klfNoTellShell)
		if r, _, e := procUnloadKeyboardLayout.Call(h); r == 0 {
			t.Logf("UnloadKeyboardLayout(%s): %v", klid, e)
		}
	}
}
