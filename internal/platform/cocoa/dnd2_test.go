package cocoa

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// Tests of the drag and drop rules of ADR-237: every drag seen (promise
// types, conformance roots, learned types), private types as Data,
// file promises and the icons of drags without an image.

func TestDraggedTypesPromisesAndRoots(t *testing.T) {
	got := draggedTypes([]string{"promise.extra", utiFileURL}, []string{"learned.a"})
	for _, want := range append(append(slices.Clone(promiseTypes), conformanceRoots...), "promise.extra", "learned.a", utiUTF8Text) {
		if !slices.Contains(got, want) {
			t.Errorf("draggedTypes lacks %s: %q", want, got)
		}
	}
	if slices.Index(got, "promise.extra") > slices.Index(got, "public.item") || got[len(got)-1] != "learned.a" {
		t.Errorf("order: promises, then the roots, then the extra types: %q", got)
	}
	if n := len(slices.Compact(slices.Sorted(slices.Values(got)))); n != len(got) {
		t.Errorf("draggedTypes has duplicates: %q", got)
	}
}

func TestLearnTypes(t *testing.T) {
	registered := draggedTypes(nil, nil)
	learned, changed := learnTypes(nil, registered, []string{utiFileURL, "com.example.private", "", "com.example.private", "dyn.ah62d4rv4gu8y"})
	if !changed || !slices.Equal(learned, []string{"com.example.private", "dyn.ah62d4rv4gu8y"}) {
		t.Errorf("learnTypes = %q, %v", learned, changed)
	}
	if again, changed := learnTypes(learned, registered, []string{"com.example.private"}); changed || len(again) != 2 {
		t.Errorf("learning a known type again = %q, %v; want no change", again, changed)
	}
	var many []string
	for i := range maxLearnedTypes + 10 {
		many = append(many, fmt.Sprintf("com.example.t%d", i))
	}
	got, _ := learnTypes(nil, registered, many)
	if len(got) != maxLearnedTypes || got[0] != "com.example.t10" || got[len(got)-1] != many[len(many)-1] {
		t.Errorf("learnTypes keeps %d types from %s to %s; want the latest %d", len(got), got[0], got[len(got)-1], maxLearnedTypes)
	}
}

func TestPrivateMIME(t *testing.T) {
	for _, uti := range []string{"com.adobe.photoshop-layers", "com.apple.finder.node", "com.Example.Thing2"} {
		m, ok := privateMIME(uti)
		if !ok || m != utiMIMEPrefix+uti || !validMIME(m) {
			t.Errorf("privateMIME(%q) = %q, %v", uti, m, ok)
			continue
		}
		if back, ok := privateUTI(m); !ok || back != uti {
			t.Errorf("privateUTI(%q) = %q, %v; want %q", m, back, ok, uti)
		}
	}
	for _, uti := range []string{"", "dyn.ah62d4rv4gu8y", "DYN.x.y", "Apple files promise pasteboard type", "com.apple.NSFilePromiseItemMetaData",
		"NSStringPboardType", "com.example/x", "com.example.ü", strings.Repeat("a.", 130)} {
		if m, ok := privateMIME(uti); ok {
			t.Errorf("privateMIME(%q) = %q, want none", uti, m)
		}
	}
	for _, m := range []string{"application/json", "application/x-uti.", "application/x-uti.dyn.x", "application/x-uti.a b"} {
		if uti, ok := privateUTI(m); ok {
			t.Errorf("privateUTI(%q) = %q, want none", m, uti)
		}
	}
}

func TestDragMIME(t *testing.T) {
	base := func(uti string) (string, bool) {
		if uti == "public.json" {
			return "application/json", true
		}
		return privateMIME(uti)
	}
	mime := dragMIME([]string{"com.example.promise"}, base)
	items := [][]string{
		{"com.apple.NSFilePromiseItemMetaData", "com.apple.pasteboard.promised-file-url", "public.jpeg-private.x"},
		{"com.example.promise", "public.json", utiUTF8Text},
	}
	got := offeredTypes(items, mime)
	want := []string{platform.MIMEURIList, utiMIMEPrefix + "public.jpeg-private.x", "application/json", platform.MIMEText}
	if !slices.Equal(got, want) {
		t.Errorf("offeredTypes of a promise drag = %q, want %q", got, want)
	}
	// The clipboard's mapping does not report promises.
	if got := offeredTypes(items[:1], base); !slices.Equal(got, []string{utiMIMEPrefix + "public.jpeg-private.x"}) {
		t.Errorf("offeredTypes without the drag mapping = %q", got)
	}
}

func TestDragIcons(t *testing.T) {
	posixPaths(t)
	items := pasteboardItems(platform.Data{
		{Type: platform.MIMEText, Bytes: []byte("hi")},
		{Type: platform.MIMEURIList, Bytes: platform.URIList([]string{"file:///tmp/a%20b.txt", "https://example.com/", "file:///tmp/c"})},
	}, customType)
	got := dragIcons(items)
	want := []dragIcon{{path: "/tmp/a b.txt"}, {uti: utiURL}, {path: "/tmp/c"}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("dragIcons = %+v, want %+v", got, want)
	}
	got = dragIcons(pasteboardItems(platform.TextData("x"), customType))
	if len(got) != 1 || got[0] != (dragIcon{uti: utiUTF8Text}) {
		t.Errorf("dragIcons of text = %+v", got)
	}
	if got := dragIcons([]pbItem{{}}); got[0] != (dragIcon{uti: "public.data"}) {
		t.Errorf("dragIcons of an empty item = %+v", got)
	}
	if f := iconFrame(platform.Point{X: 50, Y: 40}, dragIconPoints); f != (dragFrame{X: 34, Y: 24, W: 32, H: 32}) {
		t.Errorf("iconFrame = %+v", f)
	}
}

// A held drop ends the session at the release without an event, and is
// delivered later with its data, where SetDropEffect still works.
func TestDropSessionHold(t *testing.T) {
	var s dropSession
	h := &fakeDropHost{}
	h.handle = func(e platform.DragEvent) {
		switch e.Action {
		case platform.DragEnter:
			s.setAnswer(platform.DropCopy, nil)
		case platform.DragDrop:
			if len(e.Data) != 1 {
				t.Errorf("held drop delivered with %d entries", len(e.Data))
			}
			s.setAnswer(platform.DropNone, nil)
		}
	}
	types := []string{platform.MIMEURIList, platform.MIMEText}
	s.enter(h, types, platform.DropCopy|platform.DropMove, 0, platform.Point{X: 1, Y: 2}, false)
	held, read, ok := s.hold(h, platform.DropCopy, 0, platform.Point{X: 3, Y: 4})
	if !ok || held.effect != platform.DropCopy || !slices.Equal(read, types) || s.active {
		t.Fatalf("hold = %+v, %q, %v; active %v", held, read, ok, s.active)
	}
	if want := []platform.DragAction{platform.DragEnter}; !slices.Equal(dragActions(h.got), want) {
		t.Errorf("events after hold = %v, want %v", dragActions(h.got), want)
	}
	s.leave(h) // draggingEnded: after the drop: nothing
	final := s.deliverHeld(h, held, platform.Data{{Type: platform.MIMEURIList, Bytes: []byte("file:///x\r\n")}})
	if final != platform.DropNone || s.active {
		t.Errorf("deliverHeld = %v (active %v), want the handler's none", final, s.active)
	}
	last := h.got[len(h.got)-1]
	if last.Action != platform.DragDrop || last.Pos != (platform.Point{X: 3, Y: 4}) || !slices.Equal(last.Types, types) {
		t.Errorf("held drop event = %+v", last)
	}
	if want := []platform.DragAction{platform.DragEnter, platform.DragDrop}; !slices.Equal(dragActions(h.got), want) {
		t.Errorf("events = %v, want %v", dragActions(h.got), want)
	}
	// A refused release is a leave and holds nothing.
	var r dropSession
	r.enter(h, types, platform.DropCopy, 0, platform.Point{}, false)
	r.setAnswer(platform.DropNone, nil)
	if _, _, ok := r.hold(h, platform.DropCopy, 0, platform.Point{}); ok {
		t.Errorf("a refused drop was held")
	}
	if h.got[len(h.got)-1].Action != platform.DragLeave {
		t.Errorf("a refused hold delivered %v, want a leave", h.got[len(h.got)-1].Action)
	}
}

func TestPromiseDrop(t *testing.T) {
	p := promiseDrop{expected: 3}
	if p.arrived("/d/a.jpg", "") || p.arrived("", "the source failed") {
		t.Errorf("done before three files")
	}
	if !p.arrived("/d/b.jpg", "") || !p.done() {
		t.Errorf("not done after three files")
	}
	if !slices.Equal(p.paths, []string{"/d/a.jpg", "/d/b.jpg"}) || len(p.failed) != 1 {
		t.Errorf("paths %q, failed %q", p.paths, p.failed)
	}
}

func TestWithFiles(t *testing.T) {
	posixPaths(t)
	order := []string{platform.MIMEHTML, platform.MIMEURIList, platform.MIMEText}
	d := platform.Data{{Type: platform.MIMEHTML, Bytes: []byte("<b>")}, {Type: platform.MIMEText, Bytes: []byte("t")}}
	got := withFiles(d, order, []string{"/d/a b.jpg"})
	if len(got) != 3 || got[1].Type != platform.MIMEURIList || string(got[1].Bytes) != "file:///d/a%20b.jpg\r\n" {
		t.Errorf("withFiles without a URI list = %+v", got)
	}
	if len(d) != 2 {
		t.Errorf("withFiles changed its argument")
	}
	d = platform.Data{{Type: platform.MIMEURIList, Bytes: platform.URIList([]string{"file:///x"})}}
	got = withFiles(d, order, []string{"/d/a", "/d/b"})
	if want := "file:///x\r\nfile:///d/a\r\nfile:///d/b\r\n"; len(got) != 1 || string(got[0].Bytes) != want {
		t.Errorf("withFiles with a URI list = %q, want %q", got[0].Bytes, want)
	}
	if string(d[0].Bytes) != "file:///x\r\n" {
		t.Errorf("withFiles changed its argument's bytes")
	}
	if got := withFiles(nil, nil, []string{"/a"}); len(got) != 1 || got[0].Type != platform.MIMEURIList {
		t.Errorf("withFiles(nil) = %+v", got)
	}
	if got := withFiles(d, order, nil); len(got) != 1 {
		t.Errorf("withFiles without files = %+v", got)
	}
}

func TestPromisedOnly(t *testing.T) {
	for _, c := range []struct {
		items [][]string
		want  bool
	}{
		{nil, false},
		{[][]string{{utiFileURL, "com.apple.finder.node"}}, false},
		{[][]string{{"com.apple.NSFilePromiseItemMetaData", "com.apple.pasteboard.promised-file-url"}}, true},
		{[][]string{{utiFileURL, "com.apple.pasteboard.promised-file-url"}}, false}, // read from the URL
		{[][]string{{utiFileURL}, {"com.example.promise"}}, true},
		{[][]string{{utiUTF8Text}}, false},
	} {
		if got := promisedOnly(c.items, []string{"com.example.promise"}); got != c.want {
			t.Errorf("promisedOnly(%q) = %v, want %v", c.items, got, c.want)
		}
	}
}

// posixPaths skips a test whose file paths and file: URIs are macOS's
// (POSIX): platform.FilePath and FileURI follow the host, and on Windows
// they read drive letters.
func posixPaths(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("macOS file paths: platform.FilePath and FileURI follow the host")
	}
}
