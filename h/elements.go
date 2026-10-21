package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/style"
)

// Div returns a block container, the general-purpose box. Its children
// stack vertically unless its classes say otherwise (flex).
func Div(children ...tela.Child) tela.Node { return core.Primitive(style.KindDiv, children...) }

// Span returns an inline container. Inside a paragraph of text it styles a
// run of that text (color, weight...) and is a CSS inline box: its
// background, border, rounded corners and horizontal padding and margins
// apply on every line it is on (Class("rounded bg-slate-100 px-1")). With
// Class("inline-block") or "inline-flex" it is laid out as a box that sits
// on the line. As a child of a flex container it is a flex item.
func Span(children ...tela.Child) tela.Node { return core.Primitive(style.KindSpan, children...) }

// Text returns a text node. The text is laid out in the paragraph of its
// nearest block ancestor and styled by its parent element.
func Text(s string) tela.Node { return core.Text(s) }

// Textf returns a text node formatted like fmt.Sprintf.
func Textf(format string, args ...any) tela.Node { return core.Textf(format, args...) }

// H1 returns a level 1 heading. Headings are unstyled: size and weight
// come from classes (text-5xl font-bold).
func H1(children ...tela.Child) tela.Node { return core.Primitive(style.KindH1, children...) }

// H2 returns a level 2 heading. See H1.
func H2(children ...tela.Child) tela.Node { return core.Primitive(style.KindH2, children...) }

// H3 returns a level 3 heading. See H1.
func H3(children ...tela.Child) tela.Node { return core.Primitive(style.KindH3, children...) }

// H4 returns a level 4 heading. See H1.
func H4(children ...tela.Child) tela.Node { return core.Primitive(style.KindH4, children...) }

// H5 returns a level 5 heading. See H1.
func H5(children ...tela.Child) tela.Node { return core.Primitive(style.KindH5, children...) }

// H6 returns a level 6 heading. See H1.
func H6(children ...tela.Child) tela.Node { return core.Primitive(style.KindH6, children...) }

// P returns a paragraph of text and inline elements (Span, Strong, Em,
// Code, A).
func P(children ...tela.Child) tela.Node { return core.Primitive(style.KindP, children...) }

// Button returns a button. It has no background, border or padding of its
// own (Tailwind's Preflight): style it with classes. Its width fits its
// content, it sits on the line of the text around it (inline-block, on
// the baseline; Class("align-middle") moves it), and it shows the pointer
// cursor. Clicks are handled with OnClick.
func Button(children ...tela.Child) tela.Node { return core.Primitive(style.KindButton, children...) }

// Img returns an image. Give it a source with Src (a decoded image),
// SrcPath (a file of the file systems registered with tela.Assets),
// SrcBytes or SrcFS; PNG, JPEG, GIF, WebP and SVG are recognized by their
// content, and an SVG stays vector. Without Class or Style sizes it is
// drawn at the size of the image, shrunk to fit its container; with one
// size (h-8) the other follows the image's aspect ratio, as for an HTML
// img. object-contain, object-cover and the other object-* classes fit it
// in its box, and Tint paints it in one color.
func Img(children ...tela.Child) tela.Node { return core.Primitive(style.KindImg, children...) }

// Icon returns an SVG icon (tela.Icon), drawn as a vector at any size and
// scale. It is 1em by 1em (the element's font size, like an icon font)
// unless a size-*, w-* or h-* class or a Style sizes it, and a block, like
// an svg under Tailwind's Preflight (Class("inline") puts it on a line of
// text; in a flex row it is an item). Its currentColor is the text color,
// so text-* classes, theme tokens, TextColorFn and transitions recolor it
// without parsing it again; fill-*, stroke-* and stroke-[1.5] classes, on
// it or inherited from an ancestor (a Button's, as shadcn's [&_svg]
// classes do), override the document's own fill, stroke and stroke-width.
//
//	Button(Class("inline-flex items-center gap-2 text-sm"),
//	    Icon(lucide.Check, Class("size-4 stroke-[1.5]")), Text("Save"))
//
// An icon is decorative: hidden from accessibility unless it has an
// AriaLabel or AriaLabelledBy, which make it an image with that name. An
// SVG that cannot be parsed renders nothing and is reported once in the
// log.
func Icon(icon tela.Icon, props ...tela.Child) tela.Node {
	children := make([]tela.Child, 0, len(props)+1)
	children = append(children, core.IconSrc(icon.SVG))
	return core.Primitive(style.KindSVG, append(children, props...)...)
}

// Strong returns bold inline text.
func Strong(children ...tela.Child) tela.Node { return core.Primitive(style.KindStrong, children...) }

// Em returns italic inline text.
func Em(children ...tela.Child) tela.Node { return core.Primitive(style.KindEm, children...) }

// Code returns inline text in the monospace font.
func Code(children ...tela.Child) tela.Node { return core.Primitive(style.KindCode, children...) }

// A returns a link: inline text with a destination given by Href. Links are
// unstyled (no color or underline), like Tailwind's Preflight; style them
// with classes.
func A(children ...tela.Child) tela.Node { return core.Primitive(style.KindA, children...) }
