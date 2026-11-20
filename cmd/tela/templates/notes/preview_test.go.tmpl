package main

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

var (
	previewTab = telatest.ByRole("radio", "Preview")
	editTab    = telatest.ByRole("radio", "Edit")
	splitTab   = telatest.ByRole("radio", "Side by side")
	previewBox = telatest.ByRole("region", "Preview")
	task       = telatest.ByRole("checkbox", "")
)

// done counts the tasks that are checked, in the aria snapshot.
func done(a *telatest.App) int {
	n := 0
	for line := range strings.SplitSeq(a.AriaSnapshot(), "\n") {
		if strings.Contains(line, "- checkbox") && strings.HasSuffix(line, "[checked]") {
			n++
		}
	}
	return n
}

// todoText is a note with tasks, a link to another note by number and by
// title, a link to a missing note and one to the web.
const todoText = "Todo\n\n- [ ] ship it\n- [x] write it\n\n```\n- [ ] not a task, it is code\n```\n\n" +
	"- [ ] call Ana\n\nSee [the groceries](notes:3), [the weekend](<Weekend in the mountains>), " +
	"[nothing](notes:99) and [the docs](https://example.com/docs)."

func TestPreviewRendersTheNote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Expect(editTab, telatest.Checked(true))
	a.ExpectCount(previewBox, 0)

	a.Click(previewTab)
	a.ExpectCount(previewBox, 1)
	a.ExpectCount(editor, 0) // the text field is gone: this is the document
	a.ExpectRole("heading", "Welcome to Notes")
	a.ExpectText("Everything you type is kept in your library")
	a.Expect(previewTab, telatest.Checked(true))

	// The choice is a setting: the next run opens the preview too.
	b := mount(t)
	b.ExpectCount(previewBox, 1)
	b.Click(editTab)
	b.ExpectValue(editor, welcomeText)
}

// Checking a task in the preview edits the source of the note.
func TestPreviewTaskToggleEditsTheSource(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, todoText)
	a.Click(previewTab)
	a.ExpectCount(task, 3) // the line in the code block is no task
	if n := done(a); n != 1 {
		t.Fatalf("%d tasks are checked, want 1", n)
	}

	a.Click(task.Nth(0)) // ship it
	a.WaitFor("the task to be checked", func() bool { return done(a) == 2 })
	a.Click(task.Nth(0)) // and again
	a.WaitFor("the task to be unchecked", func() bool { return done(a) == 1 })
	a.Click(task.Nth(0))
	a.WaitFor("the task to be checked", func() bool { return done(a) == 2 })
	a.Click(editTab)
	want := "Todo\n\n- [x] ship it\n- [x] write it\n\n```\n- [ ] not a task, it is code\n```\n\n" + todoText[len("Todo\n\n- [ ] ship it\n- [x] write it\n\n```\n- [ ] not a task, it is code\n```\n\n"):]
	a.ExpectValue(editor, want)
	a.ExpectRole("button", "Todo, unsaved changes")
}

// A link to another note opens it inside the app; the web goes to the
// browser.
func TestPreviewLinks(t *testing.T) {
	sandbox(t)
	a := mount(t)
	typeNote(a, todoText)
	a.Click(previewTab)

	a.Click(telatest.ByRole("link", "the groceries"))
	a.ExpectRole("heading", "Groceries")
	a.Click(telatest.ByRole("button", "Todo, unsaved changes"))
	a.ExpectCount(previewBox, 1)
	a.Click(telatest.ByRole("link", "the weekend")) // by title
	a.ExpectRole("heading", "Weekend in the mountains")
	a.Click(telatest.ByRole("button", "Todo, unsaved changes"))

	a.Click(telatest.ByRole("link", "nothing"))
	a.ExpectText("There is no note 99")
	a.ExpectCount(previewBox, 1) // still on the note

	a.Click(telatest.ByRole("link", "the docs"))
	if got := a.OpenedURLs(); len(got) != 1 || got[0] != "https://example.com/docs" {
		t.Errorf("OpenedURLs = %q, want the docs", got)
	}
}

// Wide enough, the text and its preview sit side by side, and the preview
// follows the typing.
func TestPreviewSplit(t *testing.T) {
	sandbox(t)
	a := mount(t, telatest.Size(1400, 800))
	a.Click(splitTab)
	a.ExpectCount(previewBox, 1)
	a.ExpectValue(editor, welcomeText)
	a.Click(editor)
	a.Type("\n\n- [ ] typed in the editor")
	a.ExpectCount(task, 1)

}

// Too narrow for it, the split is not offered, and a window that was shrunk
// while it was on shows the text alone.
func TestPreviewSplitNeedsRoom(t *testing.T) {
	sandbox(t)
	setSetting(t, keyEditorView, viewSplit)
	a := mount(t)
	a.ExpectCount(splitTab, 0)
	a.ExpectCount(previewBox, 0)
	a.ExpectValue(editor, welcomeText)
}

func TestToggleTask(t *testing.T) {
	cases := []struct {
		src     string
		i       int
		checked bool
		want    string
	}{
		{"- [ ] a\n- [ ] b", 1, true, "- [ ] a\n- [x] b"},
		{"- [x] a\n- [X] b", 1, false, "- [x] a\n- [ ] b"},
		{"1. [ ] a\n2) [ ] b", 1, true, "1. [ ] a\n2) [x] b"},
		{"> - [ ] quoted", 0, true, "> - [x] quoted"},
		{"```\n- [ ] code\n```\n- [ ] real", 0, true, "```\n- [ ] code\n```\n- [x] real"},
		{"~~~\n- [ ] code\n~~~\n- [ ] real", 0, true, "~~~\n- [ ] code\n~~~\n- [x] real"},
		{"- [ ] a", 3, true, "- [ ] a"},
		{"no tasks", 0, true, "no tasks"},
		{"- [ ]no space after is no task\n- [ ] a", 0, true, "- [ ]no space after is no task\n- [x] a"},
	}
	for _, c := range cases {
		if got := toggleTask(c.src, c.i, c.checked); got != c.want {
			t.Errorf("toggleTask(%q, %d, %v) = %q, want %q", c.src, c.i, c.checked, got, c.want)
		}
	}
}

func TestNoteRef(t *testing.T) {
	for href, want := range map[string]string{
		"notes:3":             "3",
		"notes://3":           "3",
		"notes:/3":            "3",
		"NOTES:Groceries":     "Groceries",
		"3":                   "3",
		"./3":                 "3",
		"/notes/3":            "3",
		"Groceries":           "Groceries",
		"Weekend%20in%20town": "Weekend in town",
		"https://example.com": "",
		"mailto:ana@x.test":   "",
		"#top":                "",
		"tela-notes://open":   "",
		"":                    "",
	} {
		if got := noteRef(href); got != want {
			t.Errorf("noteRef(%q) = %q, want %q", href, got, want)
		}
	}
}
