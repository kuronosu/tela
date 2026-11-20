// Portions of this file follow goldmark's HTML renderer (renderer/html), which
// decodes the escapes and entities of a text the same way, Copyright (c) 2019
// Yusuke Inuzuka, licensed under the MIT License, and Tailwind CSS
// Typography's "prose" defaults, Copyright (c) Tailwind Labs, Inc., licensed
// under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"bufio"
	"bytes"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// MarkdownStyle is the recipe of [Markdown]: the look of a document, as
// Tailwind Typography's prose scaled to an app, in the theme's tokens, so
// that it follows the color scheme and the kit's style.
type MarkdownStyle struct {
	// Root is the box of the whole document: the base text size, line
	// height and color that every block inherits.
	Root string
	// Paragraph is a paragraph and a block of text in a list item. Its
	// bottom margin is what makes copied text have a blank line between
	// paragraphs (ADR-271: at least half the font size), and
	// whitespace-pre-line is what makes a hard line break a break.
	Paragraph string
	// H1 to H6 are the headings, with the margins that set them off.
	H1, H2, H3, H4, H5, H6 string
	// Blockquote is a quotation; its blocks are inside it.
	Blockquote string
	// List is an ordered or unordered list (a column of items).
	List string
	// Item is one item: the marker and the body in a row.
	Item string
	// ItemBody holds the blocks of an item.
	ItemBody string
	// Tight and Loose are added to the blocks of an item of a tight list
	// (no blank line between its items) and of a loose one, to set the
	// space between them (the block's own bottom margin comes first).
	Tight, Loose string
	// Marker is the marker of an item: the bullet's box and the number.
	Marker string
	// BulletDisc, BulletCircle and BulletSquare are the shapes of the
	// bullets of a list, by depth: the first level, the second, and the
	// others.
	BulletDisc, BulletCircle, BulletSquare string
	// Number is the number of an ordered item, after Marker.
	Number string
	// Task is the checkbox of a task-list item. A task whose list has no
	// OnToggleTask is read-only: TaskReadOnly is added to it.
	Task, TaskReadOnly string
	// Code is a code span; CodeBlock the box of a code block (it
	// scrolls sideways) and CodeBlockCode its text, in monospace with the
	// whitespace kept.
	Code, CodeBlock, CodeBlockCode string
	// Link is a link; Strikethrough is struck text.
	Link, Strikethrough string
	// Rule is a thematic break.
	Rule string
	// Image is an image of the app's assets; ImageAlt is the alt text that
	// takes the place of any other image.
	Image, ImageAlt string
	// Table is the box of a table (it scrolls sideways) and Grid the table
	// itself, whose column tracks the component adds; Head is a header
	// cell, Cell a body cell and LastRow is added to the cells of the last
	// row.
	Table, Grid, Head, Cell, LastRow string
	// HTML is a raw HTML block, shown as the text it is.
	HTML string
}

func init() {
	defaultStyle.Markdown = MarkdownStyle{
		Root:      "min-w-0 text-sm leading-6 break-words text-foreground",
		Paragraph: "mb-4 whitespace-pre-line last:mb-0",
		H1:        "mt-6 mb-4 text-2xl leading-tight font-semibold tracking-tight first:mt-0 last:mb-0",
		H2:        "mt-6 mb-3 border-b border-border pb-2 text-xl leading-tight font-semibold tracking-tight first:mt-0 last:mb-0",
		H3:        "mt-5 mb-2 text-lg leading-snug font-semibold tracking-tight first:mt-0 last:mb-0",
		H4:        "mt-4 mb-2 text-base font-semibold first:mt-0 last:mb-0",
		H5:        "mt-4 mb-1.5 text-sm font-semibold first:mt-0 last:mb-0",
		H6:        "mt-4 mb-1.5 text-sm font-semibold text-muted-foreground first:mt-0 last:mb-0",

		Blockquote: "mb-4 border-s-[3px] border-border ps-4 text-muted-foreground italic last:mb-0",
		List:       "mb-4 flex flex-col gap-1.5 last:mb-0",
		Item:       "flex items-start gap-2",
		ItemBody:   "flex min-w-0 flex-1 flex-col",
		Tight:      "mb-1.5 last:mb-0",
		Loose:      "mb-3 last:mb-0",
		Marker:     "shrink-0 text-muted-foreground select-none",
		// A bullet is 6 px, centered on the first line (24 px).
		BulletDisc:   "mt-[9px] size-1.5 rounded-full bg-muted-foreground",
		BulletCircle: "mt-[9px] size-1.5 rounded-full border border-muted-foreground",
		BulletSquare: "mt-[9px] size-1.5 bg-muted-foreground",
		Number:       "text-end tabular-nums",
		Task:         "mt-1",
		TaskReadOnly: "disabled:cursor-default disabled:opacity-100",

		Code:          "rounded bg-muted px-[0.3em] py-[0.15em] font-mono text-[0.875em] font-medium",
		CodeBlock:     "mb-4 w-full overflow-x-auto rounded-lg border border-border bg-muted/50 last:mb-0",
		CodeBlockCode: "block w-max min-w-full p-4 font-mono text-[0.8125rem] leading-relaxed whitespace-pre",

		Link:          "rounded-sm font-medium text-primary underline underline-offset-4 outline-none hover:text-primary/80 focus-visible:ring-[3px] focus-visible:ring-ring/50",
		Strikethrough: "line-through",
		Rule:          "mt-2 mb-6 h-px w-full shrink-0 bg-border",
		Image:         "inline-block max-w-full rounded-md align-middle",
		ImageAlt:      "text-muted-foreground italic",

		Table:   "mb-4 w-full overflow-x-auto rounded-lg border border-border last:mb-0",
		Grid:    "grid w-full",
		Head:    "border-b border-border bg-muted/50 px-3 py-2 font-medium",
		Cell:    "border-b border-border px-3 py-2",
		LastRow: "border-b-0",
		HTML:    "mb-4 font-mono text-[0.8125rem] whitespace-pre-wrap last:mb-0",
	}
}

// Markdown renders CommonMark with GitHub's extensions (tables, task lists,
// strikethrough and autolinks) as Tela elements, in the look of Tailwind
// Typography's prose at the size of an app, in the theme's tokens. The
// text is selectable as any static text is, and a selection that crosses
// blocks copies a blank line between paragraphs and one line break between
// list items and table cells.
//
//	kit.Markdown{Source: "# Notes\n\nSome **bold** text and a [link](https://tela.dev).\n\n- [x] Done\n- [ ] To do\n"}
//
// The source is parsed once each time Source changes; a rebuild with the
// same Source builds the elements again (it is proportional to the size of
// the document) but parses nothing.
//
// What a document draws:
//
//   - headings as H1 to H6, with their level for assistive technologies;
//     paragraphs; emphasis, strong text and strikethrough; code spans and
//     fenced or indented code blocks in monospace with the whitespace kept
//     (no syntax highlighting; a block scrolls sideways instead of
//     wrapping); blockquotes; thematic breaks; hard and soft line breaks;
//   - ordered lists, which start at their first number, unordered lists,
//     nested at any depth; a task-list item is a checkbox, read-only
//     unless OnToggleTask is set;
//   - tables, with the alignment of their columns;
//   - links (an A with Href) and autolinks, activated by a press or by
//     Enter: OnLink is called with the destination, or without it the
//     destination is opened with [tela.OpenURL];
//   - images, only from the files of [tela.Assets]: a destination such as
//     "img/logo.png" is loaded from there, and Tela fetches nothing over
//     the network, so any other destination (a URL with a scheme, a
//     "data:" URL) shows the image's alt text instead;
//   - raw HTML, a block or inline, as the text it is. Entities and
//     backslash escapes are decoded.
//
// A document that comes from outside the app is safe to show: nothing in it
// runs, loads or opens until the user follows a link, and the app decides
// how with OnLink.
type Markdown struct {
	// Source is the document, in Markdown.
	Source string
	// OnLink is called with the destination of a link when it is pressed
	// (the URL as an HTML renderer would write it: percent-encoded). Nil
	// opens it with [tela.OpenURL]; a destination that has no scheme (a
	// relative link, a "#heading") then does nothing. Use it to route the
	// links your app understands (a [router]'s paths) and to ask before
	// leaving for a web page.
	OnLink func(href string)
	// OnToggleTask is called when the user checks or unchecks a task-list
	// item: index counts the tasks of the document from 0, in document
	// order, and checked is the state the user asks for. The document does
	// not change by itself: edit Source and the box follows it. Nil makes
	// the tasks read-only.
	OnToggleTask func(index int, checked bool)
	// Class is added after the recipe's Root classes.
	Class string
	// Props are more props for the root element.
	Props tela.Child
}

// mdParser is the shared parser of every document: goldmark with the GFM
// extensions. It keeps no state between parses.
var mdParser = goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()

// mdDoc is a parsed document: the source bytes the nodes point into and the
// root of the tree, which is only read once built.
type mdDoc struct {
	src  []byte
	root ast.Node
}

func parseMarkdown(source string) *mdDoc {
	src := []byte(source)
	return &mdDoc{src: src, root: mdParser.Parse(text.NewReader(src))}
}

// Build returns the document's element.
func (m Markdown) Build() tela.Node {
	s := &UseStyle().Markdown
	doc := tela.UseMemo(func() *mdDoc { return parseMarkdown(m.Source) }, m.Source)
	r := &mdRender{src: doc.src, s: s, m: &m}
	r.out = bufio.NewWriter(&r.buf)
	kids := r.blocks(doc.root, mdCtx{})
	return h.Div(cls(s.Root, m.Class), m.Props, h.Fragment(kids...))
}

// mdCtx is what a block needs to know about where it is: the classes that
// set its spacing when it is in a list item, and how deep the lists are.
type mdCtx struct {
	// space is added to the classes of a block after its own margin.
	space string
	depth int
}

// mdRender builds the elements of one document.
type mdRender struct {
	src []byte
	s   *MarkdownStyle
	m   *Markdown
	// task is the index of the next task-list item.
	task int
	// buf and out decode the escapes and entities of text.
	buf bytes.Buffer
	out *bufio.Writer
}

// blocks renders the block children of n.
func (r *mdRender) blocks(n ast.Node, ctx mdCtx) []tela.Child {
	var kids []tela.Child
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if b := r.block(c, ctx); b != nil {
			kids = append(kids, b)
		}
	}
	return kids
}

func (r *mdRender) block(n ast.Node, ctx mdCtx) tela.Child {
	s := r.s
	switch n := n.(type) {
	case *ast.Heading:
		return mdHeading(n.Level)(cls(s.heading(n.Level), ctx.space), h.Fragment(r.inlines(n)...))
	case *ast.Paragraph, *ast.TextBlock:
		kids := r.inlines(n)
		if len(kids) == 0 {
			return nil // an image that is not shown, an empty item
		}
		return h.P(cls(s.Paragraph, ctx.space), h.Fragment(kids...))
	case *ast.Blockquote:
		return h.Div(h.Role("blockquote"), cls(s.Blockquote, ctx.space), h.Fragment(r.blocks(n, mdCtx{depth: ctx.depth})...))
	case *ast.FencedCodeBlock:
		return r.codeBlock(n, ctx)
	case *ast.CodeBlock:
		return r.codeBlock(n, ctx)
	case *ast.ThematicBreak:
		return h.Div(h.Role("separator"), cls(s.Rule))
	case *ast.List:
		return r.list(n, ctx)
	case *ast.HTMLBlock:
		var sb strings.Builder
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			sb.Write(seg.Value(r.src))
		}
		if n.HasClosure() {
			sb.Write(n.ClosureLine.Value(r.src))
		}
		return h.P(h.Dir("ltr"), cls(s.HTML, ctx.space), h.Text(strings.TrimRight(sb.String(), "\r\n")))
	case *east.Table:
		return r.table(n, ctx)
	}
	if n.Type() == ast.TypeBlock && n.HasChildren() {
		// A block this renderer does not know (an extension's): its blocks.
		return h.Fragment(r.blocks(n, ctx)...)
	}
	return nil
}

func (s *MarkdownStyle) heading(level int) string {
	switch level {
	case 1:
		return s.H1
	case 2:
		return s.H2
	case 3:
		return s.H3
	case 4:
		return s.H4
	case 5:
		return s.H5
	}
	return s.H6
}

func mdHeading(level int) func(...tela.Child) tela.Node {
	switch level {
	case 1:
		return h.H1
	case 2:
		return h.H2
	case 3:
		return h.H3
	case 4:
		return h.H4
	case 5:
		return h.H5
	}
	return h.H6
}

// codeBlock renders a fenced or an indented code block: the lines as they
// are, in a box that scrolls sideways. Code is left to right whatever the
// direction of the document.
func (r *mdRender) codeBlock(n ast.Node, ctx mdCtx) tela.Child {
	var sb strings.Builder
	lines := n.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		sb.Write(seg.Value(r.src))
	}
	code := strings.TrimSuffix(strings.ReplaceAll(sb.String(), "\r\n", "\n"), "\n")
	return h.Div(h.Dir("ltr"), cls(r.s.CodeBlock, ctx.space),
		h.Code(cls(r.s.CodeBlockCode), h.Text(code)))
}

// list renders an ordered or an unordered list. A task item replaces its
// marker with a checkbox.
func (r *mdRender) list(l *ast.List, ctx mdCtx) tela.Child {
	s := r.s
	space := s.Loose
	if l.IsTight {
		space = s.Tight
	}
	inner := mdCtx{space: space, depth: ctx.depth + 1}
	number := l.Start
	// The numbers are as wide as the widest one: right-aligned, so the
	// bodies line up.
	digits := 1
	if l.IsOrdered() {
		last := number
		for c := l.FirstChild(); c != nil; c = c.NextSibling() {
			last++
		}
		digits = max(len(strconv.Itoa(max(number, 0))), len(strconv.Itoa(max(last, 0))))
	}
	var items []tela.Child
	for c := l.FirstChild(); c != nil; c = c.NextSibling() {
		var marker tela.Child
		task, checked := mdTask(c)
		switch {
		case task:
			marker = r.taskCheckbox(c, checked)
		case l.IsOrdered():
			marker = h.Span(cls(s.Marker, s.Number, "min-w-["+strconv.FormatFloat(0.6*float64(digits)+0.15, 'f', -1, 64)+"em]"), h.AriaHidden(true),
				h.Text(strconv.Itoa(number)+"."))
		default:
			shape := s.BulletDisc
			switch ctx.depth {
			case 0:
			case 1:
				shape = s.BulletCircle
			default:
				shape = s.BulletSquare
			}
			marker = h.Span(cls(s.Marker, shape), h.AriaHidden(true))
		}
		number++
		items = append(items, h.Div(h.Role("listitem"), cls(s.Item),
			marker,
			h.Div(cls(s.ItemBody), h.Fragment(r.blocks(c, inner)...))))
	}
	return h.Div(h.Role("list"), cls(s.List, ctx.space), h.Fragment(items...))
}

// mdTask reports whether a list item is a task and whether it is checked: its
// first text starts with a task checkbox.
func mdTask(item ast.Node) (task, checked bool) {
	first := item.FirstChild()
	if first == nil {
		return false, false
	}
	if tb, ok := first.FirstChild().(*east.TaskCheckBox); ok {
		return true, tb.IsChecked
	}
	return false, false
}

// taskCheckbox draws the checkbox of the next task: read-only without
// OnToggleTask, else a controlled checkbox that asks for the change.
func (r *mdRender) taskCheckbox(item ast.Node, checked bool) tela.Child {
	i := r.task
	r.task++
	label := strings.TrimSpace(r.plain(item.FirstChild()))
	if on := r.m.OnToggleTask; on != nil {
		return Checkbox{Checked: &checked, OnCheckedChange: func(v bool) { on(i, v) }, AriaLabel: label, Class: r.s.Task}
	}
	return Checkbox{Checked: &checked, Disabled: true, AriaLabel: label, Class: joinClass(r.s.Task, r.s.TaskReadOnly)}
}

// table renders a GFM table as a grid with one auto track per column; its
// rows are display: contents boxes, as in [Table].
func (r *mdRender) table(t *east.Table, ctx mdCtx) tela.Child {
	s := r.s
	cols := len(t.Alignments)
	if cols == 0 {
		return nil
	}
	tracks := strings.TrimSuffix(strings.Repeat("auto_", cols), "_")
	var rows []ast.Node
	for c := t.FirstChild(); c != nil; c = c.NextSibling() {
		rows = append(rows, c)
	}
	var out []tela.Child
	for i, row := range rows {
		_, head := row.(*east.TableHeader)
		last := i == len(rows)-1
		var cells []tela.Child
		n := 0
		for c := row.FirstChild(); c != nil && n < cols; c = c.NextSibling() {
			cells = append(cells, r.tableCell(c, t.Alignments[n], head, last))
			n++
		}
		for ; n < cols; n++ { // a row with fewer cells gets empty ones
			cells = append(cells, r.tableCell(nil, t.Alignments[n], head, last))
		}
		out = append(out, h.Div(h.Role("row"), h.Class("contents"), h.Fragment(cells...)))
	}
	grid := h.Div(h.Role("table"), cls(s.Grid, "grid-cols-["+tracks+"]"), h.Fragment(out...))
	return h.Div(cls(s.Table, ctx.space), grid)
}

func (r *mdRender) tableCell(c ast.Node, align east.Alignment, head, last bool) tela.Child {
	s := r.s
	role, class := "cell", s.Cell
	if head {
		role, class = "columnheader", s.Head
	}
	if last && !head {
		class = joinClass(class, s.LastRow)
	}
	var kids []tela.Child
	if c != nil {
		kids = r.inlines(c)
	}
	return h.Div(h.Role(role), cls(class, mdAlign(align)), h.Fragment(kids...))
}

func mdAlign(a east.Alignment) string {
	switch a {
	case east.AlignRight:
		return "text-right"
	case east.AlignCenter:
		return "text-center"
	}
	return "text-left"
}

// inlines renders the inline children of n.
func (r *mdRender) inlines(n ast.Node) []tela.Child {
	var kids []tela.Child
	var run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			kids = append(kids, h.Text(run.String()))
			run.Reset()
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			v := c.Segment.Value(r.src)
			if c.IsRaw() {
				run.Write(v)
			} else {
				run.WriteString(r.decode(v))
			}
			switch {
			case c.HardLineBreak():
				run.WriteByte('\n')
			case c.SoftLineBreak():
				run.WriteByte(' ')
			}
		case *ast.String:
			run.Write(c.Value)
		case *ast.RawHTML:
			segs := c.Segments
			for i := range segs.Len() {
				seg := segs.At(i)
				run.Write(seg.Value(r.src))
			}
		case *east.TaskCheckBox:
			// The marker of the item has it.
		default:
			flush()
			if k := r.inline(c); k != nil {
				kids = append(kids, k)
			}
		}
	}
	flush()
	return kids
}

func (r *mdRender) inline(n ast.Node) tela.Child {
	s := r.s
	switch n := n.(type) {
	case *ast.CodeSpan:
		var sb strings.Builder
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			t, ok := c.(*ast.Text)
			if !ok {
				continue
			}
			v := t.Segment.Value(r.src)
			if bytes.HasSuffix(v, []byte("\n")) {
				sb.Write(v[:len(v)-1])
				sb.WriteByte(' ')
			} else {
				sb.Write(v)
			}
		}
		return h.Code(cls(s.Code), h.Text(sb.String()))
	case *ast.Emphasis:
		if n.Level >= 2 {
			return h.Strong(h.Fragment(r.inlines(n)...))
		}
		return h.Em(h.Fragment(r.inlines(n)...))
	case *east.Strikethrough:
		return h.Span(h.Role("deletion"), cls(s.Strikethrough), h.Fragment(r.inlines(n)...))
	case *ast.Link:
		return mdLink{Href: mdURL(n.Destination), Title: r.decode(n.Title), OnLink: r.m.OnLink, Class: s.Link, Kids: r.inlines(n)}
	case *ast.AutoLink:
		href := string(n.URL(r.src))
		if n.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(href), "mailto:") {
			href = "mailto:" + href
		}
		return mdLink{Href: mdURL([]byte(href)), OnLink: r.m.OnLink, Class: s.Link, Kids: []tela.Child{h.Text(string(n.Label(r.src)))}}
	case *ast.Image:
		return r.image(n)
	}
	// An inline this renderer does not know (an extension's): its content.
	return h.Fragment(r.inlines(n)...)
}

// image draws an image of the app's assets, or its alt text for any other.
func (r *mdRender) image(n *ast.Image) tela.Child {
	s := r.s
	alt := strings.TrimSpace(r.plain(n))
	if p, ok := mdAssetPath(n.Destination); ok {
		props := []tela.Child{h.SrcPath(p), h.Alt(alt), cls(s.Image)}
		if t := r.decode(n.Title); t != "" {
			props = append(props, h.AriaDescription(t))
		}
		return h.Img(props...)
	}
	if alt == "" {
		return nil
	}
	return h.Span(cls(s.ImageAlt), h.Text(alt))
}

// plain returns the text of the inline content of n, without markup.
func (r *mdRender) plain(n ast.Node) string {
	var sb strings.Builder
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch c := c.(type) {
			case *ast.Text:
				v := c.Segment.Value(r.src)
				if c.IsRaw() {
					sb.Write(v)
				} else {
					sb.WriteString(r.decode(v))
				}
				if c.SoftLineBreak() || c.HardLineBreak() {
					sb.WriteByte(' ')
				}
			case *ast.String:
				sb.Write(c.Value)
			case *east.TaskCheckBox:
			case *ast.AutoLink:
				sb.Write(c.Label(r.src))
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return sb.String()
}

// decode resolves the backslash escapes and the entities of a text, as
// goldmark's HTML writer does: it writes the text escaped, and the escaping
// is undone.
func (r *mdRender) decode(v []byte) string {
	if bytes.IndexAny(v, "\\&\x00") < 0 {
		return string(v)
	}
	r.buf.Reset()
	r.out.Reset(&r.buf)
	html.DefaultWriter.Write(r.out, v)
	_ = r.out.Flush()
	return mdUnescapeHTML(r.buf.String())
}

// mdUnescapeHTML undoes the escaping of the five characters goldmark's
// writer escapes in text.
var mdUnescapeHTML = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&").Replace

// mdURL returns a link destination as an HTML renderer writes it: entities
// and escapes resolved, unsafe bytes percent-encoded.
func mdURL(dest []byte) string {
	return string(util.URLEscape(dest, true))
}

// mdAssetPath returns the path of an image destination in the app's assets,
// and whether it is one. A destination is a path of the assets when it has
// no scheme and no host (it is not a URL, nor a "//host/path") and does not
// leave the root with "..".
func mdAssetPath(dest []byte) (string, bool) {
	d := mdURL(dest)
	if d == "" {
		return "", false
	}
	u, err := url.Parse(d)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || strings.HasPrefix(d, "//") || u.Path == "" {
		return "", false
	}
	p := path.Clean(strings.TrimPrefix(u.Path, "/"))
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", false
	}
	return p, true
}

// mdLink is a link: an A that a press or Enter activates.
type mdLink struct {
	Href, Title string
	OnLink      func(string)
	Class       string
	Kids        []tela.Child
}

// Build returns the link element.
func (l mdLink) Build() tela.Node {
	open := func(aria.PressEvent) {
		if l.OnLink != nil {
			l.OnLink(l.Href)
			return
		}
		// A destination the desktop cannot open (no scheme, a "#heading")
		// does nothing: the app that wants it handled gives OnLink.
		_ = tela.OpenURL(l.Href)
	}
	press := aria.UsePress(aria.PressOptions{OnPress: open})
	props := []tela.Child{h.Href(l.Href), press.Props, h.TabIndex(0), cls(l.Class)}
	if l.Title != "" {
		props = append(props, h.AriaDescription(l.Title))
	}
	return h.A(append(props, l.Kids...)...)
}
