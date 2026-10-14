package main

import (
	"go/doc/comment"
	"strings"
	"unicode"
)

// The standard library packages doc links may name; they link to
// pkg.go.dev.
var stdPackages = map[string]bool{
	"context": true, "errors": true, "fmt": true, "image": true, "io": true,
	"os": true, "sort": true, "strings": true, "testing": true, "time": true,
}

// lookupPackage resolves the package name of a [pkg.Name] doc link.
func (g *generator) lookupPackage(name string) (string, bool) {
	for _, p := range g.order {
		if path := p.spec.ImportPath; path == name || strings.HasSuffix(path, "/"+name) {
			return path, true
		}
	}
	if stdPackages[name] {
		return name, true
	}
	return "", false
}

// linkURL returns the URL of a doc link found in a comment of cur: an
// anchor of this page, a section of another page of the wiki, or
// pkg.go.dev.
func (g *generator) linkURL(cur *pkgInfo, l *comment.DocLink) string {
	importPath := l.ImportPath
	if importPath == "" {
		importPath = cur.spec.ImportPath
	}
	if p, ok := g.pkgs[importPath]; ok {
		if anchor, ok := p.syms[symKey(l.Recv, l.Name)]; ok {
			if p == cur {
				return "#" + anchor
			}
			return p.spec.Page + "#" + anchor
		}
		if l.Name == "" {
			if p == cur {
				return "#"
			}
			return p.spec.Page
		}
	}
	url := "https://pkg.go.dev/" + importPath
	if l.Name != "" {
		url += "#" + symKey(l.Recv, l.Name)
	}
	return url
}

// mdWriter converts doc comments to Markdown on behalf of one page.
type mdWriter struct {
	g   *generator
	cur *pkgInfo
	// note is called with the text of every heading found in a comment,
	// to detect duplicate anchors.
	note func(level int, text string)
}

// convert returns the Markdown of a doc comment. level is the level of the
// headings in the comment.
func (w *mdWriter) convert(text string, level int) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	p := &comment.Parser{LookupPackage: w.g.lookupPackage, LookupSym: w.cur.lookupSym}
	return w.blocks(p.Parse(text).Content, level)
}

// blocks renders blocks separated by blank lines, ending with a newline.
func (w *mdWriter) blocks(bs []comment.Block, level int) string {
	var parts []string
	for _, b := range bs {
		parts = append(parts, w.block(b, level))
	}
	return strings.Join(parts, "\n") + "\n"
}

// block renders one block, ending with a newline.
func (w *mdWriter) block(b comment.Block, level int) string {
	switch b := b.(type) {
	case *comment.Paragraph:
		return escapeStart(w.inline(b.Text)) + "\n"
	case *comment.Heading:
		text := w.plain(b.Text)
		if w.note != nil {
			w.note(level, text)
		}
		return strings.Repeat("#", level) + " " + text + "\n"
	case *comment.Code:
		return fence(codeLang(b.Text), detab(strings.TrimRight(b.Text, "\n")))
	case *comment.List:
		var sb strings.Builder
		for i, item := range b.Items {
			marker := "- "
			if item.Number != "" {
				marker = item.Number + ". "
			}
			if i > 0 && b.ForceBlankBetween {
				sb.WriteString("\n")
			}
			body := strings.TrimRight(w.blocks(item.Content, level), "\n")
			indent := strings.Repeat(" ", len(marker))
			for j, line := range strings.Split(body, "\n") {
				switch {
				case j == 0:
					sb.WriteString(marker + line + "\n")
				case line == "":
					sb.WriteString("\n")
				default:
					sb.WriteString(indent + line + "\n")
				}
			}
		}
		return sb.String()
	}
	return ""
}

// inline renders the inline content of a paragraph.
func (w *mdWriter) inline(ts []comment.Text) string {
	var sb strings.Builder
	for _, t := range ts {
		switch t := t.(type) {
		case comment.Plain:
			sb.WriteString(plainMarkdown(string(t)))
		case comment.Italic:
			sb.WriteString("*" + escape(oneLine(string(t))) + "*")
		case *comment.Link:
			if t.Auto {
				sb.WriteString("<" + t.URL + ">")
			} else {
				sb.WriteString("[" + w.inline(t.Text) + "](" + t.URL + ")")
			}
		case *comment.DocLink:
			sb.WriteString("[" + codeSpan(w.plain(t.Text)) + "](" + w.g.linkURL(w.cur, t) + ")")
		}
	}
	return sb.String()
}

// plain renders inline content as unformatted text.
func (w *mdWriter) plain(ts []comment.Text) string {
	var sb strings.Builder
	for _, t := range ts {
		switch t := t.(type) {
		case comment.Plain:
			sb.WriteString(oneLine(string(t)))
		case comment.Italic:
			sb.WriteString(oneLine(string(t)))
		case *comment.Link:
			sb.WriteString(w.plain(t.Text))
		case *comment.DocLink:
			sb.WriteString(w.plain(t.Text))
		}
	}
	return sb.String()
}

// oneLine joins the lines of a paragraph with spaces.
func oneLine(s string) string { return strings.ReplaceAll(s, "\n", " ") }

// plainMarkdown renders running text: `quoted` spans become code spans and
// the rest is escaped.
func plainMarkdown(s string) string {
	parts := strings.Split(oneLine(s), "`")
	var sb strings.Builder
	for i, part := range parts {
		switch {
		case i%2 == 0:
			sb.WriteString(escape(part))
		case i == len(parts)-1 || part == "":
			// An unmatched backtick, or an empty span.
			sb.WriteString("\\`" + escape(part))
		default:
			sb.WriteString(codeSpan(part))
		}
	}
	return sb.String()
}

// escape escapes the characters Markdown gives a meaning to inside text.
// An underscore between two word characters (TELA_UPDATE) is left alone,
// since it cannot start emphasis there.
func escape(s string) string {
	rs := []rune(s)
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	var sb strings.Builder
	for i, r := range rs {
		switch r {
		case '\\', '`', '*', '[', ']', '<', '>', '&', '~', '|':
			sb.WriteByte('\\')
		case '_':
			if !(i > 0 && i+1 < len(rs) && isWord(rs[i-1]) && isWord(rs[i+1])) {
				sb.WriteByte('\\')
			}
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// escapeStart escapes what would start a block at the beginning of a
// paragraph: a heading, a list or a quote.
func escapeStart(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsRune("#-+", rune(s[0])) {
		return `\` + s
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i < len(s) && (s[i] == '.' || s[i] == ')') {
		return s[:i] + `\` + s[i:]
	}
	return s
}

// codeSpan renders s as inline code.
func codeSpan(s string) string {
	s = oneLine(s)
	if strings.Contains(s, "`") {
		return "`` " + s + " ``"
	}
	return "`" + s + "`"
}

// fence renders code as a fenced block, ending with a newline.
func fence(lang, code string) string {
	ticks := "```"
	for strings.Contains(code, ticks) {
		ticks += "`"
	}
	return ticks + lang + "\n" + code + "\n" + ticks + "\n"
}

// detab replaces the tabs that indent code with four spaces, which
// browsers show narrower than their default eight.
func detab(code string) string {
	lines := strings.Split(code, "\n")
	for i, l := range lines {
		n := len(l) - len(strings.TrimLeft(l, "\t"))
		lines[i] = strings.Repeat("    ", n) + l[n:]
	}
	return strings.Join(lines, "\n")
}

// codeLang guesses the language of a code block of a doc comment, for
// syntax highlighting: Go unless it looks like a shell command or YAML.
func codeLang(code string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(code), "\n")
	switch {
	case strings.HasPrefix(first, "- ") && !strings.Contains(first, "("):
		return "yaml"
	case strings.HasPrefix(first, "tela "), isGoCommand(first),
		strings.HasPrefix(first, "$ "), strings.HasPrefix(first, "TELA_"),
		strings.HasPrefix(first, "CGO_ENABLED="):
		return "sh"
	}
	return "go"
}

// isGoCommand reports whether line runs the go tool ("go test ./...", as
// opposed to a go statement).
func isGoCommand(line string) bool {
	rest, ok := strings.CutPrefix(line, "go ")
	if !ok {
		return false
	}
	sub, _, _ := strings.Cut(rest, " ")
	switch sub {
	case "build", "run", "test", "vet", "doc", "generate", "install", "get", "mod", "fmt", "list", "version":
		return true
	}
	return false
}
