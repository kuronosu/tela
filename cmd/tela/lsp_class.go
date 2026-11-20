package main

import (
	"go/scanner"
	"go/token"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/utf16x"
)

// The text half of tela lsp: where in a Go buffer the cursor is inside a
// class string, which class it is on, and what to complete or say about it.
// Nothing here needs the type checker, so it answers while the buffer does
// not compile: the Go scanner tokenizes the text (it copes with an
// unterminated string and any other mess) and a small state machine over
// the tokens before the string decides whether it is a class string.

// ---------------------------------------------------------------------
// Documents and positions.

// lspPosition is an LSP position: a line and a UTF-16 offset in it.
type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// lspRange is an LSP range.
type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

// lspDoc is an open document: its text as the editor has it.
type lspDoc struct {
	uri     string
	path    string // the file, or "" when the document is not a file
	version int
	text    string

	// starts are the byte offsets of the line starts, built on demand. A
	// document is never edited, only replaced, so it may be read by
	// several goroutines.
	once   sync.Once
	starts []int
}

func (d *lspDoc) lineStarts() []int {
	d.once.Do(func() {
		d.starts = []int{0}
		for i := 0; i < len(d.text); i++ {
			if d.text[i] == '\n' {
				d.starts = append(d.starts, i+1)
			}
		}
	})
	return d.starts
}

// lineText returns line l without its line break.
func (d *lspDoc) lineText(l int) string {
	starts := d.lineStarts()
	if l < 0 || l >= len(starts) {
		return ""
	}
	end := len(d.text)
	if l+1 < len(starts) {
		end = starts[l+1] - 1
	}
	return strings.TrimSuffix(d.text[starts[l]:end], "\r")
}

// offset returns the byte offset of a position, clamped to the text.
func (d *lspDoc) offset(p lspPosition) int {
	starts := d.lineStarts()
	if p.Line < 0 {
		return 0
	}
	if p.Line >= len(starts) {
		return len(d.text)
	}
	start := starts[p.Line]
	end := len(d.text)
	if p.Line+1 < len(starts) {
		end = starts[p.Line+1] - 1
	}
	line := strings.TrimSuffix(d.text[start:end], "\r")
	return start + utf16x.ToByte(line, p.Character)
}

// position returns the position of a byte offset.
func (d *lspDoc) position(off int) lspPosition {
	off = max(0, min(off, len(d.text)))
	starts := d.lineStarts()
	l := sort.Search(len(starts), func(i int) bool { return starts[i] > off }) - 1
	return lspPosition{Line: l, Character: utf16x.FromByte(d.text[starts[l]:], off-starts[l])}
}

// rangeOf returns the range of the byte offsets [from, to).
func (d *lspDoc) rangeOf(from, to int) lspRange {
	return lspRange{Start: d.position(from), End: d.position(to)}
}

// ---------------------------------------------------------------------
// Finding the class string.

// classSite is a string literal in a Go buffer that holds the offset asked
// for.
type classSite struct {
	// litStart and litEnd delimit the literal, quotes included; for an
	// unterminated one litEnd is where the scanner gave up (the end of the
	// line).
	litStart, litEnd int
	// bodyStart and bodyEnd delimit its content, between the quotes.
	bodyStart, bodyEnd int
	// class says the context of the literal is a class: the arguments of
	// Class, a Class field, a value of ClassAliases.
	class bool
}

type scanTok struct {
	off int
	tok token.Token
	lit string
}

// siteAt finds the string literal whose content holds the byte offset off
// (right after the opening quote to right before the closing one), and
// whether the tokens before it make it a class string. found is false when
// off is not inside a string literal.
func siteAt(src string, off int) (site classSite, found bool) {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var sc scanner.Scanner
	sc.Init(file, []byte(src), func(token.Position, string) {}, 0)
	var toks []scanTok
	var braces []bool // per open brace: it holds a ClassAliases map
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			return site, false
		}
		start := file.Offset(pos)
		if start >= off {
			// A string starts at or after the offset: only one that
			// starts before it can hold it.
			return site, false
		}
		switch tok {
		case token.LBRACE:
			braces = append(braces, aliasBrace(toks))
		case token.RBRACE:
			if len(braces) > 0 {
				braces = braces[:len(braces)-1]
			}
		case token.STRING:
			end := start + len(lit)
			closed := len(lit) >= 2 && lit[len(lit)-1] == lit[0] && !(lit[0] == '"' && escaped(lit[:len(lit)-1]))
			bodyEnd := end
			if closed {
				bodyEnd--
			}
			if off <= bodyEnd {
				inAliases := len(braces) > 0 && braces[len(braces)-1]
				return classSite{
					litStart: start, litEnd: end, bodyStart: start + 1, bodyEnd: bodyEnd,
					class: classContext(toks, inAliases),
				}, true
			}
		}
		toks = append(toks, scanTok{start, tok, lit})
	}
}

// escaped reports whether s, an interpreted string without its closing
// quote, ends in an odd number of backslashes: the quote would be escaped.
func escaped(s string) bool {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// aliasBrace reports whether the brace that follows toks opens a map of
// class aliases: ClassAliases: map[string]string{, ClassAliases = {,
// telatest.ClassAliases(map[string]string{.
func aliasBrace(toks []scanTok) bool {
	n := len(toks)
	// An optional map[string]string.
	if n >= 5 && toks[n-5].tok == token.MAP && toks[n-4].tok == token.LBRACK &&
		toks[n-3].tok == token.IDENT && toks[n-2].tok == token.RBRACK && toks[n-1].tok == token.IDENT {
		n -= 5
	}
	if n < 2 {
		return false
	}
	switch toks[n-1].tok {
	case token.COLON, token.ASSIGN, token.LPAREN:
		return toks[n-2].tok == token.IDENT && toks[n-2].lit == "ClassAliases"
	}
	return false
}

// classContext reports whether a string literal that follows toks is a
// class string by its syntax:
//
//   - an argument of Class(...), Exit(...) or RootClass(...);
//   - the value of a field named Class or ending in Class: in a literal
//     (Class: "...") or assigned (x.Class = "...", x.Class += "...");
//   - the value of an entry of a ClassAliases map.
//
// Concatenations of strings continue the context.
func classContext(toks []scanTok, inAliases bool) bool {
	i := len(toks) - 1
	skipConcat := func() {
		for i >= 1 && toks[i].tok == token.ADD && toks[i-1].tok == token.STRING {
			i -= 2
		}
	}
	skipConcat()
	if i < 0 {
		return false
	}
	// Earlier arguments of the same call: "a", "b", <here>.
	for i >= 1 && toks[i].tok == token.COMMA && toks[i-1].tok == token.STRING {
		i -= 2
		skipConcat()
	}
	if i < 0 {
		return false
	}
	switch toks[i].tok {
	case token.LPAREN:
		return i >= 1 && toks[i-1].tok == token.IDENT && isClassFunc(toks[i-1].lit)
	case token.COLON:
		if inAliases {
			return i >= 1 && toks[i-1].tok == token.STRING
		}
		return i >= 1 && toks[i-1].tok == token.IDENT && isClassField(toks[i-1].lit)
	case token.ASSIGN, token.ADD_ASSIGN:
		return i >= 2 && toks[i-1].tok == token.IDENT && toks[i-2].tok == token.PERIOD && isClassField(toks[i-1].lit)
	}
	return false
}

func isClassFunc(name string) bool {
	return name == "Class" || name == "Exit" || name == "RootClass"
}

func isClassField(name string) bool { return strings.HasSuffix(name, "Class") }

// classAt returns the class around the offset in the content of a class
// string: its start and end offsets in src, with the cursor inside or at
// either end. ok is false when the cursor is on white space between
// classes and the class is empty.
func classAt(src string, site classSite, off int) (start, end int, ok bool) {
	body := src[site.bodyStart:site.bodyEnd]
	rel := off - site.bodyStart
	start = rel
	for start > 0 && !isClassSpace(body[start-1]) {
		start--
	}
	end = rel
	for end < len(body) && !isClassSpace(body[end]) {
		end++
	}
	return site.bodyStart + start, site.bodyStart + end, end > start
}

func isClassSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

// ---------------------------------------------------------------------
// Completion.

// The completion item kinds of the protocol that the server uses.
const (
	lspKindValue   = 12
	lspKindKeyword = 14
	lspKindSnippet = 15
	lspKindColor   = 16
	lspKindClass   = 7
)

// lspCompletionItem is an item of a completion list.
type lspCompletionItem struct {
	Label            string       `json:"label"`
	Kind             int          `json:"kind,omitempty"`
	Detail           string       `json:"detail,omitempty"`
	Documentation    string       `json:"documentation,omitempty"`
	SortText         string       `json:"sortText,omitempty"`
	FilterText       string       `json:"filterText,omitempty"`
	InsertTextFormat int          `json:"insertTextFormat,omitempty"` // 2: snippet
	TextEdit         *lspTextEdit `json:"textEdit,omitempty"`
	Command          *lspCommand  `json:"command,omitempty"`
}

type lspTextEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

type lspCommand struct {
	Title   string `json:"title"`
	Command string `json:"command"`
}

// lspCompletionList is the result of textDocument/completion.
type lspCompletionList struct {
	IsIncomplete bool                `json:"isIncomplete"`
	Items        []lspCompletionItem `json:"items"`
}

// completionLimit is how many classes one completion lists; a longer list
// is marked incomplete, so that the editor asks again as the user types.
const completionLimit = 300

// classIndex is what completion reads of a parser: its vocabulary (sorted),
// the bare utilities and the first segments of the rest.
type classIndex struct {
	vocab []string
	bare  []string // utilities without a dash: flex, hidden, rounded...
	heads []string // the first segments of the others, with the dash: bg-
}

func newClassIndex(p *style.Parser) *classIndex {
	ix := &classIndex{vocab: p.Vocabulary()}
	seen := map[string]bool{}
	for _, c := range ix.vocab {
		if strings.HasPrefix(c, "-") {
			continue
		}
		i := strings.IndexByte(c, '-')
		if i < 0 {
			ix.bare = append(ix.bare, c)
			continue
		}
		if h := c[:i+1]; !seen[h] {
			seen[h] = true
			ix.heads = append(ix.heads, h)
		}
	}
	return ix
}

// arbitraryHints are the utilities that take an arbitrary value, with an
// example the completion shows (underscores are spaces, as in Tailwind).
var arbitraryHints = []struct{ root, example string }{
	{"w", "320px"}, {"h", "50vh"}, {"size", "2.5rem"}, {"min-w", "200px"}, {"max-w", "640px"},
	{"min-h", "100px"}, {"max-h", "80vh"}, {"p", "10px"}, {"px", "10px"}, {"py", "10px"},
	{"m", "10px"}, {"mx", "10px"}, {"my", "10px"}, {"gap", "10px"}, {"top", "10px"},
	{"left", "10px"}, {"right", "10px"}, {"bottom", "10px"}, {"inset", "10px"},
	{"bg", "#1e1e2e"}, {"text", "14px"}, {"border", "3px"}, {"rounded", "12px"},
	{"grid-cols", "200px_1fr"}, {"grid-rows", "auto_1fr"}, {"basis", "30%"},
	{"leading", "1.2"}, {"tracking", "0.1em"}, {"opacity", "0.35"}, {"z", "100"},
	{"duration", "250"}, {"translate-x", "10px"}, {"translate-y", "10px"},
	{"rotate", "17deg"}, {"scale", "1.07"}, {"shadow", "0_4px_12px_#0003"}, {"ring", "3px"},
	{"aspect", "4/3"},
}

// variantHints are the parametrized variants, with an example.
var variantHints = []struct{ name, example, detail string }{
	{"min", "600px", "when the window is at least that wide"},
	{"max", "40rem", "when the window is narrower than that"},
	{"aria", "sort=ascending", "when the aria attribute has that value"},
}

// completeClass lists the completions for the class that starts at
// classStart in the document and is typed up to cursor (the class goes on
// to classEnd). Variants before the last colon are kept: the edit replaces
// only what follows it.
func completeClass(p *style.Parser, ix *classIndex, d *lspDoc, classStart, cursor, classEnd int) lspCompletionList {
	prefix := d.text[classStart:cursor]
	variants := ""
	if parts := splitColons(prefix); len(parts) > 1 {
		variants = prefix[:len(prefix)-len(parts[len(parts)-1])]
	}
	rest := prefix[len(variants):]
	from := classStart + len(variants)
	edit := func(label string) *lspTextEdit {
		return &lspTextEdit{Range: d.rangeOf(from, classEnd), NewText: label}
	}
	var items []lspCompletionItem
	aliases := p.Aliases()

	// Variants: before the first utility, or after another variant.
	for _, v := range p.VariantNames() {
		if strings.HasPrefix(v, rest) {
			label := v + ":"
			items = append(items, lspCompletionItem{
				Label: label, Kind: lspKindKeyword, Detail: p.DescribeVariant(v),
				SortText: "2" + v, TextEdit: edit(label),
				Command: retrigger(),
			})
		}
	}
	for _, v := range variantHints {
		label := v.name + "-[…]:"
		if strings.HasPrefix(v.name+"-[", rest) {
			items = append(items, lspCompletionItem{
				Label: label, Kind: lspKindSnippet, Detail: v.detail,
				FilterText: v.name + "-[", SortText: "3" + v.name, InsertTextFormat: 2,
				TextEdit: edit(v.name + "-[${1:" + v.example + "}]:"),
			})
		}
	}

	// Aliases first, then utilities.
	if rest == "" {
		for _, name := range sortedKeys(aliases) {
			items = append(items, aliasItem(name, aliases[name], edit))
		}
		for _, c := range ix.bare {
			items = append(items, utilityItem(p, c, edit))
		}
		for _, h := range ix.heads {
			items = append(items, lspCompletionItem{
				Label: h, Kind: lspKindValue, SortText: "1" + h, TextEdit: edit(h), Command: retrigger(),
			})
		}
		return lspCompletionList{IsIncomplete: true, Items: items}
	}
	list := lspCompletionList{}
	n := sort.SearchStrings(ix.vocab, rest)
	for ; n < len(ix.vocab) && strings.HasPrefix(ix.vocab[n], rest); n++ {
		c := ix.vocab[n]
		if len(items) >= completionLimit {
			list.IsIncomplete = true
			break
		}
		if exp, ok := aliases[c]; ok {
			items = append(items, aliasItem(c, exp, edit))
			continue
		}
		items = append(items, utilityItem(p, c, edit))
	}
	if strings.Contains(rest, "-") {
		for _, h := range arbitraryHints {
			if strings.HasPrefix(h.root+"-[", rest) {
				d, _ := p.Describe(h.root + "-[" + h.example + "]")
				detail := "arbitrary value, for example " + h.root + "-[" + h.example + "]"
				if s := declSummary(d); s != "" {
					detail += ": " + s
				}
				items = append(items, lspCompletionItem{
					Label: h.root + "-[…]", Kind: lspKindSnippet, Detail: detail,
					FilterText: h.root + "-[", SortText: "1" + h.root + "-[", InsertTextFormat: 2,
					TextEdit: edit(h.root + "-[${1:" + h.example + "}]"),
				})
			}
		}
	}
	list.Items = items
	return list
}

// retrigger asks the editor to open the completion list again after the
// insertion (VS Code's command; other editors ignore it).
func retrigger() *lspCommand {
	return &lspCommand{Title: "Suggest", Command: "editor.action.triggerSuggest"}
}

// splitColons splits a class at the colons outside brackets.
func splitColons(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case ':':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

func aliasItem(name, expansion string, edit func(string) *lspTextEdit) lspCompletionItem {
	return lspCompletionItem{
		Label: name, Kind: lspKindClass, Detail: "alias: " + expansion,
		SortText: "0" + name, TextEdit: edit(name),
	}
}

func utilityItem(p *style.Parser, class string, edit func(string) *lspTextEdit) lspCompletionItem {
	it := lspCompletionItem{Label: class, Kind: lspKindValue, SortText: "1" + class, TextEdit: edit(class)}
	d, err := p.Describe(class)
	if err != nil {
		return it
	}
	it.Detail = declSummary(d)
	if len(d.Declarations) > 0 && strings.HasPrefix(d.Declarations[0].Value, "#") {
		it.Kind = lspKindColor
		it.Documentation = d.Declarations[0].Value // editors draw a swatch from it
	}
	return it
}

// declSummary is the declarations on one line, for the detail of an item.
func declSummary(d style.Description) string {
	var parts []string
	for _, x := range d.Declarations {
		parts = append(parts, x.String())
	}
	s := strings.Join(parts, "; ")
	if utf8.RuneCountInString(s) > 110 {
		r := []rune(s)
		s = string(r[:109]) + "…"
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------
// Hover.

// hoverMarkdown describes class (with its variants) as Markdown, like
// Tailwind IntelliSense: the CSS it produces, and when. The error of a
// class that does not exist is its message with the suggestion.
func hoverMarkdown(p *style.Parser, class string) string {
	d, err := p.Describe(class)
	if err != nil {
		return "**`" + class + "`**\n\n" + err.Error()
	}
	var b strings.Builder
	b.WriteString("**`" + class + "`**")
	if d.Expansion != "" {
		b.WriteString(" is an alias for `" + d.Expansion + "`")
	}
	b.WriteString("\n\n")
	writeWhen(&b, d.Variants)
	if d.Expansion != "" {
		b.WriteString("```css\n")
		writeParts(&b, d.Parts, "")
		b.WriteString("```\n")
		return b.String()
	}
	if len(d.Declarations) == 0 {
		b.WriteString("Tailwind utility `" + d.Utility + "`; no CSS summary for this one.\n")
		return b.String()
	}
	b.WriteString("```css\n")
	for _, x := range d.Declarations {
		b.WriteString(x.String() + ";\n")
	}
	b.WriteString("```\n")
	return b.String()
}

func writeWhen(b *strings.Builder, variants []string) {
	if len(variants) == 0 {
		return
	}
	b.WriteString("_" + strings.Join(variants, ", ") + "_\n\n")
}

// writeParts writes the declarations of the classes of an alias, each under
// a comment with its class and the variants that apply to it.
func writeParts(b *strings.Builder, parts []style.Description, indent string) {
	for _, part := range parts {
		comment := part.Class
		if len(part.Variants) > 0 {
			comment += " (" + strings.Join(part.Variants, ", ") + ")"
		}
		switch {
		case part.Expansion != "":
			b.WriteString(indent + "/* " + comment + " is an alias for " + part.Expansion + " */\n")
			writeParts(b, part.Parts, indent)
		case len(part.Declarations) == 0:
			b.WriteString(indent + "/* " + comment + ": utility " + part.Utility + " */\n")
		default:
			b.WriteString(indent + "/* " + comment + " */\n")
			for _, x := range part.Declarations {
				b.WriteString(indent + x.String() + ";\n")
			}
		}
	}
}
