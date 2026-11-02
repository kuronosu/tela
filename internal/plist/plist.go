// Package plist encodes and decodes XML property lists, the format of a
// macOS app bundle's Info.plist. It supports the eight property list types
// and nothing else (no binary or JSON/OpenStep formats, no struct tags).
//
// It is a leaf package (ADR-189): it imports only the standard library, so
// every layer and the command line may use it, on every OS. `tela build`
// writes a bundle's Info.plist with it, and the registration of URL schemes
// and file types edits the running bundle's Info.plist in place
// (CFBundleURLTypes, CFBundleDocumentTypes); both rely on two properties:
//
//   - Decoding then encoding keeps every key and value, including the ones
//     the code does not know, so an edit never drops what another tool put
//     there.
//   - The output is deterministic: dictionary keys sorted by byte order,
//     tabs for indentation and Apple's header and DOCTYPE, so an unchanged
//     document encodes to the same bytes and "write only when it changed"
//     is a byte comparison.
//
// # Go values
//
//	plist type  Go type                     decoded as
//	dict        map[string]any ([Dict])     Dict
//	array       []any ([Array])             Array
//	string      string                      string
//	integer     any integer type            int64
//	real        float32, float64            float64
//	true/false  bool                        bool
//	date        time.Time (encoded in UTC)  time.Time (UTC)
//	data        []byte                      []byte
//
// [Marshal] also takes maps with string keys and slices of any of these
// (map[string]string, []string, ...) and encodes them as dict and array.
// Values it cannot encode (nil, structs, channels, unsigned integers above
// the int64 range, strings with characters XML cannot hold) are errors that
// name the key path.
package plist

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Dict is a property list dictionary.
type Dict = map[string]any

// Array is a property list array.
type Array = []any

// header is the prologue Apple's tools write.
const header = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
`

// maxDepth bounds the nesting that Unmarshal and Marshal accept, so that a
// hostile or cyclic document cannot exhaust the stack.
const maxDepth = 100

// dataLine is the number of base64 characters per line of a data value, as
// Apple's tools wrap them.
const dataLine = 68

// Marshal returns the XML property list of v, with a trailing newline.
func Marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(header)
	if err := encode(&b, reflect.ValueOf(v), 0, "root"); err != nil {
		return nil, err
	}
	b.WriteString("</plist>\n")
	return b.Bytes(), nil
}

var timeType = reflect.TypeOf(time.Time{})

func encode(b *bytes.Buffer, v reflect.Value, depth int, path string) error {
	if depth > maxDepth {
		return fmt.Errorf("plist: %s: nesting deeper than %d levels", path, maxDepth)
	}
	for v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) {
		if v.IsNil() {
			return fmt.Errorf("plist: %s: nil cannot be encoded; a property list has no null, leave the key out", path)
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return fmt.Errorf("plist: %s: nil cannot be encoded; a property list has no null, leave the key out", path)
	}
	tabs := strings.Repeat("\t", depth)
	if v.Type() == timeType {
		t := v.Interface().(time.Time)
		fmt.Fprintf(b, "%s<date>%s</date>\n", tabs, t.UTC().Format("2006-01-02T15:04:05Z"))
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		s, err := escape(v.String())
		if err != nil {
			return fmt.Errorf("plist: %s: %v", path, err)
		}
		fmt.Fprintf(b, "%s<string>%s</string>\n", tabs, s)
	case reflect.Bool:
		if v.Bool() {
			fmt.Fprintf(b, "%s<true/>\n", tabs)
		} else {
			fmt.Fprintf(b, "%s<false/>\n", tabs)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fmt.Fprintf(b, "%s<integer>%d</integer>\n", tabs, v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if v.Uint() > math.MaxInt64 {
			return fmt.Errorf("plist: %s: integer %d is above the int64 range", path, v.Uint())
		}
		fmt.Fprintf(b, "%s<integer>%d</integer>\n", tabs, v.Uint())
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		switch {
		case math.IsNaN(f):
			fmt.Fprintf(b, "%s<real>nan</real>\n", tabs)
		case math.IsInf(f, 1):
			fmt.Fprintf(b, "%s<real>+infinity</real>\n", tabs)
		case math.IsInf(f, -1):
			fmt.Fprintf(b, "%s<real>-infinity</real>\n", tabs)
		default:
			bits := 64
			if v.Kind() == reflect.Float32 {
				bits = 32
			}
			fmt.Fprintf(b, "%s<real>%s</real>\n", tabs, strconv.FormatFloat(f, 'g', -1, bits))
		}
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("plist: %s: a dict takes string keys, not %s", path, v.Type().Key())
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		if len(keys) == 0 {
			fmt.Fprintf(b, "%s<dict/>\n", tabs)
			return nil
		}
		fmt.Fprintf(b, "%s<dict>\n", tabs)
		for _, k := range keys {
			ks, err := escape(k.String())
			if err != nil {
				return fmt.Errorf("plist: %s: key %q: %v", path, k.String(), err)
			}
			fmt.Fprintf(b, "%s\t<key>%s</key>\n", tabs, ks)
			if err := encode(b, v.MapIndex(k), depth+1, path+"."+k.String()); err != nil {
				return err
			}
		}
		fmt.Fprintf(b, "%s</dict>\n", tabs)
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			data := make([]byte, v.Len())
			reflect.Copy(reflect.ValueOf(data), v)
			writeData(b, data, tabs)
			return nil
		}
		if v.Len() == 0 {
			fmt.Fprintf(b, "%s<array/>\n", tabs)
			return nil
		}
		fmt.Fprintf(b, "%s<array>\n", tabs)
		for i := 0; i < v.Len(); i++ {
			if err := encode(b, v.Index(i), depth+1, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		fmt.Fprintf(b, "%s</array>\n", tabs)
	default:
		return fmt.Errorf("plist: %s: a %s cannot be encoded; use a string, number, bool, time.Time, []byte, map[string]any or []any", path, v.Type())
	}
	return nil
}

func writeData(b *bytes.Buffer, data []byte, tabs string) {
	if len(data) == 0 {
		fmt.Fprintf(b, "%s<data></data>\n", tabs)
		return
	}
	enc := base64.StdEncoding.EncodeToString(data)
	fmt.Fprintf(b, "%s<data>\n", tabs)
	for len(enc) > 0 {
		n := min(dataLine, len(enc))
		fmt.Fprintf(b, "%s%s\n", tabs, enc[:n])
		enc = enc[n:]
	}
	fmt.Fprintf(b, "%s</data>\n", tabs)
}

// escape escapes the characters XML text cannot hold as is. Quotes stay as
// they are, like Apple's tools write them. Characters XML 1.0 forbids
// (most control characters) are an error: the document would not parse.
func escape(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("the string is not valid UTF-8")
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '\r':
			// XML parsers turn a raw CR (and CRLF) into LF when they read
			// it, so a CR is written as a character reference to round-trip.
			b.WriteString("&#13;")
		case r == '\t' || r == '\n' || r >= 0x20 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000:
			b.WriteRune(r)
		default:
			return "", fmt.Errorf("the character U+%04X cannot appear in XML", r)
		}
	}
	return b.String(), nil
}

// Unmarshal parses an XML property list and returns its root value: a
// [Dict], [Array], string, int64, float64, bool, time.Time or []byte.
// Binary property lists are rejected with a message that says so.
func Unmarshal(data []byte) (any, error) {
	if bytes.HasPrefix(data, []byte("bplist")) {
		return nil, errors.New("plist: this is a binary property list; only the XML format is supported (convert it with `plutil -convert xml1 file`)")
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	// Find <plist>.
	for {
		tok, err := d.Token()
		if err != nil {
			if err == io.EOF {
				return nil, errors.New("plist: no <plist> element")
			}
			return nil, fmt.Errorf("plist: %v", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local != "plist" {
				return nil, fmt.Errorf("plist: the root element is <%s>, want <plist>", se.Name.Local)
			}
			break
		}
	}
	v, end, err := decodeValue(d, 0)
	if err != nil {
		return nil, err
	}
	if end {
		return nil, errors.New("plist: <plist> holds no value")
	}
	// Only </plist> may follow, besides whitespace and comments.
	tok, err := nextToken(d)
	if err != nil {
		return nil, err
	}
	if _, ok := tok.(xml.EndElement); !ok {
		return nil, fmt.Errorf("plist: <plist> holds more than one value (<%s> after the first)", tok.(xml.StartElement).Name.Local)
	}
	return v, nil
}

// UnmarshalDict is [Unmarshal] for a document whose root is a dict, as an
// Info.plist is.
func UnmarshalDict(data []byte) (Dict, error) {
	v, err := Unmarshal(data)
	if err != nil {
		return nil, err
	}
	d, ok := v.(Dict)
	if !ok {
		return nil, fmt.Errorf("plist: the root is a %s, want a dict", typeName(v))
	}
	return d, nil
}

func typeName(v any) string {
	switch v.(type) {
	case Dict:
		return "dict"
	case Array:
		return "array"
	case string:
		return "string"
	case int64:
		return "integer"
	case float64:
		return "real"
	case bool:
		return "bool"
	case time.Time:
		return "date"
	case []byte:
		return "data"
	}
	return fmt.Sprintf("%T", v)
}

// nextToken returns the next start or end element, skipping whitespace,
// comments, processing instructions and directives. Other text is an
// error.
func nextToken(d *xml.Decoder) (xml.Token, error) {
	for {
		tok, err := d.Token()
		if err != nil {
			if err == io.EOF {
				return nil, errors.New("plist: unexpected end of document")
			}
			return nil, fmt.Errorf("plist: %v", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			if len(bytes.TrimSpace(t)) == 0 {
				continue
			}
			return nil, fmt.Errorf("plist: unexpected text %q between elements", truncate(string(t)))
		case xml.Comment, xml.ProcInst, xml.Directive:
			continue
		}
		return tok, nil
	}
}

func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 20 {
		return s[:20] + "..."
	}
	return s
}

// decodeValue reads the next value; the decoder is positioned before its
// start element. When the next element is an end element instead (the
// close of the array or dict that holds the position), it is consumed and
// end is true.
func decodeValue(d *xml.Decoder, depth int) (v any, end bool, err error) {
	if depth > maxDepth {
		return nil, false, fmt.Errorf("plist: nesting deeper than %d levels", maxDepth)
	}
	tok, err := nextToken(d)
	if err != nil {
		return nil, false, err
	}
	se, ok := tok.(xml.StartElement)
	if !ok {
		return nil, true, nil
	}
	switch se.Name.Local {
	case "dict":
		out := Dict{}
		for {
			tok, err := nextToken(d)
			if err != nil {
				return nil, false, err
			}
			t, ok := tok.(xml.StartElement)
			if !ok {
				return out, false, nil
			}
			if t.Name.Local != "key" {
				return nil, false, fmt.Errorf("plist: a dict holds <key> and a value, found <%s> where a key goes", t.Name.Local)
			}
			key, err := text(d, "key")
			if err != nil {
				return nil, false, err
			}
			if _, dup := out[key]; dup {
				return nil, false, fmt.Errorf("plist: duplicate key %q in a dict", key)
			}
			val, end, err := decodeValue(d, depth+1)
			if err != nil {
				return nil, false, fmt.Errorf("%w (key %q)", err, key)
			}
			if end {
				return nil, false, fmt.Errorf("plist: the key %q has no value", key)
			}
			out[key] = val
		}
	case "array":
		out := Array{}
		for {
			val, end, err := decodeValue(d, depth+1)
			if err != nil {
				return nil, false, err
			}
			if end {
				return out, false, nil
			}
			out = append(out, val)
		}
	}
	s, err := text(d, se.Name.Local)
	if err != nil {
		return nil, false, err
	}
	switch se.Name.Local {
	case "string":
		return s, false, nil
	case "integer":
		n, perr := parseInteger(strings.TrimSpace(s))
		if perr != nil {
			return nil, false, fmt.Errorf("plist: <integer>%s</integer> is not an integer in the int64 range", truncate(s))
		}
		return n, false, nil
	case "real":
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "nan":
			return math.NaN(), false, nil
		case "infinity", "+infinity":
			return math.Inf(1), false, nil
		case "-infinity":
			return math.Inf(-1), false, nil
		}
		f, perr := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if perr != nil {
			return nil, false, fmt.Errorf("plist: <real>%s</real> is not a number", truncate(s))
		}
		return f, false, nil
	case "true", "false":
		return se.Name.Local == "true", false, nil
	case "date":
		t, perr := time.Parse(time.RFC3339, strings.TrimSpace(s))
		if perr != nil {
			return nil, false, fmt.Errorf("plist: <date>%s</date> is not a date like 2006-01-02T15:04:05Z", truncate(s))
		}
		return t.UTC(), false, nil
	case "data":
		clean := strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\t', '\n', '\r':
				return -1
			}
			return r
		}, s)
		raw, perr := base64.StdEncoding.DecodeString(clean)
		if perr != nil {
			return nil, false, fmt.Errorf("plist: <data> is not valid base64: %v", perr)
		}
		return raw, false, nil
	}
	return nil, false, fmt.Errorf("plist: unknown element <%s>", se.Name.Local)
}

// text reads the character data of the element just opened, up to its end
// element (nested elements are an error).
func text(d *xml.Decoder, name string) (string, error) {
	var b strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			if err == io.EOF {
				return "", fmt.Errorf("plist: <%s> is not closed", name)
			}
			return "", fmt.Errorf("plist: %v", err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String(), nil
		case xml.StartElement:
			return "", fmt.Errorf("plist: <%s> holds <%s>; it takes text only", name, t.Name.Local)
		}
	}
}

// parseInteger reads an <integer> as CFPropertyList does: decimal, with an
// optional sign, or hexadecimal after "0x". A leading zero is still decimal
// (strconv's base 0 would read 010 as octal 8, and accept "0o", "0b" and
// underscores, which Apple rejects).
func parseInteger(s string) (int64, error) {
	neg := strings.HasPrefix(s, "-")
	digits := strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if neg && strings.HasPrefix(s, "-+") {
		return 0, strconv.ErrSyntax
	}
	if h, ok := strings.CutPrefix(strings.ToLower(digits), "0x"); ok {
		u, err := strconv.ParseUint(h, 16, 64)
		if err != nil || h == "" || strings.ContainsAny(h, "+-") {
			return 0, strconv.ErrSyntax
		}
		if neg {
			if u > 1<<63 {
				return 0, strconv.ErrRange
			}
			return -int64(u), nil
		}
		if u > math.MaxInt64 {
			return 0, strconv.ErrRange
		}
		return int64(u), nil
	}
	if digits == "" || strings.ContainsAny(digits, "+-") {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(s, 10, 64)
}
