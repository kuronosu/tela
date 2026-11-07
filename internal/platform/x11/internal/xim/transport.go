package xim

import (
	"errors"
	"strings"
)

// Atom names of the discovery and the X transport (spec sections 3 and
// "Transport Specific", xtrans: X).
const (
	AtomServers   = "XIM_SERVERS"   // root window: ATOM list of "@server=<name>" selections
	AtomLocales   = "LOCALES"       // selection target: "@locale=<l1>,<l2>,..."
	AtomTransport = "TRANSPORT"     // selection target: "@transport=X/,..."
	AtomXConnect  = "_XIM_XCONNECT" // ClientMessage that opens the transport
	AtomProtocol  = "_XIM_PROTOCOL" // ClientMessage carrying a message (or its last part)
	AtomMoreData  = "_XIM_MOREDATA" // ClientMessage carrying a part that is not the last
)

// CMDataSize is the data of one format-8 ClientMessage. A message longer
// than the transport's boundary (CMDataSize with transport version 0.0)
// travels in a window property announced by a format-32 ClientMessage
// (length, property atom).
const CMDataSize = 20

// ErrTruncated is returned by Split for data that ends inside a message.
var ErrTruncated = errors.New("xim: truncated message")

// Split decodes the messages in data, as read from the transport: whole
// messages back to back, possibly followed by zero padding (a
// ClientMessage's unused bytes). A zero header ends the data.
func Split(data []byte) ([]Message, error) {
	var out []Message
	for len(data) >= 4 {
		if data[0] == 0 {
			break // padding: opcode 0 does not exist
		}
		n := 4 + 4*int(le.Uint16(data[2:]))
		if n > len(data) {
			return out, ErrTruncated
		}
		out = append(out, Message{Major: data[0], Minor: data[1], Body: data[4:n]})
		data = data[n:]
	}
	return out, nil
}

// Chunks cuts a message into the 20-byte data of format-8 ClientMessages,
// the last one padded with zeros. Every chunk but the last is sent as
// _XIM_MOREDATA, the last as _XIM_PROTOCOL.
func Chunks(msg []byte) [][CMDataSize]byte {
	var out [][CMDataSize]byte
	for len(msg) > 0 || len(out) == 0 {
		var c [CMDataSize]byte
		n := copy(c[:], msg)
		msg = msg[n:]
		out = append(out, c)
	}
	return out
}

// IMName returns the input method named by XMODIFIERS ("@im=fcitx" gives
// "fcitx"), or "" when none is named. "none" is no input method, as for
// GTK and Qt. Other modifiers in the value are ignored.
func IMName(xmodifiers string) string {
	for part := range strings.SplitSeq(xmodifiers, "@") {
		if name, ok := strings.CutPrefix(part, "im="); ok {
			name = strings.TrimSpace(name)
			if strings.EqualFold(name, "none") {
				return ""
			}
			return name
		}
	}
	return ""
}

// ServerSelection is the selection a named input method owns.
func ServerSelection(name string) string { return "@server=" + name }

// HasXTransport reports whether a TRANSPORT value ("@transport=X/,tcp/...")
// offers the X transport.
func HasXTransport(v string) bool {
	v, ok := strings.CutPrefix(strings.TrimRight(v, "\x00"), "@transport=")
	if !ok {
		return false
	}
	for t := range strings.SplitSeq(v, ",") {
		if name, _, _ := strings.Cut(t, "/"); name == "X" {
			return true
		}
	}
	return false
}

// ParseLocales splits a LOCALES value ("@locale=en_US,ja_JP.UTF-8").
func ParseLocales(v string) []string {
	v, ok := strings.CutPrefix(strings.TrimRight(v, "\x00"), "@locale=")
	if !ok {
		return nil
	}
	var out []string
	for l := range strings.SplitSeq(v, ",") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// ChooseLocale picks the locale name sent in XIM_OPEN. env is the process
// locale (LC_ALL, else LC_CTYPE, else LANG), such as "ja_JP.UTF-8@mod".
// Like libX11 it tries language_territory.codeset, language.codeset,
// language_territory and language against the server's list, ignoring
// case; then en_US, then the server's first locale, then "C". The locale
// only names the user's language to the input method: Tela's text is
// UTF-8 whatever it is.
func ChooseLocale(server []string, env string) string {
	env, _, _ = strings.Cut(env, "@")
	lang, codeset, _ := strings.Cut(env, ".")
	language, territory, _ := strings.Cut(lang, "_")
	var want []string
	if language != "" && language != "C" && language != "POSIX" {
		if territory != "" && codeset != "" {
			want = append(want, language+"_"+territory+"."+codeset)
		}
		if codeset != "" {
			want = append(want, language+"."+codeset)
		}
		if territory != "" {
			want = append(want, language+"_"+territory)
		}
		want = append(want, language)
	}
	want = append(want, "en_US")
	for _, w := range want {
		for _, s := range server {
			if strings.EqualFold(w, s) {
				return s
			}
		}
	}
	if len(server) > 0 {
		return server[0]
	}
	return "C"
}
