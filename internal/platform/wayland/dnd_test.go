//go:build linux

package wayland

import (
	"errors"
	"image"
	"image/color"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// wl_data_offer and wl_data_source event opcodes, and wl_pointer's, used
// by the drag tests (ddEnter and the others are in clipboard_test.go).
const (
	doSourceActions = 1
	doAction        = 2

	dsAction       = 5
	dsDropPerfomed = 3
	dsDndFinished  = 4

	ptrEnter  = 0
	ptrLeave  = 1
	ptrMotion = 2
	ptrButton = 3
	ptrFrame  = 5
)

const (
	actCopy = uint32(protocols.WlDataDeviceManagerDndActionCopy)
	actMove = uint32(protocols.WlDataDeviceManagerDndActionMove)
)

// dragOfferMsg encodes a drag offer's announcement: data_offer, its
// targets and its source_actions.
func (fc *fakeComp) dragOfferMsg(dev, actions uint32, mimes ...string) ([]byte, uint32) {
	b, id := fc.offerMsg(dev, mimes...)
	return append(b, event(id, doSourceActions, actions)...), id
}

// expectDrag waits for the next event, which must be a DragEvent with the
// given action.
func expectDrag(t *testing.T, h *harness, a platform.DragAction) platform.DragEvent {
	t.Helper()
	e := expect[platform.DragEvent](h)
	if e.Action != a {
		t.Fatalf("got Drag%v %+v, want Drag%v", e.Action, e, a)
	}
	return e
}

// answerOf returns the accept and set_actions requests of an offer among
// reqs.
func answerOf(t *testing.T, reqs []freq, offer uint32) (accept, actions freq) {
	t.Helper()
	for _, r := range reqs {
		if r.id != offer {
			continue
		}
		switch {
		case r.is("wl_data_offer.accept"):
			accept = r
		case r.is("wl_data_offer.set_actions"):
			actions = r
		}
	}
	if accept.name == "" {
		t.Fatalf("no accept for offer %d in %v", offer, names(reqs))
	}
	return accept, actions
}

// dropWrite answers the offer's next receive request with data and
// returns the target asked for.
func dropWrite(t *testing.T, h *harness, offer uint32, data string) string {
	t.Helper()
	r := h.fc.until("wl_data_offer.receive")
	rec := r[len(r)-1]
	if rec.id != offer {
		t.Fatalf("receive %v, want from offer %d", rec, offer)
	}
	fd := int(rec.args[1].(fdArg))
	unix.Write(fd, []byte(data))
	unix.Close(fd)
	return rec.args[0].(string)
}

func TestDragTarget(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	// UI-thread state: the types the app wants and the effect it sets
	// while handling a drop.
	want := []string{platform.MIMEText}
	final := platform.DropCopy
	h.onEvent = func(w platform.Window, e platform.Event) {
		de, ok := e.(platform.DragEvent)
		if !ok {
			return
		}
		switch de.Action {
		case platform.DragEnter, platform.DragOver:
			w.SetDropEffect(platform.DefaultDropEffect(de.Allowed, de.Mods), want...)
		case platform.DragDrop:
			w.SetDropEffect(final)
		}
	}
	_, dev := clipSetup(t, h)
	surface := h.fc.id("wl_surface")

	// Enter: the types are normalized, Allowed comes from source_actions,
	// and the answer goes out after the handler: accept with the enter's
	// serial and the best target of the wanted type, then set_actions.
	msg, offer := h.fc.dragOfferMsg(dev, actCopy|actMove, "text/uri-list", "UTF8_STRING", "text/plain;charset=utf-8", "STRING", "TARGETS")
	h.fc.sendRaw(cat(msg, event(dev, ddEnter, uint32(77), surface, 10.0, 20.0, offer)))
	e := expectDrag(t, h, platform.DragEnter)
	if !slices.Equal(e.Types, []string{platform.MIMEURIList, platform.MIMEText}) || e.Allowed != platform.DropCopy|platform.DropMove ||
		e.Pos != (platform.Point{X: 10, Y: 20}) || e.FromApp {
		t.Errorf("DragEnter %+v", e)
	}
	acc, act := answerOf(t, h.fc.until("wl_data_offer.set_actions"), offer)
	if acc.u(0) != 77 || acc.args[1] != "text/plain;charset=utf-8" {
		t.Errorf("accept = %v, want serial 77 and text/plain;charset=utf-8", acc)
	}
	if act.u(0) != actCopy || act.u(1) != actCopy {
		t.Errorf("set_actions = %v, want copy, copy", act)
	}
	h.fc.send(offer, doAction, actCopy)

	// A burst of motion is one DragOver at the latest position, and an
	// unchanged answer is not sent again.
	h.fc.sendRaw(cat(
		event(dev, ddMotion, uint32(5), 11.0, 21.0),
		event(dev, ddMotion, uint32(6), 12.0, 22.0),
		event(dev, ddMotion, uint32(7), 13.0, 23.0),
	))
	if e := expectDrag(t, h, platform.DragOver); e.Pos != (platform.Point{X: 13, Y: 23}) {
		t.Errorf("DragOver at %v, want the latest position", e.Pos)
	}
	h.fc.quiet(50*time.Millisecond, "wl_data_offer.accept")

	// Drop, then the compositor's leave: the wanted type is read through
	// a pipe, DragDrop carries it, then finish and destroy; no leave.
	h.fc.sendRaw(cat(event(dev, ddDrop), event(dev, ddLeave)))
	if got := dropWrite(t, h, offer, "dropped ✓"); got != "text/plain;charset=utf-8" {
		t.Errorf("received %q, want the best text target", got)
	}
	e = expectDrag(t, h, platform.DragDrop)
	if len(e.Data) != 1 || e.Data[0].Type != platform.MIMEText || string(e.Data[0].Bytes) != "dropped ✓" {
		t.Errorf("DragDrop data %+v", e.Data)
	}
	reqs := h.fc.until("wl_data_offer.destroy")
	if _, i := find(reqs, "wl_data_offer.finish"); i < 0 {
		t.Errorf("no finish before destroy: %v", names(reqs))
	}
	if _, i := find(reqs, "wl_data_offer.set_actions"); i >= 0 {
		t.Errorf("set_actions after the drop with an unchanged effect: %v", names(reqs))
	}
	h.noEvent(50 * time.Millisecond)

	// The handler changes the effect while handling the drop: set_actions
	// with it, then finish. Then it refuses: destroy without finish.
	for _, c := range []struct {
		final  platform.DropEffect
		finish bool
	}{{platform.DropMove, true}, {platform.DropNone, false}} {
		h.ui(func() { final = c.final })
		msg, offer := h.fc.dragOfferMsg(dev, actCopy|actMove, "UTF8_STRING")
		h.fc.sendRaw(cat(msg, event(dev, ddEnter, uint32(80), surface, 1.0, 2.0, offer)))
		expectDrag(t, h, platform.DragEnter)
		h.fc.until("wl_data_offer.set_actions")
		h.fc.sendRaw(cat(event(offer, doAction, actCopy), event(dev, ddDrop), event(dev, ddLeave)))
		if got := dropWrite(t, h, offer, "x\x00"); got != "UTF8_STRING" {
			t.Errorf("received %q", got)
		}
		if e := expectDrag(t, h, platform.DragDrop); string(e.Data[0].Bytes) != "x" {
			t.Errorf("DragDrop data %q, want the NUL dropped", e.Data[0].Bytes)
		}
		reqs := h.fc.until("wl_data_offer.destroy")
		fin, i := find(reqs, "wl_data_offer.finish")
		if (i >= 0) != c.finish || fin.name != "" && fin.id != offer {
			t.Errorf("final %v: requests %v", c.final, names(reqs))
		}
		if c.finish {
			if r, j := find(reqs, "wl_data_offer.set_actions"); j < 0 || j > i || r.u(0) != actMove || r.u(1) != actMove {
				t.Errorf("final move: set_actions %v before finish, in %v", r, names(reqs))
			}
		}
		h.noEvent(30 * time.Millisecond)
	}
}

func TestDragTargetRefused(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	h.onEvent = func(w platform.Window, e platform.Event) {
		if de, ok := e.(platform.DragEvent); ok && de.Action != platform.DragLeave {
			w.SetDropEffect(platform.DefaultDropEffect(de.Allowed, de.Mods))
		}
	}
	_, dev := clipSetup(t, h)
	surface := h.fc.id("wl_surface")

	// The source allows nothing yet: accept(null), set_actions(none).
	msg, offer := h.fc.dragOfferMsg(dev, 0, "text/plain")
	h.fc.sendRaw(cat(msg, event(dev, ddEnter, uint32(90), surface, 3.0, 4.0, offer)))
	if e := expectDrag(t, h, platform.DragEnter); e.Allowed != platform.DropNone {
		t.Errorf("DragEnter allows %v", e.Allowed)
	}
	acc, act := answerOf(t, h.fc.until("wl_data_offer.set_actions"), offer)
	if acc.u(0) != 90 || acc.args[1] != "" || act.u(0) != 0 || act.u(1) != 0 {
		t.Errorf("refusal: %v %v, want accept(90, null), set_actions(0, 0)", acc, act)
	}
	// Its actions change: a DragOver with the new set, and the answer
	// follows (the default effect, copy, with the only type offered).
	h.fc.send(offer, doSourceActions, actCopy)
	if e := expectDrag(t, h, platform.DragOver); e.Allowed != platform.DropCopy || e.Pos != (platform.Point{X: 3, Y: 4}) {
		t.Errorf("DragOver %+v", e)
	}
	acc, act = answerOf(t, h.fc.until("wl_data_offer.set_actions"), offer)
	if acc.args[1] != "text/plain" || act.u(0) != actCopy {
		t.Errorf("accept %v, set_actions %v", acc, act)
	}
	// They change back to nothing; the compositor drops anyway (a race):
	// the drop is a DragLeave, nothing is read or finished.
	h.fc.send(offer, doSourceActions, uint32(0))
	expectDrag(t, h, platform.DragOver)
	h.fc.until("wl_data_offer.set_actions")
	h.fc.sendRaw(cat(event(dev, ddDrop), event(dev, ddLeave)))
	expectDrag(t, h, platform.DragLeave)
	reqs := h.fc.until("wl_data_offer.destroy")
	for _, n := range []string{"wl_data_offer.receive", "wl_data_offer.finish"} {
		if _, i := find(reqs, n); i >= 0 {
			t.Errorf("%s after a refused drop: %v", n, names(reqs))
		}
	}
	h.noEvent(30 * time.Millisecond)
}

// TestDragTargetConversions drops the well-known types (none named):
// x-special/gnome-copied-files as a URI list, HTML as is, and a text type
// whose source never sends anything, which is left out after the
// timeout.
func TestDragTargetConversions(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	h.b.transferTimeout = 150 * time.Millisecond
	h.onEvent = func(w platform.Window, e platform.Event) {
		if de, ok := e.(platform.DragEvent); ok && de.Action == platform.DragEnter {
			w.SetDropEffect(platform.DropCopy)
		}
	}
	_, dev := clipSetup(t, h)
	surface := h.fc.id("wl_surface")
	msg, offer := h.fc.dragOfferMsg(dev, actCopy, "x-special/gnome-copied-files", "UTF8_STRING", "text/html", "application/x-custom")
	h.fc.sendRaw(cat(msg, event(dev, ddEnter, uint32(5), surface, 1.0, 1.0, offer)))
	if e := expectDrag(t, h, platform.DragEnter); !slices.Equal(e.Types, []string{platform.MIMEURIList, platform.MIMEText, platform.MIMEHTML, "application/x-custom"}) {
		t.Errorf("Types %q", e.Types)
	}
	acc, _ := answerOf(t, h.fc.until("wl_data_offer.set_actions"), offer)
	if acc.args[1] != "x-special/gnome-copied-files" {
		t.Errorf("accept %v, want the first well-known type's target", acc)
	}
	h.fc.sendRaw(cat(event(offer, doAction, actCopy), event(dev, ddDrop)))
	var silent int
	for range 3 {
		r := h.fc.until("wl_data_offer.receive")
		rec := r[len(r)-1]
		fd := int(rec.args[1].(fdArg))
		switch rec.args[0] {
		case "x-special/gnome-copied-files":
			unix.Write(fd, []byte("copy\nfile:///tmp/a%20b\nfile:///tmp/c"))
		case "text/html":
			unix.Write(fd, []byte("<b>x</b>"))
		case "UTF8_STRING":
			silent = fd // held open: a source that never answers
			continue
		default:
			t.Errorf("unexpected receive %v", rec)
		}
		unix.Close(fd)
	}
	defer unix.Close(silent)
	e := expectDrag(t, h, platform.DragDrop)
	wantData := platform.Data{
		{Type: platform.MIMEURIList, Bytes: []byte("file:///tmp/a%20b\r\nfile:///tmp/c\r\n")},
		{Type: platform.MIMEHTML, Bytes: []byte("<b>x</b>")},
	}
	if len(e.Data) != len(wantData) {
		t.Fatalf("DragDrop data %q", e.Data)
	}
	for i := range wantData {
		if e.Data[i].Type != wantData[i].Type || string(e.Data[i].Bytes) != string(wantData[i].Bytes) {
			t.Errorf("DragDrop entry %d = %s %q, want %s %q", i, e.Data[i].Type, e.Data[i].Bytes, wantData[i].Type, wantData[i].Bytes)
		}
	}
	if paths, ok := e.Data.Paths(); !ok || !slices.Equal(paths, []string{"/tmp/a b", "/tmp/c"}) {
		t.Errorf("Paths = %q", paths)
	}
	h.fc.until("wl_data_offer.finish")
}

// TestDragTargetVersion2 checks a wl_data_device_manager without actions:
// copy is the only effect, no set_actions or finish is sent.
func TestDragTargetVersion2(t *testing.T) {
	g := keyboardGlobals()
	g = append(g, fakeGlobal{iface: "wl_data_device_manager", version: 2})
	h := newHarness(t, g)
	h.onEvent = func(w platform.Window, e platform.Event) {
		if de, ok := e.(platform.DragEvent); ok && de.Action == platform.DragEnter {
			w.SetDropEffect(platform.DefaultDropEffect(de.Allowed, de.Mods))
		}
	}
	_, dev := clipSetup(t, h)
	msg, offer := h.fc.offerMsg(dev, "text/plain")
	h.fc.sendRaw(cat(msg, event(dev, ddEnter, uint32(9), h.fc.id("wl_surface"), 1.0, 1.0, offer)))
	if e := expectDrag(t, h, platform.DragEnter); e.Allowed != platform.DropCopy {
		t.Errorf("Allowed %v, want copy", e.Allowed)
	}
	reqs := h.fc.until("wl_data_offer.accept")
	if acc := reqs[len(reqs)-1]; acc.args[1] != "text/plain" {
		t.Errorf("accept %v", acc)
	}
	h.fc.send(dev, ddDrop)
	dropWrite(t, h, offer, "v2")
	if e := expectDrag(t, h, platform.DragDrop); string(e.Data[0].Bytes) != "v2" {
		t.Errorf("data %q", e.Data)
	}
	reqs = h.fc.until("wl_data_offer.destroy")
	for _, n := range []string{"wl_data_offer.set_actions", "wl_data_offer.finish"} {
		if _, i := find(reqs, n); i >= 0 {
			t.Errorf("%s on a version 2 offer: %v", n, names(reqs))
		}
	}
}

// dragImage is a 4x4 image with a distinct premultiplied pixel at (0, 0).
func dragImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.SetRGBA(0, 0, color.RGBA{R: 0x40, G: 0x20, B: 0x10, A: 0x80})
	return img
}

func TestDragSource(t *testing.T) {
	g := append(standardGlobals(1), fakeGlobal{iface: "wl_data_device_manager", version: 3})
	h := newHarness(t, g)
	data := platform.Data{{Type: platform.MIMEHTML, Bytes: []byte("<i>drag</i>")}, {Type: platform.MIMEText, Bytes: []byte("drag ✓")}}
	started := make(chan error, 4)
	h.onEvent = func(w platform.Window, e platform.Event) {
		if pe, ok := e.(platform.PointerEvent); ok && pe.Action == platform.PointerDown {
			started <- w.StartDrag(platform.DragOptions{Data: data, Allowed: platform.DropCopy | platform.DropMove, Image: dragImage(), Hotspot: image.Pt(1, 2)})
		}
	}
	w := openWindow(t, h, platform.WindowOptions{}, 100, 100, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	ptr, surface := h.fc.id("wl_pointer"), h.fc.id("wl_surface")

	// Without a held press there is no grab.
	h.ui(func() {
		if err := w.StartDrag(platform.DragOptions{Data: data, Allowed: platform.DropCopy}); !errors.Is(err, platform.ErrNoGrab) {
			t.Errorf("StartDrag without a press = %v, want ErrNoGrab", err)
		}
	})

	h.fc.sendRaw(cat(
		event(ptr, ptrEnter, uint32(30), surface, 5.0, 5.0), event(ptr, ptrFrame),
		event(ptr, ptrButton, uint32(31), uint32(0), uint32(btnLeft), uint32(1)), event(ptr, ptrFrame),
		// Motion queued behind the press: the drag takes it.
		event(ptr, ptrMotion, uint32(1), 6.0, 6.0), event(ptr, ptrFrame),
	))
	if err := <-started; err != nil {
		t.Fatalf("StartDrag from PointerDown = %v", err)
	}
	for _, a := range []platform.PointerAction{platform.PointerEnter, platform.PointerDown, platform.PointerCancel, platform.PointerLeave} {
		if e := expect[platform.PointerEvent](h); e.Action != a {
			t.Errorf("got %v, want %v", e.Action, a)
		}
	}
	reqs := h.fc.until("wl_data_device.start_drag")
	src := h.fc.id("wl_data_source")
	var offered []string
	var setActions freq
	for _, r := range reqs {
		switch {
		case r.is("wl_data_source.offer") && r.id == src:
			offered = append(offered, r.args[0].(string))
		case r.is("wl_data_source.set_actions"):
			setActions = r
		}
	}
	wantTargets := append([]string{platform.MIMEHTML}, platform.SelectionTargets(platform.MIMEText)...)
	if len(offered) != len(wantTargets)+1 || !slices.Equal(offered[:len(wantTargets)], wantTargets) {
		t.Errorf("offered %q, want %q and the marker", offered, wantTargets)
	}
	if setActions.name == "" || setActions.u(0) != actCopy|actMove {
		t.Errorf("set_actions = %v, want copy|move before start_drag", setActions)
	}
	sd := reqs[len(reqs)-1]
	icon := sd.u(2)
	if sd.u(0) != src || sd.u(1) != surface || icon == 0 || icon == surface || sd.u(3) != 31 {
		t.Errorf("start_drag = %v, want source %d, origin %d, an icon surface, the press serial 31", sd, src, surface)
	}
	// The icon: an ARGB8888 buffer offset by -hotspot, damaged, committed.
	reqs = h.fc.until("wl_surface.commit")
	att, _ := find(reqs, "wl_surface.attach")
	off, _ := find(reqs, "wl_surface.offset")
	if att.id != icon || off.id != icon || off.i(0) != -1 || off.i(1) != -2 {
		t.Errorf("icon requests %v", reqs)
	}
	if buf := h.fc.buffer(att.u(0)); buf.format != uint32(protocols.WlShmFormatArgb8888) || buf.w != 4 || buf.h != 4 {
		t.Errorf("icon buffer %+v", buf)
	} else if px := h.fc.pixels(att.u(0)); px[0] != 0x10 || px[1] != 0x20 || px[2] != 0x40 || px[3] != 0x80 {
		t.Errorf("icon pixel %x, want premultiplied BGRA 10 20 40 80", px[:4])
	}

	// One drag at a time.
	h.ui(func() {
		if err := w.StartDrag(platform.DragOptions{Data: data, Allowed: platform.DropCopy}); !errors.Is(err, platform.ErrNoGrab) {
			t.Errorf("StartDrag during a drag = %v, want ErrNoGrab", err)
		}
	})
	// The compositor's pointer leave and events of the grab are silent.
	h.fc.sendRaw(cat(event(ptr, ptrLeave, uint32(32), surface), event(ptr, ptrFrame)))
	h.noEvent(30 * time.Millisecond)
	// The source serves its data like a selection.
	if got := waitBytes(t, readAllFd(requestSend(t, h, src, "STRING"))); string(got) != "drag ?" {
		t.Errorf("STRING = %q", got)
	}
	if got := waitBytes(t, readAllFd(requestSend(t, h, src, "text/html"))); string(got) != "<i>drag</i>" {
		t.Errorf("text/html = %q", got)
	}
	// The target took a move: DragDoneEvent reports it, and the source and
	// the icon are destroyed.
	h.fc.sendRaw(cat(event(src, dsAction, actCopy), event(src, dsAction, actMove), event(src, dsDropPerfomed), event(src, dsDndFinished)))
	if e := expect[platform.DragDoneEvent](h); e.Effect != platform.DropMove {
		t.Errorf("DragDoneEvent %v, want move", e.Effect)
	}
	reqs = h.fc.until("wl_surface.destroy")
	if r := reqs[len(reqs)-1]; r.id != icon {
		t.Errorf("destroyed %v, want the icon surface", r)
	}
	if _, i := find(reqs, "wl_data_source.destroy"); i < 0 {
		t.Errorf("the source was not destroyed: %v", names(reqs))
	}
	// The pointer comes back.
	h.fc.sendRaw(cat(event(ptr, ptrEnter, uint32(40), surface, 7.0, 7.0), event(ptr, ptrFrame)))
	if e := expect[platform.PointerEvent](h); e.Action != platform.PointerEnter {
		t.Errorf("got %v, want enter", e.Action)
	}

	// A drag the compositor cancels at once (no pointer leave came): the
	// pointer is given back with PointerEnter, and its events flow again.
	h.fc.sendRaw(cat(event(ptr, ptrButton, uint32(41), uint32(0), uint32(btnLeft), uint32(1)), event(ptr, ptrFrame)))
	if err := <-started; err != nil {
		t.Fatalf("second StartDrag = %v", err)
	}
	for _, a := range []platform.PointerAction{platform.PointerDown, platform.PointerCancel, platform.PointerLeave} {
		if e := expect[platform.PointerEvent](h); e.Action != a {
			t.Errorf("got %v, want %v", e.Action, a)
		}
	}
	reqs = h.fc.until("wl_data_device.start_drag")
	if sd := reqs[len(reqs)-1]; sd.u(3) != 41 {
		t.Errorf("start_drag serial %d, want the new press 41", sd.u(3))
	}
	h.fc.send(h.fc.id("wl_data_source"), dsCancelled)
	if e := expect[platform.DragDoneEvent](h); e.Effect != platform.DropNone {
		t.Errorf("DragDoneEvent %v, want none", e.Effect)
	}
	if e := expect[platform.PointerEvent](h); e.Action != platform.PointerEnter {
		t.Errorf("got %v, want the pointer back", e.Action)
	}
	h.fc.sendRaw(cat(event(ptr, ptrMotion, uint32(2), 8.0, 8.0), event(ptr, ptrFrame)))
	if e := expect[platform.PointerEvent](h); e.Action != platform.PointerMove {
		t.Errorf("got %v, want move", e.Action)
	}
}

// TestDragOwnWindow drags from a window onto itself: the target side sees
// FromApp, the drop reads the data from memory, and the source's outcome
// follows the target's finish.
func TestDragOwnWindow(t *testing.T) {
	g := append(standardGlobals(1), fakeGlobal{iface: "wl_data_device_manager", version: 3})
	h := newHarness(t, g)
	data := platform.Data{{Type: platform.MIMEHTML, Bytes: []byte("<i>own</i>")}, {Type: platform.MIMEText, Bytes: []byte("own")}}
	started := make(chan error, 1)
	h.onEvent = func(w platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.PointerEvent:
			if e.Action == platform.PointerDown {
				started <- w.StartDrag(platform.DragOptions{Data: data, Allowed: platform.DropCopy | platform.DropMove})
			}
		case platform.DragEvent:
			if e.Action == platform.DragEnter {
				w.SetDropEffect(platform.DropMove, platform.MIMEHTML)
			}
		}
	}
	openWindow(t, h, platform.WindowOptions{}, 100, 100, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	ptr, surface, dev := h.fc.id("wl_pointer"), h.fc.id("wl_surface"), h.fc.id("wl_data_device")
	h.fc.sendRaw(cat(
		event(ptr, ptrEnter, uint32(30), surface, 5.0, 5.0), event(ptr, ptrFrame),
		event(ptr, ptrButton, uint32(31), uint32(0), uint32(btnLeft), uint32(1)), event(ptr, ptrFrame),
	))
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	reqs := h.fc.until("wl_data_device.start_drag")
	src := h.fc.id("wl_data_source")
	var offered []string
	for _, r := range reqs {
		if r.is("wl_data_source.offer") {
			offered = append(offered, r.args[0].(string))
		}
	}
	if _, i := find(reqs, "wl_compositor.create_surface"); i >= 0 {
		t.Errorf("an icon surface without an image: %v", names(reqs))
	}
	// The compositor: pointer leave, then the drag enters the window.
	msg, offer := h.fc.dragOfferMsg(dev, actCopy|actMove, offered...)
	h.fc.sendRaw(cat(event(ptr, ptrLeave, uint32(32), surface), event(ptr, ptrFrame),
		msg, event(dev, ddEnter, uint32(50), surface, 5.0, 5.0, offer)))
	for _, a := range []platform.PointerAction{platform.PointerEnter, platform.PointerDown, platform.PointerCancel, platform.PointerLeave} {
		if e := expect[platform.PointerEvent](h); e.Action != a {
			t.Errorf("got %v, want %v", e.Action, a)
		}
	}
	e := expectDrag(t, h, platform.DragEnter)
	if !e.FromApp || !slices.Equal(e.Types, []string{platform.MIMEHTML, platform.MIMEText}) {
		t.Errorf("DragEnter %+v, want FromApp and our types without the marker", e)
	}
	acc, act := answerOf(t, h.fc.until("wl_data_offer.set_actions"), offer)
	if acc.args[1] != "text/html" || act.u(0) != actMove {
		t.Errorf("accept %v, set_actions %v", acc, act)
	}
	h.fc.sendRaw(cat(event(offer, doAction, actMove), event(src, dsAction, actMove), event(dev, ddDrop), event(dev, ddLeave)))
	e = expectDrag(t, h, platform.DragDrop)
	if len(e.Data) != 1 || e.Data[0].Type != platform.MIMEHTML || string(e.Data[0].Bytes) != "<i>own</i>" {
		t.Errorf("DragDrop data %q", e.Data)
	}
	reqs = h.fc.until("wl_data_offer.destroy")
	if _, i := find(reqs, "wl_data_offer.receive"); i >= 0 {
		t.Errorf("our own data was read through the compositor: %v", names(reqs))
	}
	if _, i := find(reqs, "wl_data_offer.finish"); i < 0 {
		t.Errorf("no finish: %v", names(reqs))
	}
	h.fc.sendRaw(cat(event(src, dsDropPerfomed), event(src, dsDndFinished)))
	if e := expect[platform.DragDoneEvent](h); e.Effect != platform.DropMove {
		t.Errorf("DragDoneEvent %v", e.Effect)
	}
	h.noEvent(30 * time.Millisecond)
}

func TestClipboardData(t *testing.T) {
	h := newHarness(t, clipboardGlobals(false))
	_, dev := clipSetup(t, h)
	data := platform.Data{
		{Type: platform.MIMEHTML, Bytes: []byte("<b>x</b>")},
		{Type: platform.MIMEText, Bytes: []byte("x ✓")},
		{Type: platform.MIMEURIList, Bytes: []byte("file:///a\r\nfile:///b%20c\r\n")},
		{Type: "application/x-tela-test", Bytes: []byte{1, 2, 3}},
	}
	h.ui(func() {
		if err := h.b.Clipboard().Write(data); err != nil {
			t.Errorf("Write: %v", err)
		}
	})
	reqs := h.fc.until("wl_data_device.set_selection")
	src := h.fc.id("wl_data_source")
	var offered []string
	for _, r := range reqs {
		if r.is("wl_data_source.offer") {
			offered = append(offered, r.args[0].(string))
		}
	}
	want := []string{"text/html", "text/plain;charset=utf-8", "UTF8_STRING", "text/plain", "TEXT", "STRING",
		"text/uri-list", "x-special/gnome-copied-files", "application/x-tela-test"}
	if len(offered) != len(want)+1 || !slices.Equal(offered[:len(want)], want) {
		t.Errorf("offered %q, want %q and the marker", offered, want)
	}
	for _, c := range []struct{ target, want string }{
		{"text/html", "<b>x</b>"},
		{"TEXT", "x ✓"},
		{"STRING", "x ?"},
		{"x-special/gnome-copied-files", "copy\nfile:///a\nfile:///b%20c"},
		{"application/x-tela-test", "\x01\x02\x03"},
	} {
		if got := waitBytes(t, readAllFd(requestSend(t, h, src, c.target))); string(got) != c.want {
			t.Errorf("%s = %q, want %q", c.target, got, c.want)
		}
	}

	// Our own selection is read from memory.
	type typesResult struct {
		types []string
		err   error
	}
	type readResult struct {
		data []byte
		err  error
	}
	readTypes := func() typesResult {
		ch := make(chan typesResult, 1)
		h.ui(func() {
			if err := h.b.Clipboard().ReadTypes(func(ts []string, err error) { ch <- typesResult{ts, err} }); err != nil {
				t.Errorf("ReadTypes: %v", err)
			}
		})
		select {
		case r := <-ch:
			return r
		case <-time.After(3 * time.Second):
			t.Fatal("ReadTypes never called back")
		}
		return typesResult{}
	}
	read := func(mime string) <-chan readResult {
		ch := make(chan readResult, 1)
		h.ui(func() {
			if err := h.b.Clipboard().Read(mime, func(b []byte, err error) { ch <- readResult{b, err} }); err != nil {
				t.Errorf("Read: %v", err)
			}
		})
		return ch
	}
	waitRead := func(ch <-chan readResult) readResult {
		t.Helper()
		select {
		case r := <-ch:
			return r
		case <-time.After(3 * time.Second):
			t.Fatal("Read never called back")
		}
		return readResult{}
	}
	if r := readTypes(); r.err != nil || !slices.Equal(r.types, data.Types()) {
		t.Errorf("own ReadTypes = %q, %v", r.types, r.err)
	}
	if r := waitRead(read(platform.MIMEURIList)); r.err != nil || string(r.data) != string(data[2].Bytes) {
		t.Errorf("own Read = %q, %v", r.data, r.err)
	}
	if r := waitRead(read(platform.MIMEPNG)); r.err != nil || r.data != nil {
		t.Errorf("own Read of a missing type = %q, %v; want nil, nil", r.data, r.err)
	}
	h.fc.quiet(30*time.Millisecond, "wl_data_offer.receive")

	// Another client's selection: types normalized (aliases merged, meta
	// targets and markers left out), each read from its best target and
	// converted.
	sel := h.fc.setSelection(dev, "TARGETS", "x-special/gnome-copied-files", "STRING", "text/html; charset=utf-8",
		"UTF8_STRING", "application/x-tela-source-1-1", "image/png")
	if r := readTypes(); r.err != nil || !slices.Equal(r.types, []string{platform.MIMEURIList, platform.MIMEText, "text/html;charset=utf-8", platform.MIMEPNG}) {
		t.Errorf("ReadTypes = %q, %v", r.types, r.err)
	}
	for _, c := range []struct{ mime, target, send, want string }{
		{platform.MIMEText, "UTF8_STRING", "theirs ✓", "theirs ✓"},
		{platform.MIMEURIList, "x-special/gnome-copied-files", "cut\nfile:///x\nfile:///y", "file:///x\r\nfile:///y\r\n"},
		{platform.MIMEPNG, "image/png", "\x89PNG", "\x89PNG"},
		{platform.MIMEPNG, "image/png", "", ""}, // offered but empty: not nil
	} {
		ch := read(c.mime)
		if got := dropWrite(t, h, sel, c.send); got != c.target {
			t.Errorf("Read(%s) received %q, want %q", c.mime, got, c.target)
		}
		if r := waitRead(ch); r.err != nil || r.data == nil || string(r.data) != c.want {
			t.Errorf("Read(%s) = %q (nil %v), %v; want %q", c.mime, r.data, r.data == nil, r.err, c.want)
		}
	}
	if r := waitRead(read(platform.MIMEHTML)); r.err != nil || r.data != nil {
		t.Errorf("Read of a type not offered = %q, %v", r.data, r.err)
	}

	// Writing empty data clears the selection.
	h.ui(func() {
		if err := h.b.Clipboard().Write(nil); err != nil {
			t.Errorf("Write(nil): %v", err)
		}
	})
	reqs = h.fc.until("wl_data_device.set_selection")
	if r := reqs[len(reqs)-1]; r.u(0) != 0 {
		t.Errorf("clearing: %v, want a null source", r)
	}
	if r := readTypes(); r.err != nil || r.types != nil {
		t.Errorf("ReadTypes after clearing = %q, %v", r.types, r.err)
	}
}

func TestClipboardDataUnsupported(t *testing.T) {
	h := newHarness(t, keyboardGlobals())
	kbSetup(t, h, "us", 0, 0)
	h.ui(func() {
		c := h.b.Clipboard()
		if err := c.Write(platform.TextData("x")); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("Write without a data device = %v", err)
		}
		if err := c.ReadTypes(func([]string, error) { t.Error("ReadTypes called back") }); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("ReadTypes without a data device = %v", err)
		}
		if err := c.Read(platform.MIMEText, func([]byte, error) { t.Error("Read called back") }); !errors.Is(err, platform.ErrUnsupported) {
			t.Errorf("Read without a data device = %v", err)
		}
	})
}

func TestActivateWithToken(t *testing.T) {
	h := newHarness(t, ctlGlobals("xdg_activation_v1"))
	w, _ := openCtl(t, h, platform.WindowOptions{}, 200, 100, 1)
	expect[platform.ConfigureEvent](h)
	expect[platform.FrameEvent](h)
	surface := h.fc.id("wl_surface")

	// Not mapped yet: the activation waits for the first buffer.
	h.ui(func() { w.ActivateWithToken("early-token") })
	h.fc.quiet(30*time.Millisecond, "xdg_activation_v1.activate")
	h.ui(func() {
		if err := w.Present(solid(w, 1, 2, 3), nil); err != nil {
			t.Error(err)
		}
	})
	reqs := h.fc.until("xdg_activation_v1.activate")
	if act := reqs[len(reqs)-1]; act.args[0] != "early-token" || act.u(1) != surface {
		t.Errorf("activate = %v", act)
	}

	// Mapped: activate with the token at once, without a token of our own.
	h.ui(func() { w.ActivateWithToken("launch-token") })
	reqs = h.fc.until("xdg_activation_v1.activate")
	if act := reqs[len(reqs)-1]; act.args[0] != "launch-token" || act.u(1) != surface {
		t.Errorf("activate = %v", act)
	}
	if _, i := find(reqs, "xdg_activation_v1.get_activation_token"); i >= 0 {
		t.Errorf("asked for a token although one was given: %v", names(reqs))
	}
	// "" is Activate: a token of our own.
	h.ui(func() { w.ActivateWithToken("") })
	h.fc.until("xdg_activation_token_v1.commit")
	// A hidden window is not activated.
	h.ui(func() {
		w.SetVisible(false)
		w.ActivateWithToken("hidden-token")
	})
	h.fc.quiet(30*time.Millisecond, "xdg_activation_v1.activate")
}
