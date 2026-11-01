package win32

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func reg(name string) clipFormat {
	return clipFormat{id: cfFirstRegistered + uint32(len(name)), name: name}
}

func TestFormatTypes(t *testing.T) {
	for _, tc := range []struct {
		f    clipFormat
		want string
	}{
		{clipFormat{id: cfUnicodeText}, platform.MIMEText},
		{clipFormat{id: cfHDrop}, platform.MIMEURIList},
		{clipFormat{id: cfDIBV5}, platform.MIMEPNG},
		{clipFormat{id: cfDIB}, platform.MIMEPNG},
		{reg(cfNameHTML), platform.MIMEHTML},
		{reg(cfNamePNG), platform.MIMEPNG},
		{reg("image/png"), platform.MIMEPNG},
		{reg("application/x-tela-item"), "application/x-tela-item"},
		{reg("text/html; charset=utf-8"), "text/html;charset=utf-8"},
		{reg("text/plain"), platform.MIMEText},
		{clipFormat{id: 1}, ""},                 // CF_TEXT: Windows synthesizes CF_UNICODETEXT next to it
		{clipFormat{id: 16}, ""},                // CF_LOCALE
		{reg("Shell IDList Array"), ""},         // no MIME name
		{reg("FileNameW"), ""},                  // no MIME name
		{clipFormat{id: cfFirstRegistered}, ""}, // registered, name unknown
	} {
		got, ok := formatType(tc.f)
		if tc.want == "" {
			if ok {
				t.Errorf("formatType(%+v) = %q, want none", tc.f, got)
			}
			continue
		}
		if !ok || got != tc.want {
			t.Errorf("formatType(%+v) = %q, %v; want %q", tc.f, got, ok, tc.want)
		}
	}
}

func TestOfferedTypesAndPlans(t *testing.T) {
	// What the clipboard enumerates after a browser copied an image and
	// its HTML: the formats in the owner's order, plus the synthesized
	// ones (CF_TEXT, CF_LOCALE, CF_DIB after CF_DIBV5).
	html, png, custom := reg(cfNameHTML), reg(cfNamePNG), reg("application/x-tela-item")
	offered := []clipFormat{html, {id: cfDIBV5}, png, {id: cfUnicodeText}, custom, {id: 1}, {id: 16}, {id: cfDIB}, reg("image/png")}
	want := []string{platform.MIMEHTML, platform.MIMEPNG, platform.MIMEText, "application/x-tela-item"}
	if got := offeredTypes(offered); !slices.Equal(got, want) {
		t.Errorf("offeredTypes = %q, want %q", got, want)
	}
	if got := offeredTypes(nil); got != nil {
		t.Errorf("offeredTypes(nil) = %q, want nil", got)
	}
	// PNG first, then the other registered name, then the bitmaps.
	if got, want := formatsFor(platform.MIMEPNG, offered), []clipFormat{png, reg("image/png"), {id: cfDIBV5}, {id: cfDIB}}; !slices.Equal(got, want) {
		t.Errorf("formatsFor(PNG) = %+v, want %+v", got, want)
	}
	if got := formatsFor(platform.MIMEText, offered); !slices.Equal(got, []clipFormat{{id: cfUnicodeText}}) {
		t.Errorf("formatsFor(text) = %+v", got)
	}
	if got := formatsFor("application/x-other", offered); got != nil {
		t.Errorf("formatsFor(absent type) = %+v, want nil", got)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	img := testImage()
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	dir := "/tmp/tela dnd"
	if runtime.GOOS == "windows" {
		dir = `C:\tela dnd`
	}
	files := []string{dir + "/a b.txt", dir + "/ü.png"}
	if runtime.GOOS == "windows" {
		files = []string{dir + `\a b.txt`, dir + `\ü.png`}
	}
	uris := make([]string, len(files))
	for i, p := range files {
		u, err := platform.FileURI(p)
		if err != nil {
			t.Fatal(err)
		}
		uris[i] = u
	}
	d := platform.Data{
		{Type: platform.MIMEHTML, Bytes: []byte("<b>tela ✓</b>")},
		{Type: platform.MIMEText, Bytes: []byte("tela ✓\r\nline 😀")},
		{Type: platform.MIMEPNG, Bytes: pngBytes.Bytes()},
		{Type: platform.MIMEURIList, Bytes: platform.URIList(append(uris, "https://example.com/x"))},
		{Type: "application/x-tela-item", Bytes: []byte{0, 1, 2, 0xFF}},
	}

	enc := encodeData(d, true)
	var formats []clipFormat
	raw := map[clipFormat][]byte{}
	for _, f := range enc {
		cf := clipFormat{id: f.id}
		if f.id == 0 {
			cf = reg(f.name)
		}
		formats = append(formats, cf)
		raw[cf] = f.data
	}
	wantFormats := []clipFormat{reg(cfNameHTML), {id: cfUnicodeText}, reg(cfNamePNG), {id: cfDIBV5}, {id: cfDIB}, {id: cfHDrop}, reg("application/x-tela-item")}
	if !slices.Equal(formats, wantFormats) {
		t.Fatalf("encodeData formats = %+v, want %+v", formats, wantFormats)
	}
	if got := encodeData(d, false); len(got) != len(enc)-1 {
		t.Errorf("encodeData for the clipboard has %d formats, want %d (no CF_DIB)", len(got), len(enc)-1)
	}
	if got := offeredTypes(formats); !slices.Equal(got, d.Types()) {
		t.Errorf("types of the encoded formats = %q, want %q", got, d.Types())
	}

	for _, e := range d {
		plan := formatsFor(e.Type, formats)
		if len(plan) == 0 {
			t.Errorf("no format carries %s", e.Type)
			continue
		}
		got, err := decodeFormat(plan[0], raw[plan[0]])
		if err != nil {
			t.Errorf("decode %s: %v", e.Type, err)
			continue
		}
		want := e.Bytes
		if e.Type == platform.MIMEURIList {
			want = platform.URIList(uris) // the https URI has no CF_HDROP form
		}
		if !bytes.Equal(got, want) {
			t.Errorf("round trip of %s = %q, want %q", e.Type, got, want)
		}
	}
	// The bitmaps decode to the same pixels.
	for _, f := range []clipFormat{{id: cfDIBV5}, {id: cfDIB}} {
		got, err := decodeFormat(f, raw[f])
		if err != nil {
			t.Fatalf("decode format %d: %v", f.id, err)
		}
		checkPNGPixels(t, got, img)
	}
}

func TestEncodeSkips(t *testing.T) {
	// A PNG that does not decode is written as "PNG" only, and a URI list
	// without a local file has no CF_HDROP.
	d := platform.Data{
		{Type: platform.MIMEPNG, Bytes: []byte("not a png")},
		{Type: platform.MIMEURIList, Bytes: platform.URIList([]string{"https://example.com/"})},
	}
	enc := encodeData(d, true)
	if len(enc) != 1 || enc[0].name != cfNamePNG {
		t.Errorf("encodeData = %+v, want only PNG", enc)
	}
}

func TestUnicodeText(t *testing.T) {
	b := utf16LEText("a😀\r\nb")
	if want := []byte{'a', 0, 0x3D, 0xD8, 0x00, 0xDE, '\r', 0, '\n', 0, 'b', 0, 0, 0}; !bytes.Equal(b, want) {
		t.Errorf("utf16LEText = % x, want % x", b, want)
	}
	if got := textFromUTF16LE(append(b, 'x', 0)); got != "a😀\r\nb" {
		t.Errorf("textFromUTF16LE = %q (stops at the NUL)", got)
	}
	if got := textFromUTF16LE([]byte{'a', 0, 'b'}); got != "a" {
		t.Errorf("textFromUTF16LE with an odd byte = %q", got)
	}
}

func TestHTMLClipboard(t *testing.T) {
	frag := []byte("<p>héllo <b>wörld</b></p>")
	b := htmlClipboard(frag)
	if b[len(b)-1] != 0 {
		t.Fatalf("CF_HTML is not NUL-terminated")
	}
	s := string(b[:len(b)-1])
	off := func(key string) int {
		i := strings.Index(s, key+":")
		if i < 0 {
			t.Fatalf("no %s in %q", key, s)
		}
		n := 0
		for _, c := range s[i+len(key)+1 : i+len(key)+11] {
			n = n*10 + int(c-'0')
		}
		return n
	}
	if !strings.HasPrefix(s, "Version:0.9\r\n") {
		t.Errorf("header = %q", s[:20])
	}
	if got := s[off("StartFragment"):off("EndFragment")]; got != string(frag) {
		t.Errorf("fragment by offsets = %q", got)
	}
	if got := s[off("StartHTML"):off("EndHTML")]; !strings.HasPrefix(got, "<html>") || !strings.HasSuffix(got, "</html>") {
		t.Errorf("document by offsets = %q", got)
	}
	if off("EndHTML") != len(s) {
		t.Errorf("EndHTML = %d, want %d", off("EndHTML"), len(s))
	}
	if got := htmlFragment(b); !bytes.Equal(got, frag) {
		t.Errorf("htmlFragment(htmlClipboard(x)) = %q", got)
	}
}

func TestHTMLFragmentReaders(t *testing.T) {
	// What other programs write: offsets, -1 for the optional ones, a
	// SourceURL, fragments with CR LF, bad offsets, or no header.
	doc := "<html><body>\r\n<!--StartFragment--><i>x</i><!--EndFragment-->\r\n</body></html>"
	header := func(sf, ef int) string {
		return "Version:1.0\r\nStartHTML:-1\r\nEndHTML:-1\r\nStartFragment:" + itoa10(sf) + "\r\nEndFragment:" + itoa10(ef) + "\r\nSourceURL:https://example.com/a:b\r\n"
	}
	h := header(0, 0)
	sf := len(h) + strings.Index(doc, "<i>")
	ef := len(h) + strings.Index(doc, "<!--End")
	for _, tc := range []struct {
		name, in, want string
	}{
		{"offsets", header(sf, ef) + doc, "<i>x</i>"},
		{"bad offsets fall back to the comments", header(5, 99999) + doc, "<i>x</i>"},
		{"no fragment offsets", "Version:0.9\r\nStartHTML:0000000040\r\nEndHTML:0000000050\r\n" + doc, "<i>x</i>"},
		{"no header", "<b>plain</b>", "<b>plain</b>"},
		{"NUL-terminated", header(sf, ef) + doc + "\x00garbage", "<i>x</i>"},
		{"no comments", "Version:0.9\r\nStartHTML:-1\r\n<b>y</b>", "<b>y</b>"},
	} {
		if got := string(htmlFragment([]byte(tc.in))); got != tc.want {
			t.Errorf("%s: htmlFragment = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func itoa10(n int) string {
	s := []byte("0000000000")
	for i := len(s) - 1; i >= 0 && n > 0; i-- {
		s[i] = byte('0' + n%10)
		n /= 10
	}
	return string(s)
}

func TestDropFiles(t *testing.T) {
	paths := []string{`C:\a b\ü.txt`, `\\server\share\f`, "😀"}
	b := dropFiles(paths)
	if got := binary.LittleEndian.Uint32(b); got != dropFilesHeader {
		t.Errorf("pFiles = %d", got)
	}
	if got := binary.LittleEndian.Uint32(b[16:]); got != 1 {
		t.Errorf("fWide = %d", got)
	}
	if !bytes.HasSuffix(b, []byte{0, 0, 0, 0}) {
		t.Errorf("the list does not end with an empty string")
	}
	got, err := parseDropFiles(b)
	if err != nil || !slices.Equal(got, paths) {
		t.Errorf("parseDropFiles = %q, %v; want %q", got, err, paths)
	}

	// A narrow list (old programs), at another offset.
	narrow := make([]byte, 24)
	binary.LittleEndian.PutUint32(narrow, 24)
	narrow = append(narrow, "C:\\x.txt\x00C:\\\xe9.txt\x00\x00"...)
	got, err = parseDropFiles(narrow)
	if want := []string{`C:\x.txt`, `C:\é.txt`}; err != nil || !slices.Equal(got, want) {
		t.Errorf("parseDropFiles(narrow) = %q, %v; want %q", got, err, want)
	}

	if _, err := parseDropFiles([]byte{1, 2}); err == nil {
		t.Errorf("parseDropFiles of 2 bytes: no error")
	}
	bad := make([]byte, 20)
	binary.LittleEndian.PutUint32(bad, 100)
	if _, err := parseDropFiles(bad); err == nil {
		t.Errorf("parseDropFiles with pFiles past the end: no error")
	}
	if got, err := parseDropFiles(dropFiles(nil)); err != nil || len(got) != 0 {
		t.Errorf("parseDropFiles(empty) = %q, %v", got, err)
	}
}

// testImage is a 3x2 image with transparency and every channel distinct.
func testImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(1, 0, color.NRGBA{0, 255, 0, 128})
	img.SetNRGBA(2, 0, color.NRGBA{0, 0, 255, 0})
	img.SetNRGBA(0, 1, color.NRGBA{10, 20, 30, 255})
	img.SetNRGBA(1, 1, color.NRGBA{200, 100, 50, 64})
	img.SetNRGBA(2, 1, color.NRGBA{1, 2, 3, 255})
	return img
}

func checkPNGPixels(t *testing.T, b []byte, want *image.NRGBA) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("decoding the PNG: %v", err)
	}
	if img.Bounds() != want.Rect {
		t.Fatalf("PNG bounds %v, want %v", img.Bounds(), want.Rect)
	}
	for y := range want.Rect.Dy() {
		for x := range want.Rect.Dx() {
			got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			w := want.NRGBAAt(x, y)
			if w.A == 0 {
				got.R, got.G, got.B, w.R, w.G, w.B = 0, 0, 0, 0, 0, 0
			}
			if got != w {
				t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, w)
			}
		}
	}
}

// dibHeader writes a BITMAPINFOHEADER.
func dibHeader(w, h int32, bpp uint16, compression, clrUsed uint32) []byte {
	b := make([]byte, 40)
	le := binary.LittleEndian
	le.PutUint32(b, 40)
	le.PutUint32(b[4:], uint32(w))
	le.PutUint32(b[8:], uint32(h))
	le.PutUint16(b[12:], 1)
	le.PutUint16(b[14:], bpp)
	le.PutUint32(b[16:], compression)
	le.PutUint32(b[32:], clrUsed)
	return b
}

func TestPNGFromDIB(t *testing.T) {
	opaque := func(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }
	for _, tc := range []struct {
		name string
		dib  []byte
		want []color.NRGBA // 2x2, top row first
	}{
		{
			// 24 bits, bottom-up, rows padded to 4 bytes.
			name: "24-bit bottom-up",
			dib: append(dibHeader(2, 2, 24, 0, 0),
				3, 2, 1, 6, 5, 4, 0, 0, // bottom row
				9, 8, 7, 12, 11, 10, 0, 0), // top row
			want: []color.NRGBA{opaque(7, 8, 9), opaque(10, 11, 12), opaque(1, 2, 3), opaque(4, 5, 6)},
		},
		{
			// 32 bits BI_RGB with zero fourth bytes: opaque.
			name: "32-bit no alpha, top-down",
			dib: append(dibHeader(2, -2, 32, 0, 0),
				3, 2, 1, 0, 6, 5, 4, 0,
				9, 8, 7, 0, 12, 11, 10, 0),
			want: []color.NRGBA{opaque(1, 2, 3), opaque(4, 5, 6), opaque(7, 8, 9), opaque(10, 11, 12)},
		},
		{
			// 32 bits BI_RGB with alpha in the fourth bytes.
			name: "32-bit with alpha",
			dib: append(dibHeader(2, -2, 32, 0, 0),
				3, 2, 1, 255, 6, 5, 4, 128,
				9, 8, 7, 0, 12, 11, 10, 7),
			want: []color.NRGBA{opaque(1, 2, 3), {4, 5, 6, 128}, {7, 8, 9, 0}, {10, 11, 12, 7}},
		},
		{
			// 16 bits 5-6-5 with masks after the header.
			name: "16-bit bitfields",
			dib: append(append(dibHeader(2, -1, 16, biBitfields, 0),
				0x00, 0xF8, 0, 0, 0xE0, 0x07, 0, 0, 0x1F, 0, 0, 0), // R, G, B masks
				0x00, 0xF8, 0x1F, 0x00), // red, blue
			want: []color.NRGBA{opaque(255, 0, 0), opaque(0, 0, 255)},
		},
		{
			// 8 bits with a two-color palette.
			name: "8-bit palette",
			dib: append(append(dibHeader(2, -1, 8, 0, 2),
				10, 20, 30, 0, 40, 50, 60, 0),
				1, 0, 0, 0),
			want: []color.NRGBA{opaque(60, 50, 40), opaque(30, 20, 10)},
		},
		{
			// 1 bit, default palette size.
			name: "1-bit",
			dib: append(append(dibHeader(2, -1, 1, 0, 0),
				0, 0, 0, 0, 255, 255, 255, 0),
				0x40, 0, 0, 0),
			want: []color.NRGBA{opaque(0, 0, 0), opaque(255, 255, 255)},
		},
	} {
		b, err := pngFromDIB(tc.dib)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		w := img.Bounds().Dx()
		for i, want := range tc.want {
			got := color.NRGBAModel.Convert(img.At(i%w, i/w)).(color.NRGBA)
			if want.A == 0 {
				got.R, got.G, got.B, want.R, want.G, want.B = 0, 0, 0, 0, 0, 0
			}
			if got != want {
				t.Errorf("%s: pixel %d = %v, want %v", tc.name, i, got, want)
			}
		}
	}
}

func TestPNGFromDIBV5Quirks(t *testing.T) {
	img := testImage()
	v5 := dibV5(img)
	// The masks repeated after the V5 header, as some producers write.
	quirk := append(append(bytes.Clone(v5[:bitmapV5HeaderSize]), v5[40:52]...), v5[bitmapV5HeaderSize:]...)
	for name, b := range map[string][]byte{"v5": v5, "v5 with repeated masks": quirk, "v5 with padding": append(bytes.Clone(v5), 0, 0, 0, 0, 0, 0, 0, 0)} {
		got, err := pngFromDIB(b)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		checkPNGPixels(t, got, img)
	}
	// BI_PNG carries a PNG.
	var p bytes.Buffer
	png.Encode(&p, img)
	if got, err := pngFromDIB(append(dibHeader(3, 2, 0, biPNG, 0), p.Bytes()...)); err != nil || !bytes.Equal(got, p.Bytes()) {
		t.Errorf("BI_PNG: %v", err)
	}
}

func TestPNGFromDIBErrors(t *testing.T) {
	for name, b := range map[string][]byte{
		"empty":          nil,
		"core header":    append([]byte{12, 0, 0, 0}, make([]byte, 40)...),
		"zero width":     dibHeader(0, 1, 32, 0, 0),
		"huge":           dibHeader(1<<20, 1<<20, 32, 0, 0),
		"truncated":      append(dibHeader(4, 4, 32, 0, 0), 1, 2, 3),
		"jpeg":           dibHeader(1, 1, 0, biJPEG, 0),
		"rle":            dibHeader(1, 1, 8, 1, 0),
		"no palette":     dibHeader(1, 1, 8, 0, 0),
		"24-bit fields":  append(dibHeader(1, 1, 24, biBitfields, 0), make([]byte, 16)...),
		"odd bit depth":  append(dibHeader(1, 1, 7, 0, 0), make([]byte, 600)...),
		"header too big": append([]byte{0xFF, 0, 0, 0}, make([]byte, 40)...),
	} {
		if _, err := pngFromDIB(b); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestDragKeysAndEffects(t *testing.T) {
	if got := dragKeyMods(mkLButton | mkShift | mkControl | mkAlt); got != platform.ModShift|platform.ModCtrl|platform.ModAlt {
		t.Errorf("dragKeyMods = %v", got)
	}
	if got := dragKeyMods(mkLButton | mkRButton | mkMButton); got != 0 {
		t.Errorf("dragKeyMods of buttons = %v", got)
	}
	for held, want := range map[platform.Buttons]uint32{
		platform.ButtonPrimary.Mask():                                   mkLButton,
		platform.ButtonSecondary.Mask() | platform.ButtonMiddle.Mask():  mkRButton,
		platform.ButtonMiddle.Mask():                                    mkMButton,
		platform.ButtonBack.Mask():                                      0,
		platform.ButtonPrimary.Mask() | platform.ButtonSecondary.Mask(): mkLButton,
	} {
		if got := dragButton(held); got != want {
			t.Errorf("dragButton(%b) = %#x, want %#x", held, got, want)
		}
	}
	for _, tc := range []struct {
		escape   bool
		ks, btn  uint32
		quitting bool
		want     int32
	}{
		{false, mkLButton, mkLButton, false, sOK},
		{false, mkLButton | mkControl, mkLButton, false, sOK},
		{false, mkControl, mkLButton, false, dragdropSDrop},
		{true, mkLButton, mkLButton, false, dragdropSCancel},
		{false, mkLButton, mkLButton, true, dragdropSCancel},
		{false, mkRButton, mkLButton, false, dragdropSDrop},
		{false, mkRButton, 0, false, sOK},
		{false, mkShift, 0, false, dragdropSDrop},
	} {
		if got := dropSourceContinue(tc.escape, tc.ks, tc.btn, tc.quitting); got != tc.want {
			t.Errorf("dropSourceContinue(%v, %#x, %#x, %v) = %#x, want %#x", tc.escape, tc.ks, tc.btn, tc.quitting, got, tc.want)
		}
	}
	for e, want := range map[uint32]platform.DropEffect{
		0: platform.DropNone, 1: platform.DropCopy, 2: platform.DropMove, 4: platform.DropLink,
		3: platform.DropCopy, 6: platform.DropMove, 0x80000000: platform.DropNone, // DROPEFFECT_SCROLL alone
	} {
		if got := oneEffect(e); got != want {
			t.Errorf("oneEffect(%#x) = %v, want %v", e, got, want)
		}
	}
}

func TestPremultipliedBGRA(t *testing.T) {
	img := image.NewRGBA(image.Rect(5, 5, 7, 6))
	copy(img.Pix[img.PixOffset(5, 5):], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	if got, want := premultipliedBGRA(img), []byte{3, 2, 1, 4, 7, 6, 5, 8}; !bytes.Equal(got, want) {
		t.Errorf("premultipliedBGRA = %v, want %v", got, want)
	}
}
