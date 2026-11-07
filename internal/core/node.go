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
	// nodeProp is a Prop turned into a node by Prop.Build: given to an
	// element it applies like the Prop; it cannot be rendered.
	nodeProp
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
	evMask  uint64 // the eventKinds of events (bit per kind)
	// disabled is the Disabled prop (input.go).
	disabled bool
	// draggable is the Draggable prop (dnd.go).
	draggable bool
	// winRegion is the custom title bar prop (WindowDrag, WindowNoDrag,
	// WindowControlProp; winregion.go), regionNone for none.
	winRegion uint8
	// Focus props (focus.go): TabIndex (hasTab false: the kind's
	// default), AutoFocus, an ElementRef and a focus scope.
	tabIndex  int32
	hasTab    bool
	autoFocus bool
	ref       *elemRef
	refSite   *callStack // where ref was attached (dev mode)
	scope     *scopeOpts
	src       image.Image // Img source (Src)
	img       *gfx.Image  // Img source (SrcImage)
	media     *mediaProps // SrcPath, SrcBytes, SrcFS, IconSrc and Tint (media.go)
	href      string      // A destination (Href)
	// aria holds the semantics props (aria.go), nil without any.
	aria *ariaProps
	// virt marks the nodes a VirtualList builds (virtual.go).
	virt *virtNode
	// fp holds the form props (field.go), nil without any; id is the Id
	// prop (form.go).
	fp *formProps
	id string
	// buttonDefault is the type a Button has when it was not given one
	// (DefaultButtonType): "" is HTML's "submit".
	buttonDefault string
	// exit are the Exit classes (presence.go).
	exit string
	// portal marks a Portal node (portal.go).
	portal bool
	// chrome marks the root of the fallback title bar (chrome.go).
	chrome bool
	// The engine props of package aria (ADR-096): a floating element
	// (floating.go), an overlay (overlay.go) and the Inert prop.
	float   *FloatOptions
	overlay *OverlayOptions
	inert   bool
	// animLayout is the AnimateLayout prop and layoutID the LayoutID
	// prop (layoutanim.go).
	animLayout bool
	layoutID   string
	// testID is the TestID prop (probe.go): a name for telatest to find
	// the element by, not exposed anywhere else.
	testID string
	// scheme is the Scheme prop (theme.go): the color scheme of the
	// element's subtree, ColorSchemeSystem for none.
	scheme ColorScheme
	// dir is the Dir prop (direction.go).
	dir dirValue

	// Primitives and fragments: the children as given. A zero node is a
	// hole: it renders nothing but keeps the positions of the siblings
	// after it, so that If(cond, A) does not remount the siblings when
	// cond changes.
	children []node

	// Text. textFn is a TextFn's function (text is then unused).
	text   string
	textFn func() string

	// Components.
	comp *compDesc

	// Props turned into nodes (nodeProp).
	prop *Prop

	// Fragments: dynamic marks a list made by Map or List, whose children
	// should carry keys (dev mode warns otherwise), and site is where it
	// was made (in dev mode), so the warnings come once per call site.
	dynamic bool
	site    *callStack
}

// compMode says when a component may skip its build (ARCHITECTURE
// section 7, automatic skipping).
type compMode uint8

const (
	// The zero value is Component (tela.C): it never skips because of
	// props, since the closure may capture changing values.
	_ compMode = iota
	// compProps is ComponentProps (tela.CP): it skips when the new props
	// are == to the old ones.
	compProps
	// compMemo is Memo (tela.Memo): it skips when equal(old, new).
	compMemo
	// compStruct is a struct component (a Node other than an internal
	// node or a Prop): it skips when the old and new values are equal
	// (componentEqual, which never panics).
	compStruct
)

// compDesc describes a component node, or a context provider.
type compDesc struct {
	// typ identifies the component function: the code pointer of the
	// function value. Two nodes of the same function are the same type,
	// so the Element and its hook state are kept between builds.
	// Providers have typ 0 and are identified by ctx. A struct
	// component's typ is the code pointer of its Build method, and its
	// identity is its dynamic type, rtype.
	typ uintptr
	// rtype is the dynamic type of a struct component (nil for the
	// other kinds), and ct what is known about that type.
	rtype reflect.Type
	ct    *compType
	// fn is the user's function, for its name.
	fn any
	// build calls the function (with its props, for ComponentProps); nil
	// for a struct component, whose build is the Build method of props
	// (run).
	build func() Node
	// props are the props of ComponentProps and Memo, compared with the
	// previous build's to skip it, and the value of a struct component.
	props any
	mode  compMode
	// equal compares the props of two Memo nodes of the same function.
	equal func(a, b any) bool

	// Providers (Context.Provider): the context, as a comparable key,
	// and the value provided.
	ctx   any
	value any

	// presence marks a Presence (presence.go): a component without a
	// function whose removed children exit before they unmount.
	presence bool

	// anon is set in dev mode when ComponentProps or Memo get an
	// anonymous function or a method value, whose captured variables
	// are not compared (reported once per call site at mount).
	anon *anonSite
}

// run calls the component's function and returns what it renders: the
// Build method of a struct component (called through the interface, so
// no method value is made per node), the closure otherwise.
func (c *compDesc) run() Node {
	if c.mode == compStruct {
		return c.props.(Node).Build()
	}
	return c.build()
}

// name returns the component's function name ("main.Counter"), or the
// type name of a struct component ("main.Card").

func (c *compDesc) name() string {
	if c.ctx != nil {
		return "Context.Provider"
	}
	if c.presence {
		return "Presence"
	}
	if c.ct != nil {
		return c.ct.name
	}
	if f := runtime.FuncForPC(c.typ); f != nil {
		return f.Name()
	}
	return "component"
}

// canSkip reports whether a component built from old may skip the build
// of nw, which has the same function: both are ComponentProps with ==
// props, or both are Memo and equal says their props are equal.
func canSkip(old, nw *compDesc) bool {
	if old.mode != nw.mode {
		return false
	}
	switch nw.mode {
	case compProps:
		return safeEqual(old.props, nw.props)
	case compMemo:
		return nw.equal(old.props, nw.props)
	case compStruct:
		return componentEqual(old.props, nw.props)
	}
	return false
}

// Node is what renders (tela.Node, and tela.Component, its alias; ADR-205):
// a value whose Build method returns what it renders. The constructors
// (Div, Text, Fragment, C, CP, Memo, Map...) return the internal node, an
// immutable, cheap description of UI (ARCHITECTURE sections 4 and 6) whose
// Build returns itself; any other value with a Build method, a struct
// component (ADR-088), is a Node too and becomes a component node where
// it is consumed (nodeOf): as a child, as the result of a build, as an
// item of a list, as a root. Nodes are recreated on every build; the
// Element tree keeps what persists.
//
// nil renders nothing. As a child it is a hole that keeps the positions
// of its siblings (ADR-044). The constructors never return a zero node in
// a Node, so n == nil is how a hole is told.
//
// Prop and StyleProp have the method set too, so a Prop is a Node to the
// compiler; where a node is consumed it panics with the fix. Code here
// must not compare two Nodes with ==: a struct component that holds a
// slice, a map or a func is not comparable, and == on two of them panics
// (componentEqual compares them without panicking).
type Node interface {
	Build() Node
}

// Child is what a primitive's constructor takes (tela.Child): a node (a
// child), a Prop (Class, Style, Key, an event handler, an image source)
// or a struct component, which becomes a component node. It has the
// method set of Node but is a type of its own, so that a field or a
// parameter says whether it takes props (Child) or only what renders
// (Node, tela.Component): the dev-mode check of ADR-088 looks for Props
// in the fields of type Node. A nil Child is a hole, like a nil Node.
type Child interface {
	Build() Node
}

// node is the internal form of a Node: one pointer to its immutable
// content, so that a node in a Node or a Child does not allocate and its
// identity is a pointer compare. The reconciler (flatten, reconcile,
// update, mount) works on it; the zero node is a hole. Only nodeOf and
// the constructors make one from a Node.
type node struct{ d *desc }

// Build returns n itself: a node renders as itself.
func (n node) Build() Node { return n }

// public returns n as a Node: nil for the zero node (a hole), so that a
// constructor never returns a non-nil Node that renders nothing.
func (n node) public() Node {
	if n.d == nil {
		return nil
	}
	return n
}

// String names n for messages and tests: <div>, "text", <main.Card>.
func (n node) String() string {
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
	case nodeProp:
		return "<prop " + propName(*d.prop) + ">"
	}
	return "<fragment>"
}

// Prop is a property of a primitive: its classes, typed style, key, event
// handlers or image source. Props and children may be mixed in any order;
// props apply in order (later classes win, as in a class string).
type Prop struct {
	name  string
	apply func(d *desc)
}

// Build returns a node that holds p: given to an element (or to a
// Fragment, a Provider or a Presence, which take Key only) it applies like
// p itself; where a node is consumed (nodeOf) it panics, since a prop
// renders nothing. It is what makes Prop a Child.
func (p Prop) Build() Node { return node{&desc{kind: nodeProp, prop: &p}} }

// propOf returns the prop c holds: a Prop, a StyleProp or a node made by
// Prop.Build.
func propOf(c Child) (Prop, bool) {
	switch c := c.(type) {
	case Prop:
		return c, true
	case StyleProp:
		if len(c.ops) == 0 {
			return Prop{}, true // the zero StyleProp of If(false, ...)
		}
		return Style(c.ops...), true
	case node:
		if c.d != nil && c.d.kind == nodeProp {
			return *c.d.prop, true
		}
	}
	return Prop{}, false
}

// PropOf returns the prop c holds, for tela.MergeProps: a Prop, a
// StyleProp (the zero one of an If that is off included) or the node
// made by Prop.Build; ok is false for anything else.
func PropOf(c Child) (Prop, bool) { return propOf(c) }

// addChild adds c to primitive d: a nil Child is a hole, a node is a
// child, a prop applies to d and a struct component becomes a component
// node.
func (d *desc) addChild(c Child) {
	if p, ok := propOf(c); ok {
		if p.apply != nil {
			p.apply(d)
		}
		return
	}
	d.children = append(d.children, childNode(c))
}

// childNode is toNode for a child that holds no prop (propOf reported
// false): nil is a hole, a node is itself and a struct component becomes
// a component node.
func childNode(c Child) node {
	switch c := c.(type) {
	case nil:
		return node{}
	case node:
		return c
	}
	return componentOf(c)
}

// addGroupChild adds c to d, a node without a box of its own (who:
// Fragment, Context.Provider, Presence), which takes Key as its only prop;
// fix ends the message for another prop.
func (d *desc) addGroupChild(c Child, who, fix string) {
	if p, ok := propOf(c); ok {
		if p.apply == nil {
			return // the zero Prop of If(false, ...)
		}
		if p.name != "Key" {
			panic(fmt.Sprintf("tela: %s does not take %s; %s", who, p.name, fix))
		}
		p.apply(d)
		return
	}
	d.children = append(d.children, childNode(c))
}

// NodeOf returns n as the node it renders: a node is itself, nil stays
// nil and a struct component becomes a component node. A Prop is not a
// node and panics. Every consumer converts its Node itself (Tree.Mount
// does); NodeOf is for a caller that wants the panic of a Prop root
// early, such as telatest, which turns it into an error.
func NodeOf(n Node) Node { return nodeOf(n, "the root is").public() }

// KeyOf returns the key of n and whether it has one, for tests and tools
// (a tela.Node has no Key method, ADR-205). n is converted as where it is
// consumed, so a struct component has no key and a Prop panics.
func KeyOf(n Node) (any, bool) {
	if d := nodeOf(n, "the node of KeyOf is").d; d != nil {
		return d.key, d.keyed
	}
	return nil, false
}

// asNode is the fast path of toNode: n is an internal node (an element, a
// text, a fragment, the node of C, CP, Memo or Map, or the zero node, a
// hole) and not a Prop's.
func asNode(n Node) (node, bool) {
	c, ok := n.(node)
	return c, ok && (c.d == nil || c.d.kind != nodeProp)
}

// toNode converts n where a Node is consumed (ADR-205): nil and the zero
// node are a hole, a node is itself and a struct component becomes a
// component node. A Prop (or the node of one) panics; where, called only
// then, names the place and ends with its verb ("the root is", "item 3 of
// a Map is", "Card.Build returned").
func toNode(n Node, where func() string) node {
	if c, ok := asNode(n); ok {
		return c
	}
	if p, ok := propOf(n); ok {
		panic(propNodeMessage(where(), p))
	}
	return childNode(n)
}

// nodeOf is toNode where what names the place.
func nodeOf(n Node, what string) node {
	return toNode(n, func() string { return what })
}

// propNodeMessage is the panic of a Prop where a node is consumed; what
// ends with its verb ("the root is", "Card.Build returned").
func propNodeMessage(what string, p Prop) string {
	n := propName(p)
	return fmt.Sprintf("tela: %s a Prop (%s), not a node or a component; props go to elements, such as Div(%s(...), ...) (a Prop compiles where a tela.Node is expected because it has a Build method)", what, n, n)
}

// nodesOf converts the items of a list with itemOf; who names the list
// in the panic of a Prop ("a list", "h.Nodes").
func nodesOf(nodes []Node, who string) []node {
	out := make([]node, len(nodes))
	for i, n := range nodes {
		out[i] = itemOf(n, i, who)
	}
	return out
}

// itemOf is toNode for item i of the list that who makes ("Map"): the
// message is built only for a Prop.
func itemOf(n Node, i int, who string) node {
	return toNode(n, func() string { return fmt.Sprintf("item %d of %s is", i, who) })
}

// propName names a prop for messages.
func propName(p Prop) string {
	if p.name == "" {
		return "the zero Prop"
	}
	return p.name
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
// number, a struct of those). h.Key and tela.WithKey are generic over
// comparable keys, so a slice or a map does not compile there; an
// interface holding one still panics here.
func Key(k any) Prop {
	checkKey(k)
	return Prop{name: "Key", apply: func(d *desc) { d.key, d.keyed = k, true }}
}

// Src sets the image of an Img primitive. The image is converted to a
// gfx.Image when it is first drawn and again only when another image is
// given; pass the same image.Image value on every build.
func Src(img image.Image) Prop {
	return Prop{name: "Src", apply: func(d *desc) {
		d.src, d.img = img, nil
		if d.media != nil {
			d.media.src = srcSpec{} // the last source wins (media.go)
		}
	}}
}

// SrcImage sets the image of an Img primitive from an already converted
// gfx.Image.
func SrcImage(img *gfx.Image) Prop {
	return Prop{name: "SrcImage", apply: func(d *desc) {
		d.img, d.src = img, nil
		if d.media != nil {
			d.media.src = srcSpec{}
		}
	}}
}

// Href sets the destination of an A primitive. It is stored (for the
// semantics tree); Tela does not open links itself.
func Href(url string) Prop {
	return Prop{name: "Href", apply: func(d *desc) { d.href = url }}
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
		d.children = make([]node, 0, len(children))
	}
	for _, c := range children {
		d.addChild(c)
	}
	if kind != style.KindImg && (d.src != nil || d.img != nil) {
		panic(fmt.Sprintf("core: Src on <%v>: only Img takes an image source", kind))
	}
	checkMediaProps(kind, d)
	if kind == style.KindSVG {
		// Icons are decorative unless they are named (lucide-react's
		// aria-hidden="true"): with AriaLabel or AriaLabelledBy they are
		// images with that name.
		if a := d.aria; a == nil || !a.hiddenSet && !a.hasLabel && len(a.labelledBy) == 0 {
			a = d.ariaProps()
			a.hidden, a.hiddenSet = true, true
		}
	}
	if kind != style.KindA && d.href != "" {
		panic(fmt.Sprintf("core: Href on <%v>: only A takes a destination", kind))
	}
	if kind != style.KindImg && d.aria != nil && d.aria.hasAlt {
		panic(fmt.Sprintf("core: Alt on <%v>: only Img takes an alternative text; name other elements with AriaLabel", kind))
	}
	if d.fp != nil {
		checkFormProps(kind, d)
	}
	if kind.IsTextField() {
		for _, c := range d.children {
			if c.d != nil {
				panic(fmt.Sprintf("tela: <%v> takes no children; give its text with Value, DefaultValue or BindValue, and its label with a Label", kind))
			}
		}
	}
	return node{d}
}

// Text returns a text node. Text is laid out in the paragraph of its
// nearest block-level ancestor, styled by its nearest primitive ancestor
// (ARCHITECTURE section 11).
func Text(s string) Node { return node{&desc{kind: nodeText, text: s}} }

// Textf returns a text node of fmt.Sprintf(format, args...).
func Textf(format string, args ...any) Node { return Text(fmt.Sprintf(format, args...)) }

// Component returns a node that builds fn: a component boundary with its
// own Element and hook state (ARCHITECTURE section 7, tela.C). Calling fn
// directly instead inlines it into the caller, whose hooks it then uses.
//
// Its identity is the function (its code pointer): closures of the same
// function literal are the same component. It is built whenever its
// parent builds, because fn may capture values that changed; only the
// very same node (an identical *desc, such as a node kept in a variable
// or a UseMemo) is not built again.
//
// key gives the node a key (the h.Key prop, the only prop accepted).
func Component(fn func() Node, key ...Child) Node {
	if fn == nil {
		panic("tela: C(nil): pass a component function, such as tela.C(Counter)")
	}
	return keyedComponent(&desc{kind: nodeComponent, comp: &compDesc{
		typ: reflect.ValueOf(fn).Pointer(), fn: fn, build: fn,
	}}, "C", key)
}

// keyedComponent gives the new component node d the keys of key, which
// are h.Key props: the optional last argument of C, CP and Memo, and
// returns its node. It takes Children so that If(cond, Key(id)) fits; a
// nil Child and the zero Prop (the absent If) are skipped.
func keyedComponent(d *desc, who string, key []Child) Node {
	for _, c := range key {
		if c == nil {
			continue
		}
		p, ok := propOf(c)
		if !ok {
			panic(fmt.Sprintf("tela: %s takes only Key as its last argument, not a node or a component; the element it builds takes classes and children, for example tela.%s(..., Key(id))", who, who))
		}
		if p.apply == nil {
			continue // the zero Prop of If(false, Key(...))
		}
		if p.name != "Key" {
			panic(fmt.Sprintf("tela: %s takes only Key as its last argument, not %s; classes, styles and handlers go on the elements the component builds, for example tela.%s(..., Key(id))", who, propName(p), who))
		}
		p.apply(d)
	}
	return node{d}
}

// ComponentProps returns a node that builds fn(props): a component with
// props (tela.CP). Its identity is fn, whatever the props. When the
// parent builds again and the new props are == to the old ones, and
// neither the component's state nor a context it reads changed, its
// build is skipped (Jetpack Compose's automatic skipping); a props value
// whose == panics (an interface holding a func, a slice or a map)
// counts as changed.
//
// Variables captured by fn are not compared: in dev mode, an anonymous
// function or a method value is reported once per call site.
func ComponentProps[P comparable](fn func(P) Node, props P, key ...Child) Node {
	if fn == nil {
		panic("tela: CP(nil, ...): pass a component function, such as tela.CP(Card, CardProps{...})")
	}
	c := &compDesc{
		typ: reflect.ValueOf(fn).Pointer(), fn: fn,
		build: func() Node { return fn(props) }, props: props, mode: compProps,
	}
	if devNow() {
		c.anon = noteAnon(c.typ, "CP")
	}
	return keyedComponent(&desc{kind: nodeComponent, comp: c}, "CP", key)
}

// Memo returns a node that builds fn(props), like ComponentProps, but
// skips the build when equal(old, new) reports the props equal (tela.Memo):
// for props that are not comparable, or that compare in another way.
// equal is the new node's; it must be pure.
func Memo[P any](fn func(P) Node, props P, equal func(a, b P) bool, key ...Child) Node {
	if fn == nil {
		panic("tela: Memo(nil, ...): pass a component function, such as tela.Memo(Chart, props, sameSeries)")
	}
	if equal == nil {
		panic("tela: Memo(fn, props, nil): pass the function that compares two props values, or use tela.CP for comparable props")
	}
	c := &compDesc{
		typ: reflect.ValueOf(fn).Pointer(), fn: fn,
		build: func() Node { return fn(props) }, props: props, mode: compMemo,
		equal: func(a, b any) bool {
			pa, _ := a.(P)
			pb, _ := b.(P)
			return equal(pa, pb)
		},
	}
	if devNow() {
		c.anon = noteAnon(c.typ, "Memo")
	}
	return keyedComponent(&desc{kind: nodeComponent, comp: c}, "Memo", key)
}

// WithKey returns a copy of the node of c with key k (see Key). It works
// for every kind of node, components and fragments included, and for
// struct components (tela.WithKey(Card{...}, id)). A nil c stays nil; a
// Prop panics.
func WithKey(c Node, k any) Node {
	checkKey(k)
	n := nodeOf(c, "the first argument of WithKey is")
	if n.d == nil {
		return nil
	}
	return withKey(n, k)
}

// KeyedItem is WithKey for item i of the list that who makes (h.MapKeyed,
// h.Range): a Prop panics naming the item.
func KeyedItem(c Node, k any, i int, who string) Node {
	checkKey(k)
	n := itemOf(c, i, who)
	if n.d == nil {
		return nil
	}
	return withKey(n, k)
}

// withKey returns a copy of n, which is not a hole, with key k.
func withKey(n node, k any) node {
	d := *n.d
	d.key, d.keyed = k, true
	return node{&d}
}

// Fragment groups nodes without a box of their own; they become children
// of the fragment's parent. Only Key is accepted as a prop.
func Fragment(children ...Child) Node {
	d := &desc{kind: nodeFragment, children: make([]node, 0, len(children))}
	for _, c := range children {
		d.addGroupChild(c, "Fragment", "a fragment has no box (wrap the children in a Div)")
	}
	return node{d}
}

// List returns a fragment of a dynamic list of nodes: children that come
// from data and may be inserted, removed or reordered. They should carry
// keys; dev mode warns when two or more do not, or when two share one,
// once per call site. A nil item is a hole, a struct component becomes a
// component node and a Prop panics.
func List(nodes []Node) Node { return dynamicList(nodesOf(nodes, "a list")) }

// dynamicList is List of nodes already converted.
func dynamicList(nodes []node) Node {
	d := &desc{kind: nodeFragment, children: nodes, dynamic: true}
	if devNow() {
		d.site = new(callStack)
		d.site.capture(0) // from the caller of dynamicList: List, Map or MapIndex
	}
	return node{d}
}

// NodeList returns a fragment of nodes without a box of its own: the
// children of h.Nodes. Unlike List it is not dynamic, so dev mode does not
// ask for keys: it is the slice form of Fragment(a, b, c).
func NodeList(nodes []Node) Node {
	return node{&desc{kind: nodeFragment, children: nodesOf(nodes, "h.Nodes")}}
}

// Map returns the List of f applied to every item. f may return any Node:
// nil is a hole, a struct component becomes a component node, and a Prop
// panics.
func Map[T any](items []T, f func(T) Node) Node {
	nodes := make([]node, len(items))
	for i, it := range items {
		nodes[i] = itemOf(f(it), i, "Map")
	}
	return dynamicList(nodes)
}

// MapIndex returns the List of f applied to every item and its index.
func MapIndex[T any](items []T, f func(i int, item T) Node) Node {
	nodes := make([]node, len(items))
	for i, it := range items {
		nodes[i] = itemOf(f(i, it), i, "MapIndex")
	}
	return dynamicList(nodes)
}

// If returns n when cond holds, else nil (a hole that keeps the positions
// of the siblings).
func If(cond bool, n Node) Node {
	if cond {
		return n
	}
	return nil
}

// describe returns a short description of a primitive for messages:
// `<div class="p-4">`.
func (d *desc) describe() string {
	if d.portal {
		return "<Portal>"
	}
	if len(d.classes) == 0 {
		return "<" + d.elem.String() + ">"
	}
	return fmt.Sprintf("<%s class=%q>", d.elem, strings.Join(d.classes, " "))
}
