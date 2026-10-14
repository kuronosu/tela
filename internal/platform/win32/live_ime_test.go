//go:build windows

package win32

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unicode"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

var (
	procImmGetCandidateWindow   = imm32.NewProc("ImmGetCandidateWindow")
	procImmGetCompositionWindow = imm32.NewProc("ImmGetCompositionWindow")
	procImmSetOpenStatus        = imm32.NewProc("ImmSetOpenStatus")
	procImmGetOpenStatus        = imm32.NewProc("ImmGetOpenStatus")
	procImmGetConversionStatus  = imm32.NewProc("ImmGetConversionStatus")
	procGetClassNameW           = user32.NewProc("GetClassNameW")
	procGetWindowRect           = user32.NewProc("GetWindowRect")
	procGetCaretPos             = user32.NewProc("GetCaretPos")
)

const gaRoot = 2

var (
	procWindowFromPoint     = user32.NewProc("WindowFromPoint")
	procGetAncestor         = user32.NewProc("GetAncestor")
	procImmIsIME            = imm32.NewProc("ImmIsIME")
	procImmGetDefaultIMEWnd = imm32.NewProc("ImmGetDefaultIMEWnd")
)

const vkIMEOn = 0x16

// scDigit3JIS is the scan code of the 3 key (あ in kana input).
const scDigit3JIS = 0x04

// imeStatus returns the open status and conversion mode of hwnd's context.
func imeStatus(hwnd uintptr) (open uintptr, conv uint32) {
	himc, release := imeContext(hwnd)
	defer release()
	if himc == 0 {
		return 0, 0
	}
	open, _, _ = procImmGetOpenStatus.Call(himc)
	var sent uint32
	procImmGetConversionStatus.Call(himc, uintptr(unsafe.Pointer(&conv)), uintptr(unsafe.Pointer(&sent)))
	return open, conv
}

// sendVK taps a virtual key (no scan code): the IME keys.
func sendVK(vk uint16) {
	ins := []input{
		{typ: inputKeyboard, ki: keybdInput{vk: vk}},
		{typ: inputKeyboard, ki: keybdInput{vk: vk, flags: keyeventfKeyUp}},
	}
	procSendInput.Call(uintptr(len(ins)), uintptr(unsafe.Pointer(&ins[0])), unsafe.Sizeof(ins[0]))
}

func inWine() bool { return procWineVersion.Find() == nil }

// imeContext returns the input context of hwnd and its release.
func imeContext(hwnd uintptr) (uintptr, func()) {
	h, _, _ := procImmGetContext.Call(hwnd)
	return h, func() {
		if h != 0 {
			procImmReleaseContext.Call(hwnd, h)
		}
	}
}

func candidateWindow(hwnd uintptr) (candidateForm, bool) {
	himc, release := imeContext(hwnd)
	defer release()
	if himc == 0 {
		return candidateForm{}, false
	}
	var f candidateForm
	r, _, _ := procImmGetCandidateWindow.Call(himc, 0, uintptr(unsafe.Pointer(&f)))
	return f, r != 0
}

func compositionWindow(hwnd uintptr) (compositionForm, bool) {
	himc, release := imeContext(hwnd)
	defer release()
	if himc == 0 {
		return compositionForm{}, false
	}
	var f compositionForm
	r, _, _ := procImmGetCompositionWindow.Call(himc, uintptr(unsafe.Pointer(&f)))
	return f, r != 0
}

// TestIMEContext checks the input context handling that does not need an
// IME (it also runs under Wine): a new window has no context, a text field
// associates the default one and places the candidate window at the cursor
// area, a password field and no field detach it.
func TestIMEContext(t *testing.T) {
	b := liveBackend(t)
	var w *Window
	var problems []string
	errorf := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	step := 0
	area := platform.Rect{X: 30.5, Y: 40, W: 0, H: 20}
	runLive(t, b, func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "ime context", Size: platform.Size{W: 300, H: 200}})
		if err != nil {
			t.Fatal(err)
		}
		w = pw.(*Window)
		if h, release := imeContext(w.hwnd); h != 0 {
			release()
			errorf("a new window has an input context")
		}
	}, func(ew platform.Window, e platform.Event) {
		next := func() {
			step++
			time.AfterFunc(100*time.Millisecond, b.Wake)
		}
		switch e.(type) {
		case platform.FrameEvent:
			presentSolid(t, ew, solidBlack)
			if step == 0 {
				w.SetTextInput(&platform.TextInputState{CursorArea: area})
				next()
			}
		case platform.WakeEvent:
			switch step {
			case 1:
				h, release := imeContext(w.hwnd)
				release()
				if h == 0 {
					errorf("no input context with a text field")
					break
				}
				r := imeCaretRect(area, w.Metrics().Scale)
				if f, ok := candidateWindow(w.hwnd); !ok || f.style != cfsExclude || f.area != rectOf(r) || f.pos != (point{int32(r.Min.X), int32(r.Min.Y)}) {
					errorf("candidate window %+v (%v), want CFS_EXCLUDE at %v", f, ok, r)
				}
				if f, ok := compositionWindow(w.hwnd); !ok || f.style != cfsPoint || f.pos != (point{int32(r.Min.X), int32(r.Min.Y)}) {
					errorf("composition window %+v (%v), want CFS_POINT at %v", f, ok, r.Min)
				}
				// The cursor area moves: so does the candidate window.
				area.Y = 80
				w.SetTextInput(&platform.TextInputState{CursorArea: area})
				next()
			case 2:
				r := imeCaretRect(area, w.Metrics().Scale)
				if f, ok := candidateWindow(w.hwnd); !ok || f.area != rectOf(r) {
					errorf("candidate window after a move %+v (%v), want %v", f, ok, r)
				}
				w.SetTextInput(&platform.TextInputState{CursorArea: area, Purpose: platform.TextPurposePassword})
				next()
			case 3:
				if h, release := imeContext(w.hwnd); h != 0 {
					release()
					errorf("a password field has an input context")
				}
				w.SetTextInput(&platform.TextInputState{CursorArea: area, Purpose: platform.TextPurposeEmail})
				next()
			case 4:
				if h, release := imeContext(w.hwnd); h == 0 {
					errorf("no input context after the password field")
				} else {
					release()
				}
				w.SetTextInput(nil)
				next()
			case 5:
				if h, release := imeContext(w.hwnd); h != 0 {
					release()
					errorf("an input context without a text field")
				}
				b.Quit()
			}
		}
	})
	for _, p := range problems {
		t.Error(p)
	}
}

// TestLiveIME composes Japanese with Microsoft IME through IMM32: it
// switches the thread to the Japanese input locale (restored at the end),
// opens the IME in hiragana mode, types "nihongo" with SendInput scan codes,
// converts with Space, opens the candidate list with a second Space and
// commits with Enter. It then checks ResetTextInput (the IME's composition
// is gone), the password purpose (no context, keys type ASCII), and focus
// loss in the middle of a composition (the session ends, nothing of that
// composition comes back with the focus). It needs Windows with the
// Japanese IME installed and an interactive desktop; Wine's IMM32 has no
// IME, so it skips there.
func TestLiveIME(t *testing.T) {
	if inWine() {
		t.Skip("Wine has no Japanese IME")
	}
	hkl, restore := layoutSession(t, "00000411")
	defer restore()
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{Title: "tela live IME", Size: platform.Size{W: 420, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	other, err := b.NewWindow(platform.WindowOptions{Title: "tela live IME (other)", Size: platform.Size{W: 200, H: 100}})
	if err != nil {
		t.Fatal(err)
	}
	ow := other.(*Window)

	var (
		log      []string
		imes     []platform.IMEEvent // since the last mark
		value    string
		preedit  string
		session  bool
		focused  bool
		keyTexts []string // KeyEvent texts since the last mark
		textLeak int      // KeyEvents with Text while a preedit was shown
		frames   int
	)
	start := time.Now()
	logf := func(format string, args ...any) {
		log = append(log, fmt.Sprintf("%6dms ", time.Since(start).Milliseconds())+fmt.Sprintf(format, args...))
	}
	area := platform.Rect{X: 24, Y: 50, H: 24}
	state := func(p platform.TextPurpose) *platform.TextInputState {
		return &platform.TextInputState{
			CursorArea:  area,
			Purpose:     p,
			Surrounding: &platform.SurroundingText{Text: value, Cursor: len(value), Anchor: len(value)},
		}
	}
	purpose := platform.TextPurposeNormal
	mark := func() { imes, keyTexts = nil, nil }
	lastPreedit := func() (string, bool) {
		if len(imes) == 0 {
			return "", false
		}
		return imes[len(imes)-1].Preedit, true
	}
	commits := func() string {
		var s string
		for _, e := range imes {
			s += e.Commit
		}
		return s
	}
	compLen := func() int {
		himc, release := imeContext(w.hwnd)
		defer release()
		if himc == 0 {
			return -1
		}
		n, _, _ := procImmGetCompositionStringW.Call(himc, gcsCompStr, 0, 0)
		return int(int32(n))
	}
	// kana is set when the probe finds the IME in kana input mode (かな入力,
	// a per-user setting of Microsoft IME): the keys of the JIS kana layout
	// then type the same words. The probe itself and the password field
	// type plain letters.
	kana := false
	keys := func(s string) []sk {
		romaji := map[rune]uint16{'n': scN, 'i': scI, 'h': scH, 'o': scO, 'g': scG, 'k': scK, 'a': scA, 'u': scU}
		var codes []uint16
		if kana && session {
			// に ほ ん こ ゛, あ い, う, か.
			codes = map[string][]uint16{
				"nihongo": {scI, scMinus, scY, scB, scBracketL},
				"ai":      {scDigit3JIS, scE},
				"u":       {scDigit4},
				"ka":      {scT},
			}[s]
		}
		if codes == nil {
			for _, r := range s {
				codes = append(codes, romaji[r])
			}
		}
		var out []sk
		for _, c := range codes {
			out = append(out, ktap(c)...)
		}
		return out
	}

	type liveStep struct {
		name string
		wait time.Duration
		do   func()
	}
	var steps []liveStep
	var cur int
	var noIME string // set when the probe finds no composing IME on CI
	// abort fails the test from the handler (t.Fatal cannot unwind through
	// WndProc) and stops the run.
	abort := func(format string, args ...any) {
		t.Errorf(format, args...)
		cur = len(steps)
		b.Quit()
	}
	add := func(name string, wait time.Duration, do func()) { steps = append(steps, liveStep{name, wait, do}) }
	var deadline time.Time
	var timedOut atomic.Bool
	arm := func(d time.Duration) {
		deadline = time.Now().Add(d)
		time.AfterFunc(d+10*time.Millisecond, b.Wake)
	}
	attempts := 0
	activate := func() {
		if attempts > 0 {
			us, _, _ := procLoadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(utf16("00000409"))), klfNoTellShell)
			procActivateKeyboardLayout.Call(us, 0)
			time.Sleep(150 * time.Millisecond)
		}
		procActivateKeyboardLayout.Call(hkl, 0)
	}

	add("activate the Japanese input locale", 500*time.Millisecond, func() { activate() })
	add("open the IME", 500*time.Millisecond, func() {
		for layoutLang(currentLayout()) != langJapanese && attempts < 5 {
			attempts++
			activate()
			time.Sleep(300 * time.Millisecond)
		}
		if l := currentLayout(); layoutLang(l) != langJapanese {
			abort("the input locale is %#x, not Japanese", l)
			return
		}
		himc, release := imeContext(w.hwnd)
		defer release()
		if himc == 0 {
			abort("no input context with a text field")
			return
		}
		open, conv := imeStatus(w.hwnd)
		logf("layout %#x, IME open %d, conversion %#x", currentLayout(), open, conv)
		mark()
	})
	// The IME must be on for the window's input: ImmSetOpenStatus through
	// the IMM32 bridge is not always enough for a TSF IME. A probe types
	// "a" and looks for the preedit あ; the strategies that follow turn the
	// IME on the ways a user would.
	strategy := 0
	strategies := []struct {
		name string
		do   func()
	}{
		{"ImmSetOpenStatus", func() {
			himc, release := imeContext(w.hwnd)
			procImmSetOpenStatus.Call(himc, 1)
			release()
		}},
		{"VK_IME_ON", func() { sendVK(vkIMEOn) }},
	}
	var probeAt int
	add("probe the IME", 1000*time.Millisecond, func() {
		probeAt = cur
		s := strategies[strategy]
		s.do()
		isIME, _, _ := procImmIsIME.Call(currentLayout())
		defWnd, _, _ := procImmGetDefaultIMEWnd.Call(w.hwnd)
		open, conv := imeStatus(w.hwnd)
		logf("probe with %s: layout %#x ImmIsIME %d, default IME window %#x, open %d, conversion %#x", s.name, currentLayout(), isIME, defWnd, open, conv)
	})
	add("type the probe", 3000*time.Millisecond, func() {
		open, conv := imeStatus(w.hwnd)
		logf("before the probe key: open %d, conversion %#x", open, conv)
		mark()
		sendKeys(t, keys("a"))
	})
	add("check the probe", 1000*time.Millisecond, func() {
		open, conv := imeStatus(w.hwnd)
		logf("after the probe key: open %d, conversion %#x", open, conv)
		p, _ := lastPreedit()
		if p == "あ" || p == "ち" {
			kana = p == "ち"
			logf("the IME composes after %s (kana input %v)", strategies[strategy].name, kana)
			mark()
			sendKeys(t, ktap(scEsc))
			return
		}
		if strategy++; strategy < len(strategies) {
			cur = probeAt - 1 // the next Wake runs the probe again
			return
		}
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			// Hosted runners load the Japanese layout but have no
			// Microsoft IME that composes (a service desktop). Elsewhere
			// (the Windows VM) this stays a failure.
			noIME = fmt.Sprintf("the IME never composed on this hosted runner (preedit %q)", p)
			cur = len(steps)
			b.Quit()
			return
		}
		abort("the IME never composed (preedit %q)", p)
	})
	add("type nihongo", 1500*time.Millisecond, func() { mark(); sendKeys(t, keys("nihongo")) })
	add("check the preedit, convert", 1500*time.Millisecond, func() {
		p, ok := lastPreedit()
		if !ok || p != "にほんご" {
			t.Errorf("preedit after typing nihongo = %q (%v), want にほんご", p, ok)
		}
		if commits() != "" {
			t.Errorf("committed %q while typing", commits())
		}
		r := imeCaretRect(area, w.Metrics().Scale)
		if f, ok := candidateWindow(w.hwnd); !ok || f.style != cfsExclude || f.area != rectOf(r) {
			t.Errorf("candidate form %+v (%v), want CFS_EXCLUDE %v", f, ok, r)
		} else {
			logf("candidate form %+v", f)
		}
		var cp point
		procGetCaretPos.Call(uintptr(unsafe.Pointer(&cp)))
		logf("system caret %v (%v)", cp, w.ime.caret)
		mark()
		sendKeys(t, ktap(scSpaceK))
	})
	var caretScreen point
	// Microsoft IME learns: the first conversion of にほんご is the
	// candidate committed last (日本語, or にほんご after a run that
	// committed the second candidate). A kanji conversion must show up
	// after the first or the second Space, which also opens the list.
	hasKanji := func(s string) bool {
		return strings.ContainsFunc(s, func(r rune) bool { return unicode.Is(unicode.Han, r) })
	}
	sawKanji := false
	convPreedit := ""
	checkConversion := func(after string) {
		e := imes
		if len(e) == 0 {
			t.Errorf("no IMEEvent after %s", after)
			return
		}
		last := e[len(e)-1]
		if last.Preedit == "" || last.CursorStart >= last.CursorEnd {
			t.Errorf("after %s: %+v, want a conversion with its target clause as the cursor range", after, last)
		}
		sawKanji = sawKanji || hasKanji(last.Preedit)
		convPreedit = last.Preedit
	}
	add("check the conversion, open the candidate list", 1500*time.Millisecond, func() {
		checkConversion("Space")
		mark()
		r := imeCaretRect(area, w.Metrics().Scale)
		caretScreen = clientToScreen(w.hwnd, r.Min)
		sendKeys(t, ktap(scSpaceK))
	})
	shot := make(chan struct{})
	close(shot)
	add("screenshot", 4000*time.Millisecond, func() {
		// TELA_IME_SHOT=path captures the screen with the candidate list
		// open (the job runner's screenshot.ps1). Not from the UI thread:
		// the IME may draw its windows from it.
		if path, tools := os.Getenv("TELA_IME_SHOT"), os.Getenv("TELA_TOOLS"); path != "" && tools != "" {
			shot = make(chan struct{})
			go func() {
				defer close(shot)
				out, err := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
					filepath.Join(tools, "screenshot.ps1"), path).CombinedOutput()
				if err != nil {
					t.Logf("screenshot: %v: %s", err, out)
				}
			}()
		}
	})
	add("look at the candidate list, commit", 1500*time.Millisecond, func() {
		<-shot
		checkConversion("the second Space")
		if !sawKanji {
			t.Errorf("no kanji conversion of にほんご after two Spaces")
		}
		// The candidate list must sit right below the caret: the window
		// just under the caret's bottom-left corner is the IME's.
		r := imeCaretRect(area, w.Metrics().Scale)
		bottom := caretScreen.y + int32(r.Dy())
		pt := point{caretScreen.x + 8, bottom + 12}
		hit, _, _ := procWindowFromPoint.Call(uintptr(*(*uint64)(unsafe.Pointer(&pt))))
		root, _, _ := procGetAncestor.Call(hit, gaRoot)
		// Microsoft IME's list (Windows 11) is a window with a transparent
		// margin around what it draws: its rectangle starts 34 px left of
		// the caret and 18 px above the caret's bottom, and the visible
		// list one pixel below the caret (see the screenshot).
		var wr rect
		procGetWindowRect.Call(root, uintptr(unsafe.Pointer(&wr)))
		var buf [256]uint16
		n, _, _ := procGetClassNameW.Call(root, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		class := syscall.UTF16ToString(buf[:n])
		logf("caret at screen (%d,%d)-(%d,%d); window below it: %#x %q (%d,%d)-(%d,%d)",
			caretScreen.x, caretScreen.y, caretScreen.x+int32(r.Dx()), bottom, root, class, wr.left, wr.top, wr.right, wr.bottom)
		if root == w.hwnd || root == ow.hwnd || wr.top < bottom-40 || wr.top > bottom+30 || wr.left > caretScreen.x || wr.left < caretScreen.x-80 {
			t.Errorf("the candidate list is not below the caret: window %q at (%d,%d)-(%d,%d), caret bottom-left (%d,%d)",
				class, wr.left, wr.top, wr.right, wr.bottom, caretScreen.x, bottom)
		}
		mark()
		sendKeys(t, ktap(scEnterK))
	})
	add("check the commit, start a composition", 1500*time.Millisecond, func() {
		c := commits()
		if c == "" || c != convPreedit {
			t.Errorf("committed %q, want the selected candidate %q", c, convPreedit)
		}
		if p, ok := lastPreedit(); !ok || p != "" {
			t.Errorf("preedit after the commit = %q (%v), want empty", p, ok)
		}
		mark()
		sendKeys(t, keys("ai"))
	})
	add("reset", 800*time.Millisecond, func() {
		if p, _ := lastPreedit(); p != "あい" {
			t.Errorf("preedit %q, want あい", p)
		}
		mark()
		w.ResetTextInput()
	})
	add("after the reset, type u", 1200*time.Millisecond, func() {
		if n := compLen(); n != 0 {
			t.Errorf("the IME still has a composition of %d bytes after ResetTextInput", n)
		}
		if len(imes) != 0 {
			t.Errorf("IMEEvents after ResetTextInput: %+v", imes)
		}
		mark()
		sendKeys(t, keys("u"))
	})
	add("cancel with Escape", 1200*time.Millisecond, func() {
		if p, _ := lastPreedit(); p != "う" {
			t.Errorf("preedit after the reset and u = %q, want う (alone)", p)
		}
		mark()
		sendKeys(t, ktap(scEsc))
	})
	add("password", 800*time.Millisecond, func() {
		if p, ok := lastPreedit(); !ok || p != "" {
			t.Errorf("preedit after Escape = %q (%v), want empty", p, ok)
		}
		mark()
		purpose = platform.TextPurposePassword
		w.SetTextInput(state(purpose))
	})
	add("type in the password field", 1200*time.Millisecond, func() {
		if session {
			t.Errorf("IME session active in a password field")
		}
		if h, release := imeContext(w.hwnd); h != 0 {
			release()
			t.Errorf("a password field has an input context")
		}
		mark()
		sendKeys(t, keys("ka"))
	})
	add("back to normal", 800*time.Millisecond, func() {
		if len(imes) != 0 {
			t.Errorf("IMEEvents in a password field: %+v", imes)
		}
		if got := strings.Join(keyTexts, ""); got != "ka" {
			t.Errorf("password field typed %q, want ka", got)
		}
		mark()
		purpose = platform.TextPurposeNormal
		w.SetTextInput(state(purpose))
	})
	add("compose before the focus loss", 1200*time.Millisecond, func() {
		if !session {
			t.Errorf("no IME session after the password field")
		}
		himc, release := imeContext(w.hwnd)
		if himc != 0 {
			procImmSetOpenStatus.Call(himc, 1)
		}
		release()
		mark()
		sendKeys(t, keys("ka"))
	})
	add("lose the focus", 1200*time.Millisecond, func() {
		if p, _ := lastPreedit(); p != "か" {
			t.Errorf("preedit %q, want か", p)
		}
		mark()
		forceForeground(ow.hwnd)
	})
	add("get the focus back", 1500*time.Millisecond, func() {
		if session || focused {
			t.Errorf("session %v, focus %v after the focus moved", session, focused)
		}
		if len(imes) != 0 {
			t.Errorf("IMEEvents after the focus loss: %+v", imes)
		}
		if n := compLen(); n > 0 {
			t.Errorf("the IME kept a composition of %d bytes after the focus loss", n)
		}
		mark()
		forceForeground(w.hwnd)
	})
	add("type after the focus came back", 1200*time.Millisecond, func() {
		if !session || !focused {
			t.Errorf("session %v, focus %v after the focus came back", session, focused)
		}
		if len(imes) != 0 {
			t.Errorf("IMEEvents when the focus came back (a stale composition?): %+v", imes)
		}
		mark()
		sendKeys(t, keys("u"))
	})
	add("end", 300*time.Millisecond, func() {
		if p, _ := lastPreedit(); p != "う" {
			t.Errorf("preedit %q, want う (alone)", p)
		}
		sendKeys(t, ktap(scEsc))
	})
	add("restore the input locale", 300*time.Millisecond, func() {
		himc, release := imeContext(w.hwnd)
		if himc != 0 {
			procImmSetOpenStatus.Call(himc, 0)
		}
		release()
		procActivateKeyboardLayout.Call(origLayout, 0)
	})

	if os.Getenv("TELA_KEYLOG") == "2" {
		// The key and IME messages, the layout questions and the
		// placements, in the log.
		keyTrace = func(format string, args ...any) { logf("trace "+format, args...) }
		defer func() { keyTrace = nil }()
	}
	started := false
	otherShown, focusTries := false, 0
	timer := time.AfterFunc(60*time.Second, func() { timedOut.Store(true); b.Quit() })
	defer timer.Stop()
	err = b.Run(platform.HandlerFunc(func(win platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			presentSolid(t, win, solidBlack)
			if win == w {
				frames++
			} else {
				otherShown = true
			}
			// Showing a window activates it: give w the focus once both
			// are on screen.
			if frames > 0 && otherShown && focusTries == 0 {
				focusTries++
				logf("force the focus")
				forceForeground(w.hwnd)
				time.AfterFunc(2*time.Second, b.Wake)
			}
			return
		case platform.WakeEvent:
		case platform.ConfigureEvent:
			return
		}
		if win != w {
			if _, ok := e.(platform.WakeEvent); !ok {
				return
			}
		}
		switch e := e.(type) {
		case platform.FocusEvent:
			focused = e.Focused
			fg, _, _ := procGetForegroundWindow.Call()
			var buf [256]uint16
			n, _, _ := procGetClassNameW.Call(fg, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			logf("focus %v (foreground %#x %q)", e.Focused, fg, syscall.UTF16ToString(buf[:n]))
			if e.Focused && !started {
				w.SetTextInput(state(purpose))
			}
		case platform.IMESessionEvent:
			session = e.Active
			logf("session %v", e.Active)
			if e.Active && !started {
				started = true
				cur = 0
				steps[0].do()
				arm(steps[0].wait)
			}
		case platform.IMEEvent:
			imes = append(imes, e)
			logf("IMEEvent del=%d,%d commit=%q preedit=%q cursor=%d,%d", e.DeleteBefore, e.DeleteAfter, e.Commit, e.Preedit, e.CursorStart, e.CursorEnd)
			if p := platform.CheckIMEEvent(e); p != "" {
				t.Errorf("invalid IMEEvent %+v: %s", e, p)
			}
			value = value[:max(0, len(value)-e.DeleteBefore)] + e.Commit
			preedit = e.Preedit
			w.SetTextInput(state(purpose))
		case platform.KeyEvent:
			logf("KeyEvent %v %v key=%q text=%q", e.Action, e.Code, e.Key, e.Text)
			if e.Action == platform.KeyPress && e.Text != "" {
				keyTexts = append(keyTexts, e.Text)
				if preedit != "" {
					textLeak++
				}
			}
		case platform.WakeEvent:
			if !started && focusTries > 0 && focusTries < 4 {
				focusTries++
				logf("force the focus again (focus %v, session %v)", focused, session)
				forceForeground(w.hwnd)
				time.AfterFunc(2*time.Second, b.Wake)
				return
			}
			if !started || time.Now().Before(deadline) {
				return
			}
			if cur++; cur >= len(steps) {
				b.Quit()
				return
			}
			logf("step: %s", steps[cur].name)
			steps[cur].do()
			if cur >= 0 && cur < len(steps) { // abort moves past the end
				arm(steps[cur].wait)
			}
		}
	}))
	if err != nil {
		t.Errorf("Run: %v", err)
	}
	if !started {
		for _, l := range log {
			t.Log(l)
		}
		t.Skip("the window did not get the keyboard focus and an IME session")
	}
	if noIME != "" {
		t.Skip(noIME)
	}
	if timedOut.Load() {
		t.Errorf("timed out at step %d", cur)
	}
	if textLeak > 0 {
		t.Errorf("%d KeyEvents with Text arrived while a preedit was shown", textLeak)
	}
	t.Logf("value %q", value)
	for _, l := range log {
		t.Log(l)
	}
}
