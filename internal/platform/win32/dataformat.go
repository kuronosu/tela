package win32

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/bits"
	"slices"
	"strconv"
	"strings"
	utf16pkg "unicode/utf16"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the Win32 data formats (dataobject_windows.go): how
// platform.Data maps onto clipboard formats, in both directions, for the
// clipboard and for OLE drag and drop alike ("Drag and drop and the
// clipboard" in package platform):
//
//   - MIMEText is CF_UNICODETEXT (UTF-16 with a terminating NUL; line
//     endings kept as they are).
//   - MIMEHTML is the registered "HTML Format" (CF_HTML: a header of byte
//     offsets, then a document with the fragment between
//     <!--StartFragment--> and <!--EndFragment-->).
//   - MIMEPNG is the registered "PNG", written with CF_DIBV5 (and CF_DIB for
//     drags, where nothing synthesizes it) for programs that read only
//     bitmaps; read from "PNG", else from CF_DIBV5 or CF_DIB converted to
//     PNG.
//   - MIMEURIList is CF_HDROP (DROPFILES, a list of paths): only the file:
//     URIs of local paths are written.
//   - Any other MIME type is a format registered under its name
//     (RegisterClipboardFormatW), with the bytes as they are. HGLOBAL
//     memory carries no exact length: the size is GlobalSize's, which is
//     the allocated size on current Windows (the heap keeps it), so binary
//     custom data reads back at its length.
//
// Registered formats named like a MIME type ("image/png", "text/html" as
// some programs register them) are read as that type too, after the
// standard format of a well-known type.

// Standard clipboard formats (winuser.h).
const (
	cfDIB         = 8
	cfUnicodeText = 13
	cfHDrop       = 15
	cfDIBV5       = 17

	// cfFirstRegistered is the first ID of the formats registered by name
	// (0xC000 to 0xFFFF).
	cfFirstRegistered = 0xC000
)

// Names of the registered formats of the well-known types.
const (
	cfNameHTML = "HTML Format"
	cfNamePNG  = "PNG"
)

// clipFormat is a clipboard format: a standard one by ID, or a registered
// one, whose name says what it is.
type clipFormat struct {
	id   uint32
	name string // the registered name; "" for a standard format
}

// formatType returns the Data type a format is read as, and whether it is
// one (formats without a MIME meaning, such as CF_LOCALE or "Shell IDList
// Array", are not data).
func formatType(f clipFormat) (string, bool) {
	switch f.id {
	case cfUnicodeText:
		return platform.MIMEText, true
	case cfHDrop:
		return platform.MIMEURIList, true
	case cfDIBV5, cfDIB:
		return platform.MIMEPNG, true
	}
	if f.id < cfFirstRegistered || f.name == "" {
		return "", false
	}
	switch f.name {
	case cfNameHTML:
		return platform.MIMEHTML, true
	case cfNamePNG:
		return platform.MIMEPNG, true
	}
	if !strings.Contains(f.name, "/") {
		return "", false
	}
	return platform.SelectionType(f.name)
}

// formatRank orders the formats that carry the same type when reading it:
// lower first, -1 when f does not carry mime.
func formatRank(f clipFormat, mime string) int {
	if t, ok := formatType(f); !ok || t != mime {
		return -1
	}
	switch {
	case f.id == cfUnicodeText, f.id == cfHDrop, f.name == cfNameHTML, f.name == cfNamePNG:
		return 0
	case f.id == cfDIBV5:
		return 2
	case f.id == cfDIB:
		return 3
	}
	return 1 // a registered MIME name
}

// offeredTypes returns the Data types of a list of formats, in order, each
// once at the position of its first format.
func offeredTypes(formats []clipFormat) []string {
	var types []string
	for _, f := range formats {
		if t, ok := formatType(f); ok && !slices.Contains(types, t) {
			types = append(types, t)
		}
	}
	return types
}

// formatsFor returns the formats of offered that carry mime, best first.
func formatsFor(mime string, offered []clipFormat) []clipFormat {
	var out []clipFormat
	for _, f := range offered {
		if formatRank(f, mime) >= 0 {
			out = append(out, f)
		}
	}
	slices.SortStableFunc(out, func(a, b clipFormat) int { return formatRank(a, mime) - formatRank(b, mime) })
	return out
}

// decodeFormat converts the bytes of a format to its Data type's bytes.
func decodeFormat(f clipFormat, raw []byte) ([]byte, error) {
	switch {
	case f.id == cfUnicodeText:
		return []byte(textFromUTF16LE(raw)), nil
	case f.id == cfHDrop:
		paths, err := parseDropFiles(raw)
		if err != nil {
			return nil, err
		}
		return uriListFromPaths(paths), nil
	case f.id == cfDIBV5, f.id == cfDIB:
		return pngFromDIB(raw)
	case f.name == cfNameHTML:
		return htmlFragment(raw), nil
	}
	return raw, nil
}

// encodedFormat is a format to write: a standard one (id) or one
// registered by name, and its bytes.
type encodedFormat struct {
	id   uint32 // a standard format, or 0 for name
	name string // a registered format's name
	data []byte
}

// encodeData returns the formats that carry d, in d's order (best first).
// dib adds CF_DIB next to CF_DIBV5 for images: the clipboard synthesizes it,
// a drag's data object does not. A PNG that does not decode is written as
// "PNG" only; a URI list without local files has no CF_HDROP.
func encodeData(d platform.Data, dib bool) []encodedFormat {
	var out []encodedFormat
	for _, e := range d {
		switch e.Type {
		case platform.MIMEText:
			out = append(out, encodedFormat{id: cfUnicodeText, data: utf16LEText(string(e.Bytes))})
		case platform.MIMEHTML:
			out = append(out, encodedFormat{name: cfNameHTML, data: htmlClipboard(e.Bytes)})
		case platform.MIMEPNG:
			out = append(out, encodedFormat{name: cfNamePNG, data: e.Bytes})
			img, err := png.Decode(bytes.NewReader(e.Bytes))
			if err != nil {
				break
			}
			out = append(out, encodedFormat{id: cfDIBV5, data: dibV5(img)})
			if dib {
				out = append(out, encodedFormat{id: cfDIB, data: dibV3(img)})
			}
		case platform.MIMEURIList:
			if paths := pathsFromURIList(e.Bytes); len(paths) > 0 {
				out = append(out, encodedFormat{id: cfHDrop, data: dropFiles(paths)})
			}
		default:
			out = append(out, encodedFormat{name: e.Type, data: e.Bytes})
		}
	}
	return out
}

// --- text ---

// utf16LEText encodes s as CF_UNICODETEXT bytes: UTF-16 little endian with
// a terminating NUL (invalid UTF-8 becomes U+FFFD).
func utf16LEText(s string) []byte {
	u := utf16Clipboard(s)
	out := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(out[2*i:], c)
	}
	return out
}

// textFromUTF16LE decodes CF_UNICODETEXT bytes up to the first NUL (an odd
// last byte is ignored).
func textFromUTF16LE(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return fromUTF16Clipboard(u)
}

// --- HTML Format ---

// CF_HTML's document around the fragment. The offsets in the header are
// byte offsets of the UTF-8 data, written with ten digits as Chromium and
// Firefox do, so the header's length does not depend on them.
const (
	htmlHeader = "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\nStartFragment:%010d\r\nEndFragment:%010d\r\n"
	htmlBefore = "<html>\r\n<body>\r\n<!--StartFragment-->"
	htmlAfter  = "<!--EndFragment-->\r\n</body>\r\n</html>"
)

// htmlClipboard wraps an HTML fragment as CF_HTML, NUL-terminated (the
// terminator is past EndHTML).
func htmlClipboard(fragment []byte) []byte {
	headerLen := len(fmt.Sprintf(htmlHeader, 0, 0, 0, 0))
	startHTML := headerLen
	startFrag := startHTML + len(htmlBefore)
	endFrag := startFrag + len(fragment)
	endHTML := endFrag + len(htmlAfter)
	var b bytes.Buffer
	b.Grow(endHTML + 1)
	fmt.Fprintf(&b, htmlHeader, startHTML, endHTML, startFrag, endFrag)
	b.WriteString(htmlBefore)
	b.Write(fragment)
	b.WriteString(htmlAfter)
	b.WriteByte(0)
	return b.Bytes()
}

// htmlFragment returns the fragment of CF_HTML data: the bytes between
// StartFragment and EndFragment when the header has valid offsets, else
// between the <!--StartFragment--> and <!--EndFragment--> comments, else
// the document between StartHTML and EndHTML, else everything after the
// header. Data without a header is returned as it is (up to a NUL).
func htmlFragment(b []byte) []byte {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	offsets := map[string]int{}
	body := 0
	for body < len(b) {
		end := bytes.IndexByte(b[body:], '\n')
		if end < 0 {
			end = len(b) - body
		}
		line := strings.TrimRight(string(b[body:body+end]), "\r")
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, "<") || strings.ContainsAny(key, " <") {
			break
		}
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			offsets[key] = n
		}
		body += min(end+1, len(b)-body)
	}
	valid := func(start, end string) (int, int, bool) {
		s, ok1 := offsets[start]
		e, ok2 := offsets[end]
		return s, e, ok1 && ok2 && s >= body && s <= e && e <= len(b)
	}
	if s, e, ok := valid("StartFragment", "EndFragment"); ok {
		return bytes.Clone(b[s:e])
	}
	const startMark, endMark = "<!--StartFragment-->", "<!--EndFragment-->"
	if s := bytes.Index(b[body:], []byte(startMark)); s >= 0 {
		s += body + len(startMark)
		if e := bytes.Index(b[s:], []byte(endMark)); e >= 0 {
			return bytes.Clone(b[s : s+e])
		}
	}
	if s, e, ok := valid("StartHTML", "EndHTML"); ok {
		return bytes.Clone(b[s:e])
	}
	return bytes.Clone(b[body:])
}

// --- CF_HDROP ---

// dropFilesHeader is the size of DROPFILES: pFiles, pt (two LONGs), fNC
// and fWide.
const dropFilesHeader = 20

// dropFiles returns a DROPFILES structure with the paths: wide (UTF-16)
// strings, each NUL-terminated, and an empty one at the end.
func dropFiles(paths []string) []byte {
	var u []uint16
	for _, p := range paths {
		u = append(u, utf16pkg.Encode([]rune(strings.ToValidUTF8(p, "�")))...)
		u = append(u, 0)
	}
	u = append(u, 0)
	out := make([]byte, dropFilesHeader+2*len(u))
	binary.LittleEndian.PutUint32(out[0:], dropFilesHeader) // pFiles
	binary.LittleEndian.PutUint32(out[16:], 1)              // fWide
	for i, c := range u {
		binary.LittleEndian.PutUint16(out[dropFilesHeader+2*i:], c)
	}
	return out
}

// parseDropFiles returns the paths of a DROPFILES structure. Narrow
// (ANSI) lists, which only old programs write, are decoded as UTF-8 when
// valid, else as Latin-1 (the ANSI code page is not known here).
func parseDropFiles(b []byte) ([]string, error) {
	if len(b) < dropFilesHeader {
		return nil, errors.New("win32: CF_HDROP: too short for DROPFILES")
	}
	off := binary.LittleEndian.Uint32(b[0:])
	wide := binary.LittleEndian.Uint32(b[16:]) != 0
	if off > uint32(len(b)) {
		return nil, fmt.Errorf("win32: CF_HDROP: file list at %d, past the end (%d bytes)", off, len(b))
	}
	b = b[off:]
	var paths []string
	if wide {
		var cur []uint16
		for i := 0; i+1 < len(b); i += 2 {
			c := binary.LittleEndian.Uint16(b[i:])
			if c != 0 {
				cur = append(cur, c)
				continue
			}
			if len(cur) == 0 {
				break
			}
			paths = append(paths, string(utf16pkg.Decode(cur)))
			cur = cur[:0]
		}
		return paths, nil
	}
	for len(b) > 0 && b[0] != 0 {
		end := bytes.IndexByte(b, 0)
		if end < 0 {
			end = len(b)
		}
		s := b[:end]
		if utf8.Valid(s) {
			paths = append(paths, string(s))
		} else {
			r := make([]rune, len(s))
			for i, c := range s {
				r[i] = rune(c)
			}
			paths = append(paths, string(r))
		}
		b = b[min(end+1, len(b)):]
	}
	return paths, nil
}

// uriListFromPaths returns the text/uri-list of the file: URIs of paths
// (platform.FileURI; paths it rejects are left out).
func uriListFromPaths(paths []string) []byte {
	var uris []string
	for _, p := range paths {
		if u, err := platform.FileURI(p); err == nil {
			uris = append(uris, u)
		}
	}
	return platform.URIList(uris)
}

// pathsFromURIList returns the local paths of the file: URIs of a
// text/uri-list (platform.FilePath; other URIs are left out).
func pathsFromURIList(b []byte) []string {
	var paths []string
	for _, u := range platform.ParseURIList(b) {
		if p, err := platform.FilePath(u); err == nil {
			paths = append(paths, p)
		}
	}
	return paths
}

// --- bitmaps ---

// Bitmap header constants (wingdi.h).
const (
	biBitfields      = 3
	biJPEG           = 4
	biPNG            = 5
	biAlphaBitfields = 6
	lcsSRGB          = 0x73524742 // 'sRGB'
	lcsGMImages      = 4

	bitmapV5HeaderSize = 124 // sizeof(BITMAPV5HEADER); bitmapInfoHeaderSize is in dib.go

	// maxDIBPixels bounds the bitmaps read from other programs (a corrupt
	// header must not allocate gigabytes): 16384 x 16384.
	maxDIBPixels = 1 << 28
)

// dibV5 returns img as CF_DIBV5: a BITMAPV5HEADER, 32 bits per pixel with
// BI_BITFIELDS masks for B, G, R and A (straight alpha), sRGB, bottom-up
// rows (the orientation every reader handles).
func dibV5(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, bitmapV5HeaderSize+4*w*h)
	le := binary.LittleEndian
	le.PutUint32(out[0:], bitmapV5HeaderSize)
	le.PutUint32(out[4:], uint32(int32(w)))
	le.PutUint32(out[8:], uint32(int32(h)))
	le.PutUint16(out[12:], 1)  // planes
	le.PutUint16(out[14:], 32) // bit count
	le.PutUint32(out[16:], biBitfields)
	le.PutUint32(out[20:], uint32(4*w*h))
	le.PutUint32(out[40:], 0x00FF0000) // red mask
	le.PutUint32(out[44:], 0x0000FF00) // green mask
	le.PutUint32(out[48:], 0x000000FF) // blue mask
	le.PutUint32(out[52:], 0xFF000000) // alpha mask
	le.PutUint32(out[56:], lcsSRGB)
	le.PutUint32(out[108:], lcsGMImages)
	putBGRA(out[bitmapV5HeaderSize:], img)
	return out
}

// dibV3 returns img as CF_DIB: a BITMAPINFOHEADER, 32 bits per pixel
// BI_RGB with the alpha (straight) in the fourth byte, bottom-up rows.
// Readers that ignore that byte see the color of transparent pixels.
func dibV3(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, bitmapInfoHeaderSize+4*w*h)
	le := binary.LittleEndian
	le.PutUint32(out[0:], bitmapInfoHeaderSize)
	le.PutUint32(out[4:], uint32(int32(w)))
	le.PutUint32(out[8:], uint32(int32(h)))
	le.PutUint16(out[12:], 1)
	le.PutUint16(out[14:], 32)
	le.PutUint32(out[20:], uint32(4*w*h))
	putBGRA(out[bitmapInfoHeaderSize:], img)
	return out
}

// putBGRA writes img's pixels as straight B, G, R, A, bottom row first.
func putBGRA(dst []byte, img image.Image) {
	b := img.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(n, n.Rect, img, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	for y := range h {
		src := n.Pix[y*n.Stride : y*n.Stride+4*w]
		row := dst[(h-1-y)*4*w:]
		for x := range w {
			p := src[4*x : 4*x+4]
			row[4*x], row[4*x+1], row[4*x+2], row[4*x+3] = p[2], p[1], p[0], p[3]
		}
	}
}

// pngFromDIB converts a packed DIB (CF_DIB or CF_DIBV5: a
// BITMAPINFOHEADER, V4 or V5 header, the masks and color table, then the
// pixels) to PNG. It reads 1, 4, 8, 16, 24 and 32 bits per pixel,
// uncompressed or with bit fields, either row order, and BI_PNG. 32-bit
// pixels have alpha when the header gives an alpha mask or, for BI_RGB,
// when the fourth bytes are not all zero (programs disagree on that byte;
// all zero means it is unused); alpha is taken as straight.
func pngFromDIB(b []byte) ([]byte, error) {
	le := binary.LittleEndian
	if len(b) < bitmapInfoHeaderSize {
		return nil, errors.New("win32: bitmap: too short for a header")
	}
	hs := int(le.Uint32(b[0:]))
	if hs < bitmapInfoHeaderSize || hs > len(b) {
		return nil, fmt.Errorf("win32: bitmap: unsupported header size %d", hs)
	}
	width := int(int32(le.Uint32(b[4:])))
	height := int(int32(le.Uint32(b[8:])))
	bpp := int(le.Uint16(b[14:]))
	compression := le.Uint32(b[16:])
	clrUsed := int(le.Uint32(b[32:]))
	topDown := height < 0
	if topDown {
		height = -height
	}
	if width <= 0 || height <= 0 || width > maxDIBPixels/height {
		return nil, fmt.Errorf("win32: bitmap: invalid size %dx%d", width, height)
	}
	off := hs
	switch compression {
	case biPNG:
		return bytes.Clone(b[off:]), nil
	case biJPEG:
		return nil, errors.New("win32: bitmap: JPEG bitmaps are not read")
	case 0, biBitfields, biAlphaBitfields:
	default:
		return nil, fmt.Errorf("win32: bitmap: unsupported compression %d", compression)
	}

	// Masks: in the header from BITMAPV3INFOHEADER (56 bytes) on, else
	// after a BITMAPINFOHEADER.
	var masks [4]uint32
	switch {
	case compression == 0 && bpp == 16:
		masks = [4]uint32{0x7C00, 0x03E0, 0x001F, 0}
	case compression == 0 && bpp == 32:
		masks = [4]uint32{0x00FF0000, 0x0000FF00, 0x000000FF, 0}
	case compression != 0 && hs >= 52:
		masks = [4]uint32{le.Uint32(b[40:]), le.Uint32(b[44:]), le.Uint32(b[48:]), 0}
		if hs >= 56 {
			masks[3] = le.Uint32(b[52:])
		}
	case compression != 0:
		n := 3
		if compression == biAlphaBitfields {
			n = 4
		}
		if len(b) < off+4*n {
			return nil, errors.New("win32: bitmap: truncated masks")
		}
		for i := range n {
			masks[i] = le.Uint32(b[off+4*i:])
		}
		off += 4 * n
	}
	if compression != 0 && bpp != 16 && bpp != 32 {
		return nil, fmt.Errorf("win32: bitmap: bit fields with %d bits per pixel", bpp)
	}

	var palette []color.NRGBA
	if bpp <= 8 {
		n := clrUsed
		if n == 0 || n > 1<<bpp {
			n = 1 << bpp
		}
		if len(b) < off+4*n {
			return nil, errors.New("win32: bitmap: truncated color table")
		}
		palette = make([]color.NRGBA, n)
		for i := range palette {
			q := b[off+4*i:]
			palette[i] = color.NRGBA{R: q[2], G: q[1], B: q[0], A: 0xFF}
		}
		off += 4 * n
	}

	stride := (width*bpp + 31) / 32 * 4
	need := stride * height
	// Some writers repeat the three masks after a V4 or V5 header (a
	// known quirk of CF_DIBV5 producers): skip them when they are there.
	if compression == biBitfields && hs > bitmapInfoHeaderSize && len(b) >= off+12+need &&
		le.Uint32(b[off:]) == masks[0] && le.Uint32(b[off+4:]) == masks[1] && le.Uint32(b[off+8:]) == masks[2] {
		off += 12
	}
	if len(b) < off+need {
		return nil, fmt.Errorf("win32: bitmap: %d bytes of pixels, want %d", len(b)-off, need)
	}
	pix := b[off : off+need]

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	alpha := masks[3] != 0
	if bpp == 32 && compression == 0 {
		for i := 3; i < len(pix); i += 4 {
			if pix[i] != 0 {
				alpha = true
				masks[3] = 0xFF000000
				break
			}
		}
	}
	var ch [4]maskChannel
	for i, m := range masks {
		ch[i] = newMaskChannel(m)
	}
	for y := range height {
		row := pix[y*stride : (y+1)*stride]
		dy := height - 1 - y
		if topDown {
			dy = y
		}
		dst := img.Pix[dy*img.Stride:]
		for x := range width {
			var c color.NRGBA
			switch bpp {
			case 1, 2, 4, 8:
				bit := x * bpp
				idx := int(row[bit/8]>>(8-bpp-bit%8)) & (1<<bpp - 1)
				if idx < len(palette) {
					c = palette[idx]
				} else {
					c = color.NRGBA{A: 0xFF}
				}
			case 24:
				c = color.NRGBA{R: row[3*x+2], G: row[3*x+1], B: row[3*x], A: 0xFF}
			case 16, 32:
				var v uint32
				if bpp == 16 {
					v = uint32(le.Uint16(row[2*x:]))
				} else {
					v = le.Uint32(row[4*x:])
				}
				c = color.NRGBA{R: ch[0].get(v), G: ch[1].get(v), B: ch[2].get(v), A: 0xFF}
				if alpha {
					c.A = ch[3].get(v)
				}
			default:
				return nil, fmt.Errorf("win32: bitmap: unsupported %d bits per pixel", bpp)
			}
			d := dst[4*x : 4*x+4]
			d[0], d[1], d[2], d[3] = c.R, c.G, c.B, c.A
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// maskChannel extracts one channel of a bit-field pixel, scaled to 8 bits.
type maskChannel struct {
	mask  uint32
	shift int
	max   uint32
}

func newMaskChannel(m uint32) maskChannel {
	if m == 0 {
		return maskChannel{}
	}
	shift := bits.TrailingZeros32(m)
	return maskChannel{mask: m, shift: shift, max: m >> shift}
}

func (c maskChannel) get(v uint32) uint8 {
	if c.mask == 0 {
		return 0
	}
	x := (v & c.mask) >> c.shift
	if c.max == 0xFF {
		return uint8(x)
	}
	return uint8((uint64(x)*255 + uint64(c.max)/2) / uint64(c.max))
}

// --- drag and drop ---

// Key state bits of OLE drag and drop (grfKeyState; winuser.h MK_*).
const (
	mkLButton = 0x0001
	mkRButton = 0x0002
	mkShift   = 0x0004
	mkControl = 0x0008
	mkMButton = 0x0010
	mkAlt     = 0x0020 // drag and drop only
)

// dragKeyMods returns the modifiers of an OLE key state. It has no Windows
// key bit: Super is never reported during a drag.
func dragKeyMods(ks uint32) platform.Modifiers {
	var m platform.Modifiers
	if ks&mkShift != 0 {
		m |= platform.ModShift
	}
	if ks&mkControl != 0 {
		m |= platform.ModCtrl
	}
	if ks&mkAlt != 0 {
		m |= platform.ModAlt
	}
	return m
}

// dragButton returns the MK_ bit of the held button a drag starts from:
// the primary, else the secondary, else the middle one (0 for the X
// buttons, which the key state does not carry: the drag then drops on
// any release, see dropSourceContinue).
func dragButton(held platform.Buttons) uint32 {
	switch {
	case held.Has(platform.ButtonPrimary):
		return mkLButton
	case held.Has(platform.ButtonSecondary):
		return mkRButton
	case held.Has(platform.ButtonMiddle):
		return mkMButton
	}
	return 0
}

// IDropSource::QueryContinueDrag answers (oleidl.h, winerror.h).
const (
	dragdropSDrop              = 0x00040100
	dragdropSCancel            = 0x00040101
	dragdropSUseDefaultCursors = 0x00040102
)

// dropSourceContinue is IDropSource::QueryContinueDrag: Escape (or the app
// quitting) cancels, releasing the button the drag started from drops, and
// anything else goes on. A drag from a button the key state does not
// carry drops once no mouse button is held.
func dropSourceContinue(escape bool, ks, button uint32, quitting bool) int32 {
	switch {
	case escape || quitting:
		return dragdropSCancel
	case button != 0 && ks&button == 0,
		button == 0 && ks&(mkLButton|mkRButton|mkMButton) == 0:
		return dragdropSDrop
	}
	return sOK
}

// oneEffect returns the single effect a drag's outcome reports for the
// DROPEFFECT a target returned (one bit normally; copy, then move, then
// link when it set several, so a source does not delete what may have
// been copied).
func oneEffect(e uint32) platform.DropEffect {
	return platform.DefaultDropEffect(platform.DropEffect(e)&platform.DropAll, 0)
}

// premultipliedBGRA returns img's pixels as top-down B, G, R, A rows
// (premultiplied, like img): the bits of a drag image's DIB section.
func premultipliedBGRA(img *image.RGBA) []byte {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := make([]byte, 4*w*h)
	for y := range h {
		src := img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y):]
		dst := out[4*w*y:]
		for x := range w {
			p := src[4*x : 4*x+4]
			dst[4*x], dst[4*x+1], dst[4*x+2], dst[4*x+3] = p[2], p[1], p[0], p[3]
		}
	}
	return out
}
