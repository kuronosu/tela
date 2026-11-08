//go:build windows

package win32

import (
	"math"
	"slices"
	"testing"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// The providers called the way UI Automation calls them, through their
// vtables, without a window or a UIA client (live_uia_test.go has the real
// client). They run anywhere, a service session included.

// uiaCall calls method i of interface pointer p. Like comObject.call it
// is uintptrescapes: the pointers its callers convert in its argument list
// stay on the heap.
//
//go:uintptrescapes
func uiaCall(p uintptr, i int, args ...uintptr) uint32 { return uint32(comObject(p).call(i, args...)) }

// uiaQI returns interface iid of p, or 0.
func uiaQI(t *testing.T, p uintptr, iid guid) uintptr {
	t.Helper()
	var out uintptr
	if hr := uiaCall(p, vtQueryInterface, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&out))); hr != sOK {
		return 0
	}
	return out
}

// variantValue returns a VARIANT's value as a Go value and clears it.
func variantValue(v *variant) any {
	defer v.clear()
	switch v.vt {
	case vtEmpty:
		return nil
	case vtBool:
		return uint16(v.val) != 0
	case vtI4:
		return *(*int32)(unsafe.Pointer(&v.val))
	case vtR8:
		return *(*float64)(unsafe.Pointer(&v.val))
	case vtBSTR:
		return bstrString(v.val)
	case vtUnknown:
		return "unknown"
	}
	return v.vt
}

// prop returns property id of the element at p.
func prop(t *testing.T, p uintptr, id int32) any {
	t.Helper()
	v := new(variant)
	if hr := uiaCall(p, 5, uintptr(id), uintptr(unsafe.Pointer(v))); hr != sOK {
		t.Fatalf("GetPropertyValue(%d) = %#x", id, hr)
	}
	return variantValue(v)
}

// safeArrayLen returns the number of elements of a one-dimensional
// SAFEARRAY.
func safeArrayLen(sa uintptr) int {
	if sa == 0 {
		return -1
	}
	var ub int32
	procSafeArrayGetUBound.Call(sa, 1, uintptr(unsafe.Pointer(&ub)))
	return int(ub) + 1
}

// safeArrayAt returns element i of a SAFEARRAY of VT_UNKNOWN (with a
// reference), VT_I4 or VT_R8 (into out).
func safeArrayAt(sa uintptr, i int, out unsafe.Pointer) {
	idx := int32(i)
	procSafeArrayGetElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(out))
}

func destroySafeArray(sa uintptr) {
	if sa != 0 {
		procSafeArrayDestroy.Call(sa)
	}
}

// navigate calls IRawElementProviderFragment::Navigate on the element at
// p (any of its interfaces) and returns the Simple interface found, or 0.
func navigate(t *testing.T, p uintptr, dir int32) uintptr {
	t.Helper()
	f := uiaQI(t, p, iidRawElementProviderFragment)
	if f == 0 {
		t.Fatal("no IRawElementProviderFragment")
	}
	defer comObject(f).release()
	var out uintptr
	if hr := uiaCall(f, 3, uintptr(dir), uintptr(unsafe.Pointer(&out))); hr != sOK {
		t.Fatalf("Navigate(%d) = %#x", dir, hr)
	}
	if out == 0 {
		return 0
	}
	defer comObject(out).release()
	return uiaQI(t, out, iidRawElementProviderSimple)
}

// pattern returns pattern id of the element at p, or 0.
func pattern(t *testing.T, p uintptr, id int32) uintptr {
	t.Helper()
	var out uintptr
	if hr := uiaCall(p, 4, uintptr(id), uintptr(unsafe.Pointer(&out))); hr != sOK {
		t.Fatalf("GetPatternProvider(%d) = %#x", id, hr)
	}
	return out
}

func TestUIAProvidersThroughVtables(t *testing.T) {
	if !uiaAvailable() {
		t.Skip("no UI Automation on this system")
	}
	b := newUIABridge(0, 1.5, 0)
	defer b.close()
	root := b.root.srv.iface(uiaIfSimple)

	// Before the first update the root has no children.
	if c := navigate(t, root, uiaNavigateFirstChild); c != 0 {
		t.Fatal("the root has a child before the first update")
	}
	u := loginTree()
	b.apply(&u)

	var opts int32
	uiaCall(root, 3, uintptr(unsafe.Pointer(&opts)))
	if opts != uiaProviderOptionsServerSideProvider {
		t.Errorf("ProviderOptions = %d", opts)
	}
	if v := prop(t, root, uiaFrameworkIDPropertyID); v != "Tela" {
		t.Errorf("root FrameworkId = %v", v)
	}
	if v := prop(t, root, uiaControlTypePropertyID); v != nil {
		t.Errorf("root ControlType = %v, want the host's", v)
	}
	if fr := uiaQI(t, root, iidRawElementProviderFragmentRoot); fr == 0 {
		t.Error("the root is no fragment root")
	} else {
		comObject(fr).release()
	}

	heading := navigate(t, root, uiaNavigateFirstChild)
	defer comObject(heading).release()
	if v := prop(t, heading, uiaControlTypePropertyID); v != int32(uiaTextControlTypeID) {
		t.Errorf("heading ControlType = %v", v)
	}
	if v := prop(t, heading, uiaLocalizedControlTypePropertyID); v != "heading" {
		t.Errorf("heading LocalizedControlType = %v", v)
	}
	if fr := uiaQI(t, heading, iidRawElementProviderFragmentRoot); fr != 0 {
		t.Error("an element answers IRawElementProviderFragmentRoot")
	}
	field := navigate(t, heading, uiaNavigateNextSibling)
	defer comObject(field).release()
	checkbox := navigate(t, field, uiaNavigateNextSibling)
	defer comObject(checkbox).release()
	button := navigate(t, root, uiaNavigateLastChild)
	defer comObject(button).release()
	if v := prop(t, button, uiaNamePropertyID); v != "Sign in" {
		t.Errorf("button Name = %v", v)
	}
	if v := prop(t, button, uiaControlTypePropertyID); v != int32(uiaButtonControlTypeID) {
		t.Errorf("button ControlType = %v", v)
	}
	if v := prop(t, button, uiaIsKeyboardFocusablePropertyID); v != true {
		t.Errorf("button IsKeyboardFocusable = %v", v)
	}
	if c := navigate(t, button, uiaNavigateFirstChild); c != 0 {
		t.Error("the button exposes its text")
	}
	if p := navigate(t, button, uiaNavigatePreviousSibling); p != checkbox {
		t.Error("the button's previous sibling is not the checkbox's object")
	} else {
		comObject(p).release()
	}
	if p := navigate(t, field, uiaNavigateParent); p != root {
		t.Error("the field's parent is not the root's object")
	} else {
		comObject(p).release()
	}

	// Runtime ID and bounds (scale 1.5, no window: origin 0, 0).
	frag := uiaQI(t, button, iidRawElementProviderFragment)
	var sa uintptr
	uiaCall(frag, 4, uintptr(unsafe.Pointer(&sa)))
	var rid []int32
	for i := range safeArrayLen(sa) {
		var v int32
		safeArrayAt(sa, i, unsafe.Pointer(&v))
		rid = append(rid, v)
	}
	destroySafeArray(sa)
	if !slices.Equal(rid, []int32{uiaAppendRuntimeID, 0, 8}) {
		t.Errorf("runtime ID %v", rid)
	}
	var r uiaRect
	uiaCall(frag, 5, uintptr(unsafe.Pointer(&r)))
	if r != (uiaRect{15, 180, 150, 45}) {
		t.Errorf("BoundingRectangle %+v", r)
	}
	comObject(frag).release()

	// ElementProviderFromPoint through the double-argument thunk: Go's
	// syscalls copy the first arguments into the XMM registers too, as a
	// C caller passes doubles.
	fr := uiaQI(t, root, iidRawElementProviderFragmentRoot)
	var hit uintptr
	hr := uiaCall(fr, 3, uintptr(math.Float64bits(30)), uintptr(math.Float64bits(80)), uintptr(unsafe.Pointer(&hit)))
	if hr != sOK || hit == 0 {
		t.Errorf("ElementProviderFromPoint = %#x, %#x", hr, hit)
	} else {
		s := uiaQI(t, hit, iidRawElementProviderSimple)
		if s != field {
			t.Errorf("the point (30, 80) px is not on the field")
		}
		comObject(s).release()
		comObject(hit).release()
	}
	comObject(fr).release()

	// Patterns: Invoke on the button queues the default action.
	if pattern(t, button, uiaTogglePatternID) != 0 {
		t.Error("the button has a Toggle pattern")
	}
	inv := pattern(t, button, uiaInvokePatternID)
	if inv == 0 {
		t.Fatal("no Invoke pattern on the button")
	}
	if hr := uiaCall(inv, 3); hr != sOK {
		t.Errorf("Invoke = %#x", hr)
	}
	comObject(inv).release()
	tog := pattern(t, checkbox, uiaTogglePatternID)
	var state int32 = -1
	uiaCall(tog, 4, uintptr(unsafe.Pointer(&state)))
	if state != uiaToggleOff {
		t.Errorf("ToggleState = %d", state)
	}
	uiaCall(tog, 3)
	comObject(tog).release()
	val := pattern(t, field, uiaValuePatternID)
	var bs uintptr
	uiaCall(val, 4, uintptr(unsafe.Pointer(&bs)))
	if s := bstrString(bs); s != "ana@example.com" {
		t.Errorf("Value = %q", s)
	}
	procSysFreeString.Call(bs)
	newValue := utf16("bob@example.com")
	if hr := uiaCall(val, 3, uintptr(unsafe.Pointer(newValue))); hr != sOK {
		t.Errorf("SetValue = %#x", hr)
	}
	comObject(val).release()
	acts := b.takeActions()
	want := []platform.AccessActionEvent{
		{Node: 8, Action: platform.AccessActionDefault},
		{Node: 6, Action: platform.AccessActionDefault},
		{Node: 5, Action: platform.AccessActionSetValue, Value: "bob@example.com"},
	}
	if !slices.Equal(acts, want) {
		t.Errorf("actions %+v, want %+v", acts, want)
	}

	// Text: the document, the selection (a caret after "ana"), a word.
	text := pattern(t, field, uiaTextPatternID)
	if text == 0 {
		t.Fatal("no Text pattern on the field")
	}
	defer comObject(text).release()
	var doc uintptr
	uiaCall(text, 7, uintptr(unsafe.Pointer(&doc)))
	rangeText := func(r uintptr) string {
		var s uintptr
		uiaCall(r, 12, ^uintptr(0), uintptr(unsafe.Pointer(&s)))
		defer procSysFreeString.Call(s)
		return bstrString(s)
	}
	if s := rangeText(doc); s != "ana@example.com" {
		t.Errorf("document text %q", s)
	}
	var moved int32
	uiaCall(doc, 6, uiaUnitWord) // ExpandToEnclosingUnit: the first word
	if s := rangeText(doc); s != "ana" {
		t.Errorf("first word %q", s)
	}
	uiaCall(doc, 13, uiaUnitWord, 2, uintptr(unsafe.Pointer(&moved))) // Move +2 words: "@", "example"
	if s := rangeText(doc); s != "example" || moved != 2 {
		t.Errorf("after moving two words: %q, moved %d", s, moved)
	}
	comObject(doc).release()
	var sel uintptr
	uiaCall(text, 3, uintptr(unsafe.Pointer(&sel)))
	if n := safeArrayLen(sel); n != 1 {
		t.Fatalf("selection has %d ranges", n)
	}
	var caret uintptr
	safeArrayAt(sel, 0, unsafe.Pointer(&caret))
	destroySafeArray(sel)
	if s := rangeText(caret); s != "" {
		t.Errorf("caret range text %q", s)
	}
	uiaCall(caret, 14, uiaTextPatternRangeEndpointEnd, uiaUnitCharacter, 4, uintptr(unsafe.Pointer(&moved)))
	if s := rangeText(caret); s != "@exa" {
		t.Errorf("caret extended by four: %q", s)
	}
	uiaCall(caret, 16) // Select
	if acts := b.takeActions(); !slices.Equal(acts, []platform.AccessActionEvent{
		{Node: 5, Action: platform.AccessActionSetSelection, Selection: platform.AccessSelection{Anchor: 3, Focus: 7}}}) {
		t.Errorf("Select queued %+v", acts)
	}

	// The button goes: its object answers UIA_E_ELEMENTNOTAVAILABLE, and
	// so do ranges of a node that is gone.
	u2 := loginTree()
	u2.Nodes = u2.Nodes[1:2]
	u2.Nodes[0].Children = []platform.AccessID{3, 5, 6}
	b.apply(&u2)
	v := new(variant)
	if hr := uiaCall(button, 5, uiaNamePropertyID, uintptr(unsafe.Pointer(v))); hr != uiaEElementNotAvailable {
		t.Errorf("a removed node's GetPropertyValue = %#x", hr)
	}
	if c := navigate(t, root, uiaNavigateLastChild); c != checkbox {
		t.Error("the checkbox is not the root's last child after the removal")
	} else {
		comObject(c).release()
	}
	comObject(caret).release()
	if _, ok := b.nodes[8]; ok {
		t.Error("the cache keeps the removed button")
	}
}
