package kit_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The images of the documents under test: img/logo.png is in the app's
// assets, a 48 by 32 gradient.
func init() {
	img := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for y := range 32 {
		for x := range 48 {
			img.Set(x, y, color.RGBA{R: uint8(40 + x*4), G: uint8(80 + y*4), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	tela.Assets(fstest.MapFS{"img/logo.png": &fstest.MapFile{Data: buf.Bytes()}})
}

// mdMount mounts a document 480 px wide in the kit's theme.
func mdMount(t *testing.T, md kit.Markdown, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(480, 600), telatest.Theme(kit.Theme()),
		telatest.RootClass("bg-background p-4 text-foreground")}, opts...)
	return telatest.Mount(t, md, opts...)
}

// TestMarkdownConstructs checks the tree each construct makes, in the
// aria snapshot: the roles are what assistive technologies are given.
func TestMarkdownConstructs(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"headings", "# One\n\n## Two\n\n### Three\n\n#### Four\n\n##### Five\n\n###### Six\n\nSetext\n======\n\nSetext 2\n--------\n", `
			- heading "One" [level=1]
			- heading "Two" [level=2]
			- heading "Three" [level=3]
			- heading "Four" [level=4]
			- heading "Five" [level=5]
			- heading "Six" [level=6]
			- heading "Setext" [level=1]
			- heading "Setext 2" [level=2]`},
		{"inline", "A *soft* **strong** ~~gone~~ `code` ***both***.", `
			- paragraph:
			  - text: A
			  - emphasis: soft
			  - strong: strong
			  - deletion: gone
			  - code: code
			  - emphasis:
			    - strong: both
			  - text: .`},
		{"blockquote", "> a\n>\n> > b\n", `
			- blockquote:
			  - paragraph: a
			  - blockquote:
			    - paragraph: b`},
		{"lists", "- a\n- b\n  1. one\n  2. two\n\n1. x\n", `
			- list:
			  - listitem:
			    - paragraph: a
			  - listitem:
			    - paragraph: b
			    - list:
			      - listitem:
			        - paragraph: one
			      - listitem:
			        - paragraph: two
			- list:
			  - listitem:
			    - paragraph: x`},
		{"loose list", "- a\n\n- b\n\n  c\n", `
			- list:
			  - listitem:
			    - paragraph: a
			  - listitem:
			    - paragraph: b
			    - paragraph: c`},
		{"tasks", "- [ ] foo\n- [x] bar\n  - [X] nested\n- plain\n", `
			- list:
			  - listitem:
			    - checkbox "foo" [disabled]
			    - paragraph: foo
			  - listitem:
			    - checkbox "bar" [checked] [disabled]
			    - paragraph: bar
			    - list:
			      - listitem:
			        - checkbox "nested" [checked] [disabled]
			        - paragraph: nested
			  - listitem:
			    - paragraph: plain`},
		{"code blocks", "```go\nfunc main() {}\n```\n\n    indented\n\nand `a code span`", `
			- code: "func main() {}"
			- code: indented
			- paragraph:
			  - text: and
			  - code: a code span`},
		{"table", "| Name | Size |\n| :-- | --: |\n| a | 1 |\n| b |\n| c | 3 | 4 |\n", `
			- table:
			  - row "Name Size":
			    - columnheader "Name"
			    - columnheader "Size"
			  - row "a 1":
			    - cell "a"
			    - cell "1"
			  - row "b":
			    - cell "b"
			    - cell
			  - row "c 3":
			    - cell "c"
			    - cell "3"`},
		{"thematic break", "a\n\n---\n\nb\n\n***\n", `
			- paragraph: a
			- separator
			- paragraph: b
			- separator`},
		{"links", "[text](https://x.org/a?b=1 \"Title\") <https://y.org> <me@x.org> www.z.org [ref]\n\n[ref]: /there", `
			- paragraph:
			  - link "text":
			    - /url: https://x.org/a?b=1
			  - link "https://y.org":
			    - /url: https://y.org
			  - link "me@x.org":
			    - /url: mailto:me@x.org
			  - link "www.z.org":
			    - /url: http://www.z.org
			  - link "ref":
			    - /url: /there`},
		{"html is literal", "<div>\n*not emphasis*\n</div>\n\nfoo <b>bar</b> <!-- c -->", `
			- paragraph: <div> *not emphasis* </div>
			- paragraph: foo <b>bar</b> <!-- c -->`},
		{"entities and escapes", "&amp; &copy; &#35; &#x1F600; \\*star\\* \\<tag> &ouml;\\&ouml; &nosuch;", `
			- paragraph: "& © # 😀 *star* <tag> ö&ouml; &nosuch;"`},
		{"empty", "", ``},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := mdMount(t, kit.Markdown{Source: c.src})
			a.ExpectAria(c.want)
		})
	}
}

// TestMarkdownSpecExamples checks a handful of the examples of the
// CommonMark 0.31 and GFM 0.29 specifications against their expected
// tree (the text the HTML of the specification renders).
func TestMarkdownSpecExamples(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		// CommonMark: emphasis, example 350 and 351.
		{"emphasis 350", "*foo bar*", "- paragraph:\n  - emphasis: foo bar"},
		{"strong 360", "**foo bar**", "- paragraph:\n  - strong: foo bar"},
		// Backslash escapes, example 12: not emphasis.
		{"escape 12", "\\*not emphasized*", `- paragraph: "*not emphasized*"`},
		// Entity references, example 25 and numeric 26.
		{"entity 25", "&nbsp; &amp; &copy; &AElig; &Dcaron;", `- paragraph: "& © Æ Ď"`},
		{"entity 26", "&#35; &#1234; &#992; &#0;", `- paragraph: "# Ӓ Ϡ �"`},
		// Code spans, examples 328 and 329.
		{"code span 328", "`foo`", "- paragraph:\n  - code: foo"},
		{"code span 329", "`` foo ` bar ``", "- paragraph:\n  - code: \"foo ` bar\""},
		// Setext heading, example 51 and the ATX one, 62.
		{"setext 51", "Foo *bar*\n=========\n\nFoo *bar*\n---------", "- heading \"Foo bar\" [level=1]:\n  - text: Foo\n  - emphasis: bar\n- heading \"Foo bar\" [level=2]:\n  - text: Foo\n  - emphasis: bar"},
		{"atx 62", "# foo\n## foo\n### foo\n#### foo\n##### foo\n###### foo", "- heading \"foo\" [level=1]\n- heading \"foo\" [level=2]\n- heading \"foo\" [level=3]\n- heading \"foo\" [level=4]\n- heading \"foo\" [level=5]\n- heading \"foo\" [level=6]"},
		// Seven hashes are not a heading, example 63.
		{"atx 63", "####### foo", "- paragraph: \"####### foo\""},
		// Ordered lists keep their start, examples 304 and 305 (the number
		// is a marker: it is not in the text).
		{"list 304", "123. a\n124. b", "- list:\n  - listitem:\n    - paragraph: a\n  - listitem:\n    - paragraph: b"},
		// Soft and hard line breaks, examples 645, 634 and 640.
		{"soft break 645", "foo\nbaz", "- paragraph: foo baz"},
		{"hard break 634", "foo  \nbaz", "- paragraph: foo baz"},
		{"hard break 640", "foo\\\nbaz", "- paragraph: foo baz"},
		// Raw HTML: a block (example 148) and inline (example 612), as text.
		{"html block 148", "<table><tr><td>\n<pre>\n**Hello**,\n\n_world_.\n</pre>\n</td></tr></table>", "- paragraph: <table><tr><td> <pre> **Hello**,\n- paragraph:\n  - emphasis: world\n  - text: . </pre>\n- paragraph: </td></tr></table>"},
		{"html inline 612", "<a><bab><c2c>", "- paragraph: <a><bab><c2c>"},
		// Autolinks, example 596 and GFM's extended autolinks 624 and 625.
		{"autolink 596", "<http://foo.bar.baz/test?q=hello&id=22&boolean>", "- paragraph:\n  - link \"http://foo.bar.baz/test?q=hello&id=22&boolean\":\n    - /url: http://foo.bar.baz/test?q=hello&id=22&boolean"},
		{"autolink gfm 622", "www.commonmark.org", "- paragraph:\n  - link \"www.commonmark.org\":\n    - /url: http://www.commonmark.org"},
		// GFM strikethrough, example 491, and task list items, 279 and 280.
		{"strikethrough 491", "~~Hi~~ Hello, ~there~ world!", "- paragraph:\n  - deletion: Hi\n  - text: Hello,\n  - deletion: there\n  - text: world!"},
		{"task 279", "- [ ] foo\n- [x] bar", "- list:\n  - listitem:\n    - checkbox \"foo\" [disabled]\n    - paragraph: foo\n  - listitem:\n    - checkbox \"bar\" [checked] [disabled]\n    - paragraph: bar"},
		// GFM table, example 198.
		{"table 198", "| foo | bar |\n| --- | --- |\n| baz | bim |", "- table:\n  - row \"foo bar\":\n    - columnheader \"foo\"\n    - columnheader \"bar\"\n  - row \"baz bim\":\n    - cell \"baz\"\n    - cell \"bim\""},
		// A table with an escaped pipe and inline code, example 200.
		{"table 200", "| f\\|oo  |\n| ------ |\n| b `\\|` az |", "- table:\n  - row \"f|oo\":\n    - columnheader \"f|oo\"\n  - row \"b | az\":\n    - cell \"b | az\":\n      - text: b\n      - code: \"|\"\n      - text: az"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := mdMount(t, kit.Markdown{Source: c.src})
			a.ExpectAria(c.want)
		})
	}
}

// TestMarkdownLineBreaks checks the layout of the breaks: a hard break
// starts a line, a soft one is a space.
func TestMarkdownLineBreaks(t *testing.T) {
	t.Parallel()
	height := func(src string) float32 {
		a := mdMount(t, kit.Markdown{Source: src})
		return a.Bounds(telatest.ByRole("paragraph", "")).Height
	}
	soft, hard, backslash := height("one\ntwo"), height("one  \ntwo"), height("one\\\ntwo")
	if hard < 2*soft-1 || backslash != hard {
		t.Errorf("paragraph heights: soft break %v, hard break %v, backslash break %v; want the hard ones two lines high", soft, hard, backslash)
	}
}

// TestMarkdownTaskToggle: with OnToggleTask a task is a checkbox that asks
// for the change, with the index of the task in document order, and the
// document follows the Source the app edits.
func TestMarkdownTaskToggle(t *testing.T) {
	t.Parallel()
	type toggle struct {
		index   int
		checked bool
	}
	var got []toggle
	src := "- [ ] first\n- [x] second\n  - [ ] third\n\n> - [ ] fourth\n"
	app := telatest.MountFunc(t, func() tela.Node {
		source, setSource := tela.UseState(src)
		return kit.Markdown{Source: source, OnToggleTask: func(i int, on bool) {
			got = append(got, toggle{i, on})
			lines := strings.Split(source, "\n")
			for n, l := range lines {
				if strings.Contains(l, "[") && strings.Count(strings.Join(lines[:n+1], "\n"), "[") == i+1 {
					if on {
						lines[n] = strings.Replace(l, "[ ]", "[x]", 1)
					} else {
						lines[n] = strings.Replace(l, "[x]", "[ ]", 1)
					}
				}
			}
			setSource.Set(strings.Join(lines, "\n"))
		}}
	}, telatest.Size(400, 300), telatest.Theme(kit.Theme()))
	app.ExpectAria(`
		- list:
		  - listitem:
		    - checkbox "first"
		    - paragraph: first
		  - listitem:
		    - checkbox "second" [checked]
		    - paragraph: second
		    - list:
		      - listitem:
		        - checkbox "third"
		        - paragraph: third
		- blockquote:
		  - list:
		    - listitem:
		      - checkbox "fourth"
		      - paragraph: fourth`)
	app.Click(telatest.ByRole("checkbox", "third"))
	app.Click(telatest.ByRole("checkbox", "second"))
	app.Click(telatest.ByRole("checkbox", "fourth"))
	app.Click(telatest.ByRole("checkbox", "first"))
	want := []toggle{{2, true}, {1, false}, {3, true}, {0, true}}
	if len(got) != len(want) {
		t.Fatalf("toggles %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("toggle %d is %v, want %v", i, got[i], want[i])
		}
	}
	app.Expect(telatest.ByRole("checkbox", "third"), telatest.Checked(true))
	app.Expect(telatest.ByRole("checkbox", "second"), telatest.Checked(false))
	app.Expect(telatest.ByRole("checkbox", "fourth"), telatest.Checked(true))

	// Without OnToggleTask the boxes are read-only: a click changes nothing.
	ro := mdMount(t, kit.Markdown{Source: "- [ ] a\n- [x] b\n"})
	ro.ExpectAria(`
		- list:
		  - listitem:
		    - checkbox "a" [disabled]
		    - paragraph: a
		  - listitem:
		    - checkbox "b" [checked] [disabled]
		    - paragraph: b`)
}

// TestMarkdownImages: a destination without a scheme is a file of the
// app's assets; Tela fetches nothing, so every other one shows its alt text.
func TestMarkdownImages(t *testing.T) {
	t.Parallel()
	a := mdMount(t, kit.Markdown{Source: "![Logo of *Tela*](img/logo.png \"The logo\")\n\n" +
		"![remote](https://example.org/a.png)\n\n" +
		"![scheme-relative](//example.org/a.png)\n\n" +
		"![outside](../secret.png)\n\n" +
		"![inline](data:image/png;base64,AAAA) and ![](http://example.org/no-alt.png) end\n\n" +
		"![local](./img/logo.png)\n"})
	a.ExpectAria(`
		- paragraph:
		  - img "Logo of Tela"
		- paragraph: remote
		- paragraph: scheme-relative
		- paragraph: outside
		- paragraph: inline and end
		- paragraph:
		  - img "local"`)
	if got := a.Count(telatest.ByRole("img", "")); got != 2 {
		t.Errorf("%d images on screen, want the two of the assets", got)
	}
	// The picture is drawn: its box is as large as the image, 48 by 32.
	b := a.Bounds(telatest.ByRole("img", "Logo of Tela"))
	if b.Width != 48 || b.Height != 32 {
		t.Errorf("the logo is %vx%v, want 48x32", b.Width, b.Height)
	}
}

// TestMarkdownSelection: the text is selectable, and a selection across
// blocks copies a blank line after a paragraph or a heading (the rule of
// ADR-271: its bottom margin is at least half its font size) and one line
// break between the items of a list, the cells of a table and the blocks
// that follow them.
func TestMarkdownSelection(t *testing.T) {
	t.Parallel()
	a := mdMount(t, kit.Markdown{Source: "# Title\n\nFirst paragraph with a [link](https://x.org) and `code`.\n\n- one\n- two\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\nLast.\n"})
	a.Click(telatest.ByRole("heading", "Title"))
	a.Press("Control+a")
	want := "Title\n\nFirst paragraph with a link and code.\n\none\ntwo\na\nb\n1\n2\nLast."
	if got := a.SelectedText(); got != want {
		t.Errorf("selected text %q, want %q", got, want)
	}
}

// TestMarkdownCodeBlockKeepsWhitespace: the lines of a code block are laid
// out as they are, and a line wider than the block scrolls it sideways
// instead of wrapping.
func TestMarkdownCodeBlockKeepsWhitespace(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("0123456789", 12)
	a := mdMount(t, kit.Markdown{Source: "```\nshort\n        indented\n\n" + long + "\n```\n"})
	box := a.Bounds(telatest.ByRole("code", ""))
	if box.Width < 600 {
		t.Errorf("the code is %v px wide: a 120-character line should make it wider than the 448 px block", box.Width)
	}
	// Four lines, the blank one included, and the leading spaces count.
	if want := float32(4 * 22); box.Height < want {
		t.Errorf("the code is %v px high, want at least %v for its four lines", box.Height, want)
	}
	if !strings.Contains(a.AriaSnapshot(), "short") {
		t.Errorf("the code block's text is missing:\n%s", a.AriaSnapshot())
	}
}

// TestMarkdownFollowsTheStyle: the look is a recipe; a StyleProvider
// changes it for a subtree and a Class on the component adds to the root.
func TestMarkdownFollowsTheStyle(t *testing.T) {
	t.Parallel()
	style := kit.DefaultStyle()
	style.Markdown.H1 = "text-6xl font-black"
	a := telatest.MountFunc(t, func() tela.Node {
		return kit.StyleProvider{Style: &style, Children: kit.Markdown{Source: "# Big\n\ntext", Class: "custom-root"}}
	}, telatest.Size(480, 300), telatest.Theme(kit.Theme()), telatest.AllowDev("tela: unknown class"))
	if got := a.Count(telatest.ByClass("text-6xl")); got != 1 {
		t.Errorf("%d elements with the recipe's H1 class, want 1", got)
	}
	if got := a.Count(telatest.ByClass("custom-root")); got != 1 {
		t.Errorf("%d elements with the instance's Class, want 1", got)
	}
}

// TestMarkdownLargeDocument: a long document mounts and renders each of its
// blocks once.
func TestMarkdownLargeDocument(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	for i := range 400 {
		sb.WriteString("## Section\n\nA paragraph with *emphasis* and a [link](https://x.org).\n\n- item\n- [ ] task\n\n")
		_ = i
	}
	a := mdMount(t, kit.Markdown{Source: sb.String()}, telatest.Size(480, 800))
	if got := a.Count(telatest.ByRole("heading", "Section")); got != 400 {
		t.Errorf("%d headings, want 400", got)
	}
	if got := a.Count(telatest.ByRole("checkbox", "task")); got != 400 {
		t.Errorf("%d tasks, want 400", got)
	}
}

// TestMarkdownGoldens renders a representative document in both color
// schemes at scales 1 and 2:
// `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestMarkdownGoldens ./kit`
// writes the pictures, which are looked at before they are committed.
func TestMarkdownGoldens(t *testing.T) {
	t.Parallel()
	for _, scheme := range []string{"light", "dark"} {
		t.Run(scheme, func(t *testing.T) {
			t.Parallel()
			opts := []telatest.Option{telatest.Size(640, 1130), telatest.Theme(kit.Theme()),
				telatest.RootClass("bg-background p-6 text-foreground")}
			if scheme == "dark" {
				opts = append(opts, telatest.Dark())
			}
			a := telatest.MountFunc(t, func() tela.Node {
				return h.Div(h.Class("max-w-[592px]"), kit.Markdown{Source: mdDocument})
			}, opts...)
			a.GoldenScales("markdown_"+scheme, 1, 2)
		})
	}
}
