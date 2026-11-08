//go:build windows

package win32

import (
	"sync"
	unicode16 "unicode/utf16"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// The Text pattern (ADR-246): ITextProvider on the nodes uiaHasText
// accepts (text fields and texts) and ITextRangeProvider objects over
// their text, with the units of uiatext.go. A range names its node by ID
// and reads the node's text from the current snapshot at every call
// (offsets are clamped when the text got shorter); once the node is gone
// it answers UIA_E_ELEMENTNOTAVAILABLE. Approximate, from the flat tree:
// bounding rectangles are the node's (a degenerate range gets a
// zero-width rectangle at the caret's share of the width), RangeFromPoint
// returns the start, the only attributes are IsReadOnly and CaretPosition
// (every other one is not supported), and FindAttribute finds nothing.

// The text attributes the ranges answer (UIAutomationClient.h).
const (
	uiaIsReadOnlyAttributeID    = 40015
	uiaCaretPositionAttributeID = 40038
)

// uiaRange is a text range object.
type uiaRange struct {
	b   *uiaBridge
	id  platform.AccessID
	srv *comServer

	mu sync.Mutex
	r  uiaTextRange
}

// newRange creates a range of node id with one reference.
func (b *uiaBridge) newRange(id platform.AccessID, r uiaTextRange) *uiaRange {
	tr := &uiaRange{b: b, id: id, r: r}
	tr.srv = newComServerOf([]comIface{{&uiaRangeVtbl[0], []guid{iidTextRangeProvider}}}, tr, nil)
	return tr
}

// uiaRangeOf returns the range behind an interface pointer, or nil.
func uiaRangeOf(this uintptr) *uiaRange {
	s := comServerOf(this)
	if s == nil {
		return nil
	}
	r, _ := s.impl.(*uiaRange)
	return r
}

// textOf returns the UTF-16 text of the node of a text pattern.
func textOf(n *platform.AccessNode) []uint16 { return unicode16.Encode([]rune(uiaText(n))) }

// resolveRange returns the range behind this with its node and text.
func resolveRange(this uintptr) (tr *uiaRange, n *platform.AccessNode, t *uiaUnitText, hr uintptr) {
	tr = uiaRangeOf(this)
	if tr == nil || tr.b.closed.Load() {
		return nil, nil, nil, uiaEElementNotAvailable
	}
	n, ok := tr.b.snap.Load().node(tr.id)
	if !ok || !uiaHasText(n) {
		return nil, nil, nil, uiaEElementNotAvailable
	}
	return tr, n, &uiaUnitText{text: textOf(n)}, sOK
}

// get and put read and write the range, clamped to t.
func (tr *uiaRange) get(t *uiaUnitText) uiaTextRange {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.r.clamp(len(t.text))
}

func (tr *uiaRange) put(r uiaTextRange) {
	tr.mu.Lock()
	tr.r = r
	tr.mu.Unlock()
}

// selectionRange returns the selection of a text field as a range.
func selectionRange(n *platform.AccessNode) uiaTextRange {
	v := uiaText(n)
	a, f := uiaCodePointsToUTF16(v, n.Selection.Anchor), uiaCodePointsToUTF16(v, n.Selection.Focus)
	return uiaTextRange{min(a, f), max(a, f)}
}

// rangeOut stores a new range of node id in *ret.
func (b *uiaBridge) rangeOut(ret uintptr, id platform.AccessID, r uiaTextRange) uintptr {
	*(*uintptr)(ptrFromUintptr(ret)) = b.newRange(id, r).srv.ptr()
	return sOK
}

// rangeArray returns a SAFEARRAY of new ranges of node id.
func (b *uiaBridge) rangeArray(id platform.AccessID, rs ...uiaTextRange) uintptr {
	objs := make([]*uiaRange, len(rs))
	for i, r := range rs {
		objs[i] = b.newRange(id, r)
	}
	sa := safeArray(vtUnknown, len(objs), func(i int) uintptr { return objs[i].srv.ptr() })
	for _, o := range objs {
		o.srv.release() // the array holds them
	}
	return sa
}

// --- ITextProvider ---

// ITextProvider::GetSelection: the field's selection, or none.
func uiaTextGetSelection(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, _, n, hr := resolveFor(this, uiaTextPatternID)
	if hr != sOK {
		return hr
	}
	if !n.HasSelection {
		*(*uintptr)(ptrFromUintptr(ret)) = u.b.rangeArray(n.ID)
		return sOK
	}
	*(*uintptr)(ptrFromUintptr(ret)) = u.b.rangeArray(n.ID, selectionRange(n))
	return sOK
}

// ITextProvider::GetVisibleRanges: the whole text (scroll offsets inside
// a field are not known).
func uiaTextGetVisibleRanges(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, _, n, hr := resolveFor(this, uiaTextPatternID)
	if hr != sOK {
		return hr
	}
	*(*uintptr)(ptrFromUintptr(ret)) = u.b.rangeArray(n.ID, uiaTextRange{0, len(textOf(n))})
	return sOK
}

// ITextProvider::RangeFromChild: the text has no embedded objects.
func uiaTextRangeFromChild(this, child, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	return eInvalidArg
}

// ITextProvider::RangeFromPoint (the point arrives through
// uiacall_windows_*.go): a degenerate range at the start, as the tree
// has no glyph positions.
func uiaTextRangeFromPoint(this uintptr, x, y float64, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, _, n, hr := resolveFor(this, uiaTextPatternID)
	if hr != sOK {
		return hr
	}
	return u.b.rangeOut(ret, n.ID, uiaTextRange{})
}

// ITextProvider::get_DocumentRange.
func uiaTextDocumentRange(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, _, n, hr := resolveFor(this, uiaTextPatternID)
	if hr != sOK {
		return hr
	}
	return u.b.rangeOut(ret, n.ID, uiaTextRange{0, len(textOf(n))})
}

// ITextProvider::get_SupportedTextSelection.
func uiaTextSupportedSelection(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setI32(ret, uiaSupportedTextSelectionNone)
	_, _, n, hr := resolveFor(this, uiaTextPatternID)
	if hr == sOK && n.HasSelection {
		setI32(ret, uiaSupportedTextSelectionSingle)
	}
	return hr
}

// --- ITextRangeProvider ---

// ITextRangeProvider::Clone.
func trClone(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	tr := uiaRangeOf(this)
	if tr == nil {
		return uiaEElementNotAvailable
	}
	tr.mu.Lock()
	r := tr.r
	tr.mu.Unlock()
	return tr.b.rangeOut(ret, tr.id, r)
}

// ITextRangeProvider::Compare.
func trCompare(this, other, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setBool(ret, false)
	tr, o := uiaRangeOf(this), uiaRangeOf(other)
	if tr == nil || o == nil {
		return eInvalidArg
	}
	a, b := tr.snapshot(), o.snapshot()
	setBool(ret, tr.b == o.b && tr.id == o.id && a == b)
	return sOK
}

// snapshot returns the range's offsets without a text (for comparisons
// that must work after the node is gone, as AccessKit allows).
func (tr *uiaRange) snapshot() uiaTextRange {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.r
}

// endpoint returns the offset of endpoint ep of r.
func endpoint(r uiaTextRange, ep uintptr) (int, bool) {
	switch int32(ep) {
	case uiaTextPatternRangeEndpointStart:
		return r.start, true
	case uiaTextPatternRangeEndpointEnd:
		return r.end, true
	}
	return 0, false
}

// ITextRangeProvider::CompareEndpoints.
func trCompareEndpoints(this, ep, other, otherEP, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setI32(ret, 0)
	tr, o := uiaRangeOf(this), uiaRangeOf(other)
	if tr == nil || o == nil || tr.b != o.b || tr.id != o.id {
		return eInvalidArg
	}
	a, ok1 := endpoint(tr.snapshot(), ep)
	b, ok2 := endpoint(o.snapshot(), otherEP)
	if !ok1 || !ok2 {
		return eInvalidArg
	}
	setI32(ret, int32(a-b))
	return sOK
}

// ITextRangeProvider::ExpandToEnclosingUnit.
func trExpandToEnclosingUnit(this, unit uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	tr, _, t, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	tr.put(t.expand(tr.get(t), int32(unit)))
	return sOK
}

// ITextRangeProvider::FindAttribute (the arguments arrive through
// uiacall_windows_*.go): the text has no formatting, so nothing.
func uiaFindAttribute(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	return sOK
}

// ITextRangeProvider::FindText.
func trFindText(this, text, backward, ignoreCase, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	tr, _, t, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	if found, ok := uiaFind(t.text, tr.get(t), bstrString(text), int32(backward) != 0, int32(ignoreCase) != 0); ok {
		return tr.b.rangeOut(ret, tr.id, found)
	}
	return sOK
}

// ITextRangeProvider::GetAttributeValue.
func trGetAttributeValue(this, attr, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	v := (*variant)(ptrFromUintptr(ret))
	*v = variant{}
	tr, n, t, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	switch int32(attr) {
	case uiaIsReadOnlyAttributeID:
		v.set(uiaB(n.Kind == platform.AccessText || uiaValueReadOnly(n)))
		return sOK
	case uiaCaretPositionAttributeID:
		r := tr.get(t)
		pos := int32(uiaCaretPositionUnknown)
		if r.start == r.end {
			pos = t.caretPosition(r.start)
		}
		v.set(uiaI(pos))
		return sOK
	}
	var ns uintptr
	procUiaGetReservedNotSupportedValue.Call(uintptr(unsafe.Pointer(&ns)))
	v.vt, v.val = vtUnknown, ns
	return sOK
}

// ITextRangeProvider::GetBoundingRectangles: the node's rectangle (a
// zero-width one at the caret's share of the width for a degenerate
// range), as the tree has no glyph positions.
func trGetBoundingRectangles(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	tr, n, t, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	b := n.Bounds
	if r := tr.get(t); r.start == r.end {
		if len(t.text) > 0 && !n.Has(platform.AccessMultiline) {
			b.X += b.W * float32(r.start) / float32(len(t.text))
		}
		b.W = 0
	}
	q := tr.b.screenRect(b)
	*(*uintptr)(ptrFromUintptr(ret)) = safeArrayR8(q[:])
	return sOK
}

// ITextRangeProvider::GetEnclosingElement: the node.
func trGetEnclosingElement(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	tr, n, _, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	return out(ret, tr.b.provider(n.ID), uiaIfSimple)
}

// ITextRangeProvider::GetText (maxLength -1: all).
func trGetText(this, maxLength, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	tr, _, t, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	r := tr.get(t)
	s := t.text[r.start:r.end]
	if m := int(int32(maxLength)); m >= 0 && m < len(s) {
		s = s[:m]
	}
	*(*uintptr)(ptrFromUintptr(ret)) = bstr(string(unicode16.Decode(s)))
	return sOK
}

// ITextRangeProvider::Move.
func trMove(this, unit, count, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setI32(ret, 0)
	tr, _, t, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	r, moved := t.move(tr.get(t), int32(unit), int32(count))
	tr.put(r)
	setI32(ret, moved)
	return sOK
}

// ITextRangeProvider::MoveEndpointByUnit.
func trMoveEndpointByUnit(this, ep, unit, count, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setI32(ret, 0)
	tr, _, t, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	if _, ok := endpoint(uiaTextRange{}, ep); !ok {
		return eInvalidArg
	}
	r, moved := t.moveEndpoint(tr.get(t), int32(ep) == uiaTextPatternRangeEndpointEnd, int32(unit), int32(count))
	tr.put(r)
	setI32(ret, moved)
	return sOK
}

// ITextRangeProvider::MoveEndpointByRange.
func trMoveEndpointByRange(this, ep, other, otherEP uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	tr, _, t, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	o := uiaRangeOf(other)
	if o == nil || o.b != tr.b || o.id != tr.id {
		return eInvalidArg
	}
	pos, ok := endpoint(o.get(t), otherEP)
	if _, ok2 := endpoint(uiaTextRange{}, ep); !ok || !ok2 {
		return eInvalidArg
	}
	tr.put(tr.get(t).setEndpoint(int32(ep) == uiaTextPatternRangeEndpointEnd, pos))
	return sOK
}

// ITextRangeProvider::Select: the field's selection becomes the range.
func trSelect(this uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	tr, n, t, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	if !n.Actions.Has(platform.AccessActionSetSelection) {
		return uiaEInvalidOperation
	}
	r := tr.get(t)
	sel := platform.AccessSelection{Anchor: uiaUTF16ToCodePoints(t.text, r.start), Focus: uiaUTF16ToCodePoints(t.text, r.end)}
	tr.b.post(platform.AccessActionEvent{Node: n.ID, Action: platform.AccessActionSetSelection, Selection: sel})
	return sOK
}

// ITextRangeProvider::AddToSelection and RemoveFromSelection: one
// selection only.
func trNoMultipleSelection(this uintptr) uintptr { return uiaEInvalidOperation }

// ITextRangeProvider::ScrollIntoView: the node scrolls into view.
func trScrollIntoView(this, alignToTop uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	tr, n, _, hr := resolveRange(this)
	if hr != sOK {
		return hr
	}
	if n.Actions.Has(platform.AccessActionScrollIntoView) {
		tr.b.post(platform.AccessActionEvent{Node: n.ID, Action: platform.AccessActionScrollIntoView})
	}
	return sOK
}

// ITextRangeProvider::GetChildren: no embedded objects.
func trGetChildren(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = safeArray(vtUnknown, 0, nil)
	return sOK
}
