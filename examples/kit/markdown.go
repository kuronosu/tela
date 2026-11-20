package main

import (
	"embed"
	"regexp"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

// The page of Markdown. The documents are data, in English whatever the
// language of the window: a document is the app's content, not the
// gallery's text. One image comes from the app's assets (assets/aurora.png).

//go:embed assets
var assets embed.FS

func init() { tela.Assets(assets) }

const readmeDoc = "# Tela notes\n" +
	"\n" +
	"A **pure-Go** desktop UI framework with *React-like* hooks, ~~no cgo~~ and `Tailwind` classes.\n" +
	"Read the [docs](https://telaui.dev/docs) or visit https://telaui.dev.\n" +
	"\n" +
	"## Getting started\n" +
	"\n" +
	"Install the tool, then run your first app:\n" +
	"\n" +
	"```go\n" +
	"func main() {\n" +
	"\ttela.Main(tela.App{Title: \"Hello\", Root: Hello{}})\n" +
	"}\n" +
	"```\n" +
	"\n" +
	"> Tela fetches nothing over the network.\n" +
	"> Images come from the app's own assets.\n" +
	"\n" +
	"### What is in it\n" +
	"\n" +
	"1. A layout engine\n" +
	"   - flex, grid and block\n" +
	"   - text with shaping and bidi\n" +
	"2. A style system\n" +
	"3. A kit of components\n" +
	"\n" +
	"---\n" +
	"\n" +
	"Press **Ctrl+A** and copy: the text of every block comes with it.\n"

const tasksDoc = "## Release checklist\n" +
	"\n" +
	"- [x] Write the code\n" +
	"- [x] Review the golden pictures\n" +
	"- [ ] Update the changelog\n" +
	"  - [ ] Add the new component\n" +
	"  - [ ] Mention the breaking changes\n" +
	"- [ ] Tag the release\n"

const tableDoc = "| Component | Kind | Lines |\n" +
	"| :-------- | :--: | ----: |\n" +
	"| `Button`  | control | 210 |\n" +
	"| `Table`   | data | 1240 |\n" +
	"| `Markdown` | content | 480 |\n" +
	"\n" +
	"```\n" +
	"A code block keeps its whitespace and scrolls sideways when a line is wider than the page: 0123456789 0123456789 0123456789 0123456789 0123456789\n" +
	"    indented line\n" +
	"```\n"

const imageDoc = "![An aurora, from the app's assets](assets/aurora.png \"assets/aurora.png\")\n" +
	"\n" +
	"The picture above is a file of `tela.Assets`. The next one is a remote URL, which Tela never fetches, so its alt text is shown instead: ![the Tela logo](https://telaui.dev/logo.png)\n"

const rawDoc = "A paragraph with <b>inline HTML</b> and entities: &copy; &amp; &lt;tag&gt;.\n" +
	"\n" +
	"<div class=\"note\">\n" +
	"A block of HTML is text, too: nothing in a document runs or loads.\n" +
	"</div>\n"

// taskLine matches the start of a task-list item.
var taskLine = regexp.MustCompile(`^(\s*(?:[-*+]|\d+[.)])\s+)\[[ xX]\]`)

// toggleTask returns src with the index-th task checked or unchecked, the
// edit OnToggleTask asks the app for.
func toggleTask(src string, index int, checked bool) string {
	lines := strings.Split(src, "\n")
	n := 0
	for i, l := range lines {
		m := taskLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		if n == index {
			mark := " "
			if checked {
				mark = "x"
			}
			lines[i] = m[1] + "[" + mark + "]" + l[len(m[0]):]
			break
		}
		n++
	}
	return strings.Join(lines, "\n")
}

func markdownPage() tela.Node {
	tr := useTr()
	tasks, setTasks := tela.UseState(tasksDoc)
	link, setLink := tela.UseState("")
	done := strings.Count(tasks, "[x]")
	total := done + strings.Count(tasks, "[ ]")
	linkText := tr("markdown.noLink", "No link pressed yet.")
	if link != "" {
		linkText = tr("markdown.link", "Link pressed: {href}", "href", link)
	}
	return grid(
		demo("A document", "Headings, emphasis, code, a blockquote, nested lists and a thematic break, in the theme's tokens: it follows the light and dark schemes and the style. The text is selectable.", col,
			kit.Markdown{Source: readmeDoc, OnLink: func(href string) { setLink.Set(href) }}),
		demo("Tasks", "A task-list item is a checkbox. With OnToggleTask the document asks the app for the change, with the task's index; the app edits Source.", col,
			kit.Markdown{Source: tasks, OnToggleTask: func(i int, on bool) { setTasks.Update(func(s string) string { return toggleTask(s, i, on) }) }},
			readout(tr("markdown.done", "{done} of {total} tasks done", "done", done, "total", total))),
		demo("Links", "A press or Enter calls OnLink with the destination. Without OnLink it is opened with the desktop (tela.OpenURL).", col,
			kit.Markdown{Source: "Visit the [documentation](https://telaui.dev/docs), the autolink https://telaui.dev or write to <hello@telaui.dev>. A [relative link](notes/1) goes to OnLink as it is.",
				OnLink: func(href string) { setLink.Set(href) }},
			readout(linkText)),
		demo("Tables and code", "Tables take the alignment of their columns. A code block is monospace, keeps its whitespace and scrolls sideways.", col,
			kit.Markdown{Source: tableDoc}),
		demo("Images", "Only the files of tela.Assets are images; any other source shows its alt text.", col,
			kit.Markdown{Source: imageDoc, Class: "max-w-md"}),
		demo("Raw HTML", "HTML in a document is shown as the text it is, and entities are decoded.", col,
			kit.Markdown{Source: rawDoc}),
	)
}
