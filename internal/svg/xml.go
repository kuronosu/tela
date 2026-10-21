package svg

import (
	"bytes"

	"encoding/xml"
	"errors"
	"fmt"
	"github.com/kuronosu/tela/internal/gfx"
	"io"
	"strings"
)

// Namespaces.
const (
	nsSVG   = "http://www.w3.org/2000/svg"
	nsXLink = "http://www.w3.org/1999/xlink"
)

// Limits that keep hostile documents bounded. Parse returns ErrTooComplex
// when a document exceeds them.
const (
	maxElements    = 100_000 // SVG elements in the source
	maxDepth       = 256     // element nesting in the source
	maxNodes       = 200_000 // render nodes after use expansion
	maxEntities    = 64      // entities declared in the internal DTD subset
	maxEntityValue = 128     // bytes of an entity's replacement text
	maxRefChain    = 32      // gradient href chains, nested use instances
	maxPoints      = 4 << 20 // path points over all shapes and clip paths
)

// ErrTooComplex is returned by Parse for documents beyond the limits on
// elements, nesting, entities or expanded use instances.
var ErrTooComplex = errors.New("svg: document too complex")

// attr is an attribute of an SVG element: its local name (xlink:href is
// stored as "xlink:href") and value.
type attr struct {
	name, value string
}

// element is an element of the source document in the SVG namespace (or
// in no namespace). Elements of other namespaces are dropped with their
// content. The tree is read-only after decoding, except for the cached
// declarations and style.
type element struct {
	tag      string
	attrs    []attr
	parent   *element
	children []*element

	decl   *decls    // parsed presentation attributes and style, see props.go
	styled *style    // computed style at the element's own place in the tree
	path   *gfx.Path // parsed d or points (they do not depend on the context)
}

// attr returns the value of the attribute name and whether it is present.
func (e *element) attr(name string) (string, bool) {
	for i := range e.attrs {
		if e.attrs[i].name == name {
			return e.attrs[i].value, true
		}
	}
	return "", false
}

// href returns the element's link: href, or xlink:href when href is absent
// (SVG 2: the unprefixed attribute wins).
func (e *element) href() (string, bool) {
	if v, ok := e.attr("href"); ok {
		return v, true
	}
	return e.attr("xlink:href")
}

// decode reads the XML document and returns its root element. Only the
// entities of the internal DTD subset (and XML's predefined ones) are
// expanded; their values are short (maxEntityValue), so that expansion
// cannot blow up.
func decode(data []byte) (*element, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	d.Entity = map[string]string{}
	var root *element
	var stack []*element
	skip := 0 // depth inside a dropped (foreign) element
	count := 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("svg: %w", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			if err := declareEntities(d.Entity, t); err != nil {
				return nil, err
			}
		case xml.StartElement:
			if len(stack)+skip >= maxDepth {
				return nil, ErrTooComplex
			}
			if skip > 0 || !(t.Name.Space == nsSVG || t.Name.Space == "") {
				if root == nil {
					return nil, errors.New("svg: the root element is not an svg element")
				}
				skip++
				continue
			}
			if count++; count > maxElements {
				return nil, ErrTooComplex
			}
			e := &element{tag: t.Name.Local}
			for _, a := range t.Attr {
				switch a.Name.Space {
				case "":
					e.attrs = append(e.attrs, attr{a.Name.Local, a.Value})
				case nsXLink, "xlink":
					if a.Name.Local == "href" {
						e.attrs = append(e.attrs, attr{"xlink:href", a.Value})
					}
				}
			}
			if root == nil {
				if e.tag != "svg" {
					return nil, fmt.Errorf("svg: the root element is <%s>, not <svg>", e.tag)
				}
				root = e
			} else if len(stack) > 0 {
				p := stack[len(stack)-1]
				e.parent = p
				p.children = append(p.children, e)
			}
			stack = append(stack, e)
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if root == nil {
		return nil, errors.New("svg: no svg element")
	}
	return root, nil
}

// declareEntities adds the general entities of an internal DTD subset
// (<!DOCTYPE svg [ <!ENTITY name "value"> ]>, as Adobe Illustrator writes
// them) to m. Parameter entities and external entities are ignored.
func declareEntities(m map[string]string, dir xml.Directive) error {
	s := string(dir)
	for {
		i := strings.Index(s, "<!ENTITY")
		if i < 0 {
			return nil
		}
		s = strings.TrimLeft(s[i+len("<!ENTITY"):], " \t\r\n")
		if strings.HasPrefix(s, "%") { // parameter entity
			continue
		}
		j := strings.IndexAny(s, " \t\r\n")
		if j <= 0 {
			return nil
		}
		name := s[:j]
		s = strings.TrimLeft(s[j:], " \t\r\n")
		if s == "" || (s[0] != '"' && s[0] != '\'') {
			continue // SYSTEM or PUBLIC: external, ignored
		}
		q := s[0]
		k := strings.IndexByte(s[1:], q)
		if k < 0 {
			return nil
		}
		value := s[1 : 1+k]
		s = s[2+k:]
		if len(m) >= maxEntities || len(value) > maxEntityValue {
			return ErrTooComplex
		}
		if _, ok := m[name]; !ok { // the first declaration is binding
			m[name] = value
		}
	}
}
