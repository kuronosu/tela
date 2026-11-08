//go:build windows

package win32

import (
	"errors"
	"fmt"
	"image/color"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// A UI Automation client in the test process against a real window, as
// Narrator sees it (ADR-246). The client runs on its own thread in the
// multithreaded apartment, so the window's thread keeps pumping messages
// (UIA sends it WM_GETOBJECT). Needs the interactive desktop: TELA_LIVE=1.

var (
	clsidCUIAutomation      = guid{0xff48dba4, 0x60ef, 0x4201, [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
	clsidCUIAutomation8     = guid{0xe22ad333, 0xb25f, 0x460c, [8]byte{0x83, 0xd0, 0x05, 0x81, 0x10, 0x73, 0x95, 0xc9}}
	iidUIAutomation         = guid{0x30cbe57d, 0xd9d0, 0x452a, [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee}}
	iidUIAutomation5        = guid{0x25f700c8, 0xd816, 0x4057, [8]byte{0xa9, 0xdc, 0x3c, 0xbd, 0xee, 0x77, 0xe2, 0x56}}
	iidUIAFocusHandler      = guid{0xc270f6b5, 0x5c69, 0x4290, [8]byte{0x97, 0x45, 0x7a, 0x7f, 0x97, 0x16, 0x94, 0x68}}
	iidUIANotifyHandler     = guid{0xc7cb2637, 0xe6c2, 0x4d0c, [8]byte{0x85, 0xde, 0x49, 0x48, 0xc0, 0x21, 0x75, 0xc7}}
	iidUIAInvokePattern     = guid{0xfb377fbe, 0x8ea6, 0x46d5, [8]byte{0x9c, 0x73, 0x64, 0x99, 0x64, 0x2d, 0x30, 0x59}}
	iidUIAValuePattern      = guid{0xa94cd8b1, 0x0844, 0x4cd6, [8]byte{0x9d, 0x2d, 0x64, 0x05, 0x37, 0xab, 0x39, 0xe9}}
	iidUIATogglePattern     = guid{0x94cf8058, 0x9b8d, 0x4ab9, [8]byte{0x8b, 0xfd, 0x4c, 0xd0, 0xa3, 0x3c, 0x8c, 0x70}}
	iidUIATextPattern       = guid{0x32eba289, 0x3583, 0x42c9, [8]byte{0x9c, 0x59, 0x3b, 0x6d, 0x9a, 0x1e, 0x9b, 0x6a}}
	clientFocusVtbl         [4]uintptr
	clientNotifyVtbl        [4]uintptr
	clientVtblOnce          sync.Once
	uiaTestKeep             []*variant // keeps condition values on the heap
	uiaTestKeepMu           sync.Mutex
	errUIATimeout           = errors.New("timed out")
	uiaTreeScopeDescendants = uintptr(4)
	uiaTreeScopeSubtree     = uintptr(7)
)

// IUIAutomation, IUIAutomationElement and pattern vtable indices
// (UIAutomationClient.h).
const (
	vtUIAElementFromHandle        = 6
	vtUIAGetFocusedElement        = 8
	vtUIACreatePropertyCondition  = 23
	vtUIACreateAndCondition       = 25
	vtUIAAddFocusChangedHandler   = 39
	vtUIARemoveAllEventHandlers   = 41
	vtUIAAddNotificationHandler   = 68 // IUIAutomation5
	vtElemFindFirst               = 5
	vtElemGetCurrentPropertyValue = 10
	vtElemGetCurrentPatternAs     = 14
	vtElemCurrentName             = 23
	vtInvokeInvoke                = 3
	vtValueCurrentValue           = 4
	vtToggleCurrentToggleState    = 4
	vtTextPatternGetSelection     = 5
	vtTextPatternDocumentRange    = 7
	vtTextRangeArrayLength        = 3
	vtTextRangeArrayGetElement    = 4
	vtTextRangeGetText            = 12
	vtTextRangeExpandToEnclosing  = 6
	clientUIATimeout              = 5 * time.Second
	clientUIAPoll                 = 50 * time.Millisecond
	coinitMultithreadedForUIA     = 0
)

// uiaClient is the test's UIA client, on its own MTA thread.
type uiaClient struct {
	t     *testing.T
	uia   comObject
	focus chan string
	note  chan string
}

func (c *uiaClient) hr(what string, hr int32) error {
	if hr < 0 {
		return fmt.Errorf("%s: HRESULT %#x", what, uint32(hr))
	}
	return nil
}

// condition returns a property condition (the client's VARIANT goes by
// value: variantArgs).
func (c *uiaClient) condition(id int32, v uiaVal) (comObject, error) {
	val := new(variant)
	val.set(v)
	uiaTestKeepMu.Lock()
	uiaTestKeep = append(uiaTestKeep, val)
	uiaTestKeepMu.Unlock()
	var out comObject
	args := append([]uintptr{uintptr(id)}, variantArgs(val)...)
	args = append(args, uintptr(unsafe.Pointer(&out)))
	err := c.hr("CreatePropertyCondition", c.uia.call(vtUIACreatePropertyCondition, args...))
	val.clear()
	return out, err
}

// find waits for the descendant of root with the name and control type.
func (c *uiaClient) find(root comObject, name string, controlType int32) (comObject, error) {
	byName, err := c.condition(uiaNamePropertyID, uiaS(name))
	if err != nil {
		return 0, err
	}
	defer byName.release()
	byType, err := c.condition(uiaControlTypePropertyID, uiaI(controlType))
	if err != nil {
		return 0, err
	}
	defer byType.release()
	var both comObject
	if err := c.hr("CreateAndCondition", c.uia.call(vtUIACreateAndCondition, uintptr(byName), uintptr(byType), uintptr(unsafe.Pointer(&both)))); err != nil {
		return 0, err
	}
	defer both.release()
	deadline := time.Now().Add(clientUIATimeout)
	for time.Now().Before(deadline) {
		var el comObject
		hr := root.call(vtElemFindFirst, uiaTreeScopeDescendants, uintptr(both), uintptr(unsafe.Pointer(&el)))
		if hr >= 0 && el != 0 {
			return el, nil
		}
		time.Sleep(clientUIAPoll)
	}
	return 0, fmt.Errorf("no %q of control type %d under the window: %w", name, controlType, errUIATimeout)
}

// property returns a property of an element.
func (c *uiaClient) property(el comObject, id int32) (any, error) {
	v := new(variant)
	if err := c.hr("GetCurrentPropertyValue", el.call(vtElemGetCurrentPropertyValue, uintptr(id), uintptr(unsafe.Pointer(v)))); err != nil {
		return nil, err
	}
	if v.vt == vtArray|vtR8 {
		sa := v.val
		var out []float64
		for i := range safeArrayLen(sa) {
			var f float64
			safeArrayAt(sa, i, unsafe.Pointer(&f))
			out = append(out, f)
		}
		v.clear()
		return out, nil
	}
	return variantValue(v), nil
}

// patternOf returns pattern id of an element as interface iid.
func (c *uiaClient) patternOf(el comObject, id int32, iid guid) (comObject, error) {
	var p comObject
	if err := c.hr("GetCurrentPatternAs", el.call(vtElemGetCurrentPatternAs, uintptr(id), uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&p)))); err != nil {
		return 0, err
	}
	if p == 0 {
		return 0, fmt.Errorf("no pattern %d", id)
	}
	return p, nil
}

// bstrOut calls method i of o with a BSTR out parameter after args.
func bstrOut(o comObject, i int, args ...uintptr) (string, int32) {
	var s uintptr
	hr := o.call(i, append(args, uintptr(unsafe.Pointer(&s)))...)
	defer procSysFreeString.Call(s)
	return bstrString(s), hr
}

// newClientHandler returns an event handler object for the client.
func newClientHandler(vtbl *uintptr, iid guid, ch chan string) *comServer {
	return newComServer(vtbl, []guid{iid, iidAgileObject}, ch, nil)
}

func initClientVtbls() {
	clientVtblOnce.Do(func() {
		initComVtables()
		qi, addRef, release := dropTargetVtbl[0], dropTargetVtbl[1], dropTargetVtbl[2]
		send := func(this uintptr, s string) {
			if srv := comServerOf(this); srv != nil {
				select {
				case srv.impl.(chan string) <- s:
				default:
				}
			}
		}
		// IUIAutomationFocusChangedEventHandler::HandleFocusChangedEvent.
		clientFocusVtbl = [4]uintptr{qi, addRef, release, syscall.NewCallback(func(this, sender uintptr) uintptr {
			name, _ := bstrOut(comObject(sender), vtElemCurrentName)
			send(this, name)
			return sOK
		})}
		// IUIAutomationNotificationEventHandler::HandleNotificationEvent.
		clientNotifyVtbl = [4]uintptr{qi, addRef, release, syscall.NewCallback(func(this, sender, kind, processing, display, activity uintptr) uintptr {
			send(this, fmt.Sprintf("%s (processing %d, %s)", bstrString(display), int32(processing), bstrString(activity)))
			return sOK
		})}
	})
}

// waitFor waits for a message of ch equal to want.
func waitFor(ch chan string, want string) error {
	deadline := time.After(clientUIATimeout)
	var seen []string
	for {
		select {
		case s := <-ch:
			if s == want {
				return nil
			}
			seen = append(seen, s)
		case <-deadline:
			return fmt.Errorf("no %q (got %q): %w", want, seen, errUIATimeout)
		}
	}
}

func TestLiveUIAClient(t *testing.T) {
	requireLiveIntegration(t)
	if !uiaAvailable() {
		t.Skip("no UI Automation")
	}
	b := liveBackend(t)
	pw, err := b.NewWindow(platform.WindowOptions{Title: "Tela UIA test", Size: platform.Size{W: 400, H: 300}})
	if err != nil {
		t.Fatal(err)
	}
	w := pw.(*Window)
	hwnd := w.hwnd
	cmds := make(chan func(), 8)
	actions := make(chan platform.AccessActionEvent, 16)
	var activations int
	started := false
	runLive(t, b, nil, func(win platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, win, color.RGBA{0xF0, 0xF0, 0xF0, 0xFF})
			if !started {
				started = true
				go func() {
					defer b.Quit()
					if err := runUIAClient(t, hwnd, b, cmds, actions); err != nil {
						t.Error(err)
					}
				}()
			}
		case platform.AccessibilityEvent:
			t.Logf("AccessibilityEvent %+v", e)
			if e.Active {
				activations++
				u := loginTree()
				win.SetAccessibility(u)
			}
		case platform.AccessActionEvent:
			t.Logf("AccessActionEvent %+v", e)
			actions <- e
		case platform.WakeEvent:
			for len(cmds) > 0 {
				(<-cmds)()
			}
		}
	})
	if activations != 1 {
		t.Errorf("%d activations, want 1", activations)
	}
}

// runUIAClient is the client's script.
func runUIAClient(t *testing.T, hwnd uintptr, b *Backend, cmds chan func(), actions chan platform.AccessActionEvent) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procCoInitializeEx.Call(0, coinitMultithreadedForUIA)
	defer procCoUninitialize.Call()
	initClientVtbls()
	c := &uiaClient{t: t, focus: make(chan string, 32), note: make(chan string, 32)}
	// CUIAutomation8 (Windows 8) implements IUIAutomation2 to 6;
	// CUIAutomation only the first.
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidCUIAutomation8)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidUIAutomation)), uintptr(unsafe.Pointer(&c.uia)))
	if int32(uint32(r)) < 0 {
		r, _, _ = procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidCUIAutomation)), 0, clsctxInprocServer,
			uintptr(unsafe.Pointer(&iidUIAutomation)), uintptr(unsafe.Pointer(&c.uia)))
	}
	if int32(uint32(r)) < 0 {
		return fmt.Errorf("CoCreateInstance(CUIAutomation) = %#x", uint32(r))
	}
	defer c.uia.release()
	onUI := func(f func()) {
		cmds <- f
		b.Wake()
	}

	var root comObject
	if err := c.hr("ElementFromHandle", c.uia.call(vtUIAElementFromHandle, hwnd, uintptr(unsafe.Pointer(&root)))); err != nil {
		return err
	}
	defer root.release()
	if v, err := c.property(root, uiaNamePropertyID); err != nil || v != "Tela UIA test" {
		t.Errorf("window Name = %v, %v (the host provider's title)", v, err)
	}
	if v, err := c.property(root, uiaFrameworkIDPropertyID); err != nil || v != "Tela" {
		t.Errorf("window FrameworkId = %v, %v", v, err)
	}

	// The button, found by name once the tree arrived.
	button, err := c.find(root, "Sign in", uiaButtonControlTypeID)
	if err != nil {
		return err
	}
	defer button.release()
	for id, want := range map[int32]any{uiaIsEnabledPropertyID: true, uiaIsKeyboardFocusablePropertyID: true,
		uiaFrameworkIDPropertyID: "Tela", uiaAriaRolePropertyID: "button"} {
		if v, err := c.property(button, id); err != nil || v != want {
			t.Errorf("button property %d = %v, %v; want %v", id, v, err, want)
		}
	}
	if v, err := c.property(button, uiaBoundingRectanglePropertyID); err != nil {
		t.Error(err)
	} else if r, _ := v.([]float64); len(r) != 4 || r[2] <= 0 || r[3] <= 0 {
		t.Errorf("button BoundingRectangle = %v", v)
	} else {
		t.Logf("button BoundingRectangle %v", r)
	}
	inv, err := c.patternOf(button, uiaInvokePatternID, iidUIAInvokePattern)
	if err != nil {
		return err
	}
	if err := c.hr("Invoke", inv.call(vtInvokeInvoke)); err != nil {
		t.Error(err)
	}
	inv.release()
	// UIA's client sets the focus before a pattern call (IUIAutomation2's
	// AutoSetFocus, on by default): a focus action may come first.
	for invoked := false; !invoked; {
		select {
		case e := <-actions:
			switch e {
			case platform.AccessActionEvent{Node: 8, Action: platform.AccessActionDefault}:
				invoked = true
			case platform.AccessActionEvent{Node: 8, Action: platform.AccessActionFocus}:
			default:
				t.Errorf("Invoke sent %+v", e)
			}
		case <-time.After(clientUIATimeout):
			t.Error("Invoke sent no default action")
			invoked = true
		}
	}

	// The checkbox's toggle state.
	cb, err := c.find(root, "Remember me", uiaCheckBoxControlTypeID)
	if err != nil {
		return err
	}
	tog, err := c.patternOf(cb, uiaTogglePatternID, iidUIATogglePattern)
	cb.release()
	if err != nil {
		return err
	}
	var state int32 = -1
	tog.call(vtToggleCurrentToggleState, uintptr(unsafe.Pointer(&state)))
	tog.release()
	if state != uiaToggleOff {
		t.Errorf("checkbox ToggleState %d", state)
	}

	// The text field's value and text.
	field, err := c.find(root, "Email", uiaEditControlTypeID)
	if err != nil {
		return err
	}
	defer field.release()
	val, err := c.patternOf(field, uiaValuePatternID, iidUIAValuePattern)
	if err != nil {
		return err
	}
	if s, hr := bstrOut(val, vtValueCurrentValue); hr < 0 || s != "ana@example.com" {
		t.Errorf("field Value = %q, %#x", s, uint32(hr))
	}
	val.release()
	text, err := c.patternOf(field, uiaTextPatternID, iidUIATextPattern)
	if err != nil {
		return err
	}
	var doc comObject
	text.call(vtTextPatternDocumentRange, uintptr(unsafe.Pointer(&doc)))
	if s, hr := bstrOut(doc, vtTextRangeGetText, ^uintptr(0)); hr < 0 || s != "ana@example.com" {
		t.Errorf("document range text = %q, %#x", s, uint32(hr))
	}
	doc.release()
	var sel comObject
	text.call(vtTextPatternGetSelection, uintptr(unsafe.Pointer(&sel)))
	if sel != 0 {
		var n int32
		sel.call(vtTextRangeArrayLength, uintptr(unsafe.Pointer(&n)))
		if n == 1 {
			var caret comObject
			sel.call(vtTextRangeArrayGetElement, 0, uintptr(unsafe.Pointer(&caret)))
			caret.call(vtTextRangeExpandToEnclosing, uiaUnitWord)
			if s, _ := bstrOut(caret, vtTextRangeGetText, ^uintptr(0)); s != "@" {
				t.Errorf("the word at the caret = %q, want %q", s, "@")
			}
			caret.release()
		} else {
			t.Errorf("selection has %d ranges", n)
		}
		sel.release()
	} else {
		t.Error("GetSelection returned nothing")
	}
	text.release()

	// A focus-changed event when the app focuses the field.
	fh := newClientHandler(&clientFocusVtbl[0], iidUIAFocusHandler, c.focus)
	defer fh.release()
	if err := c.hr("AddFocusChangedEventHandler", c.uia.call(vtUIAAddFocusChangedHandler, 0, fh.ptr())); err != nil {
		return err
	}
	defer c.uia.call(vtUIARemoveAllEventHandlers)
	onUI(func() {
		u := loginTree()
		f := u.Nodes[4]
		f.States |= platform.AccessFocused
		w := windowOf(hwnd)
		if w != nil {
			w.SetAccessibility(platform.AccessUpdate{Root: 1, Focus: 5, Nodes: []platform.AccessNode{f}})
		}
	})
	if err := waitFor(c.focus, "Email"); err != nil {
		t.Errorf("focus changed event: %v", err)
	}
	var focused comObject
	if c.uia.call(vtUIAGetFocusedElement, uintptr(unsafe.Pointer(&focused))) >= 0 && focused != 0 {
		if s, _ := bstrOut(focused, vtElemCurrentName); s != "Email" {
			t.Logf("GetFocusedElement: %q (the window may not have the system focus)", s)
		}
		focused.release()
	}

	// A notification event for an announcement.
	var uia5 comObject
	if c.uia.call(vtQueryInterface, uintptr(unsafe.Pointer(&iidUIAutomation5)), uintptr(unsafe.Pointer(&uia5))) < 0 {
		t.Log("no IUIAutomation5 (Windows before 10 1709): notification events not checked")
		return nil
	}
	defer uia5.release()
	nh := newClientHandler(&clientNotifyVtbl[0], iidUIANotifyHandler, c.note)
	defer nh.release()
	if err := c.hr("AddNotificationEventHandler", uia5.call(vtUIAAddNotificationHandler, uintptr(root), uiaTreeScopeSubtree, 0, nh.ptr())); err != nil {
		return err
	}
	onUI(func() {
		if w := windowOf(hwnd); w != nil {
			w.SetAccessibility(platform.AccessUpdate{Root: 1, Focus: 5,
				Announcements: []platform.Announcement{{Text: "Signed in"}, {Text: "Wrong password", Assertive: true}}})
		}
	})
	if err := waitFor(c.note, "Signed in (processing 2, Tela.Live.Polite)"); err != nil {
		t.Errorf("polite notification: %v", err)
	}
	if err := waitFor(c.note, "Wrong password (processing 1, Tela.Live.Assertive)"); err != nil {
		t.Errorf("assertive notification: %v", err)
	}
	return nil
}

// windowOf returns the Tela window of a handle (UI thread).
func windowOf(hwnd uintptr) *Window {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.windows[hwnd]
}
