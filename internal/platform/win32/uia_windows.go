//go:build windows

package win32

import (
	"image"
	"math"
	"sync"
	"sync/atomic"
	"syscall"
	unicode16 "unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform"
)

// The UI Automation bridge (platform contract v5, ADR-246): UIA providers
// implemented as Go COM servers (comserver_windows.go), after AccessKit's
// Windows adapter. uia.go holds the OS-independent rules (roles,
// properties, patterns, events), uiatext.go the text units.
//
//   - Activation. Nothing of this file runs, and uiautomationcore.dll is
//     not loaded, until a client sends the window WM_GETOBJECT for
//     UiaRootObjectId (Narrator, NVDA, Inspect, a UIA client in the
//     process). The first one creates the window's bridge, queues
//     AccessibilityEvent{Active: true} and answers with the root
//     provider (UiaReturnRawElementProvider); until core's first update
//     arrives (after the next delivery pass) the root has no children and
//     the window's host provider describes it. A nested WM_GETOBJECT,
//     which UIA's first load can send from inside UiaReturnRawElementProvider
//     (AccessKit issue #37), gets the same root: nothing here holds a lock
//     across that call. Other object IDs (MSAA's OBJID_CLIENT) get
//     DefWindowProc's default, so MSAA-only clients see a plain client
//     area. The bridge never deactivates: UIA cannot tell when its last
//     client left (UiaClientsAreListening covers event listeners only, and
//     a client that only queries listens to nothing), so it reports
//     Active true once and keeps it, as the contract allows for such
//     backends; events are raised only while UiaClientsAreListening.
//   - Threading. The providers are free-threaded: ProviderOptions is
//     ServerSideProvider without UseComThreading, so UIA calls them on its
//     own threads (MTA workers for other processes' clients, the client's
//     thread for one in this process) and never through the UI thread's
//     message loop. They answer from the latest uiaSnapshot, an immutable
//     value held in an atomic pointer that SetAccessibility replaces on the
//     UI thread, and from atomics for the window's scale and hidden band
//     (ClientToScreen is safe from any thread). A query never waits for
//     the UI thread, so a busy or blocked UI thread cannot deadlock a
//     screen reader. Syscall callbacks may run on threads Go did not
//     create (the runtime supports it on Windows); a panic in one is
//     recovered and answered with E_FAIL, since it cannot unwind through
//     UIA's frames.
//   - Providers. The root provider (fragment root) stands for the root of
//     whatever tree is current; its host provider (UiaHostProviderFromHwnd)
//     gives the window's own properties (name from the title, control
//     type Window, bounds, process), so the root reports only Tela's
//     framework ID. Every exposed node (uiaSnapshot: not Ignored, not
//     under a role whose children are presentational) gets one COM
//     object, created when UIA first reaches it and cached by ID; the
//     object answers IRawElementProviderSimple, IRawElementProviderFragment
//     and every pattern interface (GetPatternProvider returns the pattern's
//     interface of the same object when the node supports it now). A node
//     that is no longer exposed loses its object: the cache drops it, it
//     is marked dead, and every later call on it (UIA may hold it for a
//     while) returns UIA_E_ELEMENTNOTAVAILABLE. Creation and removal hold
//     the cache's mutex together with the snapshot swap, so an object is
//     never created for a node the current snapshot does not expose.
//   - Actions. Pattern calls (Invoke, Toggle, SetValue, Expand, Select,
//     ScrollIntoView, SetFocus, text Select...) check what the node
//     supports, queue a platform.AccessActionEvent and post wmTelaAccess
//     to the window; the UI thread queues the events for the next delivery
//     pass. The call returns at once (S_OK means "requested"): running the
//     app inside a UIA call would re-enter the handler from a foreign
//     thread. The outcome reaches UIA as the next update's events.
//   - Events. Each update is applied to the bridge's platform.AccessTree,
//     a new uiaSnapshot made, and the differences raised (uiaEvents) on the
//     UI thread, without locks held (UIA may call providers back from
//     inside a raise): focus changes, property changes (name, description,
//     help text, enabled, toggle, expand/collapse, value, range value,
//     selection), structure changes (children invalidated), text and text
//     selection changes, menus opened and closed, dialogs opened, a named
//     live region's name, and core's announcements as notification events
//     (UiaRaiseNotificationEvent: assertive ImportantMostRecent, polite
//     All, on the root).
//   - Coordinates. Bounds are logical pixels of the content area; UIA
//     wants physical screen pixels: the client origin (below the hidden
//     band of a maximized title-bar-overlay window) plus bounds times the
//     window's scale.

// IIDs of the provider interfaces (UIAutomationCore.h).
var (
	iidRawElementProviderSimple       = guid{0xd6dd68d1, 0x86fd, 0x4332, [8]byte{0x86, 0x66, 0x9a, 0xbe, 0xde, 0xa2, 0xd2, 0x4c}}
	iidRawElementProviderFragment     = guid{0xf7063da8, 0x8359, 0x439c, [8]byte{0x92, 0x97, 0xbb, 0xc5, 0x29, 0x9a, 0x7d, 0x87}}
	iidRawElementProviderFragmentRoot = guid{0x620ce2a5, 0xab8f, 0x40a9, [8]byte{0x86, 0xcb, 0xde, 0x3c, 0x75, 0x59, 0x9b, 0x58}}
	iidInvokeProvider                 = guid{0x54fcb24b, 0xe18e, 0x47a2, [8]byte{0xb4, 0xd3, 0xec, 0xcb, 0xe7, 0x75, 0x99, 0xa2}}
	iidToggleProvider                 = guid{0x56d00bd0, 0xc4f4, 0x433c, [8]byte{0xa8, 0x36, 0x1a, 0x52, 0xa5, 0x7e, 0x08, 0x92}}
	iidValueProvider                  = guid{0xc7935180, 0x6fb3, 0x4201, [8]byte{0xb1, 0x74, 0x7d, 0xf7, 0x3a, 0xdb, 0xf6, 0x4a}}
	iidRangeValueProvider             = guid{0x36dc7aef, 0x33e6, 0x4691, [8]byte{0xaf, 0xe1, 0x2b, 0xe7, 0x27, 0x4b, 0x3d, 0x33}}
	iidExpandCollapseProvider         = guid{0xd847d3a5, 0xcab0, 0x4a98, [8]byte{0x8c, 0x32, 0xec, 0xb4, 0x5c, 0x59, 0xad, 0x24}}
	iidSelectionItemProvider          = guid{0x2acad808, 0xb2d4, 0x452d, [8]byte{0xa4, 0x07, 0x91, 0xff, 0x1a, 0xd1, 0x67, 0xb2}}
	iidSelectionProvider              = guid{0xfb8b03af, 0x3bdf, 0x48d4, [8]byte{0xbd, 0x36, 0x1a, 0x65, 0x79, 0x3b, 0xe1, 0x68}}
	iidScrollItemProvider             = guid{0x2360c714, 0x4bf1, 0x4b26, [8]byte{0xba, 0x65, 0x9b, 0x21, 0x31, 0x61, 0x27, 0xeb}}
	iidScrollProvider                 = guid{0xb38b8077, 0x1fc3, 0x42a5, [8]byte{0x8c, 0xae, 0xd4, 0x0c, 0x22, 0x15, 0x05, 0x5a}}
	iidTextProvider                   = guid{0x3589c92c, 0x63f3, 0x4367, [8]byte{0x99, 0xbb, 0xad, 0xa6, 0x53, 0xb7, 0x7c, 0xf2}}
	iidTextRangeProvider              = guid{0x5347ad7b, 0xc355, 0x46f8, [8]byte{0xaf, 0xf5, 0x90, 0x90, 0x33, 0x58, 0x2f, 0x63}}
)

// HRESULTs of the providers (UIAutomationCoreApi.h, winerror.h).
const (
	uiaEElementNotAvailable = 0x80040201
	uiaEElementNotEnabled   = 0x80040200
	uiaEInvalidOperation    = 0x80131509
	eFail                   = 0x80004005

	// uiaRootObjectID is UiaRootObjectId, the WM_GETOBJECT object ID of
	// UIA's root.
	uiaRootObjectID = -25
	// uiaAppendRuntimeID starts a runtime ID that UIA completes with the
	// fragment root's; uiaHwndRuntimeIDBase starts a window's own.
	uiaAppendRuntimeID   = 3
	uiaHwndRuntimeIDBase = 42

	// wmTelaAccess, posted to a window: accessibility actions are queued.
	wmTelaAccess = 0x8000 + 0x70
)

// VARIANT types.
const (
	vtEmpty   = 0
	vtI4      = 3
	vtR8      = 5
	vtBSTR    = 8
	vtBool    = 11
	vtUnknown = 13
	vtArray   = 0x2000
)

// variant is VARIANT: 16 bytes on 32-bit Windows, 24 on 64-bit.
type variant struct {
	vt        uint16
	_         [3]uint16
	val, val2 uintptr
}

// set stores x (an owned BSTR for a string).
func (v *variant) set(x uiaVal) {
	*v = variant{}
	switch x.kind {
	case uiaBool:
		v.vt = vtBool
		if x.b {
			v.val = 0xFFFF // VARIANT_TRUE
		}
	case uiaInt:
		v.vt = vtI4
		*(*int32)(unsafe.Pointer(&v.val)) = x.i
	case uiaFloat:
		v.vt = vtR8
		*(*float64)(unsafe.Pointer(&v.val)) = x.f
	case uiaString:
		v.vt, v.val = vtBSTR, bstr(x.s)
	}
}

// clear frees what v owns.
func (v *variant) clear() { procVariantClear.Call(uintptr(unsafe.Pointer(v))) }

// bstr allocates a BSTR of s; the receiver frees it.
func bstr(s string) uintptr {
	u := unicode16.Encode([]rune(s))
	r, _, _ := procSysAllocStringLen.Call(uintptr(unsafe.Pointer(unsafe.SliceData(u))), uintptr(len(u)))
	return r
}

// A note on Go memory passed to the OS: a pointer becomes a uintptr only
// inside the argument list of a //go:uintptrescapes call (LazyProc.Call,
// comObject.call), which keeps its object on the heap. A uintptr made
// earlier may point into a goroutine stack that moves when it grows before
// the call (the callback that runs this code may be deep in a stack).

// bstrString returns the text of a BSTR.
func bstrString(p uintptr) string {
	if p == 0 {
		return ""
	}
	n, _, _ := procSysStringLen.Call(p)
	return string(unicode16.Decode(unsafe.Slice((*uint16)(ptrFromUintptr(p)), int(n))))
}

// newSafeArray returns an empty-valued SAFEARRAY of vt with n elements, or
// 0.
func newSafeArray(vt uint16, n int) uintptr {
	sa, _, _ := procSafeArrayCreateVector.Call(uintptr(vt), 0, uintptr(n))
	return sa
}

// safeArray returns a SAFEARRAY of VT_UNKNOWN with n elements, the
// interface pointers elem(i) (SafeArrayPutElement takes a reference).
func safeArray(vt uint16, n int, elem func(i int) uintptr) uintptr {
	sa := newSafeArray(vt, n)
	if sa == 0 {
		return 0
	}
	for i := range n {
		idx := int32(i)
		procSafeArrayPutElement.Call(sa, uintptr(unsafe.Pointer(&idx)), elem(i))
	}
	return sa
}

func safeArrayI4(vals []int32) uintptr {
	sa := newSafeArray(vtI4, len(vals))
	for i := range vals {
		if sa == 0 {
			break
		}
		idx := int32(i)
		procSafeArrayPutElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&vals[i])))
	}
	return sa
}

func safeArrayR8(vals []float64) uintptr {
	sa := newSafeArray(vtR8, len(vals))
	for i := range vals {
		if sa == 0 {
			break
		}
		idx := int32(i)
		procSafeArrayPutElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&vals[i])))
	}
	return sa
}

// safeArrayProviders returns a SAFEARRAY of the IRawElementProviderSimple
// of each node (SafeArrayPutElement takes its own references) and
// releases the nodes' references.
func safeArrayProviders(nodes []*uiaNode) uintptr {
	defer func() {
		for _, n := range nodes {
			n.srv.release()
		}
	}()
	return safeArray(vtUnknown, len(nodes), func(i int) uintptr { return nodes[i].srv.iface(uiaIfSimple) })
}

// The interfaces of a node's object, in header order.
const (
	uiaIfSimple = iota
	uiaIfFragment
	uiaIfRoot // the fragment root only
	uiaIfInvoke
	uiaIfToggle
	uiaIfValue
	uiaIfRangeValue
	uiaIfExpandCollapse
	uiaIfSelectionItem
	uiaIfSelection
	uiaIfScrollItem
	uiaIfScroll
	uiaIfText
	uiaIfCount
)

// uiaPatternIface maps a pattern ID to its interface.
var uiaPatternIface = map[int32]int{
	uiaInvokePatternID:         uiaIfInvoke,
	uiaTogglePatternID:         uiaIfToggle,
	uiaValuePatternID:          uiaIfValue,
	uiaRangeValuePatternID:     uiaIfRangeValue,
	uiaExpandCollapsePatternID: uiaIfExpandCollapse,
	uiaSelectionItemPatternID:  uiaIfSelectionItem,
	uiaSelectionPatternID:      uiaIfSelection,
	uiaScrollItemPatternID:     uiaIfScrollItem,
	uiaScrollPatternID:         uiaIfScroll,
	uiaTextPatternID:           uiaIfText,
}

// The vtables, created once per process (like comserver_windows.go's).
var (
	uiaVtblOnce          sync.Once
	uiaSimpleVtbl        [7]uintptr
	uiaFragmentVtbl      [9]uintptr
	uiaRootVtbl          [5]uintptr
	uiaInvokeVtbl        [4]uintptr
	uiaToggleVtbl        [5]uintptr
	uiaValueVtbl         [6]uintptr
	uiaRangeValueVtbl    [10]uintptr
	uiaExpandVtbl        [6]uintptr
	uiaSelectionItemVtbl [8]uintptr
	uiaSelectionVtbl     [6]uintptr
	uiaScrollItemVtbl    [4]uintptr
	uiaScrollVtbl        [11]uintptr
	uiaTextVtbl          [9]uintptr
	uiaRangeVtbl         [21]uintptr

	// The interface lists of the root's object and of an element's.
	uiaRootIfaces, uiaElementIfaces []comIface
)

// uiaInitVtables creates the callbacks, once per process.
func uiaInitVtables() {
	uiaVtblOnce.Do(func() {
		initComVtables()
		qi, addRef, release := dropTargetVtbl[0], dropTargetVtbl[1], dropTargetVtbl[2]
		cb := syscall.NewCallback
		fromPoint, rangeSetValue, setScrollPercent, textFromPoint, findAttribute := uiaArchCallbacks()
		uiaSimpleVtbl = [7]uintptr{qi, addRef, release,
			cb(uiaProviderOptions), cb(uiaGetPatternProvider), cb(uiaGetPropertyValue), cb(uiaHostRawElementProvider)}
		uiaFragmentVtbl = [9]uintptr{qi, addRef, release,
			cb(uiaNavigate), cb(uiaGetRuntimeID), cb(uiaBoundingRectangle), cb(uiaGetEmbeddedFragmentRoots),
			cb(uiaSetFocus), cb(uiaFragmentRoot)}
		uiaRootVtbl = [5]uintptr{qi, addRef, release, fromPoint, cb(uiaGetFocus)}
		uiaInvokeVtbl = [4]uintptr{qi, addRef, release, cb(uiaInvoke)}
		uiaToggleVtbl = [5]uintptr{qi, addRef, release, cb(uiaToggle), cb(uiaToggleStateGet)}
		uiaValueVtbl = [6]uintptr{qi, addRef, release, cb(uiaValueSetValue), cb(uiaValueGet), cb(uiaValueIsReadOnly)}
		uiaRangeValueVtbl = [10]uintptr{qi, addRef, release, rangeSetValue,
			cb(uiaRangeGet(func(n *platform.AccessNode) float64 { return n.Range.Now })),
			cb(uiaRangeIsReadOnly),
			cb(uiaRangeGet(func(n *platform.AccessNode) float64 {
				if n.Range.HasMax {
					return n.Range.Max
				}
				return max(n.Range.Now, 100)
			})),
			cb(uiaRangeGet(func(n *platform.AccessNode) float64 {
				if n.Range.HasMin {
					return n.Range.Min
				}
				return min(n.Range.Now, 0)
			})),
			cb(uiaRangeGet(func(*platform.AccessNode) float64 { return 0 })), // LargeChange: not known
			cb(uiaRangeGet(func(*platform.AccessNode) float64 { return 0 })), // SmallChange: not known
		}
		uiaExpandVtbl = [6]uintptr{qi, addRef, release, cb(uiaExpand), cb(uiaCollapse), cb(uiaExpandStateGet)}
		uiaSelectionItemVtbl = [8]uintptr{qi, addRef, release, cb(uiaSelect), cb(uiaSelect), cb(uiaUnselect),
			cb(uiaIsSelectedGet), cb(uiaSelectionContainer)}
		uiaSelectionVtbl = [6]uintptr{qi, addRef, release, cb(uiaGetSelection), cb(uiaCanSelectMultiple), cb(uiaIsSelectionRequired)}
		uiaScrollItemVtbl = [4]uintptr{qi, addRef, release, cb(uiaScrollIntoView)}
		uiaScrollVtbl = [11]uintptr{qi, addRef, release, cb(uiaScroll), setScrollPercent,
			cb(uiaScrollGet(false, true)), cb(uiaScrollGet(true, true)), // horizontal, vertical percent
			cb(uiaScrollGet(false, false)), cb(uiaScrollGet(true, false)), // view sizes
			cb(uiaScrollable(false)), cb(uiaScrollable(true))}
		uiaTextVtbl = [9]uintptr{qi, addRef, release, cb(uiaTextGetSelection), cb(uiaTextGetVisibleRanges),
			cb(uiaTextRangeFromChild), textFromPoint, cb(uiaTextDocumentRange), cb(uiaTextSupportedSelection)}
		uiaRangeVtbl = [21]uintptr{qi, addRef, release,
			cb(trClone), cb(trCompare), cb(trCompareEndpoints), cb(trExpandToEnclosingUnit), findAttribute,
			cb(trFindText), cb(trGetAttributeValue), cb(trGetBoundingRectangles), cb(trGetEnclosingElement),
			cb(trGetText), cb(trMove), cb(trMoveEndpointByUnit), cb(trMoveEndpointByRange), cb(trSelect),
			cb(trNoMultipleSelection), cb(trNoMultipleSelection), cb(trScrollIntoView), cb(trGetChildren)}

		ifaces := func(root bool) []comIface {
			f := []comIface{
				uiaIfSimple:         {&uiaSimpleVtbl[0], []guid{iidRawElementProviderSimple}},
				uiaIfFragment:       {&uiaFragmentVtbl[0], []guid{iidRawElementProviderFragment}},
				uiaIfRoot:           {nil, nil},
				uiaIfInvoke:         {&uiaInvokeVtbl[0], []guid{iidInvokeProvider}},
				uiaIfToggle:         {&uiaToggleVtbl[0], []guid{iidToggleProvider}},
				uiaIfValue:          {&uiaValueVtbl[0], []guid{iidValueProvider}},
				uiaIfRangeValue:     {&uiaRangeValueVtbl[0], []guid{iidRangeValueProvider}},
				uiaIfExpandCollapse: {&uiaExpandVtbl[0], []guid{iidExpandCollapseProvider}},
				uiaIfSelectionItem:  {&uiaSelectionItemVtbl[0], []guid{iidSelectionItemProvider}},
				uiaIfSelection:      {&uiaSelectionVtbl[0], []guid{iidSelectionProvider}},
				uiaIfScrollItem:     {&uiaScrollItemVtbl[0], []guid{iidScrollItemProvider}},
				uiaIfScroll:         {&uiaScrollVtbl[0], []guid{iidScrollProvider}},
				uiaIfText:           {&uiaTextVtbl[0], []guid{iidTextProvider}},
			}
			if root {
				f[uiaIfRoot] = comIface{&uiaRootVtbl[0], []guid{iidRawElementProviderFragmentRoot}}
			}
			return f
		}
		uiaRootIfaces, uiaElementIfaces = ifaces(true), ifaces(false)
	})
}

var (
	uiaAvailOnce sync.Once
	uiaAvail     bool
)

// uiaAvailable reports whether the system has UI Automation's provider
// API (Windows 7 and later do); it loads the two DLLs.
func uiaAvailable() bool {
	uiaAvailOnce.Do(func() {
		uiaAvail = available(procUiaReturnRawElementProvider) && available(procUiaHostProviderFromHwnd) &&
			available(procSysAllocStringLen) && available(procSafeArrayCreateVector)
	})
	return uiaAvail
}

// --- the bridge ---

// uiaBridge is a window's UI Automation bridge, created at the first
// WM_GETOBJECT for the UIA root.
type uiaBridge struct {
	hwnd   uintptr
	snap   atomic.Pointer[uiaSnapshot]
	scale  atomic.Uint64 // float64 bits: the window's scale
	top    atomic.Int32  // the hidden band above the content (overlay.go)
	closed atomic.Bool
	root   *uiaNode // the fragment root: the current tree's root

	// mu guards nodes, and orders object creation with snapshot swaps.
	mu    sync.Mutex
	nodes map[platform.AccessID]*uiaNode

	// tree is the tree as applied (UI thread).
	tree *platform.AccessTree

	// The actions waiting for the UI thread.
	actMu   sync.Mutex
	actions []platform.AccessActionEvent
	posted  bool
}

// uiaNode is the COM object of an exposed node, or the root's (root set:
// it stands for the current root, whatever its ID).
type uiaNode struct {
	b    *uiaBridge
	id   platform.AccessID
	root bool
	dead atomic.Bool
	srv  *comServer
}

func newUIABridge(hwnd uintptr, scale float64, top int) *uiaBridge {
	uiaInitVtables()
	b := &uiaBridge{hwnd: hwnd, nodes: map[platform.AccessID]*uiaNode{}}
	b.snap.Store(newUIASnapshot(nil))
	b.setGeometry(scale, top)
	b.root = b.newNode(0, true)
	return b
}

// newNode creates the object of node id with one reference.
func (b *uiaBridge) newNode(id platform.AccessID, root bool) *uiaNode {
	u := &uiaNode{b: b, id: id, root: root}
	ifs := uiaElementIfaces
	if root {
		ifs = uiaRootIfaces
	}
	u.srv = newComServerOf(ifs, u, nil)
	return u
}

// setGeometry records the window's scale and hidden band (UI thread).
func (b *uiaBridge) setGeometry(scale float64, top int) {
	b.scale.Store(math.Float64bits(scale))
	b.top.Store(int32(top))
}

// provider returns the object of node id with a reference for the
// caller, or nil when the current snapshot does not expose it. Any thread.
func (b *uiaBridge) provider(id platform.AccessID) *uiaNode {
	if id == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed.Load() {
		return nil
	}
	s := b.snap.Load()
	if id == s.root() {
		b.root.srv.addRef()
		return b.root
	}
	if _, ok := s.node(id); !ok {
		return nil
	}
	u := b.nodes[id]
	if u == nil {
		u = b.newNode(id, false)
		b.nodes[id] = u // the cache's reference
	}
	u.srv.addRef()
	return u
}

// out stores the interface i of u in *ret, handing u's reference to the
// caller (nil stores NULL).
func out(ret uintptr, u *uiaNode, i int) uintptr {
	if u == nil {
		*(*uintptr)(ptrFromUintptr(ret)) = 0
		return sOK
	}
	*(*uintptr)(ptrFromUintptr(ret)) = u.srv.iface(i)
	return sOK
}

// apply applies update u (UI thread) and raises its events.
func (b *uiaBridge) apply(u *platform.AccessUpdate) {
	prev := b.tree
	tree, removed, err := prev.Apply(u)
	if err != nil {
		panic(err) // core's updates are valid by contract; headless panics too
	}
	b.tree = tree
	// The snapshot follows the update's changes when the last one is of
	// the tree it was applied to (ADR-277).
	old := b.snap.Load()
	var cur *uiaSnapshot
	if old != nil && prev != nil && old.tree == prev && prev.Root() == tree.Root() {
		changed := make([]platform.AccessID, len(u.Nodes))
		for i := range u.Nodes {
			changed[i] = u.Nodes[i].ID
		}
		cur = updateUIASnapshot(old, tree, changed, removed)
	} else {
		cur = newUIASnapshot(tree)
	}
	b.mu.Lock()
	b.snap.Store(cur)
	var gone map[platform.AccessID]*uiaNode
	kill := func(id platform.AccessID, n *uiaNode) {
		n.dead.Store(true)
		delete(b.nodes, id)
		if gone == nil {
			gone = map[platform.AccessID]*uiaNode{}
		}
		gone[id] = n
	}
	if cur.full {
		for id, n := range b.nodes {
			if !cur.exposed(id) {
				kill(id, n)
			}
		}
	} else {
		for _, id := range cur.lost {
			if n, ok := b.nodes[id]; ok {
				kill(id, n)
			}
		}
	}
	b.mu.Unlock()
	if uiaListening() {
		b.raise(uiaEvents(old, cur, u), gone)
	}
	for _, n := range gone {
		n.srv.release()
	}
}

// uiaListening reports UiaClientsAreListening.
func uiaListening() bool {
	r, _, _ := procUiaClientsAreListening.Call()
	return r != 0
}

// raise raises events (UI thread, no lock held). gone holds the objects
// of the nodes the update removed, for the events about them (a menu
// closed).
func (b *uiaBridge) raise(events []uiaEvent, gone map[platform.AccessID]*uiaNode) {
	for _, e := range events {
		u := b.provider(e.node)
		if u == nil {
			if g := gone[e.node]; g != nil {
				g.srv.addRef()
				u = g
			} else {
				u = b.newNode(e.node, false)
				u.dead.Store(true)
			}
		}
		p := u.srv.iface(uiaIfSimple)
		switch e.kind {
		case uiaEventAutomation:
			procUiaRaiseAutomationEvent.Call(p, uintptr(e.id))
		case uiaEventProperty:
			o, n := new(variant), new(variant)
			o.set(e.old)
			n.set(e.new)
			uiaRaisePropertyChanged(p, e.id, o, n)
			o.clear()
			n.clear()
		case uiaEventStructure:
			rid := b.runtimeID(u)
			procUiaRaiseStructureChangedEvent.Call(p, uiaStructureChildrenInvalidated, uintptr(unsafe.Pointer(&rid[0])), uintptr(len(rid)))
		case uiaEventNotification:
			if available(procUiaRaiseNotificationEvent) {
				activity := "Tela.Live.Polite"
				if e.id == uiaProcessingImportantMostRecent {
					activity = "Tela.Live.Assertive"
				}
				d, a := bstr(e.text), bstr(activity)
				procUiaRaiseNotificationEvent.Call(p, uiaNotificationKindOther, uintptr(e.id), d, a)
				procSysFreeString.Call(d)
				procSysFreeString.Call(a)
			}
		}
		u.srv.release()
	}
}

// runtimeID returns the runtime ID of u: the window's for the root,
// UiaAppendRuntimeId and the node ID's halves for an element.
func (b *uiaBridge) runtimeID(u *uiaNode) []int32 {
	if u.root {
		return []int32{uiaHwndRuntimeIDBase, int32(b.hwnd)}
	}
	return []int32{uiaAppendRuntimeID, int32(uint64(u.id) >> 32), int32(uint32(u.id))}
}

// post queues an action for the UI thread (any thread).
func (b *uiaBridge) post(e platform.AccessActionEvent) {
	b.actMu.Lock()
	b.actions = append(b.actions, e)
	need := !b.posted
	b.posted = true
	b.actMu.Unlock()
	if need && !postMessage(b.hwnd, wmTelaAccess, 0, 0) {
		b.actMu.Lock()
		b.posted = false
		b.actMu.Unlock()
	}
}

// takeActions returns the queued actions (UI thread).
func (b *uiaBridge) takeActions() []platform.AccessActionEvent {
	b.actMu.Lock()
	defer b.actMu.Unlock()
	a := b.actions
	b.actions, b.posted = nil, false
	return a
}

// close disconnects the bridge from UIA (the window is going).
func (b *uiaBridge) close() {
	b.mu.Lock()
	b.closed.Store(true)
	nodes := b.nodes
	b.nodes = nil
	b.snap.Store(newUIASnapshot(nil))
	b.mu.Unlock()
	b.root.dead.Store(true)
	for _, n := range nodes {
		n.dead.Store(true)
	}
	// Tells UIA the window has no provider any more (WM_DESTROY's rule).
	procUiaReturnRawElementProvider.Call(b.hwnd, 0, 0, 0)
	if available(procUiaDisconnectProvider) {
		procUiaDisconnectProvider.Call(b.root.srv.iface(uiaIfSimple))
	}
	for _, n := range nodes {
		n.srv.release()
	}
	b.root.srv.release()
	b.takeActions()
}

// screenRect converts a rectangle of the content (logical pixels) to
// physical screen pixels.
func (b *uiaBridge) screenRect(r platform.Rect) [4]float64 {
	s := math.Float64frombits(b.scale.Load())
	o := clientToScreen(b.hwnd, image.Pt(0, int(b.top.Load())))
	return [4]float64{float64(o.x) + float64(r.X)*s, float64(o.y) + float64(r.Y)*s, float64(r.W) * s, float64(r.H) * s}
}

// toContent converts a physical screen point to logical content
// coordinates.
func (b *uiaBridge) toContent(x, y float64) (float32, float32) {
	s := math.Float64frombits(b.scale.Load())
	if s <= 0 {
		s = 1
	}
	o := clientToScreen(b.hwnd, image.Pt(0, int(b.top.Load())))
	return float32((x - float64(o.x)) / s), float32((y - float64(o.y)) / s)
}

// --- the window's side (UI thread) ---

// onGetObject answers WM_GETOBJECT for the UIA root; ok is false for
// every other object ID (DefWindowProc answers them).
func (w *Window) onGetObject(wparam, lparam uintptr) (ret uintptr, ok bool) {
	// The object ID is a DWORD; some senders sign-extend it into the
	// 64-bit LPARAM and others do not (AccessKit's normalize_objid).
	if int32(uint32(lparam)) != uiaRootObjectID || w.Closed() || !uiaAvailable() {
		return 0, false
	}
	if w.uia == nil {
		w.uia = newUIABridge(w.hwnd, w.scale(), w.top)
		w.b.enqueue(w, platform.AccessibilityEvent{Active: true})
	}
	r, _, _ := procUiaReturnRawElementProvider.Call(w.hwnd, wparam, lparam, w.uia.root.srv.iface(uiaIfSimple))
	return r, true
}

// onAccessActions queues the actions UIA clients asked for.
func (w *Window) onAccessActions() {
	if w.uia == nil {
		return
	}
	for _, e := range w.uia.takeActions() {
		if platform.CheckAccessActionEvent(e) == "" {
			w.b.enqueue(w, e)
		}
	}
}

// uiaGeometryChanged records a new scale or hidden band for the bridge.
func (w *Window) uiaGeometryChanged() {
	if w.uia != nil {
		w.uia.setGeometry(w.scale(), w.top)
	}
}

// closeUIA disconnects the window's bridge.
func (w *Window) closeUIA() {
	if b := w.uia; b != nil {
		w.uia = nil
		b.close()
	}
}

// --- callbacks ---

// uiaRecover is deferred by every callback: a panic cannot unwind through
// UIA's frames, so it becomes E_FAIL.
func uiaRecover(hr *uintptr) {
	if recover() != nil {
		*hr = eFail
	}
}

// uiaNodeOf returns the object behind an interface pointer, or nil.
func uiaNodeOf(this uintptr) *uiaNode {
	s := comServerOf(this)
	if s == nil {
		return nil
	}
	u, _ := s.impl.(*uiaNode)
	return u
}

// resolve returns the snapshot and node of the object behind this. n is
// nil for the root before the first update; hr is not S_OK when the
// object's node is gone.
func resolve(this uintptr) (u *uiaNode, s *uiaSnapshot, n *platform.AccessNode, hr uintptr) {
	u = uiaNodeOf(this)
	if u == nil || u.dead.Load() || u.b.closed.Load() {
		return nil, nil, nil, uiaEElementNotAvailable
	}
	s = u.b.snap.Load()
	id := u.id
	if u.root {
		id = s.root()
		if id == 0 {
			return u, s, nil, sOK
		}
	}
	n, ok := s.node(id)
	if !ok {
		return nil, nil, nil, uiaEElementNotAvailable
	}
	return u, s, n, sOK
}

// resolveFor is resolve for a pattern: the node must support it now.
func resolveFor(this uintptr, pattern int32) (*uiaNode, *uiaSnapshot, *platform.AccessNode, uintptr) {
	u, s, n, hr := resolve(this)
	if hr != sOK {
		return nil, nil, nil, hr
	}
	if n == nil || !uiaSupports(n, pattern) {
		return nil, nil, nil, uiaEInvalidOperation
	}
	return u, s, n, sOK
}

// act queues action e on node n after UIA's checks: a disabled node
// refuses (UIA_E_ELEMENTNOTENABLED), and so does one without the action
// (UIA_E_INVALIDOPERATION).
func (u *uiaNode) act(n *platform.AccessNode, e platform.AccessActionEvent) uintptr {
	if n.Has(platform.AccessDisabled) && e.Action != platform.AccessActionScrollIntoView {
		return uiaEElementNotEnabled
	}
	if !n.Actions.Has(e.Action) {
		return uiaEInvalidOperation
	}
	e.Node = n.ID
	u.b.post(e)
	return sOK
}

func setI32(p uintptr, v int32) { *(*int32)(ptrFromUintptr(p)) = v }

func setBool(p uintptr, v bool) {
	if v {
		setI32(p, 1)
	} else {
		setI32(p, 0)
	}
}

// IRawElementProviderSimple::get_ProviderOptions.
func uiaProviderOptions(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setI32(ret, uiaProviderOptionsServerSideProvider)
	return sOK
}

// IRawElementProviderSimple::GetPatternProvider.
func uiaGetPatternProvider(this, pattern, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, _, n, hr := resolve(this)
	if hr != sOK {
		return hr
	}
	i, ok := uiaPatternIface[int32(pattern)]
	if !ok || n == nil || !uiaSupports(n, int32(pattern)) {
		return sOK // NULL: not supported
	}
	u.srv.addRef()
	return out(ret, u, i)
}

// IRawElementProviderSimple::GetPropertyValue.
func uiaGetPropertyValue(this, id, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	v := (*variant)(ptrFromUintptr(ret))
	*v = variant{}
	u, s, n, hr := resolve(this)
	if hr != sOK {
		return hr
	}
	pid := int32(id)
	if u.root {
		// The host provider describes the window; the root adds only
		// the framework.
		if pid == uiaFrameworkIDPropertyID {
			v.set(uiaS(uiaFrameworkID))
		}
		return sOK
	}
	if pid == uiaControllerForPropertyID {
		if n.Controls != 0 {
			if c := u.b.provider(n.Controls); c != nil {
				v.vt, v.val = vtArray|vtUnknown, safeArrayProviders([]*uiaNode{c})
			}
		}
		return sOK
	}
	v.set(uiaProperty(s, n, pid))
	return sOK
}

// IRawElementProviderSimple::get_HostRawElementProvider: the window's for
// the root, none for the elements.
func uiaHostRawElementProvider(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u := uiaNodeOf(this)
	if u == nil || u.dead.Load() {
		return uiaEElementNotAvailable
	}
	if !u.root {
		return sOK
	}
	r, _, _ := procUiaHostProviderFromHwnd.Call(u.b.hwnd, ret)
	return r
}

// IRawElementProviderFragment::Navigate.
func uiaNavigate(this, dir, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, s, n, hr := resolve(this)
	if hr != sOK {
		return hr
	}
	if n == nil || (u.root && int32(dir) != uiaNavigateFirstChild && int32(dir) != uiaNavigateLastChild) {
		return sOK // the host provider navigates around the window
	}
	return out(ret, u.b.provider(s.navigate(n.ID, int32(dir))), uiaIfFragment)
}

// IRawElementProviderFragment::GetRuntimeId: NULL for the root (its host
// provider's), else UiaAppendRuntimeId and the node ID.
func uiaGetRuntimeID(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u := uiaNodeOf(this)
	if u == nil {
		return uiaEElementNotAvailable
	}
	if u.root {
		return sOK
	}
	// A dead object keeps its ID, so a client can still match it with an
	// event about it.
	*(*uintptr)(ptrFromUintptr(ret)) = safeArrayI4(u.b.runtimeID(u))
	return sOK
}

// uiaRect is UiaRect.
type uiaRect struct{ left, top, width, height float64 }

// IRawElementProviderFragment::get_BoundingRectangle: empty for the root
// (its host provider's).
func uiaBoundingRectangle(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	r := (*uiaRect)(ptrFromUintptr(ret))
	*r = uiaRect{}
	u, _, n, hr := resolve(this)
	if hr != sOK || u.root || n == nil {
		return hr
	}
	q := u.b.screenRect(n.Bounds)
	*r = uiaRect{q[0], q[1], q[2], q[3]}
	return sOK
}

// IRawElementProviderFragment::GetEmbeddedFragmentRoots: none.
func uiaGetEmbeddedFragmentRoots(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	return sOK
}

// IRawElementProviderFragment::SetFocus.
func uiaSetFocus(this uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolve(this)
	if hr != sOK || n == nil || u.root {
		return hr
	}
	return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionFocus})
}

// IRawElementProviderFragment::get_FragmentRoot.
func uiaFragmentRoot(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u := uiaNodeOf(this)
	if u == nil || u.dead.Load() {
		return uiaEElementNotAvailable
	}
	u.b.root.srv.addRef()
	return out(ret, u.b.root, uiaIfRoot)
}

// IRawElementProviderFragmentRoot::ElementProviderFromPoint (the
// arguments arrive through uiacall_windows_*.go): x and y are physical
// screen pixels.
func uiaElementProviderFromPoint(this uintptr, x, y float64, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, s, n, hr := resolve(this)
	if hr != sOK || n == nil {
		return hr
	}
	cx, cy := u.b.toContent(x, y)
	return out(ret, u.b.provider(s.at(cx, cy)), uiaIfFragment)
}

// IRawElementProviderFragmentRoot::GetFocus: the focused element, NULL
// when it is the root.
func uiaGetFocus(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, s, n, hr := resolve(this)
	if hr != sOK || n == nil || s.focus == s.root() {
		return hr
	}
	return out(ret, u.b.provider(s.focus), uiaIfFragment)
}

// IInvokeProvider::Invoke.
func uiaInvoke(this uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolveFor(this, uiaInvokePatternID)
	if hr != sOK {
		return hr
	}
	return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionDefault})
}

// IToggleProvider::Toggle: the default action (a click toggles).
func uiaToggle(this uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolveFor(this, uiaTogglePatternID)
	if hr != sOK {
		return hr
	}
	return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionDefault})
}

// IToggleProvider::get_ToggleState.
func uiaToggleStateGet(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setI32(ret, uiaToggleOff)
	_, _, n, hr := resolveFor(this, uiaTogglePatternID)
	if hr == sOK {
		setI32(ret, uiaToggleState(n))
	}
	return hr
}

// IValueProvider::SetValue.
func uiaValueSetValue(this, value uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolveFor(this, uiaValuePatternID)
	if hr != sOK {
		return hr
	}
	if uiaValueReadOnly(n) && !n.Has(platform.AccessDisabled) {
		return uiaEInvalidOperation
	}
	var s string
	if value != 0 {
		s = windows.UTF16PtrToString((*uint16)(ptrFromUintptr(value)))
	}
	return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionSetValue, Value: s})
}

// IValueProvider::get_Value.
func uiaValueGet(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	_, _, n, hr := resolveFor(this, uiaValuePatternID)
	if hr != sOK {
		return hr
	}
	*(*uintptr)(ptrFromUintptr(ret)) = bstr(uiaValue(n))
	return sOK
}

// IValueProvider::get_IsReadOnly.
func uiaValueIsReadOnly(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setBool(ret, true)
	_, _, n, hr := resolveFor(this, uiaValuePatternID)
	if hr == sOK {
		setBool(ret, uiaValueReadOnly(n))
	}
	return hr
}

// IRangeValueProvider::SetValue (the argument arrives through
// uiacall_windows_*.go). The tree has no settable number, only steps: the
// value moves one increment or decrement toward v, as one arrow key
// would; a client that wants more calls again (the next update has the
// new value).
func uiaRangeValueSetValue(this uintptr, v float64) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolveFor(this, uiaRangeValuePatternID)
	if hr != sOK {
		return hr
	}
	if (n.Range.HasMin && v < n.Range.Min) || (n.Range.HasMax && v > n.Range.Max) || v != v {
		return eInvalidArg
	}
	switch {
	case v > n.Range.Now:
		return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionIncrement})
	case v < n.Range.Now:
		return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionDecrement})
	}
	return sOK
}

// uiaRangeGet returns an IRangeValueProvider getter of a double.
func uiaRangeGet(f func(*platform.AccessNode) float64) func(this, ret uintptr) uintptr {
	return func(this, ret uintptr) (hr uintptr) {
		defer uiaRecover(&hr)
		*(*float64)(ptrFromUintptr(ret)) = 0
		_, _, n, hr := resolveFor(this, uiaRangeValuePatternID)
		if hr == sOK {
			*(*float64)(ptrFromUintptr(ret)) = f(n)
		}
		return hr
	}
}

// IRangeValueProvider::get_IsReadOnly.
func uiaRangeIsReadOnly(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setBool(ret, true)
	_, _, n, hr := resolveFor(this, uiaRangeValuePatternID)
	if hr == sOK {
		setBool(ret, uiaRangeReadOnly(n))
	}
	return hr
}

// IExpandCollapseProvider::Expand and Collapse: the node's expand or
// collapse action, else its default action (a disclosure button toggles
// on click). Expanding an expanded node is an invalid operation
// (AccessKit).
func uiaExpand(this uintptr) uintptr { return uiaSetExpanded(this, true) }

func uiaCollapse(this uintptr) uintptr { return uiaSetExpanded(this, false) }

func uiaSetExpanded(this uintptr, expand bool) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolveFor(this, uiaExpandCollapsePatternID)
	if hr != sOK {
		return hr
	}
	state := uiaExpandState(n)
	if (expand && state == uiaExpanded) || (!expand && state != uiaExpanded) {
		return uiaEInvalidOperation
	}
	a := platform.AccessActionCollapse
	if expand {
		a = platform.AccessActionExpand
	}
	if !n.Actions.Has(a) {
		a = platform.AccessActionDefault
	}
	return u.act(n, platform.AccessActionEvent{Action: a})
}

// IExpandCollapseProvider::get_ExpandCollapseState.
func uiaExpandStateGet(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setI32(ret, uiaLeafNode)
	_, _, n, hr := resolveFor(this, uiaExpandCollapsePatternID)
	if hr == sOK {
		setI32(ret, uiaExpandState(n))
	}
	return hr
}

// ISelectionItemProvider::Select and AddToSelection: a click selects an
// item that is not selected (AccessKit).
func uiaSelect(this uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolveFor(this, uiaSelectionItemPatternID)
	if hr != sOK || uiaIsSelected(n) {
		return hr
	}
	return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionDefault})
}

// ISelectionItemProvider::RemoveFromSelection: a click on a selected item.
func uiaUnselect(this uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolveFor(this, uiaSelectionItemPatternID)
	if hr != sOK || !uiaIsSelected(n) {
		return hr
	}
	return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionDefault})
}

// ISelectionItemProvider::get_IsSelected.
func uiaIsSelectedGet(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	setBool(ret, false)
	_, _, n, hr := resolveFor(this, uiaSelectionItemPatternID)
	if hr == sOK {
		setBool(ret, uiaIsSelected(n))
	}
	return hr
}

// ISelectionItemProvider::get_SelectionContainer.
func uiaSelectionContainer(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, s, n, hr := resolveFor(this, uiaSelectionItemPatternID)
	if hr != sOK {
		return hr
	}
	return out(ret, u.b.provider(s.selectionContainer(n.ID)), uiaIfSimple)
}

// ISelectionProvider::GetSelection.
func uiaGetSelection(this, ret uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	*(*uintptr)(ptrFromUintptr(ret)) = 0
	u, s, n, hr := resolveFor(this, uiaSelectionPatternID)
	if hr != sOK {
		return hr
	}
	var items []*uiaNode
	for _, id := range s.selected(n.ID) {
		if p := u.b.provider(id); p != nil {
			items = append(items, p)
		}
	}
	*(*uintptr)(ptrFromUintptr(ret)) = safeArrayProviders(items)
	return sOK
}

// ISelectionProvider::get_CanSelectMultiple and get_IsSelectionRequired.
func uiaCanSelectMultiple(this, ret uintptr) uintptr {
	return uiaSelectionFlag(this, ret, platform.AccessMultiSelectable)
}

func uiaIsSelectionRequired(this, ret uintptr) uintptr {
	return uiaSelectionFlag(this, ret, platform.AccessRequired)
}

func uiaSelectionFlag(this, ret uintptr, state platform.AccessStates) (hr uintptr) {
	defer uiaRecover(&hr)
	setBool(ret, false)
	_, _, n, hr := resolveFor(this, uiaSelectionPatternID)
	if hr == sOK {
		setBool(ret, n.Has(state))
	}
	return hr
}

// IScrollItemProvider::ScrollIntoView.
func uiaScrollIntoView(this uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolveFor(this, uiaScrollItemPatternID)
	if hr != sOK {
		return hr
	}
	return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionScrollIntoView})
}

// uiaScrollStep is a small scroll step in logical pixels (three lines of
// text, a wheel notch); a large one is most of the container's size.
const uiaScrollStep = 40

// uiaScrollAmount returns the distance of ScrollAmount a over a container
// of size size.
func uiaScrollAmount(a uintptr, size float32) float32 {
	switch int32(a) {
	case uiaScrollLargeDecrement:
		return -size * 0.875
	case uiaScrollSmallDecrement:
		return -uiaScrollStep
	case uiaScrollLargeIncrement:
		return size * 0.875
	case uiaScrollSmallIncrement:
		return uiaScrollStep
	}
	return 0
}

// IScrollProvider::Scroll.
func uiaScroll(this, h, v uintptr) (hr uintptr) {
	defer uiaRecover(&hr)
	u, _, n, hr := resolveFor(this, uiaScrollPatternID)
	if hr != sOK {
		return hr
	}
	dx, dy := uiaScrollAmount(h, n.Bounds.W), uiaScrollAmount(v, n.Bounds.H)
	if dx == 0 && dy == 0 {
		return sOK
	}
	return u.act(n, platform.AccessActionEvent{Action: platform.AccessActionScrollBy, DX: dx, DY: dy})
}

// IScrollProvider::SetScrollPercent: the tree has no scroll offsets.
func uiaSetScrollPercent(this uintptr) uintptr { return uiaEInvalidOperation }

// uiaScrollGet returns the getter of a scroll percent or view size. The
// tree has no scroll offsets or content sizes: the percent is 0 on the
// axis the container scrolls (NoScroll on the other) and the view size
// 100; approximate, so that clients see which way it scrolls.
func uiaScrollGet(vertical, percent bool) func(this, ret uintptr) uintptr {
	return func(this, ret uintptr) (hr uintptr) {
		defer uiaRecover(&hr)
		*(*float64)(ptrFromUintptr(ret)) = 100
		_, _, n, hr := resolveFor(this, uiaScrollPatternID)
		if hr == sOK && percent {
			v := 0.0
			if !uiaScrollsOn(n, vertical) {
				v = uiaScrollNoScroll
			}
			*(*float64)(ptrFromUintptr(ret)) = v
		}
		return hr
	}
}

// uiaScrollsOn reports whether a scroll container scrolls on an axis:
// vertically unless aria-orientation says horizontal.
func uiaScrollsOn(n *platform.AccessNode, vertical bool) bool {
	if n.Orientation == "horizontal" {
		return !vertical
	}
	return vertical
}

// uiaScrollable returns the getter of HorizontallyScrollable or
// VerticallyScrollable.
func uiaScrollable(vertical bool) func(this, ret uintptr) uintptr {
	return func(this, ret uintptr) (hr uintptr) {
		defer uiaRecover(&hr)
		setBool(ret, false)
		_, _, n, hr := resolveFor(this, uiaScrollPatternID)
		if hr == sOK {
			setBool(ret, uiaScrollsOn(n, vertical))
		}
		return hr
	}
}
