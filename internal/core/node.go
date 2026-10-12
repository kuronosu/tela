package core

import (
	"fmt"
	"image"
	"reflect"
	"runtime"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// nodeKind is the kind of a Node.
type nodeKind uint8

const (
	// nodeElement is a primitive (Div, Span, H1, Button, Img...): it
	// creates an Element that owns a Box.
	nodeElement nodeKind = iota + 1
	// nodeText is a string of text, laid out in the paragraph of its
	// nearest block-level ancestor.
	nodeText
	// nodeComponent is a function that builds other Nodes.
	nodeComponent
	// nodeFragment is a list of Nodes without a box of its own.
	nodeFragment
)

// desc is the immutable content of a Node. Nodes are built by the
// constructors and never modified afterwards: a Node may be shared and
// reused between builds.
type desc struct {
	kind nodeKind

	// key identifies the node among its siblings (Key prop, WithKey).
	key   any
	keyed bool

	// Primitives.
	elem    style.Kind
	classes []string   // Class props, in order of appearance
	props   []style.Op // Style props (typed, applied after the classes)
	events  []EventProp
	src     image.Image // Img source (Src)
	img     *gfx.Image  // Img source (SrcImage)

	// Primitives and fragments: the children as given. A zero Node is a
	// hole: it renders nothing but keeps the positions of the siblings
	// after it, so that If(cond, A) does not remount the siblings when
	// cond changes.
	children []Node

	// Text.
	text string

	// Components.
	comp *compDesc

	// Fragments: dynamic marks a list made by Map or List, whose children
	// should carry keys (dev mode warns otherwise).
	dynamic bool
}

// compDesc describes a component node.
type compDesc struct {
	// typ identifies the component function: the code pointer of the
	// function value. Two nodes of the same function are the same type,
	// so the Element and its hook state are kept between builds.
	typ uintptr
	// fn is the user's function, for its name.
	fn any
	// build calls the function (with its props, for ComponentProps).
	build func() Node
	// props are the props of ComponentProps, kept for phase 4's skipping
	// of builds with equal props.
	props any
}

// name returns the component's function name ("main.Counter").
func (c *compDesc) name() string {
	if f := runtime.FuncForPC(c.typ); f != nil {
		return f.Name()
	}
	return "component"
}

// Node is an immutable, cheap description of UI (ARCHITECTURE sections 4
// and 6): a primitive element, a text, a component or a fragment. Nodes
// are recreated on every build; the Element tree keeps what persists.
//
// The zero Node renders nothing. As a child it is a hole that keeps the
// positions of its siblings (see Element).
type Node struct{ d *desc }

// IsZero reports whether n is the zero Node, which renders nothing.
func (n Node) IsZero() bool { return n.d == nil }

// Key returns the node's key and whether it has one.
func (n Node) Key() (any, bool) {
	if n.d == nil {
		return nil, false
	}
	return n.d.key, n.d.keyed
}

func (n Node) String() string {
	d := n.d
	if d == nil {
		return "<nil>"
	}
	switch d.kind {
	case nodeElement:
		return "<" + d.elem.String() + ">"
	case nodeText:
		return fmt.Sprintf("%q", d.text)
	case nodeComponent:
		return "<" + d.comp.name() + ">"
	}
	return "<fragment>"
}

// Child is what a primitive's constructor takes: a Node (a child) or a
// Prop (Class, Style, Key, an event handler, an image source). The
// interface is sealed: only Node and Prop implement it, so passing
// anything else is a compile error. A nil Child is a hole, like the zero
// Node.
type Child interface {
	applyChild(d *desc)
}

func (n Node) applyChild(d *desc) { d.children = append(d.children, n) }

// Prop is a property of a primitive: its classes, typed style, key, event
// handlers or image source. Props and children may be mixed in any order;
// props apply in order (later classes win, as in a class string).
type Prop struct {
	name  string
	apply func(d *desc)
}

func (p Prop) applyChild(d *desc) {
	if p.apply != nil {
		p.apply(d)
	}
}

// EventProp is an event handler prop, such as OnClick. In phase 3 handlers
// are stored on the Element but not dispatched (ADR-040).
type EventProp struct {
	// Name is the event name ("click").
	Name string
	// Handler is the handler function as the public API typed it.
	Handler any
}

// Class adds Tailwind classes to a primitive (ARCHITECTURE section 10).
// Class may repeat; the strings concatenate in order.
func Class(classes string) Prop {
	return Prop{name: "Class", apply: func(d *desc) {
		if classes != "" {
			d.classes = append(d.classes, classes)
		}
	}}
}

// Style adds typed style props, applied after the classes (last wins).
func Style(ops ...style.Op) Prop {
	return Prop{name: "Style", apply: func(d *desc) { d.props = append(d.props, ops...) }}
}

// Key gives a node an identity among its siblings: reconciliation keeps
// the Element (and its state) of the sibling with the same key and type,
// wherever it moves. k must be a non-nil comparable value (a string, a
// number, a struct of those).
func Key(k any) Prop {
	checkKey(k)
	return Prop{name: "Key", apply: func(d *desc) { d.key, d.keyed = k, true }}
}

// On adds an event handler prop. The public API wraps it (OnClick...).
func On(event string, handler any) Prop {
	return Prop{name: "On", apply: func(d *desc) {
		d.events = append(d.events, EventProp{Name: event, Handler: handler})
	}}
}

// Src sets the image of an Img primitive. The image is converted to a
// gfx.Image when it is first drawn and again only when another image is
// given; pass the same image.Image value on every build.
func Src(img image.Image) Prop {
	return Prop{name: "Src", apply: func(d *desc) { d.src, d.img = img, nil }}
}

// SrcImage sets the image of an Img primitive from an already converted
// gfx.Image.
func SrcImage(img *gfx.Image) Prop {
	return Prop{name: "SrcImage", apply: func(d *desc) { d.img, d.src = img, nil }}
}

func checkKey(k any) {
	if k == nil {
		panic("core: Key(nil): a key must be a non-nil comparable value, such as a string or an int ID")
	}
	if t := reflect.TypeOf(k); !t.Comparable() {
		panic(fmt.Sprintf("core: Key(%s): keys must be comparable (strings, numbers, or structs of those); use the item's ID", t))
	}
}

// Primitive returns a primitive node of the given kind (Div, Span, H1,
// Button, Img...) with its props and children. Text and Root are not
// primitives that users create: Text has its own constructor, and the
// root is created by the Tree.
func Primitive(kind style.Kind, children ...Child) Node {
	if kind == style.KindText || kind == style.KindRoot || kind.String() == "?" {
		panic(fmt.Sprintf("core: Primitive(%v): not a primitive kind; use Text for text", kind))
	}
	d := &desc{kind: nodeElement, elem: kind}
	if len(children) > 0 {
		d.children = make([]Node, 0, len(children))
	}
	for _, c := range children {
		if c == nil {
			d.children = append(d.children, Node{})
			continue
		}
		c.applyChild(d)
	}
	if kind != style.KindImg && (d.src != nil || d.img != nil) {
		panic(fmt.Sprintf("core: Src on <%v>: only Img takes an image source", kind))
	}
	return Node{d}
}

// Text returns a text node. Text is laid out in the paragraph of its
// nearest block-level ancestor, styled by its nearest primitive ancestor
// (ARCHITECTURE section 11).
func Text(s string) Node { return Node{&desc{kind: nodeText, text: s}} }

// Textf returns a text node of fmt.Sprintf(format, args...).
func Textf(format string, args ...any) Node { return Text(fmt.Sprintf(format, args...)) }

// Component returns a node that builds fn: a component with its own
// Element and hook state (ARCHITECTURE section 7). Calling fn directly
// instead inlines it into the caller, whose hooks it then uses.
func Component(fn func() Node) Node {
	if fn == nil {
		panic("core: Component(nil): pass a component function")
	}
	return Node{&desc{kind: nodeComponent, comp: &compDesc{
		typ: reflect.ValueOf(fn).Pointer(), fn: fn, build: fn,
	}}}
}

// ComponentProps returns a node that builds fn(props): a component with
// props. Its identity is fn, whatever the props.
func ComponentProps[P any](fn func(P) Node, props P) Node {
	if fn == nil {
		panic("core: ComponentProps(nil, ...): pass a component function")
	}
	return Node{&desc{kind: nodeComponent, comp: &compDesc{
		typ: reflect.ValueOf(fn).Pointer(), fn: fn,
		build: func() Node { return fn(props) }, props: props,
	}}}
}

// WithKey returns a copy of n with key k (see Key). It works for every
// kind of node, components and fragments included. The zero Node stays
// zero.
func WithKey(n Node, k any) Node {
	checkKey(k)
	if n.d == nil {
		return n
	}
	c := *n.d
	c.key, c.keyed = k, true
	return Node{&c}
}

// Fragment groups nodes without a box of their own; they become children
// of the fragment's parent. Only Key is accepted as a prop.
func Fragment(children ...Child) Node {
	d := &desc{kind: nodeFragment, children: make([]Node, 0, len(children))}
	for _, c := range children {
		switch c := c.(type) {
		case nil:
			d.children = append(d.children, Node{})
		case Node:
			d.children = append(d.children, c)
		case Prop:
			if c.name != "Key" {
				panic(fmt.Sprintf("core: Fragment does not take %s; a fragment has no box (wrap the children in a Div)", c.name))
			}
			c.apply(d)
		}
	}
	return Node{d}
}

// List returns a fragment of a dynamic list of nodes: children that come
// from data and may be inserted, removed or reordered. They should carry
// keys; dev mode warns when two or more do not.
func List(nodes []Node) Node {
	return Node{&desc{kind: nodeFragment, children: nodes, dynamic: true}}
}

// Map returns the List of f applied to every item.
func Map[T any](items []T, f func(T) Node) Node {
	nodes := make([]Node, len(items))
	for i, it := range items {
		nodes[i] = f(it)
	}
	return List(nodes)
}

// If returns n when cond holds, else the zero Node (a hole that keeps the
// positions of the siblings).
func If(cond bool, n Node) Node {
	if cond {
		return n
	}
	return Node{}
}

// describe returns a short description of a primitive for messages:
// `<div class="p-4">`.
func (d *desc) describe() string {
	if len(d.classes) == 0 {
		return "<" + d.elem.String() + ">"
	}
	return fmt.Sprintf("<%s class=%q>", d.elem, strings.Join(d.classes, " "))
}
