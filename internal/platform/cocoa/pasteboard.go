package cocoa

import (
	"bytes"
	"fmt"
	"image/png"
	"slices"
	"strings"

	"golang.org/x/image/tiff"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the clipboard (clipboard_darwin.go): how platform.Data
// maps onto NSPasteboard types (uniform type identifiers), in both
// directions, tested on every OS.
//
//   - MIMEText is public.utf8-plain-text, written and read as a string
//     (stringForType: converts the other text types: UTF-16, the legacy
//     NSStringPboardType).
//   - MIMEHTML is public.html, a string too (Safari, Chrome and the Cocoa
//     text system read and write it with the string methods).
//   - MIMEPNG is public.png. Read from public.png, else from public.tiff
//     (what many Cocoa apps put on the pasteboard for images) decoded with
//     golang.org/x/image/tiff and encoded as PNG. Only PNG is written.
//   - MIMEURIList is one pasteboard item per URI, as Finder writes files:
//     public.file-url for file: URIs, public.url for others, the first one
//     in the item that holds the other types. Read from every item's
//     public.file-url (Finder writes file reference URLs,
//     file:///.file/id=…, which are resolved to paths), else public.url.
//   - Any other MIME type is the UTType of the MIME type (macOS 11+:
//     +[UTType typeWithMIMEType:], a declared type such as public.json or a
//     dynamic dyn.… one) and its bytes as they are; a type with parameters,
//     which UTType tags do not carry, or one UTType refuses, is a custom
//     identifier naming it (customType). Types are read back through the
//     UTType's preferred MIME type.

// Pasteboard types of the well-known Data types.
const (
	utiUTF8Text = "public.utf8-plain-text"
	utiHTML     = "public.html"
	utiPNG      = "public.png"
	utiTIFF     = "public.tiff"
	utiFileURL  = "public.file-url"
	utiURL      = "public.url"
)

// wellKnownTypes maps pasteboard types to the well-known Data types, in
// the order a read prefers them within one Data type.
var wellKnownTypes = []struct{ uti, mime string }{
	{utiUTF8Text, platform.MIMEText},
	{"public.utf16-plain-text", platform.MIMEText},
	{"public.utf16-external-plain-text", platform.MIMEText},
	{"public.plain-text", platform.MIMEText},
	{"NSStringPboardType", platform.MIMEText},
	{utiHTML, platform.MIMEHTML},
	{"Apple HTML pasteboard type", platform.MIMEHTML},
	{utiPNG, platform.MIMEPNG},
	{"Apple PNG pasteboard type", platform.MIMEPNG},
	{utiTIFF, platform.MIMEPNG},
	{"NeXT TIFF v4.0 pasteboard type", platform.MIMEPNG},
	{utiFileURL, platform.MIMEURIList},
	{utiURL, platform.MIMEURIList},
}

// wellKnownType returns the Data type of a well-known pasteboard type.
func wellKnownType(uti string) (string, bool) {
	for _, w := range wellKnownTypes {
		if w.uti == uti {
			return w.mime, true
		}
	}
	return "", false
}

// isTIFF reports whether a pasteboard type holds TIFF data (read as PNG).
func isTIFF(uti string) bool { return uti == utiTIFF || uti == "NeXT TIFF v4.0 pasteboard type" }

// isStringType reports whether a pasteboard type is read and written with
// the string methods (stringForType:, setString:forType:), which convert
// between the encodings apps use for text.
func isStringType(uti string) bool {
	m, ok := wellKnownType(uti)
	return ok && (m == platform.MIMEText || m == platform.MIMEHTML)
}

// customPrefix starts the identifiers of MIME types that have no UTType.
const customPrefix = "dev.telaui.mime."

// customType returns the identifier naming a MIME type that has no UTType:
// customPrefix and the MIME type with every byte but a-z and 0-9 written
// as "-" and two lowercase hex digits. Identifiers are compared without
// case and allow only letters, digits, '-' and '.', so the encoding keeps
// case and every other character.
func customType(mime string) string {
	var b strings.Builder
	b.WriteString(customPrefix)
	for i := 0; i < len(mime); i++ {
		c := mime[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "-%02x", c)
		}
	}
	return b.String()
}

// customMIME decodes an identifier made by customType (ok false for any
// other, or one whose MIME type is invalid).
func customMIME(uti string) (string, bool) {
	rest, ok := strings.CutPrefix(uti, customPrefix)
	if !ok {
		return "", false
	}
	var b []byte
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c != '-' {
			b = append(b, c)
			continue
		}
		if i+2 >= len(rest) {
			return "", false
		}
		hi, lo := unhex(rest[i+1]), unhex(rest[i+2])
		if hi < 0 || lo < 0 {
			return "", false
		}
		b = append(b, byte(hi<<4|lo))
		i += 2
	}
	m := string(b)
	if !validMIME(m) {
		return "", false
	}
	return m, true
}

// validMIME reports whether m is "type/subtype" with optional parameters,
// printable ASCII without spaces (platform.CheckMIME's rule, which panics
// instead).
func validMIME(m string) bool {
	if len(m) > 255 {
		return false
	}
	base, _, _ := strings.Cut(m, ";")
	typ, sub, ok := strings.Cut(base, "/")
	if !ok || typ == "" || sub == "" || strings.Contains(sub, "/") {
		return false
	}
	for i := 0; i < len(m); i++ {
		if c := m[i]; c <= ' ' || c >= 0x7f {
			return false
		}
	}
	return true
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	}
	return -1
}

// needsCustomType reports whether a custom MIME type is written under
// customType rather than its UTType: one with parameters, which UTType
// tags do not carry.
func needsCustomType(mime string) bool { return strings.Contains(mime, ";") }

// pbEntry is one representation in a pasteboard item.
type pbEntry struct {
	uti  string
	data []byte
	text bool // written with setString:forType:
}

// pbItem is one NSPasteboardItem.
type pbItem []pbEntry

// pasteboardItems returns the items that carry d, in d's order: one item
// with every entry, and one more item for each URI of a URI list after
// its first (files are one item each, as Finder writes them). utiOf
// gives the pasteboard type of a custom MIME type. Empty data has no
// items; a URI list without URIs adds nothing.
func pasteboardItems(d platform.Data, utiOf func(mime string) string) []pbItem {
	var first pbItem
	var more []pbItem
	for _, e := range d {
		switch e.Type {
		case platform.MIMEText:
			first = append(first, pbEntry{uti: utiUTF8Text, data: e.Bytes, text: true})
		case platform.MIMEHTML:
			first = append(first, pbEntry{uti: utiHTML, data: e.Bytes, text: true})
		case platform.MIMEPNG:
			first = append(first, pbEntry{uti: utiPNG, data: e.Bytes})
		case platform.MIMEURIList:
			for i, u := range platform.ParseURIList(e.Bytes) {
				ent := pbEntry{uti: urlType(u), data: []byte(u), text: true}
				if i == 0 {
					first = append(first, ent)
				} else {
					more = append(more, pbItem{ent})
				}
			}
		default:
			first = append(first, pbEntry{uti: utiOf(e.Type), data: e.Bytes})
		}
	}
	if len(first) == 0 && len(more) == 0 {
		return nil
	}
	if len(first) == 0 {
		return more
	}
	return append([]pbItem{first}, more...)
}

// urlType returns the pasteboard type of a URI: public.file-url for a
// file: URI, public.url otherwise.
func urlType(uri string) string {
	if len(uri) >= 5 && strings.EqualFold(uri[:5], "file:") {
		return utiFileURL
	}
	return utiURL
}

// isFileReferenceURL reports whether a file URL is a file reference URL
// (file:///.file/id=…), which Finder puts on the pasteboard and which
// names a file by its ID instead of its path.
func isFileReferenceURL(u string) bool { return strings.HasPrefix(u, "file:///.file/") }

// offeredTypes returns the Data types of the pasteboard's items, each a
// list of pasteboard types in order: each Data type once, at the position
// of its first pasteboard type. mimeOf maps a type that is not well known
// (ok false: not data).
func offeredTypes(items [][]string, mimeOf func(uti string) (string, bool)) []string {
	var out []string
	for _, types := range items {
		for _, uti := range types {
			m, ok := wellKnownType(uti)
			if !ok {
				m, ok = mimeOf(uti)
			}
			if ok && !slices.Contains(out, m) {
				out = append(out, m)
			}
		}
	}
	return out
}

// typesFor returns the pasteboard types of offered (the pasteboard's types,
// in order) to read mime from, best first: the well-known ones in
// wellKnownTypes' order, then the preferred ones given (the UTType and
// custom identifier of a custom type), then every other offered type that
// mimeOf maps to mime.
func typesFor(mime string, offered, preferred []string, mimeOf func(uti string) (string, bool)) []string {
	var out []string
	add := func(uti string) {
		if slices.Contains(offered, uti) && !slices.Contains(out, uti) {
			out = append(out, uti)
		}
	}
	for _, w := range wellKnownTypes {
		if w.mime == mime {
			add(w.uti)
		}
	}
	for _, p := range preferred {
		add(p)
	}
	for _, uti := range offered {
		if _, ok := wellKnownType(uti); ok {
			continue
		}
		if m, ok := mimeOf(uti); ok && m == mime {
			add(uti)
		}
	}
	return out
}

// pngFromTIFF converts TIFF data to PNG (golang.org/x/image/tiff decodes
// the baseline formats and LZW, Deflate and PackBits compression, which
// covers what NSImage writes).
func pngFromTIFF(b []byte) ([]byte, error) {
	img, err := tiff.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("cocoa: decoding the pasteboard's TIFF image: %w", err)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, fmt.Errorf("cocoa: encoding the pasteboard's TIFF image as PNG: %w", err)
	}
	return out.Bytes(), nil
}
