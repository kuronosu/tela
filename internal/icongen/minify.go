package icongen

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	nsSVG   = "http://www.w3.org/2000/svg"
	nsXLink = "http://www.w3.org/1999/xlink"
	nsXML   = "http://www.w3.org/XML/1998/namespace"
)

// ErrNotMinifiable is returned by Minify for a document with an internal
// DTD subset that declares entities: its text cannot be written back
// without the declarations, so such a document is kept as written.
var ErrNotMinifiable = errors.New("the document declares entities and is kept as written")

// mnode is an element or a text node of the document being minified.
type mnode struct {
	name     xml.Name // as written: Space is the prefix
	uri      string   // the namespace of the element, resolved like encoding/xml does
	attrs    []xml.Attr
	children []*mnode
	text     string
	isText   bool
}

// isSVG reports whether the element is one Tela reads: in the SVG
// namespace, or unprefixed in a document that declares no namespace.
func (n *mnode) isSVG() bool { return n.uri == nsSVG || n.uri == "" }

// Minify rewrites an SVG document without what does not draw, so that
// Tela draws the result exactly as it draws src:
//
//   - the XML declaration, processing instructions, comments and the
//     DOCTYPE;
//   - elements and attributes in namespaces other than SVG and XLink
//     (Inkscape's sodipodi: and inkscape:, RDF metadata, Illustrator's
//     i:...) and the namespace declarations nothing uses;
//   - metadata, title and desc elements, and empty defs;
//   - class attributes, and id attributes nothing refers to as #id, unless
//     the document has a style element (a sheet may select them);
//   - the version, baseProfile and data-* attributes;
//   - white space between elements, and runs of white space inside
//     list-valued attributes (d, points, viewBox, transform...).
//
// The text of text-like elements and of style is kept as it is. A
// document that declares entities is not rewritten: Minify returns
// ErrNotMinifiable. src must be a valid SVG document; Minify reports
// malformed XML but does not check anything else.
func Minify(src []byte) ([]byte, error) {
	d := xml.NewDecoder(bytes.NewReader(src))
	d.Strict = true
	d.Entity = map[string]string{}

	scopes := []map[string]string{{}}
	resolve := func(prefix string) string {
		for i := len(scopes) - 1; i >= 0; i-- {
			if u, ok := scopes[i][prefix]; ok {
				return u
			}
		}
		if prefix != "" {
			return prefix // undeclared: encoding/xml leaves the prefix as the namespace
		}
		return ""
	}
	var stack []*mnode
	var root *mnode
	for {
		tok, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.Directive:
			if bytes.Contains(t, []byte("<!ENTITY")) {
				return nil, ErrNotMinifiable
			}
		case xml.StartElement:
			sc := map[string]string{}
			for _, a := range t.Attr {
				switch {
				case a.Name.Space == "" && a.Name.Local == "xmlns":
					sc[""] = a.Value
				case a.Name.Space == "xmlns":
					sc[a.Name.Local] = a.Value
				}
			}
			scopes = append(scopes, sc)
			n := &mnode{name: t.Name, uri: resolve(t.Name.Space), attrs: t.Attr}
			if root == nil {
				if t.Name.Local != "svg" {
					return nil, fmt.Errorf("the root element is <%s>, not <svg>", t.Name.Local)
				}
				root = n
			} else if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.children = append(p.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].name != t.Name {
				return nil, fmt.Errorf("unexpected end element </%s>", qname(t.Name))
			}
			stack = stack[:len(stack)-1]
			scopes = scopes[:len(scopes)-1]
		case xml.CharData:
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.children = append(p.children, &mnode{isText: true, text: string(t)})
			}
		}
	}
	if len(stack) > 0 {
		return nil, fmt.Errorf("element <%s> is not closed", qname(stack[len(stack)-1].name))
	}
	if root == nil {
		return nil, errors.New("no svg element")
	}

	m := &minifier{used: map[string]bool{}}
	m.scan(root)
	m.collectRefs(root)
	root = m.prune(root, map[string]string{"xml": nsXML})
	if root == nil {
		return nil, errors.New("the root element is not in the SVG namespace")
	}
	m.collectUsed(root)
	var out bytes.Buffer
	m.write(&out, root)
	return out.Bytes(), nil
}

type minifier struct {
	hasStyle bool            // a style element exists: classes and ids may be selected
	refs     string          // every attribute value (but ids) and text: "#id" is searched in it
	used     map[string]bool // prefixes of the kept elements and attributes
}

func (m *minifier) scan(n *mnode) {
	if !n.isText && n.name.Local == "style" {
		m.hasStyle = true
	}
	for _, c := range n.children {
		m.scan(c)
	}
}

// collectRefs gathers the text in which "#id" references can appear.
func (m *minifier) collectRefs(root *mnode) {
	var sb strings.Builder
	var walk func(n *mnode)
	walk = func(n *mnode) {
		if n.isText {
			sb.WriteString(n.text)
			sb.WriteByte('\n')
			return
		}
		for _, a := range n.attrs {
			if a.Name.Local == "id" && a.Name.Space == "" {
				continue
			}
			sb.WriteString(a.Value)
			sb.WriteByte('\n')
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(root)
	m.refs = sb.String()
}

// referenced reports whether some attribute value or text mentions #id.
func (m *minifier) referenced(id string) bool {
	ref := "#" + id
	s := m.refs
	for {
		i := strings.Index(s, ref)
		if i < 0 {
			return false
		}
		rest := s[i+len(ref):]
		// "#a" is not a reference to "ab": an id ends at a character that
		// cannot be part of a name.
		if rest == "" || !isNameChar(rest[0]) {
			return true
		}
		s = rest
	}
}

func isNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':' || c >= 0x80
}

// preserveText names the elements whose character data keeps its white
// space.
var preserveText = map[string]bool{"text": true, "tspan": true, "textPath": true, "style": true, "script": true}

// listAttrs are the attributes whose value is a path or a list: a run of
// white space in them means one separator.
var listAttrs = map[string]bool{
	"d": true, "points": true, "viewBox": true, "transform": true, "gradientTransform": true,
	"patternTransform": true, "stroke-dasharray": true, "preserveAspectRatio": true, "class": true,
}

// droppedElements are the SVG elements that are never drawn and carry no
// information Tela reads.
var droppedElements = map[string]bool{"metadata": true, "title": true, "desc": true}

// prune returns the node without what does not draw, or nil to drop it.
// ns maps the prefixes declared at the parent.
func (m *minifier) prune(n *mnode, ns map[string]string) *mnode {
	return m.pruneIn(n, ns, false)
}

func (m *minifier) pruneIn(n *mnode, ns map[string]string, preserve bool) *mnode {
	if n.isText {
		if !preserve && strings.TrimSpace(n.text) == "" {
			return nil
		}
		return n
	}
	if !n.isSVG() || droppedElements[n.name.Local] {
		return nil
	}
	local := make(map[string]string, len(ns))
	for k, v := range ns {
		local[k] = v
	}
	for _, a := range n.attrs {
		if a.Name.Space == "xmlns" {
			local[a.Name.Local] = a.Value
		}
	}
	out := &mnode{name: n.name, uri: n.uri}
	for _, a := range n.attrs {
		if a, ok := m.keepAttr(a, local); ok {
			out.attrs = append(out.attrs, a)
		}
	}
	childPreserve := preserve || preserveText[n.name.Local]
	for _, c := range n.children {
		if k := m.pruneIn(c, local, childPreserve); k != nil {
			out.children = append(out.children, k)
		}
	}
	if n.name.Local == "defs" && len(out.children) == 0 {
		return nil
	}
	return out
}

// keepAttr decides whether an attribute stays, and returns it normalized.
func (m *minifier) keepAttr(a xml.Attr, ns map[string]string) (xml.Attr, bool) {
	switch {
	case a.Name.Space == "" && a.Name.Local == "xmlns":
		return a, true
	case a.Name.Space == "xmlns":
		// Declarations of the SVG and XLink namespaces; collectUsed and
		// write drop those no kept name uses.
		return a, a.Value == nsSVG || a.Value == nsXLink
	case a.Name.Space != "":
		// Tela reads xlink:href (even when the prefix is not declared);
		// xml: attributes are kept as they are valid anywhere. The rest is
		// editor data.
		return a, a.Name.Space == "xlink" || a.Name.Space == "xml" || ns[a.Name.Space] == nsXLink
	}
	switch name := a.Name.Local; {
	case name == "class":
		if !m.hasStyle {
			return a, false
		}
	case name == "id":
		return a, m.hasStyle || m.referenced(a.Value)
	case name == "version" || name == "baseProfile" || strings.HasPrefix(name, "data-"):
		return a, false
	}
	if listAttrs[a.Name.Local] {
		a.Value = strings.Join(strings.Fields(a.Value), " ")
	}
	return a, true
}

// collectUsed records the prefixes of the kept elements and attributes.
func (m *minifier) collectUsed(n *mnode) {
	if n.isText {
		return
	}
	m.used[n.name.Space] = true
	for _, a := range n.attrs {
		if a.Name.Space != "xmlns" {
			m.used[a.Name.Space] = true
		}
	}
	for _, c := range n.children {
		m.collectUsed(c)
	}
}

// write serializes n.
func (m *minifier) write(b *bytes.Buffer, n *mnode) {
	if n.isText {
		escapeText(b, n.text)
		return
	}
	b.WriteByte('<')
	writeName(b, n.name)
	for _, a := range n.attrs {
		if a.Name.Space == "xmlns" && !m.used[a.Name.Local] {
			continue
		}
		b.WriteByte(' ')
		writeName(b, a.Name)
		b.WriteString(`="`)
		escapeAttr(b, a.Value)
		b.WriteByte('"')
	}
	if len(n.children) == 0 {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	for _, c := range n.children {
		m.write(b, c)
	}
	b.WriteString("</")
	writeName(b, n.name)
	b.WriteByte('>')
}

func writeName(b *bytes.Buffer, n xml.Name) { b.WriteString(qname(n)) }

// qname is the name as written: prefix:local.
func qname(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func escapeText(b *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\r':
			b.WriteString("&#xD;")
		default:
			b.WriteByte(c)
		}
	}
}

func escapeAttr(b *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '"':
			b.WriteString("&quot;")
		case '\r':
			b.WriteString("&#xD;")
		case '\n':
			b.WriteString("&#xA;")
		case '\t':
			b.WriteString("&#x9;")
		default:
			b.WriteByte(c)
		}
	}
}
