// Portions of this file are ported from Playwright
// (packages/injected/src/ariaSnapshot.ts and
// packages/isomorphic/yaml.ts, version 1.61.1):
//
// Copyright (c) Microsoft Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package semantics

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/utf16x"
)

// ariaNode is a node of Playwright's aria tree: a role node with its
// name, states, properties and children, which are role nodes or strings.
type ariaNode struct {
	n        *Node // nil for the fragment root
	role     string
	name     string
	children []ariaChild
	url      string
}

// ariaChild is a child of an ariaNode: a node, or a string when node is
// nil; src is the text node the string came from (the first of the texts
// merged into it, 0 for the spaces between blocks and for a field's
// value).
type ariaChild struct {
	node *ariaNode
	text string
	src  NodeID
}

// Snapshot returns the aria snapshot of the tree rooted at root: the YAML
// that Playwright's locator.ariaSnapshot() prints for the equivalent
// HTML (see the package documentation), without a trailing newline. It is
// empty for a tree without visible content.
func Snapshot(root *Node) string {
	yaml, _ := SnapshotIDs(root)
	return yaml
}

// SnapshotIDs returns the aria snapshot of [Snapshot] and, for each of its
// lines, the ID of the node the line prints (0 for a text line and for the
// /url line of a link). A node keeps its ID for the life of its element,
// so a tool can name the same element in later snapshots.
func SnapshotIDs(root *Node) (string, []NodeID) {
	yaml, ids, _ := SnapshotTexts(root)
	return yaml, ids
}

// SnapshotTexts is [SnapshotIDs] with, for each line, the text node a
// text line prints: the first of the texts it merges (a line joins the
// texts that follow each other, "Count: " and "3" of one paragraph), 0
// for the other lines. A text node has no line ID of its own, since the
// line may stand for several, but a tool can act on the text it shows.
func SnapshotTexts(root *Node) (yaml string, ids, texts []NodeID) {
	if root == nil {
		return "", nil, nil
	}
	top := &ariaNode{role: "fragment"}
	if root.Kind == KindRoot {
		visitChildren(top, root)
	} else {
		visit(top, root)
	}
	normalizeStrings(top)
	normalizeGenerics(top)
	var r renderer
	for _, c := range top.children {
		if c.node == nil {
			r.addText("- text: "+yamlEscapeValue(c.text), c.src)
		} else {
			r.render(c.node, 0)
		}
	}
	return strings.Join(r.lines, "\n"), r.ids, r.texts
}

// visit adds node n to the aria tree under parent (generateAriaTree's
// visit and processElement).
func visit(parent *ariaNode, n *Node) {
	if n.Has(StateHidden) {
		return // hidden elements and their subtrees are left out
	}
	switch n.Kind {
	case KindText:
		if parent.role != "textbox" && n.Text != "" {
			parent.children = append(parent.children, ariaChild{text: n.Text, src: n.ID})
		}
		return
	case KindRoot:
		visitChildren(parent, n)
		return
	}
	target := parent
	if !n.Flattened() {
		an := &ariaNode{n: n, role: n.Role, name: NormalizeWhiteSpace(n.Name)}
		if (n.Role == "textbox" || n.Role == "searchbox" || n.Role == "combobox") && n.Value != "" {
			// An input's value is its content (toAriaNode), whatever its
			// role; Tela sets the value of text fields only, so the roles
			// are those that fields are given.
			an.children = append(an.children, ariaChild{text: n.Value})
		}
		if n.Role == "link" && n.URL != "" {
			an.url = n.URL
		}
		parent.children = append(parent.children, ariaChild{node: an})
		target = an
	}
	if !n.Inline {
		target.children = append(target.children, ariaChild{text: " "})
	}
	visitChildren(target, n)
	if !n.Inline {
		target.children = append(target.children, ariaChild{text: " "})
	}
	// Also on the parent of a flattened element, as processElement does.
	if len(target.children) == 1 && target.children[0].node == nil && target.children[0].text == target.name {
		target.children = nil
	}
}

func visitChildren(parent *ariaNode, n *Node) {
	for _, c := range n.Children {
		visit(parent, c)
	}
}

// normalizeStrings merges consecutive strings, collapses their white
// space and drops the empty ones; a single string child equal to the name
// is dropped (normalizeStringChildren).
func normalizeStrings(an *ariaNode) {
	var out []ariaChild
	var buf strings.Builder
	pending := false
	var src NodeID // the first text with something to show
	flush := func() {
		if !pending {
			return
		}
		if s := NormalizeWhiteSpace(buf.String()); s != "" {
			out = append(out, ariaChild{text: s, src: src})
		}
		buf.Reset()
		pending, src = false, 0
	}
	for _, c := range an.children {
		if c.node == nil {
			buf.WriteString(c.text)
			pending = true
			if src == 0 && NormalizeWhiteSpace(c.text) != "" {
				src = c.src
			}
			continue
		}
		flush()
		normalizeStrings(c.node)
		out = append(out, c)
	}
	flush()
	an.children = out
	if len(an.children) == 1 && an.children[0].node == nil && an.children[0].text == an.name {
		an.children = nil
	}
}

// normalizeGenerics removes the explicit generic nodes without a name or
// children (normalizeGenericRoles: in the default mode no node has a ref,
// so only empty generics go).
func normalizeGenerics(an *ariaNode) {
	var out []ariaChild
	for _, c := range an.children {
		if c.node == nil {
			out = append(out, c)
			continue
		}
		normalizeGenerics(c.node)
		if c.node.role == "generic" && c.node.name == "" && len(c.node.children) == 0 {
			continue
		}
		out = append(out, c)
	}
	an.children = out
}

// key returns the YAML key of a role node: role, quoted name and states
// (createKey).
func key(an *ariaNode) string {
	var b strings.Builder
	b.WriteString(an.role)
	if an.name != "" && utf16x.Len(an.name) <= 900 {
		b.WriteByte(' ')
		if strings.HasPrefix(an.name, "/") && strings.HasSuffix(an.name, "/") {
			b.WriteString(an.name)
		} else {
			b.WriteString(jsonString(an.name))
		}
	}
	n, role := an.n, an.role
	if slices.Contains(checkedRoles, role) {
		switch n.Checked {
		case ToggleMixed:
			b.WriteString(" [checked=mixed]")
		case ToggleTrue:
			b.WriteString(" [checked]")
		}
	}
	if slices.Contains(disabledRoles, role) && n.Has(StateDisabled) {
		b.WriteString(" [disabled]")
	}
	if slices.Contains(expandedRoles, role) && n.Expanded == ToggleTrue {
		b.WriteString(" [expanded]")
	}
	// Playwright reads the aria-invalid attribute alone (getAriaInvalid),
	// not a field's own validity: the AriaInvalid prop, not StateInvalid.
	if slices.Contains(invalidRoles, role) && n.Invalid == ToggleTrue {
		b.WriteString(" [invalid]")
	}
	if slices.Contains(levelRoles, role) && n.Level > 0 {
		b.WriteString(" [level=" + strconv.Itoa(n.Level) + "]")
	}
	if slices.Contains(pressedRoles, role) {
		switch n.Pressed {
		case ToggleMixed:
			b.WriteString(" [pressed=mixed]")
		case ToggleTrue:
			b.WriteString(" [pressed]")
		}
	}
	if slices.Contains(selectedRoles, role) && n.Selected == ToggleTrue {
		b.WriteString(" [selected]")
	}
	return b.String()
}

// renderer collects the lines of a snapshot, the node each one prints and
// the text node of each text line.
type renderer struct {
	lines []string
	ids   []NodeID
	texts []NodeID
}

func (r *renderer) add(line string, id NodeID) {
	r.lines = append(r.lines, line)
	r.ids = append(r.ids, id)
	r.texts = append(r.texts, 0)
}

// addText adds a text line made from text node src.
func (r *renderer) addText(line string, src NodeID) {
	r.add(line, 0)
	r.texts[len(r.texts)-1] = src
}

// render appends the lines of role node an at depth (renderAriaTree's
// visit).
func (r *renderer) render(an *ariaNode, depth int) {
	indent := strings.Repeat("  ", depth)
	k := indent + "- " + yamlEscapeKey(key(an))
	hasProps := an.url != ""
	switch {
	case len(an.children) == 0 && !hasProps:
		r.add(k, an.n.ID)
		return
	case len(an.children) == 1 && an.children[0].node == nil && !hasProps:
		r.add(k+": "+yamlEscapeValue(an.children[0].text), an.n.ID)
		return
	}
	r.add(k+":", an.n.ID)
	if hasProps {
		r.add(indent+"  - /url: "+yamlEscapeValue(an.url), 0)
	}
	for _, c := range an.children {
		if c.node == nil {
			r.addText(indent+"  - text: "+yamlEscapeValue(c.text), c.src)
		} else {
			r.render(c.node, depth+1)
		}
	}
}

// ---------------------------------------------------------------------
// Strings: JavaScript's semantics, which Playwright's output depends on.

// jsSpace reports whether r matches JavaScript's \s.
func jsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// NormalizeWhiteSpace removes zero-width spaces and soft hyphens, trims
// and collapses runs of white space to one space, with JavaScript's
// definition of white space (Playwright's normalizeWhiteSpace).
func NormalizeWhiteSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch {
		case r == 0x200b || r == 0xad:
			continue
		case jsSpace(r):
			space = b.Len() > 0
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TrimSpace trims JavaScript white space (String.prototype.trim).
func TrimSpace(s string) string { return strings.TrimFunc(s, jsSpace) }

// jsonString quotes s like JavaScript's JSON.stringify.
func jsonString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteByte(hexDigit(byte(r) >> 4))
				b.WriteByte(hexDigit(byte(r) & 15))
				continue
			}
			if r == utf8.RuneError {
				r = 0xfffd
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func hexDigit(v byte) byte { return "0123456789abcdef"[v] }

// yamlEscapeKey quotes a key with single quotes when YAML needs it
// (yamlEscapeKeyIfNeeded).
func yamlEscapeKey(s string) string {
	if !yamlNeedsQuotes(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// yamlEscapeValue quotes a value with double quotes when YAML needs it
// (yamlEscapeValueIfNeeded).
func yamlEscapeValue(s string) string {
	if !yamlNeedsQuotes(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r <= 0x1f || r >= 0x7f && r <= 0x9f:
			b.WriteString(`\x`)
			b.WriteByte(hexDigit(byte(r) >> 4))
			b.WriteByte(hexDigit(byte(r) & 15))
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// yamlNeedsQuotes reports whether s must be quoted to read back as the
// same YAML string (yamlStringNeedsQuotes).
func yamlNeedsQuotes(s string) bool {
	if s == "" {
		return true
	}
	first, _ := utf8.DecodeRuneInString(s)
	last, _ := utf8.DecodeLastRuneInString(s)
	if jsSpace(first) || jsSpace(last) {
		return true
	}
	prevSpace := false
	for i, r := range s {
		switch {
		case r <= 0x08, r == 0x0b, r == 0x0c, r >= 0x0e && r <= 0x1f, r >= 0x7f && r <= 0x9f:
			return true // control characters
		case r == '\n' || r == '\r':
			return true // line breaks
		case r == ':':
			// ':' followed by white space or at the end
			next, _ := utf8.DecodeRuneInString(s[i+1:])
			if i+1 == len(s) || jsSpace(next) {
				return true
			}
		case r == '#' && prevSpace:
			return true // a comment
		case r == '{' || r == '}' || r == '`':
			return true
		}
		prevSpace = jsSpace(r)
	}
	switch first {
	case '-', '&', '*', ']', ',', '?', '!', '>', '|', '@', '"', '\'', '#', '%', '[':
		return true
	}
	switch strings.ToLower(s) {
	case "y", "n", "yes", "no", "true", "false", "on", "off", "null":
		return true
	}
	return jsNumber(s)
}

var (
	jsDecimal = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)
	jsRadix   = regexp.MustCompile(`^0([xX][0-9a-fA-F]+|[oO][0-7]+|[bB][01]+)$`)
)

// jsNumber reports whether JavaScript's Number(s) is not NaN for a string
// without leading or trailing white space.
func jsNumber(s string) bool {
	switch s {
	case "Infinity", "+Infinity", "-Infinity":
		return true
	}
	return jsDecimal.MatchString(s) || jsRadix.MatchString(s)
}
