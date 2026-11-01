package platform

import (
	"errors"
	"image"
	"io/fs"
	"math"
	"runtime"
	"slices"
	"testing"
)

// Tests of the phase-13 contract (ADR-210): data, drag and drop, menus,
// shortcuts, settings, secrets, instances and associations.

func TestFileURI(t *testing.T) {
	for _, c := range []struct {
		path    string
		windows bool
		uri     string
	}{
		{"/home/u/a b.txt", false, "file:///home/u/a%20b.txt"},
		{"/tmp/100%#?.txt", false, "file:///tmp/100%25%23%3F.txt"},
		{"/tmp/ñandú", false, "file:///tmp/%C3%B1and%C3%BA"},
		{"/", false, "file:///"},
		{`C:\Users\a b\x.txt`, true, "file:///C:/Users/a%20b/x.txt"},
		{`c:/x`, true, "file:///c:/x"},
		{`\\server\share\dir\f.txt`, true, "file://server/share/dir/f.txt"},
		{`\\?\C:\very\long`, true, "file:///C:/very/long"},
		{`\\?\UNC\srv\share\f`, true, "file://srv/share/f"},
	} {
		got, err := fileURI(c.path, c.windows)
		if err != nil || got != c.uri {
			t.Errorf("fileURI(%q, %v) = %q, %v; want %q", c.path, c.windows, got, err, c.uri)
			continue
		}
		back, err := filePath(got, c.windows)
		want := c.path
		switch c.path {
		case `c:/x`:
			want = `c:\x`
		case `\\?\C:\very\long`:
			want = `C:\very\long`
		case `\\?\UNC\srv\share\f`:
			want = `\\srv\share\f`
		}
		if err != nil || back != want {
			t.Errorf("filePath(%q, %v) = %q, %v; want %q", got, c.windows, back, err, want)
		}
	}
	for _, c := range []struct {
		path    string
		windows bool
	}{{"rel/x", false}, {"", false}, {"/a\x00b", false}, {`x\y`, true}, {`C:x`, true}, {`\\server`, true}} {
		if u, err := fileURI(c.path, c.windows); err == nil {
			t.Errorf("fileURI(%q, %v) = %q, want an error", c.path, c.windows, u)
		}
	}
}

func TestFilePath(t *testing.T) {
	for _, c := range []struct {
		uri     string
		windows bool
		path    string
	}{
		{"file:/etc/hosts", false, "/etc/hosts"},
		{"file://localhost/etc/a%20b", false, "/etc/a b"},
		{"FILE:///x+y", false, "/x+y"},
		{"file:///x?query#frag", false, "/x"},
		{"file:///C|/Windows", true, `C:\Windows`},
		{"file:///C:", true, `C:\`},
		{"file://localhost/D:/a%20b", true, `D:\a b`},
		{"file:////server/share/f", true, `\\server\share\f`},
	} {
		got, err := filePath(c.uri, c.windows)
		if err != nil || got != c.path {
			t.Errorf("filePath(%q, %v) = %q, %v; want %q", c.uri, c.windows, got, err, c.path)
		}
	}
	for _, c := range []struct {
		uri     string
		windows bool
	}{
		{"https://example.com/x", false}, {"file://host/x", false}, {"file:relative", false},
		{"file:///a%00b", false}, {"file:///bad%zz", false}, {"file:///no/drive", true}, {"file://host", false},
	} {
		if p, err := filePath(c.uri, c.windows); err == nil {
			t.Errorf("filePath(%q, %v) = %q, want an error", c.uri, c.windows, p)
		}
	}
}

func TestURIListAndData(t *testing.T) {
	uris := []string{"file:///a", "https://x.org/"}
	b := URIList(uris)
	if string(b) != "file:///a\r\nhttps://x.org/\r\n" {
		t.Errorf("URIList = %q", b)
	}
	if got := ParseURIList([]byte("# comment\r\nfile:///a\n\n  https://x.org/ \r\nfile:///b\x00")); !slices.Equal(got, []string{"file:///a", "https://x.org/", "file:///b"}) {
		t.Errorf("ParseURIList = %q", got)
	}
	g := GnomeCopiedFiles(uris, true)
	if string(g) != "cut\nfile:///a\nhttps://x.org/" {
		t.Errorf("GnomeCopiedFiles = %q", g)
	}
	if got, cut, ok := ParseGnomeCopiedFiles(g); !ok || !cut || !slices.Equal(got, uris) {
		t.Errorf("ParseGnomeCopiedFiles = %q, %v, %v", got, cut, ok)
	}
	if _, _, ok := ParseGnomeCopiedFiles([]byte("move\nfile:///a")); ok {
		t.Errorf("ParseGnomeCopiedFiles accepted an unknown operation")
	}

	d := Data{{Type: MIMEHTML, Bytes: []byte("<b>x</b>")}, {Type: MIMEText, Bytes: []byte("x")},
		{Type: MIMEURIList, Bytes: []byte("file:///tmp/a%20b\r\nhttps://x/\r\n")}}
	CheckData("test", d)
	if s, ok := d.Text(); !ok || s != "x" {
		t.Errorf("Text = %q, %v", s, ok)
	}
	if !slices.Equal(d.Types(), []string{MIMEHTML, MIMEText, MIMEURIList}) {
		t.Errorf("Types = %q", d.Types())
	}
	wantPath := "/tmp/a b"
	if runtime.GOOS == "windows" {
		d[2].Bytes = []byte("file:///C:/tmp/a%20b\r\nhttps://x/\r\n")
		wantPath = `C:\tmp\a b`
	}
	if p, ok := d.Paths(); !ok || !slices.Equal(p, []string{wantPath}) {
		t.Errorf("Paths = %q, %v", p, ok)
	}
	if sel := d.Select([]string{MIMEURIList, MIMEHTML}); !slices.Equal(sel.Types(), []string{MIMEHTML, MIMEURIList}) {
		t.Errorf("Select keeps the source order: %q", sel.Types())
	}
	c := d.Clone()
	c[0].Bytes[0] = 'X'
	if d[0].Bytes[0] != '<' {
		t.Errorf("Clone shares bytes")
	}
	if (Data{}).Clone() != nil || TextData("hi")[0].Type != MIMEText {
		t.Errorf("Clone of empty, or TextData")
	}
	if _, err := FileData(); err == nil {
		t.Errorf("FileData() with no paths succeeded")
	}
	expectPanic(t, "not a MIME type", func() { CheckData("x", Data{{Type: "text"}}) })
	expectPanic(t, "not a MIME type", func() { CheckData("x", Data{{Type: "text/plain; charset=utf-8"}}) })
	expectPanic(t, "twice", func() { CheckData("x", Data{{Type: MIMEPNG}, {Type: MIMEPNG}}) })
	expectPanic(t, "not a MIME type", func() { CheckMIME("x", "a/b/c") })
	CheckMIME("x", "application/x-tela-card;v=2")
}

func TestSelectionTypes(t *testing.T) {
	for target, want := range map[string]string{
		"UTF8_STRING": MIMEText, "STRING": MIMEText, "text/plain": MIMEText, "text/plain;charset=UTF-8": MIMEText,
		"text/plain; charset=utf8": MIMEText, "text/html; charset=utf-8": "text/html;charset=utf-8",
		"text/plain;charset=iso-8859-1": "text/plain;charset=iso-8859-1",
		"x-special/gnome-copied-files":  MIMEURIList, "image/png": MIMEPNG,
		"TARGETS": "", "MULTIPLE": "", "TIMESTAMP": "", "SAVE_TARGETS": "",
	} {
		got, ok := SelectionType(target)
		if (want == "") == ok || got != want {
			t.Errorf("SelectionType(%q) = %q, %v; want %q", target, got, ok, want)
		}
	}
	if got := SelectionTargets(MIMEText); got[0] != MIMEText || !slices.Contains(got, "UTF8_STRING") {
		t.Errorf("SelectionTargets(text) = %q", got)
	}
	if got := SelectionTargets(MIMEURIList); !slices.Equal(got, []string{MIMEURIList, "x-special/gnome-copied-files"}) {
		t.Errorf("SelectionTargets(uri-list) = %q", got)
	}
	// Every target a backend offers maps back to its Data type.
	for _, mime := range []string{MIMEText, MIMEURIList, MIMEHTML, "application/x-card"} {
		for _, target := range SelectionTargets(mime) {
			if got, ok := SelectionType(target); !ok || got != mime {
				t.Errorf("SelectionType(%q) = %q, want %q", target, got, mime)
			}
		}
	}
}

func TestDropEffects(t *testing.T) {
	if DropAll.String() != "copy|move|link" || DropNone.String() != "none" || DropEffect(8).String() != "DropEffect(invalid)" {
		t.Errorf("DropEffect names")
	}
	for _, c := range []struct {
		allowed DropEffect
		mods    Modifiers
		want    DropEffect
	}{
		{DropAll, 0, DropCopy}, {DropMove, 0, DropMove}, {DropLink, 0, DropLink}, {DropNone, ModCtrl, DropNone},
		{DropAll, ModShift, DropMove}, {DropAll, ModCtrl, DropCopy}, {DropAll, ModCtrl | ModShift, DropLink},
		{DropCopy | DropMove, ModCtrl | ModShift, DropCopy}, {DropMove | DropLink, ModCtrl, DropMove},
	} {
		if got := DefaultDropEffect(c.allowed, c.mods); got != c.want {
			t.Errorf("DefaultDropEffect(%v, %v) = %v, want %v", c.allowed, c.mods, got, c.want)
		}
	}
	offered := []string{"application/x-card", MIMEHTML, "text/x-moz-url", MIMEText}
	if got := DropTypes(offered, nil); !slices.Equal(got, []string{MIMEHTML, MIMEText}) {
		t.Errorf("DropTypes(nil) = %q", got)
	}
	if got := DropTypes(offered, []string{MIMEText, "application/x-card", "image/png"}); !slices.Equal(got, []string{"application/x-card", MIMEText}) {
		t.Errorf("DropTypes(want) = %q", got)
	}
	CheckDropEffect("x", DropMove, true)
	expectPanic(t, "set of effects", func() { CheckDropEffect("x", DropCopy|DropMove, true) })
	expectPanic(t, "invalid DropEffect", func() { CheckDropEffect("x", 16, false) })

	opts := DragOptions{Data: TextData("x"), Allowed: DropCopy, Image: image.NewRGBA(image.Rect(0, 0, 32, 32)), Hotspot: image.Pt(-4, 40)}
	CheckDragOptions(opts)
	for msg, change := range map[string]func(*DragOptions){
		"Data is empty":    func(o *DragOptions) { o.Data = nil },
		"Allowed is":       func(o *DragOptions) { o.Allowed = DropNone },
		"Image is empty":   func(o *DragOptions) { o.Image = image.NewRGBA(image.Rect(0, 0, 0, 5)) },
		"at most":          func(o *DragOptions) { o.Image = image.NewRGBA(image.Rect(0, 0, 2048, 1)) },
		"not a MIME type":  func(o *DragOptions) { o.Data = Data{{Type: "png"}} },
		"invalid DropEff…": func(o *DragOptions) { o.Allowed = 0x80 },
	} {
		o := opts
		change(&o)
		if msg == "invalid DropEff…" {
			msg = "invalid DropEffect"
		}
		expectPanic(t, msg, func() { CheckDragOptions(o) })
	}
	if DragDrop.String() != "drop" || DragAction(9).String() != "DragAction(invalid)" {
		t.Errorf("DragAction names")
	}
}

func TestMenusAndPopups(t *testing.T) {
	menu := []MenuItem{{ID: 1, Label: "Save", Accelerator: "Ctrl+S"}, {Label: "More", Submenu: []MenuItem{{ID: 2, Label: "Quit"}}}}
	CheckMenu("m", menu)
	CheckPopupMenu(Point{X: 3, Y: 4}, menu, true)
	expectPanic(t, "control character", func() { CheckMenu("m", []MenuItem{{ID: 1, Accelerator: "Ctrl\tS"}}) })
	expectPanic(t, "m[1].Submenu[0]", func() { CheckMenu("m", []MenuItem{{ID: 2}, {Submenu: []MenuItem{{ID: 2}}}}) })
	expectPanic(t, "empty menu", func() { CheckPopupMenu(Point{}, nil, true) })
	expectPanic(t, "nil callback", func() { CheckPopupMenu(Point{}, menu, false) })
	expectPanic(t, "finite", func() { CheckPopupMenu(Point{X: float32(math.Inf(1))}, menu, true) })
}

func TestAccelerators(t *testing.T) {
	for a, want := range map[Accelerator]string{
		{Mods: ModShift | ModCtrl, Key: "k"}:    "Ctrl+Shift+K",
		{Mods: ModSuper | ModAlt, Key: KeyF1}:   "Alt+Super+F1",
		{Key: " "}:                              "Space",
		{Mods: ModCtrl, Key: "é"}:               "Ctrl+É",
		{Mods: ModCtrl, Key: KeyArrowUp}:        "Ctrl+ArrowUp",
		{Mods: ModCtrl | ModAlt, Key: "/"}:      "Ctrl+Alt+/",
		{Mods: ModCtrl, Key: KeyMediaPlayPause}: "Ctrl+MediaPlayPause",
	} {
		if got := a.String(); got != want {
			t.Errorf("%+v.String() = %q, want %q", a, got, want)
		}
		CheckAccelerator("x", a)
	}
	for msg, a := range map[string]Accelerator{
		"no Key":            {Mods: ModCtrl},
		"cannot be":         {Mods: ModCtrl, Key: KeyShift},
		"upper case":        {Mods: ModCtrl, Key: "K"},
		"control character": {Key: "\t"},
		"one character":     {Key: "ab"},
		"invalid modifiers": {Mods: 0x40, Key: "a"},
	} {
		expectPanic(t, msg, func() { CheckAccelerator("x", a) })
	}
	CheckShortcut(Shortcut{ID: "show", Accelerator: Accelerator{Mods: ModSuper, Key: "n"}, Description: "Show notes"})
	expectPanic(t, "no Description", func() { CheckShortcut(Shortcut{ID: "show", Accelerator: Accelerator{Key: "n"}}) })
	expectPanic(t, "invalid shortcut ID", func() { CheckShortcut(Shortcut{Accelerator: Accelerator{Key: "n"}, Description: "x"}) })
	if ShortcutRevoked.String() != "revoked" || ShortcutReleased.String() != "released" {
		t.Errorf("shortcut enum names")
	}
}

func TestSystemSettingsV2(t *testing.T) {
	for scale, want := range map[float64]float64{0: 1, 1.25: 1.25, -1: 1, math.NaN(): 1, 9: MaxTextScale} {
		if got := (SystemSettings{TextScale: scale}).TextScaleFactor(); got != want {
			t.Errorf("TextScaleFactor(%v) = %v, want %v", scale, got, want)
		}
	}
	CheckSystemSettings(SystemSettings{TextScale: 1.5, HighContrast: true, HasAccentColor: true})
	expectPanic(t, "TextScale", func() { CheckSystemSettings(SystemSettings{TextScale: math.NaN()}) })
	expectPanic(t, "TextScale", func() { CheckSystemSettings(SystemSettings{TextScale: 5}) })
	expectPanic(t, "negative", func() { CheckSystemSettings(SystemSettings{DoubleClickInterval: -1}) })
	// Settings stay comparable (SettingsEvent handling compares them).
	if (SystemSettings{TextScale: 1}) == (SystemSettings{TextScale: 2}) {
		t.Errorf("settings compare equal")
	}
}

func TestSecretsAndInstances(t *testing.T) {
	if !errors.Is(ErrSecretNotFound, fs.ErrNotExist) {
		t.Errorf("ErrSecretNotFound does not wrap fs.ErrNotExist")
	}
	if err := CheckSecret("com.example.Notes", "key", make([]byte, MaxSecretSize)); err != nil {
		t.Errorf("a secret of MaxSecretSize: %v", err)
	}
	if err := CheckSecret("com.example.Notes", "key", make([]byte, MaxSecretSize+1)); !errors.Is(err, ErrSecretTooLarge) {
		t.Errorf("an oversized secret: %v", err)
	}
	expectPanic(t, "invalid secret name", func() { CheckSecretName("x", "app", "") })
	expectPanic(t, "invalid app ID", func() { CheckSecretName("x", "my app", "n") })
	for _, id := range []string{"com.example.Notes", "notes", "my-app_2", "Ñandú"} {
		CheckAppID("x", id)
	}
	for _, id := range []string{"", "a/b", `a\b`, "a b", "a\x00"} {
		expectPanic(t, "invalid app ID", func() { CheckAppID("x", id) })
	}
	l := Launch{Args: []string{"a"}}
	c := l.Clone()
	c.Args[0] = "b"
	if l.Args[0] != "a" {
		t.Errorf("Launch.Clone shares Args")
	}
	t.Setenv("XDG_ACTIVATION_TOKEN", "")
	t.Setenv("DESKTOP_STARTUP_ID", "id_TIME42")
	if got := CurrentLaunch(); got.Token != "id_TIME42" || got.Dir == "" {
		t.Errorf("CurrentLaunch = %+v", got)
	}
	t.Setenv("XDG_ACTIVATION_TOKEN", "wl-token")
	if got := CurrentLaunch(); got.Token != "wl-token" {
		t.Errorf("CurrentLaunch prefers XDG_ACTIVATION_TOKEN: %+v", got)
	}
}

func TestAssociationChecks(t *testing.T) {
	app := HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: "/opt/notes/notes", Icon: "/opt/notes/icon.png"}
	CheckHandlerApp("x", app, true)
	CheckHandlerApp("x", HandlerApp{ID: "n", Name: "N", Exec: `C:\Program Files\n.exe`, Icon: `C:\n.exe,0`}, true)
	CheckHandlerApp("x", HandlerApp{ID: "n"}, false)
	expectPanic(t, "not an absolute path", func() { CheckHandlerApp("x", HandlerApp{ID: "n", Name: "N", Exec: "notes"}, true) })
	expectPanic(t, "Name is empty", func() { CheckHandlerApp("x", HandlerApp{ID: "n", Exec: "/n"}, true) })
	for _, a := range []Association{{Scheme: "notes"}, {Scheme: "web+notes"}, {Extension: ".note", MIMEType: "application/x-note", Description: "Note"}} {
		CheckAssociation("x", a)
	}
	for msg, a := range map[string]Association{
		"empty Association":  {},
		"both":               {Scheme: "a", Extension: ".a"},
		"invalid scheme":     {Scheme: "notes:"},
		"standard scheme":    {Scheme: "HTTPS"},
		"invalid extension":  {Extension: "note", MIMEType: "a/b", Description: "x"},
		"needs a MIMEType":   {Extension: ".note", Description: "x"},
		"needs a Descripti…": {Extension: ".note", MIMEType: "a/b"},
	} {
		if msg == "needs a Descripti…" {
			msg = "needs a Description"
		}
		expectPanic(t, msg, func() { CheckAssociation("x", a) })
	}
	if (Association{Scheme: "notes"}).String() != "notes:" {
		t.Errorf("Association.String")
	}
	if err := TakenError(Association{Scheme: "notes"}, "org.x"); !errors.Is(err, ErrAssociationTaken) {
		t.Errorf("TakenError does not wrap ErrAssociationTaken")
	}
	var u Associations = UnsupportedAssociations{}
	if _, err := u.Status(app, Association{Scheme: "notes"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("UnsupportedAssociations.Status = %v", err)
	}
	expectPanic(t, "empty Association", func() { _, _ = u.Unregister(app, Association{}) })
}

func TestUnsupportedDesktopV3(t *testing.T) {
	d := UnsupportedDesktop{}
	errs := []error{
		d.RegisterShortcut(Shortcut{ID: "a", Accelerator: Accelerator{Key: "a"}, Description: "A"}),
		d.UnregisterShortcut("a"),
		d.SetSecret("app", "n", []byte("x")),
		d.DeleteSecret("app", "n"),
	}
	_, err := d.Secret("app", "n")
	errs = append(errs, err)
	_, err = d.ClaimInstance("app", Launch{})
	errs = append(errs, err)
	_, err = d.Associations().Status(HandlerApp{ID: "app"}, Association{Scheme: "x"})
	errs = append(errs, err)
	for i, err := range errs {
		if !errors.Is(err, ErrUnsupported) {
			t.Errorf("call %d: %v, want ErrUnsupported", i, err)
		}
	}
	if err := d.SetSecret("app", "n", make([]byte, MaxSecretSize+1)); !errors.Is(err, ErrSecretTooLarge) {
		t.Errorf("SetSecret of an oversized secret = %v, want ErrSecretTooLarge before ErrUnsupported", err)
	}
	expectPanic(t, "no Description", func() { _ = d.RegisterShortcut(Shortcut{ID: "a", Accelerator: Accelerator{Key: "a"}}) })
	expectPanic(t, "invalid app ID", func() { _, _ = d.ClaimInstance("", Launch{}) })
	var _ Event = DragEvent{}
	var _ Event = DragDoneEvent{}
	var _ Event = MenuEvent{}
	var _ Event = ShortcutEvent{}
	var _ Event = ShortcutStatusEvent{}
	var _ Event = LaunchEvent{}
}
