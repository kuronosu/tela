package remote

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// eventSamples has one value of every event type of package platform,
// with every field set.
var eventSamples = []platform.Event{
	platform.ConfigureEvent{Metrics: platform.Metrics{Size: platform.Size{W: 801.5, H: 600.25}, Scale: 1.25}},
	platform.FrameEvent{Time: 123456789 * time.Nanosecond},
	platform.CloseRequestedEvent{},
	platform.FocusEvent{Focused: true},
	platform.WakeEvent{},
	platform.SettingsEvent{Settings: platform.SystemSettings{DarkMode: true, ReducedMotion: true, DoubleClickInterval: 400 * time.Millisecond,
		AccentColor: color.NRGBA{R: 53, G: 132, B: 228, A: 255}, HasAccentColor: true, HighContrast: true, TextScale: 1.25}},
	platform.PointerEvent{Action: platform.PointerDown, Kind: platform.PointerPen, ID: 7, Pos: platform.Point{X: 1.5, Y: -2.25},
		Button: platform.ButtonSecondary, Buttons: 3, Mods: platform.ModCtrl | platform.ModShift, Time: time.Second},
	platform.ScrollEvent{Kind: platform.PointerMouse, Pos: platform.Point{X: 3, Y: 4}, Delta: platform.Point{X: -1, Y: 2.5},
		Steps: platform.Point{X: 0.5, Y: -1}, Phase: platform.ScrollPhaseEnd, Mods: platform.ModAlt, Time: 2 * time.Second},
	platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeKeyA, Key: "é", Text: "é", Location: platform.KeyLocationRight,
		Repeat: true, Synthetic: true, Mods: platform.ModSuper, Time: 3 * time.Second},
	platform.IMEEvent{DeleteBefore: 3, DeleteAfter: 4, Commit: "日本", Preedit: "ご", CursorStart: 0, CursorEnd: 3},
	platform.IMESessionEvent{Active: true},
	platform.WindowStateEvent{State: platform.WindowState{Maximized: true, Fullscreen: true, Minimized: true,
		ClientSideDecorations: true, HasPosition: true, Position: image.Point{X: -1920, Y: 37}, TitleBarControls: platform.Rect{X: 8, Y: 6.5, W: 52, H: 16}}},
	platform.NotificationEvent{ID: 9, Kind: platform.NotificationActionInvoked, Action: "reply"},
	platform.TrayEvent{Tray: 2, Kind: platform.TrayMenuItem, Item: 41},
	platform.DragEvent{Action: platform.DragDrop, Pos: platform.Point{X: 10.5, Y: -3}, Types: []string{platform.MIMEURIList, platform.MIMEText},
		Allowed: platform.DropCopy | platform.DropMove, Mods: platform.ModCtrl, FromApp: true,
		Data: platform.Data{{Type: platform.MIMEURIList, Bytes: []byte("file:///tmp/a.txt\r\n")}, {Type: platform.MIMEText, Bytes: []byte("a")}},
		Time: 4 * time.Second},
	platform.DragDoneEvent{Effect: platform.DropMove},
	platform.MenuEvent{Item: 77},
	platform.ShortcutEvent{ID: "show", Action: platform.ShortcutReleased, Time: 5 * time.Second},
	platform.ShortcutStatusEvent{ID: "show", Status: platform.ShortcutRefused, Trigger: "Ctrl+Alt+K", Reason: "declined"},
	platform.LaunchEvent{Launch: platform.Launch{Args: []string{"notes:open?id=3", "/tmp/x y"}, Dir: "/home/u", Token: "tok_TIME42"}},
	platform.QuitRequestedEvent{},
	platform.AppMenuEvent{Item: 78},
}

// TestEveryEventHasAnEncoding fails when package platform gains an event
// type (a type with an isEvent method) that eventSamples, and therefore
// the wire encoding, does not cover.
func TestEveryEventHasAnEncoding(t *testing.T) {
	for _, e := range eventSamples {
		if got := roundTrip(t, e); !reflect.DeepEqual(got, e) {
			t.Errorf("round trip of %T: got %+v, want %+v", e, got, e)
		}
	}
	// go test runs a package's tests in its source directory, so package
	// platform is the parent directory. A test binary run elsewhere (built
	// with go test -c and copied to a machine without the repository) has
	// no sources to compare eventSamples with.
	dir := filepath.Join("..")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var declared []string
	fset := token.NewFileSet()
	sources := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		if f.Name.Name != "platform" {
			continue
		}
		sources++
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "isEvent" || fd.Recv == nil || len(fd.Recv.List) != 1 {
				continue
			}
			typ := fd.Recv.List[0].Type
			if s, ok := typ.(*ast.StarExpr); ok {
				typ = s.X
			}
			if id, ok := typ.(*ast.Ident); ok {
				declared = append(declared, id.Name)
			}
		}
	}
	if sources == 0 {
		abs, _ := filepath.Abs(dir)
		t.Skipf("no source file of package platform in %s (the test binary runs outside the source tree): the event types cannot be listed; the round trips above ran", abs)
	}
	if len(declared) < 10 {
		t.Fatalf("found only %d event types in package platform (%v); the parser is looking in the wrong place", len(declared), declared)
	}
	var sampled []string
	for _, e := range eventSamples {
		sampled = append(sampled, reflect.TypeOf(e).Name())
	}
	slices.Sort(declared)
	slices.Sort(sampled)
	for _, name := range declared {
		if !slices.Contains(sampled, name) {
			t.Errorf("platform.%s has no wire encoding: add it to enc.event and dec.event in protocol.go, to eventSamples, and to the host's routing", name)
		}
	}
	for _, name := range sampled {
		if !slices.Contains(declared, name) {
			t.Errorf("eventSamples has %s, which is not an event type of package platform", name)
		}
	}
}

// TestEventRoundTripRandom round-trips random values of every event type:
// a field added to an event struct but not to its encoding fails here.
func TestEventRoundTripRandom(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, sample := range eventSamples {
		typ := reflect.TypeOf(sample)
		for range 200 {
			v, ok := quick.Value(typ, r)
			if !ok {
				t.Fatalf("cannot generate %v", typ)
			}
			nilEmpty(v)
			e := v.Interface().(platform.Event)
			if got := roundTrip(t, e); !reflect.DeepEqual(got, e) {
				t.Fatalf("round trip of %T: got %+v, want %+v", e, got, e)
			}
		}
	}
}

// nilEmpty sets every empty slice in v (a value of a generated event) to
// nil: the wire does not tell nil from empty lists, and decodes both as
// nil.
func nilEmpty(v reflect.Value) {
	switch v.Kind() {
	case reflect.Slice:
		if v.Len() == 0 {
			if v.CanSet() {
				v.Set(reflect.Zero(v.Type()))
			}
			return
		}
		for i := range v.Len() {
			nilEmpty(v.Index(i))
		}
	case reflect.Struct:
		for i := range v.NumField() {
			nilEmpty(v.Field(i))
		}
	}
}

func roundTrip(t *testing.T, e platform.Event) platform.Event {
	t.Helper()
	m := newMsg(msgEvent)
	m.event(e)
	b := m.bytes()
	d := &dec{b: b[5:]}
	got := d.event()
	if err := d.end(); err != nil {
		t.Fatalf("decode %T: %v", e, err)
	}
	return got
}

func TestTextInputAndOptionsRoundTrip(t *testing.T) {
	states := []*platform.TextInputState{
		nil,
		{CursorArea: platform.Rect{X: 1, Y: 2, W: 0, H: 18}, Purpose: platform.TextPurposeURL, Multiline: true},
		{Purpose: platform.TextPurposePassword, Surrounding: &platform.SurroundingText{Text: "héllo", Cursor: 3, Anchor: 1}},
	}
	for _, s := range states {
		e := newMsg(msgSetTextInput)
		e.textInput(s)
		d := &dec{b: e.bytes()[5:]}
		got := d.textInput()
		if err := d.end(); err != nil || !got.Equal(s) {
			t.Errorf("text input %+v: got %+v (%v)", s, got, err)
		}
	}
	o := platform.WindowOptions{Title: "t", AppID: "com.example", Size: platform.Size{W: 3, H: 4}, MinSize: platform.Size{W: 1, H: 2},
		MaxSize: platform.Size{W: 30, H: 40}, FixedSize: true, Position: image.Pt(-1920, 37), HasPosition: true,
		Maximized: true, Fullscreen: true, Hidden: true, AlwaysOnTop: true, Frameless: true, Modal: true, RestoreToken: "wayland:abc",
		TitleBar: platform.TitleBarColors{Background: platform.WindowColor{R: 1, G: 2, B: 3, Set: true}, Border: platform.WindowColor{R: 255, Set: true}},
		Backdrop: platform.BackdropMicaAlt}
	e := newMsg(msgNewWindow)
	e.windowOptions(o, 7)
	d := &dec{b: e.bytes()[5:]}
	if got, parent := d.windowOptions(); d.end() != nil || got != o || parent != 7 {
		t.Errorf("window options: got %+v (parent %d), want %+v (parent 7)", got, parent, o)
	}
}

// TestWindowOptionsEncodeEveryField fails when platform.WindowOptions
// gains a field that the wire encoding does not carry: random options
// (Parent aside, which travels as an ID) must round-trip.
func TestWindowOptionsEncodeEveryField(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	typ := reflect.TypeFor[platform.WindowOptions]()
	for range 200 {
		var o platform.WindowOptions
		v := reflect.ValueOf(&o).Elem()
		for i := range typ.NumField() {
			f := v.Field(i)
			if typ.Field(i).Name == "Parent" {
				continue
			}
			rv, ok := quick.Value(f.Type(), r)
			if !ok {
				t.Fatalf("cannot generate %s", typ.Field(i).Name)
			}
			f.Set(rv)
			if typ.Field(i).Name == "Backdrop" {
				f.Set(reflect.ValueOf(platform.Backdrop(r.Intn(len(platform.BackdropNames())))))
			}
		}
		e := newMsg(msgNewWindow)
		e.windowOptions(o, 3)
		d := &dec{b: e.bytes()[5:]}
		got, parent := d.windowOptions()
		if err := d.end(); err != nil || !reflect.DeepEqual(got, o) || parent != 3 {
			t.Fatalf("window options round trip: got %+v (%v), want %+v", got, err, o)
		}
	}
}

// TestDesktopEncodings round-trips the payloads of window control and
// desktop services: icons, hit regions, file dialogs, message boxes,
// notifications and tray menus.
func TestDesktopEncodings(t *testing.T) {
	icon := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for i := range icon.Pix {
		icon.Pix[i] = byte(i * 7)
	}
	small := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	small.Pix = []byte{1, 2, 3, 4}
	e := &enc{}
	e.images([]image.Image{icon, small})
	regions := []platform.HitRegion{{Kind: platform.HitCaption, Rect: platform.Rect{W: 300, H: 32}},
		{Kind: platform.HitClose, Rect: platform.Rect{X: 268, W: 32, H: 32}}}
	e.regions(regions)
	fd := platform.FileDialogOptions{Title: "Open", Directory: "/tmp", Name: "a.txt", Multiple: true,
		Filters: []platform.FileFilter{{Name: "Text", Patterns: []string{"*.txt", "*.md"}}, {Name: "All", Patterns: []string{"*"}}}}
	e.fileDialog(fd, 4)
	mb := platform.MessageBoxOptions{Kind: platform.MessageQuestion, Title: "Quit?", Text: "Unsaved", Buttons: platform.ButtonsYesNoCancel}
	e.messageBox(mb, 2)
	n := platform.Notification{Title: "Saved", Body: "notes.txt", Icon: []image.Image{small},
		Actions: []platform.NotificationAction{{ID: "open", Label: "Open"}}}
	e.notification(n)
	tray := platform.TrayOptions{Icon: []image.Image{icon}, Tooltip: "Notes", Menu: []platform.MenuItem{
		{ID: 1, Label: "Show"}, {Separator: true},
		{Label: "More", Submenu: []platform.MenuItem{{ID: 2, Label: "Pin", Checkable: true, Checked: true}, {ID: 3, Label: "Off", Disabled: true}}},
	}}
	e.tray(tray)
	e.strs([]string{"/a", "/b c"})

	d := &dec{b: e.b}
	if got := d.images(); len(got) != 2 || !equalIcons(got, []image.Image{icon, small}) {
		t.Errorf("images: got %v", got)
	}
	if got := d.regions(); !platform.EqualHitRegions(got, regions) {
		t.Errorf("regions: got %v", got)
	}
	if got, p := d.fileDialog(); !reflect.DeepEqual(got, fd) || p != 4 {
		t.Errorf("file dialog: got %+v parent %d", got, p)
	}
	if got, p := d.messageBox(); got != mb || p != 2 {
		t.Errorf("message box: got %+v parent %d", got, p)
	}
	if got := d.notification(); got.Title != n.Title || got.Body != n.Body || !reflect.DeepEqual(got.Actions, n.Actions) || !equalIcons(got.Icon, n.Icon) {
		t.Errorf("notification: got %+v", got)
	}
	if got := d.tray(); !equalTray(got, tray) {
		t.Errorf("tray: got %+v", got)
	}
	if got := d.strs(); !slices.Equal(got, []string{"/a", "/b c"}) {
		t.Errorf("strings: got %v", got)
	}
	if err := d.end(); err != nil {
		t.Errorf("end: %v", err)
	}

	// Corrupt counts and sizes fail instead of allocating.
	bad := &enc{}
	bad.u32(1)
	bad.u32(5000)
	bad.u32(1)
	d = &dec{b: bad.b}
	d.images()
	if d.end() == nil {
		t.Errorf("an icon wider than MaxIconSide decoded")
	}
	bad = &enc{}
	bad.u32(1 << 30)
	d = &dec{b: bad.b}
	d.regions()
	if d.end() == nil {
		t.Errorf("a huge region count decoded")
	}
	deep := []platform.MenuItem{{ID: 1, Label: "x"}}
	for range maxDepth + 1 {
		deep = []platform.MenuItem{{Label: "sub", Submenu: deep}}
	}
	bad = &enc{}
	bad.menu(deep)
	d = &dec{b: bad.b}
	d.menu(0)
	if d.end() == nil {
		t.Errorf("a menu nested deeper than maxDepth decoded")
	}
}

func TestDecodeErrors(t *testing.T) {
	d := &dec{b: []byte{evPointer, 1}}
	d.event()
	if d.end() == nil {
		t.Errorf("truncated event decoded without error")
	}
	d = &dec{b: []byte{200}}
	d.event()
	if d.end() == nil {
		t.Errorf("unknown event tag decoded without error")
	}
	e := newMsg(msgPresent)
	e.rects([]image.Rectangle{image.Rect(0, 0, 20, 20)})
	d = &dec{b: e.bytes()[5:]}
	d.rects(image.Rect(0, 0, 10, 10))
	if d.end() == nil {
		t.Errorf("damage outside the frame decoded without error")
	}
	d = &dec{b: []byte{1, 2}}
	d.u8()
	if d.end() == nil {
		t.Errorf("trailing bytes not reported")
	}
	e = &enc{}
	e.menu([]platform.MenuItem{{ID: 1, Role: platform.RoleHelpMenu + 1}})
	d = &dec{b: e.b}
	if d.menu(0); d.end() == nil {
		t.Errorf("an invalid menu role decoded without error")
	}
}

// chunkReader returns its data a few bytes at a time.
type chunkReader struct {
	data []byte
	n    int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), c.n, len(c.data))
	copy(p, c.data[:n])
	c.data = c.data[n:]
	return n, nil
}
func (*chunkReader) write([]byte, *os.File) error    { return nil }
func (*chunkReader) takeFile() *os.File              { return nil }
func (*chunkReader) SetReadDeadline(time.Time) error { return nil }
func (*chunkReader) Close() error                    { return nil }

func TestFrameReader(t *testing.T) {
	var stream []byte
	var want []string
	for i := range 50 {
		e := newMsg(msgCtlNotify)
		s := strings.Repeat("x", i*997)
		e.str(s)
		stream = append(stream, e.bytes()...)
		want = append(want, s)
	}
	for _, chunk := range []int{1, 3, 4096, 1 << 20} {
		r := newFrameReader(&chunkReader{data: slices.Clone(stream), n: chunk})
		r.buf = make([]byte, 16) // force growth
		for i, w := range want {
			typ, p, err := r.next()
			if err != nil {
				t.Fatalf("chunk %d frame %d: %v", chunk, i, err)
			}
			d := &dec{b: p}
			if typ != msgCtlNotify || d.str() != w || d.end() != nil {
				t.Fatalf("chunk %d frame %d: wrong content", chunk, i)
			}
		}
		if _, _, err := r.next(); err != io.EOF {
			t.Errorf("chunk %d: end of stream: %v, want EOF", chunk, err)
		}
	}
	// A truncated frame is an unexpected EOF, not a clean end.
	r := newFrameReader(&chunkReader{data: stream[:10], n: 4})
	if _, _, err := r.next(); err != nil {
		t.Fatalf("first frame: %v", err)
	}
	if _, _, err := r.next(); err != io.ErrUnexpectedEOF {
		t.Errorf("truncated frame: %v, want ErrUnexpectedEOF", err)
	}
}

// TestPhase13Encodings round-trips the payloads of protocol 5 (ADR-214):
// data, drag options with an image, shortcuts, launches, associations and
// menus with accelerators.
func TestPhase13Encodings(t *testing.T) {
	img := image.NewRGBA(image.Rect(2, 3, 5, 5)) // not at the origin: rows are packed
	for i := range img.Pix {
		img.Pix[i] = byte(i * 5)
	}
	data := platform.Data{{Type: platform.MIMEHTML, Bytes: []byte("<b>x</b>")}, {Type: "application/x-tela", Bytes: nil}}
	drag := platform.DragOptions{Data: data, Allowed: platform.DropAll, Image: img, Hotspot: image.Pt(-1, 40)}
	sc := platform.Shortcut{ID: "show", Accelerator: platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt, Key: platform.KeyF1}, Description: "Show"}
	launch := platform.Launch{Args: []string{"a"}, Dir: "/d", Token: "t"}
	app := platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: "/usr/bin/notes", Args: []string{"--open"}, Icon: "/i.png"}
	as := platform.Association{Extension: ".note", MIMEType: "application/x-note", Description: "Note"}
	menu := []platform.MenuItem{{ID: 1, Label: "Save", Accelerator: "Ctrl+S"}, {Label: "Edit", Role: platform.RoleEditMenu, Submenu: []platform.MenuItem{{ID: 2, Label: "Undo", Accelerator: "Ctrl+Z", Role: platform.RoleUndo}}}}

	e := &enc{}
	e.data(data)
	e.dragOptions(drag)
	e.dragOptions(platform.DragOptions{Data: platform.TextData("x"), Allowed: platform.DropCopy})
	e.shortcut(sc)
	e.launch(launch)
	e.handlerApp(app)
	e.association(as)
	e.menu(menu)
	e.optBytes(nil)
	e.optBytes([]byte{})
	e.optBytes([]byte{1})

	d := &dec{b: e.b}
	if got := d.data(); !reflect.DeepEqual(got, data) {
		t.Errorf("data: got %+v", got)
	}
	got := d.dragOptions()
	if !reflect.DeepEqual(got.Data, data) || got.Allowed != drag.Allowed || got.Hotspot != drag.Hotspot || got.Image == nil || got.Image.Rect != image.Rect(0, 0, 3, 2) {
		t.Errorf("drag options: got %+v", got)
	} else {
		for y := range 2 {
			for x := range 3 {
				if got.Image.RGBAAt(x, y) != img.RGBAAt(x+2, y+3) {
					t.Fatalf("drag image pixel (%d,%d) differs", x, y)
				}
			}
		}
	}
	if got := d.dragOptions(); got.Image != nil || got.Allowed != platform.DropCopy {
		t.Errorf("drag options without an image: got %+v", got)
	}
	if got := d.shortcut(); got != sc {
		t.Errorf("shortcut: got %+v", got)
	}
	if got := d.launch(); !reflect.DeepEqual(got, launch) {
		t.Errorf("launch: got %+v", got)
	}
	if got := d.handlerApp(); !reflect.DeepEqual(got, app) {
		t.Errorf("handler app: got %+v", got)
	}
	if got := d.association(); got != as {
		t.Errorf("association: got %+v", got)
	}
	if got := d.menu(0); !reflect.DeepEqual(got, menu) {
		t.Errorf("menu: got %+v", got)
	}
	if got := d.optBytes(); got != nil {
		t.Errorf("absent bytes: got %v", got)
	}
	if got := d.optBytes(); got == nil || len(got) != 0 {
		t.Errorf("empty bytes: got %v", got)
	}
	if got := d.optBytes(); !slices.Equal(got, []byte{1}) {
		t.Errorf("bytes: got %v", got)
	}
	if err := d.end(); err != nil {
		t.Errorf("end: %v", err)
	}

	bad := &enc{}
	bad.boolean(true)
	bad.u32(platform.MaxDragImageSide + 1)
	bad.u32(1)
	d = &dec{b: bad.b}
	d.rgba()
	if d.end() == nil {
		t.Errorf("a drag image wider than MaxDragImageSide decoded")
	}
}

// TestOutcomeSentinels checks that the errors the layers above test with
// errors.Is cross the connection.
func TestOutcomeSentinels(t *testing.T) {
	for _, want := range []error{platform.ErrUnsupported, platform.ErrNoGrab, platform.ErrSecretNotFound, platform.ErrSecretTooLarge,
		platform.TakenError(platform.Association{Scheme: "notes"}, "other.desktop")} {
		e := &enc{}
		encodeOutcome(e, want)
		got := outcome(&dec{b: e.b})
		if got == nil || got.Error() != want.Error() && !errors.Is(want, platform.ErrUnsupported) {
			t.Errorf("outcome of %v: %v", want, got)
		}
		for _, s := range []error{platform.ErrUnsupported, platform.ErrNoGrab, platform.ErrSecretNotFound, platform.ErrSecretTooLarge,
			platform.ErrAssociationTaken, fs.ErrNotExist} {
			if errors.Is(want, s) != errors.Is(got, s) {
				t.Errorf("errors.Is(%v, %v) = %v across the connection, want %v", got, s, errors.Is(got, s), errors.Is(want, s))
			}
		}
	}
}
