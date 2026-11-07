//go:build windows

package win32

import (
	"image"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// OLE drag and drop ("Desktop integration on each OS: Win32" in package
// platform):
//
//   - Target. Every window registers an IDropTarget (RegisterDragDrop at
//     creation, RevokeDragDrop at close) when the UI thread has OLE
//     (OleInitialize, com_windows.go). OLE calls it on the UI thread: from
//     DoDragDrop's modal loop for this app's drags, and through the
//     message loop (Run's or any modal one) for another program's. Each
//     call is delivered synchronously (droptarget.go has the rules) and
//     its answer returned. The offered formats are read once, at
//     DragEnter; the drop reads the wanted ones with GetData. A drag of
//     this app is recognized by its data object (iidTelaDataObject),
//     whose Data is read as it is. The shell's IDropTargetHelper shows
//     the drag image over the window, as Explorer does.
//   - Source. StartDrag cancels the window's pointer sequence (Windows
//     still holds the button: the press goes on as the drag), releases
//     the capture and posts a message; DoDragDrop runs when it is
//     dispatched, outside the delivery pass (like the dialogs; its modal
//     loop keeps WndProc, and so the deliveries, running). The data
//     object renders every format when the drag starts. The image goes
//     to the shell's IDragSourceHelper, which keeps it in the data object
//     (SetData). DRAGDROP_S_DROP reports the effect the target returned
//     (an optimized move, where the target moved the data itself, returns
//     DROPEFFECT_NONE: the source keeps nothing to delete, which DropNone
//     says), DRAGDROP_S_CANCEL and errors DropNone.

// Private messages to the message-only window, apart from the backend's
// (WM_APP + 1, 2) and the desktop services' (WM_APP + 0x40 on).
const (
	wmTelaDrag  = 0x8000 + 0x50 // start the requested drag
	wmTelaPopup = 0x8000 + 0x51 // show the next popup menu
)

// Interfaces and classes of the shell's drag image helpers, and the drop
// target and source interfaces.
var (
	iidDropTarget       = guid{0x00000122, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidDropSource       = guid{0x00000121, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	clsidDragDropHelper = guid{0x4657278A, 0x411B, 0x11D2, [8]byte{0x83, 0x9A, 0x00, 0xC0, 0x4F, 0xD9, 0x18, 0xD0}}
	iidDropTargetHelper = guid{0x4657278B, 0x411B, 0x11D2, [8]byte{0x83, 0x9A, 0x00, 0xC0, 0x4F, 0xD9, 0x18, 0xD0}}
	iidDragSourceHelper = guid{0xDE5BF786, 0x477A, 0x11D2, [8]byte{0x83, 0x9D, 0x00, 0xC0, 0x4F, 0xD9, 0x18, 0xD0}}
)

// IDropTargetHelper and IDragSourceHelper vtable indices (shobjidl_core.h).
const (
	vtDropTargetHelperDragEnter            = 3
	vtDropTargetHelperDragLeave            = 4
	vtDropTargetHelperDragOver             = 5
	vtDropTargetHelperDrop                 = 6
	vtDragSourceHelperInitializeFromBitmap = 3
)

// shDragImage is SHDRAGIMAGE.
type shDragImage struct {
	size     [2]int32 // SIZE
	offset   point    // the cursor's position in the image
	bitmap   uintptr  // HBITMAP, owned by the helper once it accepts it
	colorKey uint32   // COLORREF; CLR_NONE: the alpha channel is used
}

// windowIntegration is a window's phase-13 state: its drop target and
// session, and its menu bar (menu_windows.go).
type windowIntegration struct {
	target  *comServer // the registered IDropTarget, nil without OLE
	session dropSession
	formats []clipFormat      // the session's offered formats (another program's drag)
	own     *dataObjectServer // the session's data object, for a drag of this app

	menu     uintptr               // the menu bar's HMENU, or 0
	menuCmds []platform.MenuItemID // its command table (buildMenu)

	mnemonics    []rune   // the access keys of its items (buildBarMenu), lower-cased
	keys         menuKeys // the keyboard state that opens it (keyboard.go)
	cuesHeld     bool     // an Alt or F10 opening showed the access keys: hide them at the exit
	barFont      uintptr  // the bar items' font (menubar_dark_windows.go) at barFontDPI
	barFontDPI   uint32
	barFontOwned bool
}

// backendIntegration is the backend's phase-13 state.
type backendIntegration struct {
	helper      comObject // IDropTargetHelper, or 0
	helperTried bool

	drag       *outgoingDrag // the drag StartDrag requested, until DragDoneEvent
	dragPosted bool

	popups      []popupRequest // waiting, in request order
	popupPosted bool
	popupOpen   bool

	// dropErr is why the data of the latest drop was not read (the
	// source stopped answering: errDropStalled), or nil. The drop itself
	// completes, without data.
	dropErr error
}

// outgoingDrag is a drag of this app.
type outgoingDrag struct {
	w       *Window
	data    platform.Data
	allowed platform.DropEffect
	image   *image.RGBA
	hotspot image.Point
	button  uint32 // MK_ bit of the button it started from
	running bool   // DoDragDrop is running
}

// integrationMessage handles the private messages of drags and popup
// menus that reach the message-only window; ok is false for any other.
func (b *Backend) integrationMessage(hwnd uintptr, msg uint32) (ret uintptr, ok bool) {
	if hwnd != b.msgHwnd {
		return 0, false
	}
	switch msg {
	case wmTelaDrag:
		b.runDrag()
	case wmTelaPopup:
		b.runPopup()
	default:
		return 0, false
	}
	return 0, true
}

// postLater posts a private message to the message-only window: now, or
// after the running delivery pass when there is one (a modal loop must not
// start inside a pass, which would block delivery until it ended).
func (b *Backend) postLater(msg uint32) {
	if b.stopped.Load() {
		return
	}
	if b.pumping {
		b.desk.later = append(b.desk.later, postedMessage{b.msgHwnd, msg, 0, 0})
		return
	}
	postMessage(b.msgHwnd, msg, 0, 0)
}

// integrationShutdown drops the waiting requests (Run returned: no answer
// follows) and releases the drop target helper before OLE goes.
func (b *Backend) integrationShutdown() {
	in := &b.integ
	in.popups, in.drag = nil, nil
	if in.helper != 0 {
		in.helper.release()
		in.helper = 0
	}
	in.helperTried = false
}

// recoverCallback is deferred by the COM callbacks that run the handler:
// a panic cannot unwind through OLE's frames, so it stops the loop and Run
// raises it, as WndProc does.
func (b *Backend) recoverCallback() {
	if r := recover(); r != nil {
		b.fail(r)
	}
}

// --- target ---

// dropTargetServer is a window's IDropTarget.
type dropTargetServer struct{ w *Window }

// registerDrop makes the window a drop target. Without OLE (the thread is
// in the multithreaded apartment) the window takes no drops.
func (w *Window) registerDrop() {
	if err := w.b.desk.initCOM(); err != nil {
		return
	}
	s := newComServer(&dropTargetVtbl[0], []guid{iidDropTarget}, &dropTargetServer{w: w}, nil)
	if r, _, _ := procRegisterDragDrop.Call(w.hwnd, s.ptr()); int32(uint32(r)) < 0 {
		s.release()
		return
	}
	w.integ.target = s
}

// closeIntegration runs before the window is destroyed: it undoes
// registerDrop (OLE keeps the target in a window property), so a drag
// over the window gets no more events (they would be dropped with the
// window's), and destroys a menu bar detached for fullscreen (an attached
// one goes with the window).
func (w *Window) closeIntegration() {
	in := &w.integ
	if s := in.target; s != nil {
		in.target = nil
		procRevokeDragDrop.Call(w.hwnd)
		s.release()
	}
	in.session, in.formats, in.own = dropSession{}, nil, nil
	if in.menu != 0 && w.fs != nil {
		procDestroyMenu.Call(in.menu)
	}
	in.menu, in.menuCmds, in.mnemonics = 0, nil, nil
	w.releaseBarFont()
}

// dropTargetOf returns the open window of an IDropTarget, or nil.
func dropTargetOf(this uintptr) *Window {
	s := comServerOf(this)
	if s == nil {
		return nil
	}
	t, _ := s.impl.(*dropTargetServer)
	if t == nil || t.w.Closed() || t.w.b.failed || t.w.b.closed {
		return nil
	}
	return t.w
}

// winDropHost delivers a window's drag events synchronously.
type winDropHost struct{ w *Window }

func (h winDropHost) deliverDrag(e platform.DragEvent) bool {
	b := h.w.b
	b.dirty = true
	b.loop.QueueDrag(h.w, e)
	if b.pumping || b.handler == nil {
		return false
	}
	b.pump()
	return true
}

func (h winDropHost) now() time.Duration { return h.w.b.Now() }

func getEffect(p uintptr) uint32 {
	if p == 0 {
		return 0
	}
	return *(*uint32)(ptrFromUintptr(p))
}

func putEffect(p uintptr, e platform.DropEffect) {
	if p != 0 {
		*(*uint32)(ptrFromUintptr(p)) = uint32(e)
	}
}

// dragPos converts a drag position (screen pixels) to the content's
// logical pixels.
func (w *Window) dragPos(pt point) platform.Point {
	x, y := w.screenToApp(int(pt.x), int(pt.y))
	return logicalPoint(x, y, w.scale())
}

// dropHelper returns the shell's IDropTargetHelper, created at the first
// drag (0 where the shell has none).
func (b *Backend) dropHelper() comObject {
	in := &b.integ
	if !in.helperTried {
		in.helperTried = true
		var h comObject
		r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidDragDropHelper)), 0, clsctxInprocServer,
			uintptr(unsafe.Pointer(&iidDropTargetHelper)), uintptr(unsafe.Pointer(&h)))
		if int32(uint32(r)) >= 0 {
			in.helper = h
		}
	}
	return in.helper
}

// IDropTarget::DragEnter.
func dropTargetEnter(this, dataObj uintptr, keyState uint32, pt point, pEffect uintptr) (hr uintptr) {
	w := dropTargetOf(this)
	if w == nil {
		putEffect(pEffect, platform.DropNone)
		return sOK
	}
	defer w.b.recoverCallback()
	obj := comObject(dataObj)
	in := &w.integ
	in.own, in.formats = ownDataObject(obj), nil
	var types []string
	if in.own != nil {
		types = in.own.data.Types()
	} else {
		// Bounded: a source that hangs offers nothing (stallguard.go).
		in.formats = nil
		var formats []clipFormat
		if !guarded(func(g *stallGuard) { formats = enumFormats(obj, g) }) {
			in.formats = formats
		}
		types = offeredTypes(in.formats)
	}
	// The window gets no pointer events during the drag: a pointer the
	// app believes over it leaves first (this app's drag already
	// cancelled its sequence).
	w.mods = w.modifiers()
	w.ptr.captureLost(w.emitFn)
	if w.ptr.inside {
		w.ptr.leave(w.emitFn)
	}
	effect := in.session.enter(winDropHost{w}, types, platform.DropEffect(getEffect(pEffect)), dragKeyMods(keyState), w.dragPos(pt), in.own != nil)
	putEffect(pEffect, effect)
	if h := w.b.dropHelper(); h != 0 && !w.Closed() {
		// The helper reads the drag image from the source's data object.
		guarded(func(*stallGuard) {
			h.call(vtDropTargetHelperDragEnter, w.hwnd, dataObj, uintptr(unsafe.Pointer(&pt)), uintptr(effect))
		})
	}
	return sOK
}

// IDropTarget::DragOver, which OLE also calls on a timer.
func dropTargetOver(this uintptr, keyState uint32, pt point, pEffect uintptr) (hr uintptr) {
	w := dropTargetOf(this)
	if w == nil {
		putEffect(pEffect, platform.DropNone)
		return sOK
	}
	defer w.b.recoverCallback()
	effect := w.integ.session.over(winDropHost{w}, platform.DropEffect(getEffect(pEffect)), dragKeyMods(keyState), w.dragPos(pt))
	putEffect(pEffect, effect)
	if h := w.b.dropHelper(); h != 0 {
		h.call(vtDropTargetHelperDragOver, uintptr(unsafe.Pointer(&pt)), uintptr(effect))
	}
	return sOK
}

// IDropTarget::DragLeave.
func dtDragLeave(this uintptr) (hr uintptr) {
	w := dropTargetOf(this)
	if w == nil {
		return sOK
	}
	defer w.b.recoverCallback()
	if h := w.b.dropHelper(); h != 0 {
		h.call(vtDropTargetHelperDragLeave)
	}
	in := &w.integ
	in.formats, in.own = nil, nil
	in.session.leave(winDropHost{w})
	return sOK
}

// IDropTarget::Drop.
func dropTargetDrop(this, dataObj uintptr, keyState uint32, pt point, pEffect uintptr) (hr uintptr) {
	w := dropTargetOf(this)
	if w == nil {
		putEffect(pEffect, platform.DropNone)
		return sOK
	}
	defer w.b.recoverCallback()
	in := &w.integ
	obj, own, formats := comObject(dataObj), in.own, in.formats
	in.formats, in.own = nil, nil
	allowed := platform.DropEffect(getEffect(pEffect))
	// The helper first: it takes the drag image away while the app
	// handles the drop.
	if h := w.b.dropHelper(); h != 0 {
		guarded(func(*stallGuard) {
			h.call(vtDropTargetHelperDrop, dataObj, uintptr(unsafe.Pointer(&pt)), uintptr(in.session.answer()&allowed))
		})
	}
	read := func(types []string) platform.Data {
		if own != nil {
			return own.data.Select(types).Clone()
		}
		// Bounded: a source that stops answering ends the read, and the
		// drop completes without data (stallguard.go).
		var data platform.Data
		var err error
		guarded(func(g *stallGuard) { data, err = readDataObjectGuarded(obj, formats, types, g) })
		w.b.integ.dropErr = err
		return data
	}
	putEffect(pEffect, in.session.drop(winDropHost{w}, allowed, dragKeyMods(keyState), w.dragPos(pt), read))
	return sOK
}

// SetDropEffect answers the drag over the window; OLE gets the answer when
// the IDropTarget call that delivered the event returns.
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
	if !w.Closed() {
		w.integ.session.setAnswer(effect, types)
	}
}

// --- source ---

// dropSourceServer is a drag's IDropSource.
type dropSourceServer struct {
	b      *Backend
	button uint32
}

// IDropSource::QueryContinueDrag(fEscapePressed, grfKeyState).
func dsQueryContinueDrag(this, escape, keyState uintptr) uintptr {
	s := comServerOf(this)
	if s == nil {
		return dragdropSCancel
	}
	src, _ := s.impl.(*dropSourceServer)
	if src == nil {
		return dragdropSCancel
	}
	quitting := src.b.loop.Quitting() || src.b.failed
	return uintptr(uint32(dropSourceContinue(escape != 0, uint32(keyState), src.button, quitting)))
}

// IDropSource::GiveFeedback: the system's cursors.
func dsGiveFeedback(this, effect uintptr) uintptr { return dragdropSUseDefaultCursors }

// StartDrag starts an OLE drag of opts.Data. The window must hold a
// pointer button pressed in it (the press goes on as the drag) and no
// other drag may be in progress, else ErrNoGrab. The window's sequence
// ends with a PointerCancel, delivered before DoDragDrop starts; one
// DragDoneEvent follows.
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	b := w.b
	if w.Closed() || b.closed || b.integ.drag != nil || w.ptr.held == 0 {
		return platform.ErrNoGrab
	}
	if err := b.desk.initCOM(); err != nil {
		return err
	}
	d := &outgoingDrag{w: w, data: opts.Data.Clone(), allowed: opts.Allowed, hotspot: opts.Hotspot, button: dragButton(w.ptr.held)}
	if img := opts.Image; img != nil {
		d.image = image.NewRGBA(image.Rect(0, 0, img.Rect.Dx(), img.Rect.Dy()))
		for y := range img.Rect.Dy() {
			copy(d.image.Pix[y*d.image.Stride:], img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y):][:4*img.Rect.Dx()])
		}
	}
	b.integ.drag = d
	// OLE takes the pointer from here on.
	w.mods = w.modifiers()
	w.ptr.captureLost(w.emitFn)
	w.releaseCapture()
	if !b.integ.dragPosted {
		b.integ.dragPosted = true
		b.postLater(wmTelaDrag)
	}
	return nil
}

// runDrag runs the requested drag: DoDragDrop, then its DragDoneEvent.
func (b *Backend) runDrag() {
	in := &b.integ
	in.dragPosted = false
	d := in.drag
	if d == nil || d.running {
		return
	}
	if b.pumping {
		in.dragPosted = true
		b.postLater(wmTelaDrag)
		return
	}
	b.pump() // the PointerCancel, before DoDragDrop captures the mouse
	if d.w.Closed() || b.closed || b.loop.Quitting() || b.failed {
		in.drag = nil
		return
	}
	src := newDataObject(d.data)
	if d.image != nil {
		setDragImage(src, d.image, d.hotspot)
	}
	ds := newComServer(&dropSourceVtbl[0], []guid{iidDropSource}, &dropSourceServer{b: b, button: d.button}, nil)
	d.running = true
	var effect uint32
	r, _, _ := procDoDragDrop.Call(src.ptr(), ds.ptr(), uintptr(d.allowed), uintptr(unsafe.Pointer(&effect)))
	ds.release()
	src.release()
	in.drag = nil
	result := platform.DropNone
	if uint32(r) == dragdropSDrop {
		if e := oneEffect(effect); d.allowed.Has(e) {
			result = e
		}
	}
	if !d.w.Closed() && !b.closed {
		b.enqueue(d.w, platform.DragDoneEvent{Effect: result})
		b.pump()
	}
}

// setDragImage gives the drag image to the shell's IDragSourceHelper,
// which stores it in the data object; without the helper, or when it
// refuses the bitmap, the drag shows the cursor only.
func setDragImage(src *comServer, img *image.RGBA, hotspot image.Point) {
	var helper comObject
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidDragDropHelper)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidDragSourceHelper)), uintptr(unsafe.Pointer(&helper)))
	if int32(uint32(r)) < 0 || helper == 0 {
		return
	}
	defer helper.release()
	w, h := img.Rect.Dx(), img.Rect.Dy()
	bmi := bitmapInfo{bitmapInfoHeader: bitmapInfoHeader{
		size:     bitmapInfoHeaderSize,
		width:    int32(w),
		height:   -int32(h), // top-down
		planes:   1,
		bitCount: 32,
	}}
	var bits uintptr
	hbm, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bmi)), 0 /* DIB_RGB_COLORS */, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 || bits == 0 {
		return
	}
	pix := premultipliedBGRA(img)
	copy(unsafe.Slice((*byte)(ptrFromUintptr(bits)), len(pix)), pix)
	sdi := shDragImage{size: [2]int32{int32(w), int32(h)}, offset: point{int32(hotspot.X), int32(hotspot.Y)}, bitmap: hbm, colorKey: 0xFFFFFFFF}
	if hr := helper.call(vtDragSourceHelperInitializeFromBitmap, uintptr(unsafe.Pointer(&sdi)), src.ptr()); hr < 0 {
		procDeleteObject.Call(hbm)
	}
}
