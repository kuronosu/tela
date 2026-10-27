package win32

import (
	"errors"
	"image"
	"image/color"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure logic of the desktop services (dialog.go, shell.go, tray.go),
// tested on every OS; live_desktop_test.go shows the real thing on Windows.

func TestFileDialogFlags(t *testing.T) {
	const base = 0x1 // a default the dialog had, kept
	common := uint32(base | fosForceFileSystem | fosNoChangeDir | fosPathMustExist)
	for _, tc := range []struct {
		name     string
		kind     fileDialogKind
		multiple bool
		want     uint32
	}{
		{"open", dialogOpenFiles, false, common | fosFileMustExist},
		{"open multiple", dialogOpenFiles, true, common | fosFileMustExist | fosAllowMultiSelect},
		{"save", dialogSaveFile, false, common | fosOverwritePrompt},
		{"save ignores multiple", dialogSaveFile, true, common | fosOverwritePrompt},
		{"folder", dialogPickFolder, false, common | fosPickFolders},
		{"folder ignores multiple", dialogPickFolder, true, common | fosPickFolders},
	} {
		if got := fileDialogFlags(tc.kind, base, tc.multiple); got != tc.want {
			t.Errorf("%s: fileDialogFlags = %#x, want %#x", tc.name, got, tc.want)
		}
	}
}

func TestFilterSpecs(t *testing.T) {
	got := filterSpecs([]platform.FileFilter{
		{Name: "Images", Patterns: []string{"*.png", "*.jpg"}},
		{Patterns: []string{"*.txt"}},
		{Name: "All files", Patterns: []string{"*"}},
		{Name: "bad\x00name", Patterns: []string{"*.x"}},
	})
	want := []filterSpec{
		{"Images", "*.png;*.jpg"},
		{"*.txt", "*.txt"},
		{"All files", "*"},
		{"badname", "*.x"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("filterSpecs = %q, want %q", got, want)
	}
	if got := filterSpecs(nil); len(got) != 0 {
		t.Errorf("filterSpecs(nil) = %q, want none", got)
	}
}

func TestDefaultExtension(t *testing.T) {
	for _, tc := range []struct {
		patterns []string
		want     string
	}{
		{[]string{"*.png", "*.jpg"}, "png"},
		{[]string{"*.tar.gz"}, ""},
		{[]string{"*"}, ""},
		{[]string{"*.*"}, ""},
		{[]string{"*.p?g"}, ""},
		{[]string{"report.txt"}, ""},
		{[]string{"*."}, ""},
	} {
		if got := defaultExtension([]platform.FileFilter{{Name: "x", Patterns: tc.patterns}}); got != tc.want {
			t.Errorf("defaultExtension(%q) = %q, want %q", tc.patterns, got, tc.want)
		}
	}
	if got := defaultExtension(nil); got != "" {
		t.Errorf("defaultExtension(nil) = %q, want \"\"", got)
	}
}

func TestHRESULT(t *testing.T) {
	canceled, changed := hrCanceled, hrChangedMode
	if uint32(canceled) != 0x800704C7 || uint32(changed) != 0x80010106 {
		t.Errorf("hrCanceled = %#x, hrChangedMode = %#x", uint32(canceled), uint32(changed))
	}
	if code, ok := hresultFacilityWin32(hrCanceled); !ok || code != 1223 { // ERROR_CANCELLED
		t.Errorf("hresultFacilityWin32(hrCanceled) = %d, %v; want 1223, true", code, ok)
	}
	if _, ok := hresultFacilityWin32(hrChangedMode); ok {
		t.Errorf("RPC_E_CHANGED_MODE is not a Win32 code")
	}
	if got := hresultString(hrChangedMode); got != "HRESULT 0x80010106" {
		t.Errorf("hresultString = %q", got)
	}
}

func TestMessageBoxStyle(t *testing.T) {
	for _, tc := range []struct {
		kind    platform.MessageKind
		buttons platform.MessageButtons
		owned   bool
		want    uint32
	}{
		{platform.MessageInfo, platform.ButtonsOK, true, mbOK | mbIconInformation},
		{platform.MessageWarning, platform.ButtonsOKCancel, true, mbOKCancel | mbIconWarning},
		{platform.MessageError, platform.ButtonsYesNo, true, mbYesNo | mbIconError},
		{platform.MessageQuestion, platform.ButtonsYesNoCancel, true, mbYesNoCancel | mbIconQuestion},
		{platform.MessageInfo, platform.ButtonsOK, false, mbOK | mbIconInformation | mbSetForeground},
	} {
		got := messageBoxStyle(platform.MessageBoxOptions{Kind: tc.kind, Buttons: tc.buttons}, tc.owned)
		if got != tc.want {
			t.Errorf("messageBoxStyle(%d, %d, owned %v) = %#x, want %#x", tc.kind, tc.buttons, tc.owned, got, tc.want)
		}
	}
}

func TestMessageBoxAnswer(t *testing.T) {
	for _, tc := range []struct {
		id      int32
		buttons platform.MessageButtons
		want    platform.MessageAnswer
		ok      bool
	}{
		{idOK, platform.ButtonsOK, platform.AnswerOK, true},
		{idOK, platform.ButtonsOKCancel, platform.AnswerOK, true},
		{idCancel, platform.ButtonsOKCancel, platform.AnswerCancel, true},
		{idYes, platform.ButtonsYesNo, platform.AnswerYes, true},
		{idNo, platform.ButtonsYesNo, platform.AnswerNo, true},
		{idCancel, platform.ButtonsYesNoCancel, platform.AnswerCancel, true},
		// Not a button of the set: the set's dismiss answer.
		{idCancel, platform.ButtonsOK, platform.AnswerOK, true},
		{idCancel, platform.ButtonsYesNo, platform.AnswerNo, true},
		{99, platform.ButtonsYesNoCancel, platform.AnswerCancel, true},
		// 0: MessageBoxW failed.
		{0, platform.ButtonsYesNo, platform.AnswerNo, false},
	} {
		got, ok := messageBoxAnswer(tc.id, tc.buttons)
		if got != tc.want || ok != tc.ok {
			t.Errorf("messageBoxAnswer(%d, %d) = %v, %v; want %v, %v", tc.id, tc.buttons, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCheckShellURI(t *testing.T) {
	for _, uri := range []string{"https://example.com/a?b=c", "mailto:someone@example.com", "ms-settings:display", "tel:+1555"} {
		if err := checkShellURI(uri); err != nil {
			t.Errorf("checkShellURI(%q) = %v, want nil", uri, err)
		}
	}
	for _, uri := range []string{"C:/Windows/notepad.exe", `c:\Windows\notepad.exe`, "file:///C:/x", "no scheme", "", "https://a\x00b"} {
		if err := checkShellURI(uri); err == nil {
			t.Errorf("checkShellURI(%q) = nil, want an error", uri)
		}
	}
}

func TestShellExecuteError(t *testing.T) {
	if err := shellExecuteError(33, "x"); err != nil {
		t.Errorf("shellExecuteError(33) = %v, want nil (a code above 32 is success)", err)
	}
	for _, tc := range []struct {
		code uintptr
		is   error
		text string
	}{
		{2, fs.ErrNotExist, "does not exist"},
		{3, fs.ErrNotExist, "does not exist"},
		{5, fs.ErrPermission, "permission denied"},
		{31, errNoApplication, "no application"},
		{0, nil, "out of memory"},
		{8, nil, "out of memory"},
		{27, nil, "association"},
		{29, nil, "DDE"},
		{17, nil, "error 17"},
	} {
		err := shellExecuteError(tc.code, `C:\x.abc`)
		switch {
		case err == nil:
			t.Errorf("shellExecuteError(%d) = nil", tc.code)
		case tc.is != nil && !errors.Is(err, tc.is):
			t.Errorf("shellExecuteError(%d) = %v, want it to wrap %v", tc.code, err, tc.is)
		case !strings.Contains(err.Error(), tc.text) || !strings.Contains(err.Error(), `C:\\x.abc`):
			t.Errorf("shellExecuteError(%d) = %q, want the target and %q", tc.code, err, tc.text)
		}
	}
}

func TestTrayCallback(t *testing.T) {
	// Version 4: event and icon ID in lParam, anchor point in wParam
	// (signed: a monitor left of the primary one has negative x).
	event, uid, x, y := trayCallback(mouseParam(-20, 1060), uintptr(7)<<16|trayContextMenu)
	if event != trayContextMenu || uid != 7 || x != -20 || y != 1060 {
		t.Errorf("trayCallback = %#x, %d, (%d, %d); want WM_CONTEXTMENU, 7, (-20, 1060)", event, uid, x, y)
	}
	event, uid, _, _ = trayCallback(0, uintptr(0xFFFF)<<16|ninSelect)
	if event != ninSelect || uid != 0xFFFF {
		t.Errorf("trayCallback = %#x, %d; want NIN_SELECT, 65535", event, uid)
	}
}

func mouseParam(x, y int) uintptr { return uintptr(uint16(int16(x))) | uintptr(uint16(int16(y)))<<16 }

func TestBalloonEvent(t *testing.T) {
	for event, want := range map[uint16]platform.NotificationEventKind{
		ninBalloonUserClick: platform.NotificationActivated,
		ninBalloonTimeout:   platform.NotificationClosed,
		ninBalloonHide:      platform.NotificationClosed,
	} {
		if got, ok := balloonEvent(event); !ok || got != want {
			t.Errorf("balloonEvent(%#x) = %v, %v; want %v", event, got, ok, want)
		}
	}
	for _, event := range []uint16{ninBalloonShow, ninSelect, ninKeySelect, trayContextMenu, wmLButtonUp} {
		if _, ok := balloonEvent(event); ok {
			t.Errorf("balloonEvent(%#x) reports an event", event)
		}
	}
}

func TestPutUTF16(t *testing.T) {
	decode := func(u []uint16) string {
		return fromUTF16Clipboard(u)
	}
	var buf [8]uint16
	putUTF16(buf[:], "tela")
	if got := decode(buf[:]); got != "tela" || buf[4] != 0 {
		t.Errorf("putUTF16(tela) = %q", got)
	}
	putUTF16(buf[:], "0123456789")
	if got := decode(buf[:]); got != "0123456" || buf[7] != 0 {
		t.Errorf("putUTF16 cut = %q, want the first 7 characters and a NUL", got)
	}
	// "abcdef" then an emoji (a surrogate pair) at units 6 and 7: the pair
	// does not fit before the NUL and is dropped whole.
	putUTF16(buf[:], "abcdef😀")
	if got := decode(buf[:]); got != "abcdef" || buf[6] != 0 {
		t.Errorf("putUTF16 with a cut surrogate pair = %q (%x)", got, buf)
	}
	putUTF16(buf[:], "a\x00b\xff")
	if got := decode(buf[:]); got != "ab\uFFFD" {
		t.Errorf("putUTF16 sanitizes = %q", got)
	}
	putUTF16(buf[:], "")
	if buf != [8]uint16{} {
		t.Errorf("putUTF16(\"\") left %x", buf)
	}
	putUTF16(nil, "x") // no room: nothing to do
}

func TestBalloonText(t *testing.T) {
	for _, tc := range []struct{ title, body, wantTitle, wantBody string }{
		{"Saved", "The file was saved.", "Saved", "The file was saved."},
		{"", "Body only", "", "Body only"},
		{"Title only", "", "", "Title only"},
		{"", "", "", " "},
	} {
		title, body := balloonText(tc.title, tc.body)
		if title != tc.wantTitle || body != tc.wantBody {
			t.Errorf("balloonText(%q, %q) = %q, %q; want %q, %q", tc.title, tc.body, title, body, tc.wantTitle, tc.wantBody)
		}
	}
}

func TestBuildMenu(t *testing.T) {
	entries, cmds := buildMenu([]platform.MenuItem{
		{ID: 100, Label: "Open & Show"},
		{Separator: true, Label: "ignored", Disabled: true},
		{Label: "More", Submenu: []platform.MenuItem{
			{ID: 7, Label: "Dark\tmode", Checkable: true, Checked: true},
			{ID: 8, Label: "Light", Checkable: true},
			{Label: "Deeper", Disabled: true, Submenu: []platform.MenuItem{{ID: 9, Label: "x"}}},
		}},
		{ID: 1 << 30, Label: "Quit", Disabled: true},
	})
	if want := []platform.MenuItemID{100, 7, 8, 9, 1 << 30}; !slices.Equal(cmds, want) {
		t.Fatalf("cmds = %v, want %v", cmds, want)
	}
	want := []menuEntry{
		{label: "Open && Show", flags: mfString, cmd: 1},
		{flags: mfSeparator},
		{label: "More", flags: mfPopup, sub: []menuEntry{
			{label: "Dark mode", flags: mfChecked, cmd: 2},
			{label: "Light", cmd: 3},
			{label: "Deeper", flags: mfPopup | mfGrayed, sub: []menuEntry{{label: "x", cmd: 4}}},
		}},
		{label: "Quit", flags: mfGrayed, cmd: 5},
	}
	if !menuEntriesEqual(entries, want) {
		t.Errorf("buildMenu entries = %+v\nwant %+v", entries, want)
	}
	if entries, cmds := buildMenu(nil); len(entries) != 0 || len(cmds) != 0 {
		t.Errorf("buildMenu(nil) = %v, %v", entries, cmds)
	}
}

func menuEntriesEqual(a, b []menuEntry) bool {
	return slices.EqualFunc(a, b, func(x, y menuEntry) bool {
		return x.label == y.label && x.flags == y.flags && x.cmd == y.cmd && menuEntriesEqual(x.sub, y.sub)
	})
}

func TestShellIconSide(t *testing.T) {
	for _, tc := range []struct {
		base int
		dpi  uint32
		want int
	}{
		{16, 96, 16}, {16, 120, 20}, {16, 144, 24}, {16, 168, 28}, {16, 192, 32}, {16, 0, 16},
		{32, 96, 32}, {32, 144, 48}, {32, 192, 64},
	} {
		if got := shellIconSide(tc.base, tc.dpi); got != tc.want {
			t.Errorf("shellIconSide(%d, %d) = %d, want %d", tc.base, tc.dpi, got, tc.want)
		}
	}
}

func TestShellIconBGRA(t *testing.T) {
	// Same size: a channel swap, alpha kept straight. The image's origin
	// need not be (0, 0).
	img := image.NewNRGBA(image.Rect(5, 5, 7, 7))
	img.SetNRGBA(5, 5, color.NRGBA{R: 255, G: 128, B: 0, A: 128})
	img.SetNRGBA(6, 6, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	got := shellIconBGRA(img, 2)
	want := []byte{0, 128, 255, 128, 0, 0, 0, 0, 0, 0, 0, 0, 30, 20, 10, 255}
	if !slices.Equal(got, want) {
		t.Errorf("shellIconBGRA same size = %v, want %v", got, want)
	}

	// Premultiplied input comes out straight.
	pm := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pm.SetRGBA(0, 0, color.RGBA{R: 100, A: 128})
	if got := shellIconBGRA(pm, 1); got[2] < 198 || got[2] > 200 || got[3] != 128 {
		t.Errorf("shellIconBGRA premultiplied = %v, want red near 199 and alpha 128", got)
	}

	// Another size: scaled to fit, centered, aspect kept. A 32x16 opaque
	// image in 16x16 is 16x8 in the middle rows.
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for i := 3; i < len(wide.Pix); i += 4 {
		wide.Pix[i-3], wide.Pix[i] = 255, 255
	}
	got = shellIconBGRA(wide, 16)
	if len(got) != 16*16*4 {
		t.Fatalf("len = %d", len(got))
	}
	alpha := func(x, y int) byte { return got[4*(y*16+x)+3] }
	for _, p := range []image.Point{{0, 0}, {8, 2}, {15, 15}, {8, 13}} {
		if a := alpha(p.X, p.Y); a != 0 {
			t.Errorf("padding at %v has alpha %d, want 0", p, a)
		}
	}
	for _, p := range []image.Point{{0, 4}, {8, 8}, {15, 11}} {
		if a, r := alpha(p.X, p.Y), got[4*(p.Y*16+p.X)+2]; a != 255 || r != 255 {
			t.Errorf("content at %v = alpha %d red %d, want opaque red", p, a, r)
		}
	}
}

func TestShellIconMaskSize(t *testing.T) {
	for side, want := range map[int]int{16: 32, 20: 80, 24: 96, 32: 128, 1: 2} {
		if got := shellIconMaskSize(side); got != want {
			t.Errorf("shellIconMaskSize(%d) = %d, want %d", side, got, want)
		}
	}
}

func TestIconIDs(t *testing.T) {
	var ids iconIDs
	a, b := ids.take(), ids.take()
	if a != 1 || b != 2 {
		t.Fatalf("first IDs = %d, %d; want 1, 2", a, b)
	}
	ids.release(a)
	// A released ID is not given again at once.
	if c := ids.take(); c != 3 {
		t.Errorf("after releasing 1, take = %d, want 3", c)
	}
	// The IDs go round, skipping 0 and the ones in use.
	ids.last = 0xFFFE
	if c, d := ids.take(), ids.take(); c != 0xFFFF || d != 1 {
		t.Errorf("wrap = %d, %d; want 65535, 1", c, d)
	}
	full := iconIDs{used: map[uint16]bool{}}
	for id := 1; id <= 0xFFFF; id++ {
		full.used[uint16(id)] = true
	}
	if id := full.take(); id != 0 {
		t.Errorf("take with every ID in use = %d, want 0", id)
	}
}

func TestMenuLabel(t *testing.T) {
	for in, want := range map[string]string{"Save": "Save", "R&D": "R&&D", "a\tb": "a b", "&&": "&&&&"} {
		if got := menuLabel(in); got != want {
			t.Errorf("menuLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
