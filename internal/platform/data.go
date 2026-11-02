package platform

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
)

// Well-known MIME types of [Data] (ADR-210). Backends map them onto each
// OS's formats ("Drag and drop and the clipboard" in the package
// documentation); any other MIME type is a custom type, carried as is
// where the OS can carry it.
const (
	// MIMEText is UTF-8 plain text: Win32 CF_UNICODETEXT (converted to
	// and from UTF-16); the Linux targets "text/plain;charset=utf-8",
	// UTF8_STRING, "text/plain", TEXT and STRING (SelectionTargets).
	MIMEText = "text/plain;charset=utf-8"
	// MIMEHTML is an HTML fragment in UTF-8: Win32 "HTML Format"
	// (registered, with its Version/StartHTML/EndHTML header, which the
	// backend adds and strips); "text/html" on Linux.
	MIMEHTML = "text/html"
	// MIMEPNG is a PNG image: the Win32 registered "PNG" format, with
	// CF_DIBV5 and CF_DIB written as fallbacks for programs that read
	// only bitmaps, and read when no PNG is offered (converted to PNG);
	// "image/png" on Linux.
	MIMEPNG = "image/png"
	// MIMEURIList is a list of URIs (RFC 2483, CRLF-separated), file:
	// URIs for files ([URIList], [FileURI]): Win32 CF_HDROP (a DROPFILES
	// list of paths, files only); "text/uri-list" on Linux, plus
	// "x-special/gnome-copied-files" for file managers
	// ([GnomeCopiedFiles]).
	MIMEURIList = "text/uri-list"
)

// mimeGnomeCopiedFiles is the Linux target file managers (Nautilus,
// Nemo, Caja, Dolphin) read and write for copied and cut files.
const mimeGnomeCopiedFiles = "x-special/gnome-copied-files"

// DataEntry is one representation of [Data]: a MIME type and its bytes.
type DataEntry struct {
	// Type is a MIME type: "type/subtype" with optional parameters
	// (";charset=utf-8"). Types are compared as exact strings; the
	// well-known ones are the MIME… constants.
	Type string
	// Bytes is the content. The receiver must not modify it.
	Bytes []byte
}

// Data is what a drag or the clipboard carries: the same content in one or
// more representations, best first (a rich text editor offers HTML, then
// plain text). Each Type appears once ([CheckData]). Data is plain data:
// it crosses the remote protocol and can be compared entry by entry.
type Data []DataEntry

// TextData returns Data holding s as MIMEText.
func TextData(s string) Data { return Data{{Type: MIMEText, Bytes: []byte(s)}} }

// FileData returns Data holding the given absolute paths as a MIMEURIList
// of file: URIs (see FileURI). It fails for an empty list or a path that
// is not absolute.
func FileData(paths ...string) (Data, error) {
	if len(paths) == 0 {
		return nil, errors.New("platform: FileData with no paths")
	}
	uris := make([]string, len(paths))
	for i, p := range paths {
		u, err := FileURI(p)
		if err != nil {
			return nil, err
		}
		uris[i] = u
	}
	return Data{{Type: MIMEURIList, Bytes: URIList(uris)}}, nil
}

// Types returns the MIME types of d, in order.
func (d Data) Types() []string {
	if len(d) == 0 {
		return nil
	}
	out := make([]string, len(d))
	for i, e := range d {
		out[i] = e.Type
	}
	return out
}

// Get returns the bytes of the entry of type mime.
func (d Data) Get(mime string) ([]byte, bool) {
	for _, e := range d {
		if e.Type == mime {
			return e.Bytes, true
		}
	}
	return nil, false
}

// Has reports whether d has an entry of type mime.
func (d Data) Has(mime string) bool {
	_, ok := d.Get(mime)
	return ok
}

// Text returns the MIMEText entry as a string.
func (d Data) Text() (string, bool) {
	b, ok := d.Get(MIMEText)
	return string(b), ok
}

// Paths returns the local paths of the file: URIs of the MIMEURIList
// entry, in order; other URIs (https:, a file on another host) are
// skipped. ok is false when d has no URI list or it names no local file.
func (d Data) Paths() (paths []string, ok bool) {
	b, ok := d.Get(MIMEURIList)
	if !ok {
		return nil, false
	}
	for _, u := range ParseURIList(b) {
		if p, err := FilePath(u); err == nil {
			paths = append(paths, p)
		}
	}
	return paths, len(paths) > 0
}

// Clone returns a deep copy of d (nil for an empty d), so a backend can
// keep data the caller may modify afterwards.
func (d Data) Clone() Data {
	if len(d) == 0 {
		return nil
	}
	out := make(Data, len(d))
	for i, e := range d {
		out[i] = DataEntry{Type: e.Type, Bytes: bytes.Clone(e.Bytes)}
	}
	return out
}

// Select returns the entries of d whose types are in types, in the order
// of d (a drop delivers what the app asked for, in the source's order).
func (d Data) Select(types []string) Data {
	var out Data
	for _, e := range d {
		if slices.Contains(types, e.Type) {
			out = append(out, e)
		}
	}
	return out
}

// CheckData panics with an actionable message if d has an entry whose
// Type is not a MIME type ("type/subtype", optional ";parameters", no
// spaces or control characters) or a Type that appears twice. what names
// the call in the message. Empty Data is valid (a cleared clipboard).
func CheckData(what string, d Data) {
	for i, e := range d {
		if !validMIME(e.Type) {
			panicf("platform: %s: entry %d has type %q, which is not a MIME type; use \"type/subtype\" such as %q or a custom \"application/x-myapp-item\"", what, i, e.Type, MIMEText)
		}
		for _, o := range d[:i] {
			if o.Type == e.Type {
				panicf("platform: %s: the type %q appears twice; give each type once, best first", what, e.Type)
			}
		}
	}
}

// CheckMIME panics with an actionable message if t is not a MIME type (see
// CheckData). what names the call.
func CheckMIME(what, t string) {
	if !validMIME(t) {
		panicf("platform: %s: %q is not a MIME type; use \"type/subtype\" such as %q", what, t, MIMEText)
	}
}

// validMIME reports whether t is "type/subtype" with optional parameters,
// printable ASCII without spaces (Wayland and X11 carry types as strings
// and atoms; Win32 registers custom ones by name).
func validMIME(t string) bool {
	if len(t) > 255 {
		return false
	}
	base, _, _ := strings.Cut(t, ";")
	typ, sub, ok := strings.Cut(base, "/")
	if !ok || typ == "" || sub == "" || strings.Contains(sub, "/") {
		return false
	}
	for i := 0; i < len(t); i++ {
		if c := t[i]; c <= ' ' || c >= 0x7f {
			return false
		}
	}
	return true
}

// SelectionType maps a target offered on a Linux selection or drag
// (Wayland wl_data_offer MIME types, X11 target atoms) to the Data type
// it is delivered as: UTF8_STRING, TEXT, STRING and "text/plain" with a
// UTF-8 charset or none become MIMEText; "x-special/gnome-copied-files"
// becomes MIMEURIList; spaces are removed ("text/html; charset=utf-8" is
// "text/html;charset=utf-8"); X11 meta targets (TARGETS, MULTIPLE,
// TIMESTAMP, SAVE_TARGETS, DELETE) and other names that are not MIME
// types are not data (ok false). Backends list each Data type once, at
// the position of its first target, and keep the target they read it
// from (the best one: SelectionTargets' order for text).
func SelectionType(target string) (mime string, ok bool) {
	switch target {
	case "UTF8_STRING", "TEXT", "STRING", "COMPOUND_TEXT":
		return MIMEText, true
	case mimeGnomeCopiedFiles:
		return MIMEURIList, true
	}
	t := strings.Join(strings.Fields(target), "")
	if !validMIME(t) {
		return "", false
	}
	base, params, _ := strings.Cut(t, ";")
	if strings.EqualFold(base, "text/plain") {
		cs := strings.ToLower(params)
		if cs == "" || cs == "charset=utf-8" || cs == "charset=utf8" {
			return MIMEText, true
		}
	}
	return t, true
}

// SelectionTargets returns the Linux targets a backend offers for the Data
// type mime, preferred first: MIMEText as "text/plain;charset=utf-8",
// UTF8_STRING, "text/plain", TEXT and STRING (STRING served as Latin-1,
// the others as UTF-8); MIMEURIList as "text/uri-list" and
// "x-special/gnome-copied-files" (GnomeCopiedFiles); any other type as
// itself.
func SelectionTargets(mime string) []string {
	switch mime {
	case MIMEText:
		return []string{MIMEText, "UTF8_STRING", "text/plain", "TEXT", "STRING"}
	case MIMEURIList:
		return []string{MIMEURIList, mimeGnomeCopiedFiles}
	}
	return []string{mime}
}

// URIList encodes uris as a text/uri-list (RFC 2483): one URI per line,
// each ended by CRLF.
func URIList(uris []string) []byte {
	var b bytes.Buffer
	for _, u := range uris {
		b.WriteString(u)
		b.WriteString("\r\n")
	}
	return b.Bytes()
}

// ParseURIList decodes a text/uri-list: lines ended by CRLF or LF, with
// comment lines (starting with '#') and blank lines skipped, and
// surrounding spaces and a trailing NUL (some X11 sources) removed.
func ParseURIList(b []byte) []string {
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r\x00"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// GnomeCopiedFiles encodes uris as the x-special/gnome-copied-files
// target of Linux file managers: "copy" (or "cut") and one URI per line,
// LF-separated, without a final newline.
func GnomeCopiedFiles(uris []string, cut bool) []byte {
	op := "copy"
	if cut {
		op = "cut"
	}
	return []byte(op + "\n" + strings.Join(uris, "\n"))
}

// ParseGnomeCopiedFiles decodes x-special/gnome-copied-files: the
// operation line ("copy" or "cut") and the URIs. ok is false when the
// first line is neither.
func ParseGnomeCopiedFiles(b []byte) (uris []string, cut, ok bool) {
	first, rest, _ := strings.Cut(string(b), "\n")
	switch strings.TrimSpace(strings.TrimRight(first, "\r\x00")) {
	case "copy":
	case "cut":
		cut = true
	default:
		return nil, false, false
	}
	return ParseURIList([]byte(rest)), cut, true
}

// FileURI returns the file: URI (RFC 8089) of an absolute path of this
// OS, with every byte but unreserved characters and '/' percent-encoded
// (UTF-8 for non-ASCII names). Unix: "/home/a b" is
// "file:///home/a%20b". Windows: "C:\a b" is "file:///C:/a%20b", the UNC
// path "\\server\share\f" is "file://server/share/f", and the "\\?\"
// prefixes of long paths are dropped. A relative path is an error.
func FileURI(path string) (string, error) { return fileURI(path, runtime.GOOS == "windows") }

// FilePath returns the local path of a file: URI, for this OS: the
// inverse of FileURI. It accepts "file:/p", "file:///p" and
// "file://localhost/p"; on Windows also "file:///C|/p" and a host as a UNC
// server ("file://server/share/f"). Other schemes, a host on Unix, and a
// path with a NUL byte are errors.
func FilePath(uri string) (string, error) { return filePath(uri, runtime.GOOS == "windows") }

// fileURI is FileURI with the path syntax chosen by windows, so the tests
// cover both on every OS.
func fileURI(path string, windows bool) (string, error) {
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("platform: FileURI(%q): the path holds a NUL byte", path)
	}
	if !windows {
		if !strings.HasPrefix(path, "/") {
			return "", fmt.Errorf("platform: FileURI(%q): not an absolute path", path)
		}
		return "file://" + EscapePath(path), nil
	}
	p := strings.ReplaceAll(path, `\`, "/")
	switch {
	case strings.HasPrefix(p, "//?/UNC/"):
		p = "//" + p[len("//?/UNC/"):]
	case strings.HasPrefix(p, "//?/"), strings.HasPrefix(p, "//./"):
		p = p[len("//?/"):]
	}
	if strings.HasPrefix(p, "//") { // UNC: //server/share/...
		server, rest, _ := strings.Cut(p[2:], "/")
		if server == "" || rest == "" {
			return "", fmt.Errorf("platform: FileURI(%q): a UNC path needs a server and a share", path)
		}
		return "file://" + EscapePath(server) + "/" + EscapePath(rest), nil
	}
	if len(p) >= 3 && isDriveLetter(p[0]) && p[1] == ':' && p[2] == '/' {
		return "file:///" + p[:2] + EscapePath(p[2:]), nil
	}
	return "", fmt.Errorf("platform: FileURI(%q): not an absolute path (a drive or a UNC path)", path)
}

// filePath is FilePath with the path syntax chosen by windows.
func filePath(uri string, windows bool) (string, error) {
	const scheme = "file:"
	if len(uri) < len(scheme) || !strings.EqualFold(uri[:len(scheme)], scheme) {
		return "", fmt.Errorf("platform: FilePath(%q): not a file: URI", uri)
	}
	rest := uri[len(scheme):]
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	host := ""
	switch {
	case strings.HasPrefix(rest, "////"): // file:////server/share (UNC, as some Windows programs write it)
		rest = rest[2:]
		fallthrough
	case strings.HasPrefix(rest, "//"):
		rest = rest[2:]
		i := strings.IndexByte(rest, '/')
		if i < 0 {
			return "", fmt.Errorf("platform: FilePath(%q): no path", uri)
		}
		host, rest = rest[:i], rest[i:]
	case !strings.HasPrefix(rest, "/"):
		return "", fmt.Errorf("platform: FilePath(%q): not an absolute path", uri)
	}
	p, err := unescapePath(rest)
	if err != nil {
		return "", fmt.Errorf("platform: FilePath(%q): %w", uri, err)
	}
	if strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("platform: FilePath(%q): the path holds a NUL byte", uri)
	}
	if strings.EqualFold(host, "localhost") {
		host = ""
	}
	if !windows {
		if host != "" {
			return "", fmt.Errorf("platform: FilePath(%q): a file on another host", uri)
		}
		return p, nil
	}
	if host != "" {
		h, err := unescapePath(host)
		if err != nil {
			return "", fmt.Errorf("platform: FilePath(%q): %w", uri, err)
		}
		return `\\` + h + strings.ReplaceAll(p, "/", `\`), nil
	}
	// "/C:/x" or the legacy "/C|/x".
	if len(p) >= 3 && isDriveLetter(p[1]) && (p[2] == ':' || p[2] == '|') && (len(p) == 3 || p[3] == '/') {
		d := p[1:2] + ":" + p[3:]
		if len(p) == 3 {
			d += "/"
		}
		return strings.ReplaceAll(d, "/", `\`), nil
	}
	return "", fmt.Errorf("platform: FilePath(%q): not a Windows path (a drive or a server)", uri)
}

func isDriveLetter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }

// EscapePath percent-encodes every byte of p but RFC 3986's unreserved
// characters and '/' (upper-case hex): the path of a file: URI, and the
// Path key of a freedesktop .trashinfo file, which GIO writes with
// g_uri_escape_string(path, "/", FALSE).
func EscapePath(p string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-._~/", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// unescapePath decodes %XX escapes (any byte; '+' stays '+', unlike a
// query).
func unescapePath(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) || unhex(s[i+1]) < 0 || unhex(s[i+2]) < 0 {
			return "", fmt.Errorf("invalid escape %q", s[i:min(i+3, len(s))])
		}
		b.WriteByte(byte(unhex(s[i+1])<<4 | unhex(s[i+2])))
		i += 2
	}
	return b.String(), nil
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// CheckClipboardRead panics with an actionable message if the arguments
// of Clipboard.Read are invalid: a nil callback or a type that is not a
// MIME type.
func CheckClipboardRead(mime string, done bool) {
	checkDone("Clipboard.Read", done)
	CheckMIME("Clipboard.Read", mime)
}
