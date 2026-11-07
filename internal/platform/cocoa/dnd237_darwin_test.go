//go:build darwin

package cocoa

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The drag and drop tests of ADR-237: another app's private types, file
// promises, and the icons of a drag without an image. They drive the
// destination methods with the fake dragging info of dnd_darwin_test.go
// and log what AppKit does, since a real drag of another app needs a
// person.

// TestDropTargetPrivateTypes: a drag that offers only a private type of
// another app is seen (the views are registered for the promise types and
// the hierarchy's roots, and a drag's types are learned), its type is the
// "application/x-uti." Data type, and the drop reads it.
func TestDropTargetPrivateTypes(t *testing.T) {
	const private = "com.example.tela-private"
	mime := utiMIMEPrefix + private
	var before, after []string
	var monitor bool
	var pb darwin.ID
	onEvent := func(r *dndRig, e platform.Event) {
		if d, ok := e.(platform.DragEvent); ok && d.Action == platform.DragEnter {
			r.w.SetDropEffect(platform.DropCopy, mime)
		}
	}
	steps := []dndStep{
		{name: "learn the private type, then drop it", fn: func(r *dndRig) {
			monitor = r.b.dnd.monitor != 0
			before = stringsOf(darwin.Send(r.w.view, darwin.Sel("registeredDraggedTypes")))
			pb = newDragPasteboard(t, pbItem{{uti: private, data: []byte("private bytes")}})
			r.b.learnDragTypes(pb) // what the global monitor does as another app's drag starts
			after = stringsOf(darwin.Send(r.w.view, darwin.Sel("registeredDraggedTypes")))
			fakeDrag.pb, fakeDrag.source = pb, 0
			r.drag(platform.Point{X: 30, Y: 30}, opCopy)
			r.call("draggingEntered:")
			r.call("performDragOperation:")
			r.call("draggingEnded:")
		}},
	}
	r := runDnD(t, "tela private types", onEvent, steps...)
	onMain(func() {
		releaseDragPasteboard(pb)
		fakeDrag.pb = 0
	})
	if r == nil {
		return
	}
	t.Logf("global drag monitor installed: %v; registered before %d types, after learning %d", monitor, len(before), len(after))
	t.Logf("answers: %q; events: %q", r.answers, r.eventLines())
	if !monitor {
		t.Errorf("no global monitor of left mouse drags")
	}
	for _, want := range append([]string{"com.apple.NSFilePromiseItemMetaData"}, conformanceRoots...) {
		if !slices.Contains(before, want) {
			t.Errorf("the view is not registered for %s", want)
		}
	}
	if !slices.Contains(after, private) {
		t.Errorf("the learned type %s is not registered: %q", private, after)
	}
	var drop *platform.DragEvent
	for _, d := range r.dragEvents() {
		t.Logf("%s types %q", dragString(d), d.Types)
		if d.Action == platform.DragEnter && !slices.Contains(d.Types, mime) {
			t.Errorf("DragEnter types %q lack %s", d.Types, mime)
		}
		if d.Action == platform.DragDrop {
			drop = &d
		}
	}
	if drop == nil {
		t.Fatalf("no DragDrop")
	}
	if got, _ := drop.Data.Get(mime); string(got) != "private bytes" {
		t.Errorf("dropped %s = %q, want the private bytes", mime, got)
	}
}

// promiseTest is the state of the file promise test's provider delegate.
var promiseTest struct {
	once   sync.Once
	class  darwin.Class
	err    error
	writes int
}

// promiseDelegateClass registers TelaTestPromiseDelegate: an
// NSFilePromiseProviderDelegate that names the file and writes it.
func promiseDelegateClass() (darwin.Class, error) {
	promiseTest.once.Do(func() {
		promiseTest.class, promiseTest.err = darwin.RegisterClass("TelaTestPromiseDelegate", darwin.GetClass("NSObject"), nil, []darwin.Method{
			{Sel: darwin.Sel("filePromiseProvider:fileNameForType:"), Fn: func(_ darwin.ID, _ darwin.SEL, _, _ darwin.ID) darwin.ID {
				return darwin.AutoString("promised.txt")
			}},
			{Sel: darwin.Sel("filePromiseProvider:writePromiseToURL:completionHandler:"), Fn: func(_ darwin.ID, _ darwin.SEL, _, url, handler darwin.ID) {
				promiseTest.writes++
				err := os.WriteFile(darwin.URLPath(url), []byte("promised bytes"), 0o644)
				if err != nil {
					fmt.Fprintf(os.Stderr, "promise delegate: %v\n", err)
				}
				darwin.CallBlock(handler, 0)
			}},
		})
	})
	return promiseTest.class, promiseTest.err
}

// TestDropPromise drops a file promise (an NSFilePromiseProvider written
// to the fake drag's pasteboard): the types are a URI list, the drop
// answers at once without an event, the window refuses drags while it
// waits, and DragDrop brings the received file. Whether the receiver
// fulfils a promise outside a real drag session is AppKit's: the test
// logs what happens and checks the rules that hold either way. The drop
// folder is gone after Run.
func TestDropPromise(t *testing.T) {
	saved := promiseTimeout
	promiseTimeout = 4 * time.Second
	t.Cleanup(func() { promiseTimeout = saved })
	var pb, provider darwin.ID
	var folders []string
	var pendingAfterDrop bool
	var refusedWhilePending string
	onEvent := func(r *dndRig, e platform.Event) {
		if d, ok := e.(platform.DragEvent); ok && d.Action == platform.DragEnter {
			r.w.SetDropEffect(platform.DropCopy, platform.MIMEURIList)
		}
	}
	steps := []dndStep{
		{name: "drop a file promise", fn: func(r *dndRig) {
			cls, err := promiseDelegateClass()
			if err != nil {
				t.Errorf("registering the promise delegate: %v", err)
				return
			}
			del := darwin.Send(darwin.SendClass(cls, sel.alloc), sel.init)
			provider = darwin.Send(darwin.SendClass(darwin.GetClass("NSFilePromiseProvider"), sel.alloc),
				darwin.Sel("initWithFileType:delegate:"), darwin.AutoString("public.plain-text"), del)
			if provider == 0 {
				t.Errorf("NSFilePromiseProvider could not be made")
				return
			}
			pb = darwin.Retain(darwin.SendClass(darwin.GetClass("NSPasteboard"), darwin.Sel("pasteboardWithUniqueName")))
			darwin.Send(pb, pbSel.clearContents)
			ok := darwin.SendBool(pb, pbSel.writeObjects, darwin.NSArray(provider))
			var lists [][]string
			for _, it := range items(pb) {
				lists = append(lists, stringsOf(darwin.Send(it, pbSel.types)))
			}
			t.Logf("promise pasteboard (written %v): %q; readable promise types %q; receivers %v", ok, lists, promiseReadableTypes(), promiseReceivers(pb) != 0)
			fakeDrag.pb, fakeDrag.source = pb, 0
			r.drag(platform.Point{X: 40, Y: 40}, opCopy)
			r.call("draggingEntered:")
			r.call("performDragOperation:")
			r.call("draggingEnded:")
			pendingAfterDrop = r.w.dnd.promised != nil
			folders = slices.Clone(r.b.dnd.folders)
			if pendingAfterDrop {
				r.call("draggingEntered:") // refused while waiting
				refusedWhilePending = r.answers[len(r.answers)-1]
			}
		}},
	}
	for range 30 { // 6 s: the promise or the shortened timeout
		steps = append(steps, dndStep{name: "wait", fn: func(r *dndRig) {}})
	}
	r := runDnD(t, "tela file promise", onEvent, steps...)
	onMain(func() {
		releaseDragPasteboard(pb)
		darwin.Release(provider)
		fakeDrag.pb = 0
	})
	if r == nil {
		return
	}
	t.Logf("answers %q; events %q; waiting after the drop %v; delegate writes %d; folders %q",
		r.answers, r.eventLines(), pendingAfterDrop, promiseTest.writes, folders)
	drags := r.dragEvents()
	if len(drags) == 0 || drags[0].Action != platform.DragEnter || !slices.Contains(drags[0].Types, platform.MIMEURIList) {
		t.Fatalf("drag events %q; want a DragEnter offering %s", r.eventLines(), platform.MIMEURIList)
	}
	if len(r.answers) < 2 || r.answers[1] != "performDragOperation: true" {
		t.Errorf("answers %q, want the drop accepted", r.answers)
	}
	if !pendingAfterDrop {
		t.Logf("note: the pasteboard gave no NSFilePromiseReceiver; the drop was delivered at once")
	} else if refusedWhilePending != "draggingEntered: 0x0" {
		t.Errorf("a drag while the promise waits got %q, want draggingEntered: 0x0", refusedWhilePending)
	}
	var drop *platform.DragEvent
	for i := range drags {
		if drags[i].Action == platform.DragDrop {
			drop = &drags[i]
		}
		if drags[i].Action == platform.DragLeave {
			t.Errorf("a DragLeave in a drop's session: %q", r.eventLines())
		}
	}
	if drop == nil {
		t.Fatalf("no DragDrop within the run (the promise timeout is %v)", promiseTimeout)
	}
	files := platform.ParseURIList(func() []byte { b, _ := drop.Data.Get(platform.MIMEURIList); return b }())
	t.Logf("dropped files %q", files)
	for _, u := range files {
		p, err := platform.FilePath(u)
		if err != nil || !strings.Contains(p, "tela-drops") {
			t.Errorf("dropped %q, want a file in a tela-drops folder", u)
		}
	}
	if promiseTest.writes > 0 && len(files) != 1 {
		t.Errorf("the delegate wrote the promise, but the drop has %d files", len(files))
	}
	for _, dir := range folders {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("the drop folder %s is still there after Run (%v)", filepath.Base(dir), err)
		}
	}
}

// TestDragIconImages: a drag without an image shows a file's icon and a
// type's icon, 32 points square.
func TestDragIconImages(t *testing.T) {
	newBackend(t)
	file := filepath.Join(t.TempDir(), "icon.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	onMain(func() {
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		imgs := dragIconImages([]dragIcon{{path: file}, {uti: utiUTF8Text}, {uti: "com.example.no-such-type"}})
		for i, img := range imgs {
			if img == 0 {
				t.Errorf("icon %d: no image", i)
				continue
			}
			size := darwin.SendPoint(img, darwin.Sel("size"))
			t.Logf("icon %d: %vx%v points", i, size.X, size.Y)
			if size.X != dragIconPoints || size.Y != dragIconPoints {
				t.Errorf("icon %d is %vx%v points, want %d", i, size.X, size.Y, dragIconPoints)
			}
		}
	})
}
