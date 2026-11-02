package cocoa

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"reflect"
	"testing"
	"time"

	"golang.org/x/image/tiff"

	"github.com/kuronosu/tela/internal/platform"
)

func TestCustomTypeRoundTrip(t *testing.T) {
	for _, m := range []string{
		"application/x-tela-test",
		"application/x-Tela.Item+json;version=2",
		"text/x-a_b",
		"application/vnd.ms-excel",
	} {
		uti := customType(m)
		for i := len(customPrefix); i < len(uti); i++ {
			c := uti[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				t.Errorf("customType(%q) = %q has %q, which a uniform type identifier would not keep as is", m, uti, c)
			}
		}
		got, ok := customMIME(uti)
		if !ok || got != m {
			t.Errorf("customMIME(customType(%q) = %q) = %q, %v", m, uti, got, ok)
		}
	}
	for _, uti := range []string{"public.png", "dev.telaui.mime.", "dev.telaui.mime.text-2", "dev.telaui.mime.text-zzplain", "dev.telaui.mime.text"} {
		if m, ok := customMIME(uti); ok {
			t.Errorf("customMIME(%q) = %q, true; want false", uti, m)
		}
	}
}

func TestPasteboardItems(t *testing.T) {
	custom := func(m string) string { return "uti." + m }
	d := platform.Data{
		{Type: platform.MIMEHTML, Bytes: []byte("<b>x</b>")},
		{Type: platform.MIMEURIList, Bytes: platform.URIList([]string{"file:///a/b%20c", "https://telaui.dev/", "file:///d"})},
		{Type: platform.MIMEText, Bytes: []byte("x")},
		{Type: platform.MIMEPNG, Bytes: []byte{0x89, 'P'}},
		{Type: "application/x-y", Bytes: []byte{0, 1}},
	}
	want := []pbItem{
		{
			{uti: utiHTML, data: []byte("<b>x</b>"), text: true},
			{uti: utiFileURL, data: []byte("file:///a/b%20c"), text: true},
			{uti: utiUTF8Text, data: []byte("x"), text: true},
			{uti: utiPNG, data: []byte{0x89, 'P'}},
			{uti: "uti.application/x-y", data: []byte{0, 1}},
		},
		{{uti: utiURL, data: []byte("https://telaui.dev/"), text: true}},
		{{uti: utiFileURL, data: []byte("file:///d"), text: true}},
	}
	if got := pasteboardItems(d, custom); !reflect.DeepEqual(got, want) {
		t.Errorf("pasteboardItems =\n%v\nwant\n%v", got, want)
	}
	if got := pasteboardItems(nil, custom); got != nil {
		t.Errorf("pasteboardItems(nil) = %v, want none", got)
	}
	if got := pasteboardItems(platform.Data{{Type: platform.MIMEURIList, Bytes: []byte("# nothing\r\n")}}, custom); got != nil {
		t.Errorf("an empty URI list gives items %v, want none", got)
	}
	files := pasteboardItems(platform.Data{{Type: platform.MIMEURIList, Bytes: platform.URIList([]string{"file:///a", "file:///b"})}}, custom)
	if len(files) != 2 || files[0][0].uti != utiFileURL || files[1][0].uti != utiFileURL {
		t.Errorf("two files give %v, want two public.file-url items", files)
	}
}

func TestOfferedTypes(t *testing.T) {
	mimeOf := func(uti string) (string, bool) {
		switch uti {
		case "public.rtf":
			return "text/rtf", true
		case "public.json":
			return "application/json", true
		}
		return customMIME(uti)
	}
	items := [][]string{
		{"public.html", "Apple HTML pasteboard type", "public.utf8-plain-text", "NSStringPboardType", "com.apple.private", "public.rtf", "public.tiff", "public.png"},
		{"public.file-url", customType("application/x-tela"), "public.json"},
		{"public.url"},
	}
	want := []string{platform.MIMEHTML, platform.MIMEText, "text/rtf", platform.MIMEPNG, platform.MIMEURIList, "application/x-tela", "application/json"}
	if got := offeredTypes(items, mimeOf); !reflect.DeepEqual(got, want) {
		t.Errorf("offeredTypes = %q, want %q", got, want)
	}
	if got := offeredTypes(nil, mimeOf); got != nil {
		t.Errorf("offeredTypes(nil) = %q, want nil", got)
	}
}

func TestTypesFor(t *testing.T) {
	mimeOf := func(uti string) (string, bool) {
		if uti == "com.example.text" {
			return platform.MIMEText, true
		}
		if uti == "dyn.abc" {
			return "application/x-tela", true
		}
		return "", false
	}
	offered := []string{"NSStringPboardType", "com.example.text", "public.tiff", "public.utf8-plain-text", "public.png", "dyn.abc", customType("application/x-tela")}
	for _, c := range []struct {
		mime      string
		preferred []string
		want      []string
	}{
		{platform.MIMEText, nil, []string{"public.utf8-plain-text", "NSStringPboardType", "com.example.text"}},
		{platform.MIMEPNG, nil, []string{"public.png", "public.tiff"}},
		{"application/x-tela", []string{"dyn.missing", customType("application/x-tela")}, []string{customType("application/x-tela"), "dyn.abc"}},
		{"application/x-none", []string{"dyn.none"}, nil},
	} {
		if got := typesFor(c.mime, offered, c.preferred, mimeOf); !reflect.DeepEqual(got, c.want) {
			t.Errorf("typesFor(%q) = %q, want %q", c.mime, got, c.want)
		}
	}
}

func TestPNGFromTIFF(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(2, 1, color.NRGBA{0, 0, 255, 128})
	var tb bytes.Buffer
	if err := tiff.Encode(&tb, img, &tiff.Options{Compression: tiff.Deflate}); err != nil {
		t.Fatal(err)
	}
	p, err := pngFromTIFF(tb.Bytes())
	if err != nil {
		t.Fatalf("pngFromTIFF: %v", err)
	}
	got, err := png.Decode(bytes.NewReader(p))
	if err != nil {
		t.Fatalf("the result is not a PNG: %v", err)
	}
	for _, pt := range []image.Point{{0, 0}, {1, 0}, {2, 1}} {
		if g, w := color.NRGBAModel.Convert(got.At(pt.X, pt.Y)), img.At(pt.X, pt.Y); g != w {
			t.Errorf("pixel %v = %v, want %v", pt, g, w)
		}
	}
	if _, err := pngFromTIFF([]byte("not a tiff")); err == nil {
		t.Errorf("pngFromTIFF of garbage succeeded")
	}
}

func TestURLHelpers(t *testing.T) {
	if urlType("file:///a") != utiFileURL || urlType("FILE:///a") != utiFileURL || urlType("https://x/") != utiURL || urlType("fil") != utiURL {
		t.Errorf("urlType maps file and other URIs wrongly")
	}
	if !isFileReferenceURL("file:///.file/id=6571367.2773272/") || isFileReferenceURL("file:///Users/a/.file") {
		t.Errorf("isFileReferenceURL is wrong")
	}
	if !isStringType(utiHTML) || !isStringType("public.utf16-plain-text") || isStringType(utiPNG) || isStringType(utiFileURL) {
		t.Errorf("isStringType is wrong")
	}
	if !isTIFF(utiTIFF) || !isTIFF("NeXT TIFF v4.0 pasteboard type") || isTIFF(utiPNG) {
		t.Errorf("isTIFF is wrong")
	}
	if !needsCustomType("application/x-a;v=2") || needsCustomType("application/json") {
		t.Errorf("needsCustomType is wrong")
	}
}

func TestSettingsFrom(t *testing.T) {
	s := settingsFrom(rawSettings{Dark: true, AccentR: 0, AccentG: 122.0 / 255, AccentB: 1, HasAccent: true,
		ReduceMotion: true, IncreaseContrast: true, DoubleClick: 0.5})
	want := platform.SystemSettings{DarkMode: true, ReducedMotion: true, HighContrast: true,
		DoubleClickInterval: 500 * time.Millisecond, AccentColor: color.NRGBA{0, 122, 255, 255}, HasAccentColor: true, TextScale: 1}
	if s != want {
		t.Errorf("settingsFrom = %+v, want %+v", s, want)
	}
	platform.CheckSystemSettings(s)
	if s := settingsFrom(rawSettings{}); s.HasAccentColor || s.DoubleClickInterval != 0 || s.TextScale != 1 {
		t.Errorf("settingsFrom(zero) = %+v", s)
	}
	for _, c := range []struct {
		v    float64
		want uint8
	}{{-0.1, 0}, {0, 0}, {0.5, 128}, {1, 255}, {1.2, 255}, {math.NaN(), 0}} {
		if got := channel8(c.v); got != c.want {
			t.Errorf("channel8(%v) = %d, want %d", c.v, got, c.want)
		}
	}
	for _, c := range []struct {
		sec  float64
		want time.Duration
	}{{0.25, 250 * time.Millisecond}, {0.4999, 500 * time.Millisecond}, {0, 0}, {-1, 0}, {math.NaN(), 0}, {60, 0}} {
		if got := doubleClickFrom(c.sec); got != c.want {
			t.Errorf("doubleClickFrom(%v) = %v, want %v", c.sec, got, c.want)
		}
	}
}
