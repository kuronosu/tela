package assoc

import (
	"strings"
)

// keyFile is a key file of the Desktop Entry specification ("Basic
// format of the file"): groups of key=value lines, as in desktop entries
// and mimeapps.list. It keeps every line as it was, so editing one key
// leaves the rest of a file the user owns intact (comments, order, other
// groups, unknown keys).
type keyFile struct {
	lines []string
}

func parseKeyFile(data []byte) *keyFile {
	s := string(data)
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return &keyFile{}
	}
	return &keyFile{lines: strings.Split(s, "\n")}
}

func (k *keyFile) bytes() []byte {
	if len(k.lines) == 0 {
		return []byte{}
	}
	return []byte(strings.Join(k.lines, "\n") + "\n")
}

// groupOf returns the name of the group header on line l, if it is one.
func groupOf(l string) (string, bool) {
	l = strings.TrimSpace(l)
	if len(l) >= 2 && l[0] == '[' && l[len(l)-1] == ']' {
		return l[1 : len(l)-1], true
	}
	return "", false
}

// entryOf splits line l into its key and raw value, if it is an entry
// (not a comment, a blank line or a header). Spaces around '=' are
// ignored, as the specification says.
func entryOf(l string) (key, value string, ok bool) {
	t := strings.TrimSpace(l)
	if t == "" || t[0] == '#' || t[0] == '[' {
		return "", "", false
	}
	key, value, ok = strings.Cut(t, "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimLeft(value, " \t"), true
}

// groupRange returns the lines [start, end) of the entries of group (start
// is the line after the header), or ok false when it is missing. A
// repeated group is invalid; the first one is used.
func (k *keyFile) groupRange(group string) (start, end int, ok bool) {
	for i, l := range k.lines {
		g, isHeader := groupOf(l)
		if !isHeader {
			continue
		}
		if ok {
			return start, i, true
		}
		if g == group {
			start, ok = i+1, true
		}
	}
	return start, len(k.lines), ok
}

// sameKey compares keys without regard to case: MIME types (the keys of
// mimeapps.list) are case-insensitive, and desktop entry keys are only
// looked up by their standard spelling.
func sameKey(a, b string) bool { return strings.EqualFold(a, b) }

// get returns the raw (still escaped) value of key in group.
func (k *keyFile) get(group, key string) (string, bool) {
	start, end, ok := k.groupRange(group)
	if !ok {
		return "", false
	}
	for _, l := range k.lines[start:end] {
		if kk, v, ok := entryOf(l); ok && sameKey(kk, key) {
			return v, true
		}
	}
	return "", false
}

// keys returns the keys of group, in order.
func (k *keyFile) keys(group string) []string {
	start, end, ok := k.groupRange(group)
	if !ok {
		return nil
	}
	var out []string
	for _, l := range k.lines[start:end] {
		if kk, _, ok := entryOf(l); ok {
			out = append(out, kk)
		}
	}
	return out
}

// set sets key in group to the raw value, replacing the existing line or
// adding one after the group's last entry (adding the group at the end
// when it is missing).
func (k *keyFile) set(group, key, value string) {
	line := key + "=" + value
	start, end, ok := k.groupRange(group)
	if !ok {
		if n := len(k.lines); n > 0 && strings.TrimSpace(k.lines[n-1]) != "" {
			k.lines = append(k.lines, "")
		}
		k.lines = append(k.lines, "["+group+"]", line)
		return
	}
	last := start - 1
	for i := start; i < end; i++ {
		kk, _, isEntry := entryOf(k.lines[i])
		if isEntry && sameKey(kk, key) {
			k.lines[i] = line
			return
		}
		if isEntry {
			last = i
		}
	}
	k.lines = append(k.lines[:last+1], append([]string{line}, k.lines[last+1:]...)...)
}

// del removes key from group; it reports whether it was there.
func (k *keyFile) del(group, key string) bool {
	start, end, ok := k.groupRange(group)
	if !ok {
		return false
	}
	for i := start; i < end; i++ {
		if kk, _, ok := entryOf(k.lines[i]); ok && sameKey(kk, key) {
			k.lines = append(k.lines[:i], k.lines[i+1:]...)
			return true
		}
	}
	return false
}

// unescapeValue decodes the escapes of a string value: \s, \n, \t, \r and
// \\. Other escapes (such as a list's \;) are kept as they are; splitList
// handles lists.
func unescapeValue(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c != '\\' || i+1 == len(v) {
			b.WriteByte(c)
			continue
		}
		i++
		switch v[i] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default: // \; and unknown escapes are kept for splitList
			b.WriteByte('\\')
			b.WriteByte(v[i])
		}
	}
	return b.String()
}

// escapeValue encodes a string value: the inverse of unescapeValue for
// values without ';' list semantics.
func escapeValue(v string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`)
	v = r.Replace(v)
	if strings.HasPrefix(v, " ") {
		v = `\s` + v[1:]
	}
	return v
}

// splitList splits a raw list value ("a;b;c;") into its elements,
// unescaped; empty elements are dropped.
func splitList(raw string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if s := strings.TrimSpace(unescapeValue(b.String())); s != "" {
			out = append(out, s)
		}
		b.Reset()
	}
	for i := 0; i < len(raw); i++ {
		switch {
		case raw[i] == '\\' && i+1 < len(raw) && raw[i+1] == ';':
			b.WriteByte(';')
			i++
		case raw[i] == '\\' && i+1 < len(raw):
			b.WriteByte(raw[i])
			b.WriteByte(raw[i+1])
			i++
		case raw[i] == ';':
			flush()
		default:
			b.WriteByte(raw[i])
		}
	}
	flush()
	return out
}

// joinList encodes a list value (MIME types, desktop file IDs) with the
// trailing ';' the specification recommends; a ';' in an element is
// escaped.
func joinList(elems []string) string {
	if len(elems) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range elems {
		b.WriteString(strings.ReplaceAll(escapeValue(e), ";", `\;`))
		b.WriteByte(';')
	}
	return b.String()
}

// quoteExecArg quotes one argument of an Exec key (Desktop Entry
// specification, "The Exec key"): inside double quotes, '"', '`', '$' and
// '\' are escaped with a backslash, and a literal '%' is written "%%" (a
// field code otherwise). The result is then escaped as a string value
// (escapeValue), so a literal backslash becomes four in the file, as the
// specification requires. Every argument is quoted, which is always
// allowed and keeps reserved characters (spaces, quotes, '~', '#', …)
// literal.
func quoteExecArg(a string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range a {
		switch r {
		case '"', '`', '$', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '%':
			b.WriteString("%%")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
