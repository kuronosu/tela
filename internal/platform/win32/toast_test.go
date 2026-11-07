package win32

import (
	"encoding/xml"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// The pure logic of the WinRT toasts (toast.go), tested on every OS;
// toast_windows_test.go checks the COM objects on Windows and
// live_toast_test.go shows real toasts.

// TestPinterfaceIIDs derives the IDs of the toast event delegates from their
// signatures and checks them against the values of the Windows SDK headers,
// two independent sources for the IIDs toast_windows.go uses. IVector<String>
// and IAsyncOperation<Boolean> check the algorithm on well-known IDs.
func TestPinterfaceIIDs(t *testing.T) {
	for _, tc := range []struct{ name, sig, want string }{
		{"IVector<String>", "pinterface({913337e9-11a1-4345-a3a2-4e7f956e222d};string)", "98b9acc1-4b56-532e-ac73-03d5291cca90"},
		{"IAsyncOperation<Boolean>", "pinterface({9fc2b0bb-e446-44e2-aa61-9cab8f636af2};b1)", "cdb5efb3-5788-509d-9be1-71ccb8a3362a"},
		{"Activated", sigToastActivatedHandler, iidStrToastActivatedHandler},
		{"Dismissed", sigToastDismissedHandler, iidStrToastDismissedHandler},
		{"Failed", sigToastFailedHandler, iidStrToastFailedHandler},
	} {
		if got := pinterfaceIID(tc.sig); got != tc.want {
			t.Errorf("%s: pinterfaceIID(%q) = %s, want %s", tc.name, tc.sig, got, tc.want)
		}
	}
}

func TestParseGUIDFields(t *testing.T) {
	d1, d2, d3, d4 := parseGUIDFields(iidStrAgileObject)
	if d1 != 0x94ea2b94 || d2 != 0xe9cc || d3 != 0x49e0 || d4 != [8]byte{0xc0, 0xff, 0xee, 0x64, 0xca, 0x8f, 0x5b, 0x90} {
		t.Errorf("parseGUIDFields(%s) = %x %x %x %x", iidStrAgileObject, d1, d2, d3, d4)
	}
	defer func() {
		if recover() == nil {
			t.Errorf("parseGUIDFields of a malformed literal did not panic")
		}
	}()
	parseGUIDFields("94ea2b94-e9cc-49e0-c0ff")
}

func TestToastAUMID(t *testing.T) {
	long := strings.Repeat("a", 200)
	for _, id := range []string{"dev.telaui.notes", "notes", strings.Repeat("b", maxAUMID)} {
		if got := toastAUMID(id); got != id {
			t.Errorf("toastAUMID(%q) = %q, want it unchanged", id, got)
		}
	}
	seen := map[string]string{}
	for _, id := range []string{long, long + "x", "café.notes", "cafe_.notes", "a\x7fb"} {
		got := toastAUMID(id)
		if len(got) > maxAUMID || strings.ContainsFunc(got, func(r rune) bool { return r <= ' ' || r >= 0x7F || r == '\\' || r == '/' }) {
			t.Errorf("toastAUMID(%q) = %q: not a valid AUMID", id, got)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("toastAUMID(%q) = toastAUMID(%q) = %q", id, other, got)
		}
		seen[got] = id
	}
}

func TestToastArguments(t *testing.T) {
	for _, tc := range []struct {
		id     platform.NotificationID
		action string
		want   string
	}{
		{3, "", "tela-notification=3"},
		{3, "save", "tela-notification=3&action=save"},
		{4294967295, "a&b=c d", "tela-notification=4294967295&action=a%26b%3Dc+d"},
	} {
		got := toastArguments(tc.id, tc.action)
		if got != tc.want {
			t.Errorf("toastArguments(%d, %q) = %q, want %q", tc.id, tc.action, got, tc.want)
		}
		id, action, ok := parseToastArguments(got)
		if !ok || id != tc.id || action != tc.action {
			t.Errorf("parseToastArguments(%q) = %d, %q, %v", got, id, action, ok)
		}
	}
	for _, s := range []string{"", "default", "action=x", "tela-notification=0", "tela-notification=x", "tela-notification=1;%zz"} {
		if _, _, ok := parseToastArguments(s); ok {
			t.Errorf("parseToastArguments(%q) ok, want refused", s)
		}
	}
}

// toastDoc is the part of the toast schema the tests read back.
type toastDoc struct {
	XMLName xml.Name `xml:"toast"`
	Launch  string   `xml:"launch,attr"`
	Binding struct {
		Template string   `xml:"template,attr"`
		Texts    []string `xml:"text"`
		Images   []struct {
			Placement string `xml:"placement,attr"`
			Src       string `xml:"src,attr"`
		} `xml:"image"`
	} `xml:"visual>binding"`
	Actions []struct {
		Content   string `xml:"content,attr"`
		Arguments string `xml:"arguments,attr"`
	} `xml:"actions>action"`
}

func parseToast(t *testing.T, s string) toastDoc {
	t.Helper()
	var d toastDoc
	if err := xml.Unmarshal([]byte(s), &d); err != nil {
		t.Fatalf("toast XML does not parse: %v\n%s", err, s)
	}
	return d
}

func TestToastXML(t *testing.T) {
	c := toastContent{
		ID: 7, Title: `Fish & "Chips" <now>`, Body: "line one\nline two\x01", Image: "file:///C:/t/a%20b.png",
		Actions: []platform.NotificationAction{{ID: "open", Label: "Open & see"}, {ID: "x&y", Label: ""}},
	}
	s := c.xml()
	d := parseToast(t, s)
	if d.Launch != "tela-notification=7" || d.Binding.Template != "ToastGeneric" {
		t.Errorf("launch %q, template %q", d.Launch, d.Binding.Template)
	}
	if want := []string{`Fish & "Chips" <now>`, "line one\nline two\uFFFD"}; strings.Join(d.Binding.Texts, "|") != strings.Join(want, "|") {
		t.Errorf("texts = %q, want %q", d.Binding.Texts, want)
	}
	if len(d.Binding.Images) != 1 || d.Binding.Images[0].Placement != "appLogoOverride" || d.Binding.Images[0].Src != c.Image {
		t.Errorf("images = %+v", d.Binding.Images)
	}
	if len(d.Actions) != 2 || d.Actions[0].Content != "Open & see" || d.Actions[0].Arguments != "tela-notification=7&action=open" ||
		d.Actions[1].Content != "x&y" || d.Actions[1].Arguments != toastArguments(7, "x&y") {
		t.Errorf("actions = %+v", d.Actions)
	}

	for _, tc := range []struct {
		title, body string
		want        []string
	}{
		{"Only", "", []string{"Only"}},
		{"", "Body", []string{"Body"}},
		{"", "", []string{" "}},
	} {
		d := parseToast(t, toastContent{ID: 1, Title: tc.title, Body: tc.body}.xml())
		if strings.Join(d.Binding.Texts, "|") != strings.Join(tc.want, "|") || len(d.Actions) != 0 || len(d.Binding.Images) != 0 {
			t.Errorf("title %q body %q: texts %q, %d actions, %d images", tc.title, tc.body, d.Binding.Texts, len(d.Actions), len(d.Binding.Images))
		}
	}

	var many []platform.NotificationAction
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		many = append(many, platform.NotificationAction{ID: id, Label: id})
	}
	if d := parseToast(t, toastContent{ID: 1, Title: "t", Actions: many}.xml()); len(d.Actions) != maxToastActions {
		t.Errorf("%d actions in the XML, want %d", len(d.Actions), maxToastActions)
	}
}

func TestFileURI(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Users\a b\x.png`:         "file:///C:/Users/a%20b/x.png",
		`C:\t\100%#.png`:             "file:///C:/t/100%25%23.png",
		`\\server\share\icons\x.png`: "file://server/share/icons/x.png",
		`D:\Ünïcode\x.png`:           "file:///D:/%C3%9Cn%C3%AFcode/x.png",
	} {
		if got := fileURI(in); got != want {
			t.Errorf("fileURI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToastEvents(t *testing.T) {
	type out struct {
		ev       platform.NotificationEvent
		report   bool
		finished bool
	}
	for _, tc := range []struct {
		name string
		in   toastEvent
		want out
	}{
		{"body", toastEvent{id: 2, kind: toastActivated, args: "tela-notification=2"}, out{platform.NotificationEvent{ID: 2, Kind: platform.NotificationActivated}, true, true}},
		{"button", toastEvent{id: 2, kind: toastActivated, args: "tela-notification=2&action=save"}, out{platform.NotificationEvent{ID: 2, Kind: platform.NotificationActionInvoked, Action: "save"}, true, true}},
		{"foreign arguments", toastEvent{id: 2, kind: toastActivated, args: "something"}, out{platform.NotificationEvent{ID: 2, Kind: platform.NotificationActivated}, true, true}},
		{"user closed", toastEvent{id: 3, kind: toastDismissed, reason: toastUserCanceled}, out{platform.NotificationEvent{ID: 3, Kind: platform.NotificationClosed}, true, true}},
		{"hidden", toastEvent{id: 3, kind: toastDismissed, reason: toastApplicationHidden}, out{platform.NotificationEvent{ID: 3, Kind: platform.NotificationClosed}, true, true}},
		{"timed out", toastEvent{id: 3, kind: toastDismissed, reason: toastTimedOut}, out{platform.NotificationEvent{ID: 3}, false, false}},
		{"failed", toastEvent{id: 4, kind: toastFailed}, out{platform.NotificationEvent{ID: 4, Kind: platform.NotificationClosed}, true, true}},
	} {
		ev, report, finished := tc.in.notificationEvent()
		if got := (out{ev, report, finished}); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestRegisterAUMID(t *testing.T) {
	reg := &assoc.MemRegistry{}
	key := aumidKeyPrefix + "dev.telaui.notes"
	owned, changed, err := registerAUMID(reg, "dev.telaui.notes", "notes", `C:\c\icon.png`)
	if err != nil || !owned || !changed {
		t.Fatalf("first registration: owned %v changed %v err %v", owned, changed, err)
	}
	for name, want := range map[string]string{aumidMarker: "1", "DisplayName": "notes", "IconUri": `C:\c\icon.png`} {
		if v, ok, _ := reg.String(key, name); !ok || v != want {
			t.Errorf("%s = %q, %v; want %q", name, v, ok, want)
		}
	}
	writes := reg.Writes()
	if _, changed, err := registerAUMID(reg, "dev.telaui.notes", "notes", `C:\c\icon.png`); changed || err != nil || reg.Writes() != writes {
		t.Errorf("same registration again: changed %v err %v, %d writes", changed, err, reg.Writes()-writes)
	}
	// No icon this time: the registered one stays.
	if _, changed, _ := registerAUMID(reg, "dev.telaui.notes", "Notes", ""); !changed {
		t.Errorf("new display name: not changed")
	}
	if v, _, _ := reg.String(key, "IconUri"); v != `C:\c\icon.png` {
		t.Errorf("IconUri = %q after a registration without an icon", v)
	}

	// Someone else's key is left alone.
	other := aumidKeyPrefix + "com.example.installed"
	_ = reg.SetString(other, "DisplayName", "Installed")
	_ = reg.SetString(other, "CustomActivator", "{00000000-0000-0000-0000-000000000001}")
	writes = reg.Writes()
	owned, changed, err = registerAUMID(reg, "com.example.installed", "installed", `C:\x.png`)
	if owned || changed || err != nil || reg.Writes() != writes {
		t.Errorf("foreign key: owned %v changed %v err %v, %d writes", owned, changed, err, reg.Writes()-writes)
	}
}

func TestToastIconCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "icons")
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.SetNRGBA(1, 1, color.NRGBA{R: 200, A: 255})
	now := time.Now()
	p1, err := writeToastIcon(dir, img, now)
	if err != nil {
		t.Fatalf("writeToastIcon: %v", err)
	}
	p2, err := writeToastIcon(dir, img, now)
	if err != nil || p2 != p1 {
		t.Errorf("same image again: %q, %v; want %q", p2, err, p1)
	}
	img.SetNRGBA(2, 2, color.NRGBA{G: 200, A: 255})
	p3, err := writeToastIcon(dir, img, now)
	if err != nil || p3 == p1 {
		t.Errorf("another image: %q, %v", p3, err)
	}
	old := now.Add(-toastIconMaxAge - time.Hour)
	for _, p := range []string{p1, p3} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	// keep compares paths ignoring case, as Windows file names are.
	pruneToastIcons(dir, []string{strings.ToUpper(p3)}, now)
	if _, err := os.Stat(p1); !os.IsNotExist(err) {
		t.Errorf("old icon %s not pruned (%v)", p1, err)
	}
	if _, err := os.Stat(p3); err != nil {
		t.Errorf("kept icon %s pruned: %v", p3, err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temporary file %s left", e.Name())
		}
	}
}

func TestToastGroup(t *testing.T) {
	a, b := toastGroup(time.Unix(0, 1)), toastGroup(time.Unix(1, 0))
	if a == b || !strings.HasPrefix(a, "tela-") || len(b) > 64 {
		t.Errorf("toastGroup = %q, %q", a, b)
	}
}
