//go:build linux

package x11

import (
	"bytes"
	"errors"
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// sourceRig is a data rig with a scene on the root (1): the app's window
// 7 at (100, 50) 300x200 and window 8 at (100, 300) 300x200 (not
// reparented), and another client's frame 800 at (500, 0) holding its
// XdndAware client window 801.
func sourceRig(t *testing.T) *dataRig {
	g := newDataRig()
	fakeWindow(g.b, 8, image.Pt(300, 200), platform.WindowOptions{})
	g.feed()
	g.r.take()
	x := g.x
	x.origin[7], x.origin[8], x.origin[800], x.origin[801] = image.Pt(100, 50), image.Pt(100, 300), image.Pt(500, 0), image.Pt(510, 20)
	x.children[1] = []fakeChild{
		{7, image.Rect(100, 50, 400, 250)},
		{8, image.Rect(100, 300, 400, 500)},
		{800, image.Rect(500, 0, 900, 400)},
	}
	x.children[800] = []fakeChild{{801, image.Rect(510, 20, 890, 390)}}
	x.setProp(801, g.b.atoms.xdndAware, xproto.AtomAtom, 32, card32s(5))
	// The pointer enters window 7 and presses there.
	x.pointer, x.mask = image.Pt(130, 70), xproto.KeyButMaskButton1
	got := g.feed(xproto.EnterNotifyEvent{Event: 7, EventX: 30, EventY: 20, RootX: 130, RootY: 70},
		xproto.ButtonPressEvent{Event: 7, Detail: 1, EventX: 30, EventY: 20, RootX: 130, RootY: 70})
	if got != "pointer enter; pointer down" {
		t.Fatalf("press = %q", got)
	}
	return g
}

func (g *dataRig) motion(x, y int) string {
	g.x.pointer = image.Pt(x, y)
	return g.feed(xproto.MotionNotifyEvent{Event: 7, RootX: int16(x), RootY: int16(y), State: xproto.KeyButMaskButton1, Time: 40})
}

func (g *dataRig) releaseButton() string {
	g.x.mask = 0
	return g.feed(xproto.ButtonReleaseEvent{Event: 7, Detail: 1, RootX: int16(g.x.pointer.X), RootY: int16(g.x.pointer.Y),
		State: xproto.KeyButMaskButton1, Time: 50})
}

var dragData = platform.Data{{Type: platform.MIMEText, Bytes: []byte("drag")}, {Type: "image/png", Bytes: []byte("PNG")}}

func TestDragSourceToXdndTarget(t *testing.T) {
	g := sourceRig(t)
	a := &g.b.atoms
	if err := g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy | platform.DropMove}); err != nil {
		t.Fatal(err)
	}
	want := "grab pointer win=7 grabbing; own XdndSelection win=7; " +
		"prop XdndTypeList win=7 ATOM = [text/plain;charset=utf-8 UTF8_STRING text/plain TEXT STRING image/png]; " +
		"prop XdndActionList win=7 ATOM = [XdndActionCopy XdndActionMove]; " +
		`prop XdndActionDescription win=7 STRING = "Copy\x00Move"; grab keyboard win=7; cursor no-drop`
	if got := g.x.take(); got != want {
		t.Errorf("StartDrag sent:\n got %s\nwant %s", got, want)
	}
	// The source window loses the pointer, then the drag enters it.
	if got, w := g.pass(), "pointer cancel; pointer leave; drag enter 30,20 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app"; got != w {
		t.Errorf("start = %q, want %q", got, w)
	}
	if err := g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy}); !errors.Is(err, platform.ErrNoGrab) {
		t.Errorf("second StartDrag = %v", err)
	}

	// Over the other client's window: leave ours, XdndEnter (more than
	// three types: XdndTypeList) and a position with the default action.
	if got, w := g.motion(600, 100), "drag leave 30,20 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app"; got != w {
		t.Errorf("leaving = %q", got)
	}
	want = "msg XdndEnter to=801 win=801 [7 83886081 text/plain;charset=utf-8 UTF8_STRING text/plain]; " +
		"msg XdndPosition to=801 win=801 [7 0 600,100 40 XdndActionCopy]"
	if got := g.x.take(); got != want {
		t.Errorf("entering the target:\n got %s\nwant %s", got, want)
	}
	// One position in flight: the next waits for the status.
	g.motion(610, 100)
	if got := g.x.take(); got != "" {
		t.Errorf("position while waiting: %s", got)
	}
	g.feed(clientMsg(7, a.xdndStatus, 801, 3, 0, 0, uint32(a.xdndActionCopy)))
	if got, w := g.x.take(), "cursor copy; msg XdndPosition to=801 win=801 [7 0 610,100 40 XdndActionCopy]"; got != w {
		t.Errorf("after the status: %s", got)
	}
	g.feed(clientMsg(7, a.xdndStatus, 801, 3, 0, 0, uint32(a.xdndActionCopy)))

	// The release drops; the target converts, then finishes.
	g.releaseButton()
	if got, w := g.x.take(), "msg XdndDrop to=801 win=801 [7 0 50 0 0]; ungrab pointer; ungrab keyboard"; got != w {
		t.Errorf("drop: %s", got)
	}
	g.feed(xproto.SelectionRequestEvent{Owner: 7, Requestor: 801, Selection: a.xdndSelection, Target: g.x.id("image/png"), Property: g.x.id("XdndData"), Time: 50})
	if got, w := g.x.take(), `prop XdndData win=801 image/png = "PNG"; notify win=801 XdndSelection image/png prop=XdndData`; got != w {
		t.Errorf("serving the drop: %s", got)
	}
	if got, w := g.feed(clientMsg(7, a.xdndFinished, 801, 1, uint32(a.xdndActionCopy))), "done copy"; got != w {
		t.Errorf("finish = %q", got)
	}
	if got, w := g.x.take(), "own XdndSelection win=0"; got != w {
		t.Errorf("after the drag: %s", got)
	}
}

func TestDragSourceToOwnWindow(t *testing.T) {
	g := sourceRig(t)
	g.r.react = func(w platform.Window, e platform.Event) {
		if de, ok := e.(platform.DragEvent); ok && w == platform.Window(g.b.windows[8]) && de.Action != platform.DragLeave {
			w.SetDropEffect(platform.DropMove, "image/png")
		}
	}
	if err := g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy | platform.DropMove}); err != nil {
		t.Fatal(err)
	}
	g.pass()
	got := g.motion(200, 400)
	if w := "drag leave 30,20 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app; drag enter 100,100 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app"; got != w {
		t.Errorf("into window 8 = %q, want %q", got, w)
	}
	g.motion(201, 400)
	g.x.take()
	got = strings.Trim(g.releaseButton()+"; "+g.pass(), "; ")
	if w := "drag drop 101,100 [text/plain;charset=utf-8 image/png] allowed=copy|move mods=0 app data=[image/png]; done move"; got != w {
		t.Errorf("drop on window 8 = %q, want %q", got, w)
	}
	if got, w := g.x.take(), "ungrab pointer; ungrab keyboard; own XdndSelection win=0"; got != w {
		t.Errorf("server: %s", got)
	}
}

func TestDragSourceCancelAndRefusal(t *testing.T) {
	// Escape cancels.
	g := sourceRig(t)
	a := &g.b.atoms
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(600, 100)
	g.x.take()
	got := g.feed(xproto.KeyPressEvent{Event: 7, Detail: 9})
	if w := "done none"; got != w {
		t.Errorf("Escape = %q", got)
	}
	if got, w := g.x.take(), "msg XdndLeave to=801 win=801 [7 0 0 0 0]; ungrab pointer; ungrab keyboard; own XdndSelection win=0"; got != w {
		t.Errorf("Escape sent %s", got)
	}

	// A release before the status: the drop waits for it, and a refusal
	// leaves.
	g = sourceRig(t)
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(600, 100)
	g.releaseButton()
	g.x.take()
	got = g.feed(clientMsg(7, a.xdndStatus, 801, 2, 0, 0, 0))
	if w := "done none"; got != w {
		t.Errorf("refused = %q", got)
	}
	if got := g.x.take(); !strings.HasPrefix(got, "msg XdndLeave to=801") {
		t.Errorf("refused sent %s", got)
	}

	// No status at all: the timeout leaves.
	g = sourceRig(t)
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(600, 100)
	g.releaseButton()
	g.x.fire(dragStatusTimeout)
	if got := g.pass(); got != "done none" {
		t.Errorf("status timeout = %q", got)
	}

	// Released over nothing.
	g = sourceRig(t)
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(950, 450)
	if got := g.releaseButton(); got != "drag leave 30,20 [text/plain;charset=utf-8] allowed=copy mods=0 app; done none" &&
		got != "done none" {
		t.Errorf("drop over nothing = %q", got)
	}

	// Without a held press: ErrNoGrab.
	g = sourceRig(t)
	g.releaseButton()
	if err := g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy}); !errors.Is(err, platform.ErrNoGrab) {
		t.Errorf("StartDrag without a press = %v", err)
	}
}

func TestDragSourceWindowCloses(t *testing.T) {
	g := sourceRig(t)
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy})
	g.pass()
	g.motion(600, 100)
	g.x.take()
	g.w.Close()
	if got := g.x.take(); !strings.HasPrefix(got, "msg XdndLeave to=801 win=801 [7 0 0 0 0]; ungrab pointer") {
		t.Errorf("close sent %s", got)
	}
	if g.b.integ.drag != nil {
		t.Errorf("the drag outlived its window")
	}
}

// dragImage is a 4x2 image: an opaque row, then a half-transparent pixel,
// two transparent ones and an opaque one (premultiplied).
func dragImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(10, 20, 14, 22)) // not at the origin
	for x := range 4 {
		img.Pix[4*x], img.Pix[4*x+3] = 200, 255
	}
	row := img.Pix[img.Stride:]
	copy(row, []byte{100, 50, 0, 200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255})
	return img
}

func TestDragSourceImage(t *testing.T) {
	// A compositing manager and an ARGB visual: an ARGB window with the
	// premultiplied pixels, following the pointer with the hotspot.
	g := sourceRig(t)
	a := &g.b.atoms
	g.b.argbVisual = 0x55
	g.x.owners[g.x.id("_NET_WM_CM_S0")] = 66
	g.x.take()
	if err := g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy, Image: dragImage(), Hotspot: image.Pt(1, 1)}); err != nil {
		t.Fatal(err)
	}
	if got, want := g.x.take(), "grab pointer win=7 grabbing; own XdndSelection win=7; "+
		"prop XdndTypeList win=7 ATOM = [text/plain;charset=utf-8 UTF8_STRING text/plain TEXT STRING image/png]; "+
		"prop XdndActionList win=7 ATOM = [XdndActionCopy]; grab keyboard win=7; "+
		"create icon (129,69)-(133,71) argb bounding=none; "+
		"prop _NET_WM_WINDOW_TYPE win=950 ATOM = [_NET_WM_WINDOW_TYPE_DND]; map 950; cursor no-drop"; got != want {
		t.Errorf("StartDrag with an image sent:\n got %s\nwant %s", got, want)
	}
	if got, want := g.x.iconPix[16:24], []byte{0, 50, 100, 200, 0, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Errorf("ARGB pixels % x, want % x (BGRA, premultiplied)", got, want)
	}
	g.pass()
	g.motion(600, 100)
	if got := g.x.take(); !strings.HasPrefix(got, "move 950 to 599,99; msg XdndEnter to=801") {
		t.Errorf("the image does not follow the pointer: %s", got)
	}
	g.motion(600, 100)
	if got := g.x.take(); strings.Contains(got, "move 950") {
		t.Errorf("the image moved without the pointer: %s", got)
	}
	g.feed(clientMsg(7, a.xdndStatus, 801, 1, 0, 0, uint32(a.xdndActionCopy))) // and the position that waited
	g.feed(clientMsg(7, a.xdndStatus, 801, 1, 0, 0, uint32(a.xdndActionCopy)))
	g.x.take()
	g.releaseButton()
	if got, w := g.x.take(), "msg XdndDrop to=801 win=801 [7 0 50 0 0]; destroy 950; ungrab pointer; ungrab keyboard"; got != w {
		t.Errorf("drop: %s", got)
	}

	// No compositing manager: the root visual, shaped by the opaque
	// pixels, which are unpremultiplied; Escape removes the window.
	g = sourceRig(t)
	g.b.argbVisual = 0x55
	g.x.take()
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy, Image: dragImage()})
	if got := g.x.take(); !strings.Contains(got, "create icon (130,70)-(134,72) rgb bounding=3; ") {
		t.Errorf("StartDrag without compositing sent %s", got)
	}
	if got, want := g.x.iconPix[16:32], []byte{0, 64, 128, 255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 0, 0, 255}; !bytes.Equal(got, want) {
		t.Errorf("opaque pixels % x, want % x", got, want)
	}
	g.pass()
	g.feed(xproto.KeyPressEvent{Event: 7, Detail: 9})
	if got := g.x.take(); !strings.HasPrefix(got, "destroy 950; ungrab pointer") {
		t.Errorf("Escape sent %s", got)
	}

	// Without input shapes the window would hide the target: no image.
	g = sourceRig(t)
	g.x.noShape = true
	g.x.take()
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy, Image: dragImage()})
	if got := g.x.take(); strings.Contains(got, "icon") {
		t.Errorf("an image without input shapes: %s", got)
	}
}

func TestDragIconShape(t *testing.T) {
	if got := opaqueRects(dragImage()); !slices.Equal(got, []xproto.Rectangle{
		{X: 0, Y: 0, Width: 4, Height: 1}, {X: 0, Y: 1, Width: 1, Height: 1}, {X: 3, Y: 1, Width: 1, Height: 1},
	}) {
		t.Errorf("rects = %v", got)
	}
	// Rows with the same runs make one band; an opaque image needs no
	// shape, a transparent one has an empty shape.
	img := image.NewRGBA(image.Rect(0, 0, 5, 4))
	for y := range 4 {
		for x := range 5 {
			if y < 3 || x != 2 {
				img.Pix[img.PixOffset(x, y)+3] = 255
			}
		}
	}
	if got := opaqueRects(img); !slices.Equal(got, []xproto.Rectangle{
		{X: 0, Y: 0, Width: 5, Height: 3}, {X: 0, Y: 3, Width: 2, Height: 1}, {X: 3, Y: 3, Width: 2, Height: 1},
	}) {
		t.Errorf("banded rects = %v", got)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	if got := opaqueRects(img); got != nil {
		t.Errorf("opaque image: %v", got)
	}
	if got := opaqueRects(image.NewRGBA(image.Rect(0, 0, 3, 3))); got == nil || len(got) != 0 {
		t.Errorf("transparent image: %#v", got)
	}

	req := shapeRectanglesRequest(129, shapeSet, shapeBounding, 0x123, []xproto.Rectangle{{X: -1, Y: 2, Width: 3, Height: 4}})
	want := []byte{129, 1, 6, 0, 0, 0, 3, 0, 0x23, 0x01, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 2, 0, 3, 0, 4, 0}
	if !bytes.Equal(req, want) {
		t.Errorf("ShapeRectangles = % x\nwant % x", req, want)
	}
	if req := shapeRectanglesRequest(129, shapeSet, shapeInput, 0x123, nil); len(req) != 16 || req[5] != shapeInput || req[2] != 4 {
		t.Errorf("empty input shape = % x", req)
	}
}

func TestFindARGBVisual(t *testing.T) {
	setup := &xproto.SetupInfo{PixmapFormats: []xproto.Format{{Depth: 24, BitsPerPixel: 32}, {Depth: 32, BitsPerPixel: 32}}}
	screen := &xproto.ScreenInfo{AllowedDepths: []xproto.DepthInfo{
		{Depth: 24, Visuals: []xproto.VisualInfo{{VisualId: 0x21, Class: xproto.VisualClassTrueColor, RedMask: 0xff0000, GreenMask: 0xff00, BlueMask: 0xff}}},
		{Depth: 32, Visuals: []xproto.VisualInfo{
			{VisualId: 0x40, Class: xproto.VisualClassDirectColor, RedMask: 0xff0000, GreenMask: 0xff00, BlueMask: 0xff},
			{VisualId: 0x41, Class: xproto.VisualClassTrueColor, RedMask: 0xff0000, GreenMask: 0xff00, BlueMask: 0xff},
		}},
	}}
	if got := findARGBVisual(setup, screen); got != 0x41 {
		t.Errorf("ARGB visual = %#x, want 0x41", got)
	}
	setup.PixmapFormats = setup.PixmapFormats[:1]
	if got := findARGBVisual(setup, screen); got != 0 {
		t.Errorf("without a 32-bpp format for depth 32: %#x", got)
	}
}

func TestDragSourceAsk(t *testing.T) {
	g := sourceRig(t)
	a := &g.b.atoms
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropCopy | platform.DropMove})
	g.pass()
	g.motion(600, 100)
	g.x.take()
	g.feed(clientMsg(7, a.xdndStatus, 801, 1, 0, 0, uint32(a.xdndActionCopy)))
	g.x.take()
	// Alt alone asks; the target accepts the ask.
	g.x.mask = xproto.KeyButMaskButton1 | xproto.KeyButMaskMod1
	g.feed(xproto.MotionNotifyEvent{Event: 7, RootX: 601, RootY: 100, State: xproto.KeyButMaskButton1 | xproto.KeyButMaskMod1, Time: 45})
	if got, w := g.x.take(), "msg XdndPosition to=801 win=801 [7 0 601,100 45 XdndActionAsk]"; got != w {
		t.Errorf("Alt: %s", got)
	}
	g.feed(clientMsg(7, a.xdndStatus, 801, 1, 0, 0, uint32(a.xdndActionAsk)))
	if got, w := g.x.take(), "cursor context-menu"; got != w {
		t.Errorf("ask accepted: %s", got)
	}
	// The user picks move in the target's menu.
	g.releaseButton()
	g.x.take()
	if got, w := g.feed(clientMsg(7, a.xdndFinished, 801, 1, uint32(a.xdndActionMove))), "done move"; got != w {
		t.Errorf("finish = %q", got)
	}

	// One allowed action: nothing to choose, so Alt does not ask, and
	// no description is set.
	g = sourceRig(t)
	g.x.take()
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropMove | platform.DropLink})
	if got := g.x.take(); !strings.Contains(got, "XdndActionList win=7 ATOM = [XdndActionMove XdndActionLink]") ||
		!strings.Contains(got, `XdndActionDescription win=7 STRING = "Move\x00Link"`) {
		t.Errorf("move|link: %s", got)
	}
	g = sourceRig(t)
	g.x.take()
	g.w.StartDrag(platform.DragOptions{Data: dragData, Allowed: platform.DropLink})
	g.pass()
	g.x.mask = xproto.KeyButMaskButton1 | xproto.KeyButMaskMod1
	g.feed(xproto.MotionNotifyEvent{Event: 7, RootX: 600, RootY: 100, State: xproto.KeyButMaskButton1 | xproto.KeyButMaskMod1, Time: 45})
	if got := g.x.take(); strings.Contains(got, "Ask") || strings.Contains(got, "Description") || !strings.Contains(got, "XdndActionLink]") {
		t.Errorf("link only: %s", got)
	}
}
