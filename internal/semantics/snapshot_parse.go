package semantics

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// SnapshotLine is a parsed line of the YAML that [Snapshot] writes: an
// element with a role, its name and states. The parser lives next to the
// writer (key, yamlEscapeKey) so that a change to the format breaks the
// round-trip test here and not a consumer's refs.
type SnapshotLine struct {
	Indent string // leading spaces
	Quoted bool   // the key was single-quoted
	Key    string // the key, unquoted: role "name" [states]
	Rest   string // what follows the key: "", ":", ": value"

	Role     string
	Name     string
	HasName  bool   // the line prints a name (a longer one is left out)
	States   string // " [checked] [disabled]"...
	Disabled bool
}

// snapRoleRe matches an ARIA role at the start of a key.
var snapRoleRe = regexp.MustCompile(`^[a-z]+`)

// snapStatesRe matches the states after the name.
var snapStatesRe = regexp.MustCompile(`^(?: \[[a-z]+(?:=[a-z0-9]+)?\])*$`)

// ParseSnapshotLine parses one line of the YAML of [Snapshot]. ok is false
// for the lines that are not an element with a role (text, /url, a line it
// does not know).
func ParseSnapshotLine(line string) (l SnapshotLine, ok bool) {
	trimmed := strings.TrimLeft(line, " ")
	l.Indent = line[:len(line)-len(trimmed)]
	if !strings.HasPrefix(trimmed, "- ") {
		return l, false
	}
	body := trimmed[2:]
	if strings.HasPrefix(body, "'") {
		// A single-quoted key: '' stands for '.
		var b strings.Builder
		i := 1
		for ; i < len(body); i++ {
			if body[i] == '\'' {
				if i+1 < len(body) && body[i+1] == '\'' {
					b.WriteByte('\'')
					i++
					continue
				}
				break
			}
			b.WriteByte(body[i])
		}
		if i >= len(body) {
			return l, false
		}
		l.Quoted, l.Key, l.Rest = true, b.String(), body[i+1:]
	} else {
		// A plain key cannot contain ": " nor end with ':' (it would be
		// quoted).
		switch i := strings.Index(body, ": "); {
		case i >= 0:
			l.Key, l.Rest = body[:i], body[i:]
		case strings.HasSuffix(body, ":"):
			l.Key, l.Rest = body[:len(body)-1], ":"
		default:
			l.Key = body
		}
	}
	l.Role = snapRoleRe.FindString(l.Key)
	if l.Role == "" || l.Key == "text" {
		return l, false // "- text: ..." or "- /url: ..."
	}
	after := l.Key[len(l.Role):]
	switch {
	case after == "":
	case strings.HasPrefix(after, ` "`):
		dec := json.NewDecoder(strings.NewReader(after[1:]))
		var name string
		if err := dec.Decode(&name); err != nil {
			return l, false
		}
		l.Name, l.HasName = name, true
		after = after[1+int(dec.InputOffset()):]
	case strings.HasPrefix(after, " /"):
		// A name that looks like a regular expression is printed as is;
		// the states follow its closing slash (which is the opening one
		// for the name "/").
		i := -1
		for j := 2; j <= len(after); j++ {
			if after[j-1] == '/' && snapStatesRe.MatchString(after[j:]) {
				i = j
				break
			}
		}
		if i < 0 {
			return l, false
		}
		l.Name, l.HasName = after[1:i], true
		after = after[i:]
	}
	if !snapStatesRe.MatchString(after) {
		return l, false
	}
	l.States = after
	l.Disabled = strings.Contains(after, " [disabled]")
	return l, true
}

// WithSuffix returns the line with suffix (such as " [ref=e1]") added to
// its key, after the states, and the rest of the line unchanged. A quoted
// key stays quoted; a plain one is quoted if YAML needs it for the longer
// text (as the writer decides, yamlEscapeKey). The result parses as the
// same line with the longer key.
func (l SnapshotLine) WithSuffix(suffix string) string {
	key := l.Key + suffix
	if l.Quoted {
		key = "'" + strings.ReplaceAll(key, "'", "''") + "'"
	} else {
		key = yamlEscapeKey(key)
	}
	return l.Indent + "- " + key + l.Rest
}

// Describe names the element of the line: `button "Save"`.
func (l SnapshotLine) Describe() string {
	if l.HasName {
		return l.Role + " " + strconv.Quote(l.Name)
	}
	return l.Role
}
