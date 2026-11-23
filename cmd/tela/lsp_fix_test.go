package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// codeActions asks for the code actions of a range.
func (c *lspClient) codeActions(uri string, r lspRange, diags []lspDiagnostic) []lspCodeAction {
	c.t.Helper()
	if diags == nil {
		diags = []lspDiagnostic{}
	}
	res := c.mustRequest("textDocument/codeAction", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"range":        r,
		"context":      map[string]any{"diagnostics": diags},
	})
	var out []lspCodeAction
	if err := json.Unmarshal(res, &out); err != nil {
		c.t.Fatalf("codeAction result %s: %v", res, err)
	}
	return out
}

// applyEdits applies the edits of one document to text.
func applyLSPEdits(t *testing.T, text string, edits []lspTextEdit) string {
	t.Helper()
	d := &lspDoc{text: text}
	// From the end, so that the offsets of the earlier ones hold.
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		text = text[:d.offset(e.Range.Start)] + e.NewText + text[d.offset(e.Range.End):]
	}
	return text
}

// An unknown class has a quick fix that replaces it with the suggestion.
func TestLSPQuickFix(t *testing.T) {
	t.Parallel()
	needGo(t) // it lints, which runs go list
	c := startLSP(t)
	caps := c.initialize()["capabilities"].(map[string]any)
	if caps["codeActionProvider"] == nil {
		t.Fatalf("capabilities = %v, want a codeActionProvider", caps)
	}
	_, uri := bufferFor(t, "quickfix.go")
	text := lspBufferHead + `func Buffer() tela.Node {
	return Div(Class("flex hover:bg-rde-500 rounded-xxl"), Text("x"))
}
`
	c.open(uri, text)
	diags := c.diagnostics(uri, func(d []lspDiagnostic) bool { return len(d) == 2 })

	for _, d := range diags {
		// The range of the diagnostic itself, and a cursor inside it.
		for _, r := range []lspRange{d.Range, {Start: d.Range.Start, End: d.Range.Start}} {
			acts := c.codeActions(uri, r, []lspDiagnostic{d})
			if len(acts) != 1 {
				t.Fatalf("code actions for %q = %+v, want one", d.Message, acts)
			}
			a := acts[0]
			if a.Kind != "quickfix" || !a.IsPreferred || !strings.HasPrefix(a.Title, "Replace with ") {
				t.Errorf("action = %+v, want a preferred quickfix titled \"Replace with ...\"", a)
			}
			got := applyLSPEdits(t, text, a.Edit.Changes[uri])
			switch {
			case strings.Contains(d.Message, "bg-rde-500"):
				if want := strings.Replace(text, "hover:bg-rde-500", "hover:bg-red-500", 1); got != want {
					t.Errorf("the fix of bg-rde-500 gives\n%s\nwant\n%s", got, want)
				}
			default:
				if want := strings.Replace(text, "rounded-xxl", "rounded-2xl", 1); got != want {
					t.Errorf("the fix of rounded-xxl gives\n%s\nwant\n%s", got, want)
				}
			}
			if len(a.Diagnostics) != 1 {
				t.Errorf("the action should name the diagnostic it fixes: %+v", a.Diagnostics)
			}
		}
	}

	// Away from the findings, and for a client that asks for another kind:
	// nothing.
	away := lspRange{Start: lspPosition{Line: 0, Character: 0}, End: lspPosition{Line: 0, Character: 3}}
	if acts := c.codeActions(uri, away, nil); len(acts) != 0 {
		t.Errorf("code actions away from a finding = %+v, want none", acts)
	}
	res := c.mustRequest("textDocument/codeAction", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"range":        diags[0].Range,
		"context":      map[string]any{"diagnostics": diags, "only": []string{"refactor"}},
	})
	if string(res) != "[]" && string(res) != "null" {
		t.Errorf("code actions of kind refactor = %s, want none", res)
	}

	// An edit makes the findings stale: no action offers an edit of old
	// offsets.
	c.change(uri, 2, "// moved\n"+text)
	if acts := c.codeActions(uri, diags[0].Range, nil); len(acts) != 0 {
		t.Errorf("code actions on a changed buffer before the next run = %+v, want none", acts)
	}
	// The run that follows has the new ones.
	nd := c.diagnostics(uri, func(d []lspDiagnostic) bool {
		return len(d) == 2 && d[0].Range.Start.Line == diags[0].Range.Start.Line+1
	})
	if acts := c.codeActions(uri, nd[0].Range, nil); len(acts) != 1 {
		t.Errorf("code actions after the run = %+v, want one", acts)
	}
}

// A class string held in a constant is a class string where the constant is
// declared (hover, completion), also in the first moments after an edit, and
// its findings are reported there.
func TestLSPConstantClassStrings(t *testing.T) {
	t.Parallel()
	needGo(t) // it lints, which runs go list
	c := startLSP(t)
	c.initialize()
	_, uri := bufferFor(t, "consts.go")
	body := "const card = \"flex rounded-|lg p-4\"\n\nfunc Buffer() tela.Node { return Div(Class(card)) }\n"
	text, pos := withCursor(t, lspBufferHead+body)
	c.open(uri, text)
	if md := c.hover(uri, pos); md != "" {
		t.Errorf("hover before the lint run = %q, want none: nothing says card is a class string", md)
	}
	c.diagnostics(uri, func([]lspDiagnostic) bool { return true })
	if md := c.hover(uri, pos); !strings.Contains(md, "border-radius: 0.5rem;") {
		t.Errorf("hover over a constant's class after the run:\n%s", md)
	}
	// Typing in the constant: completion answers before the next run.
	typed, tpos := withCursor(t, lspBufferHead+strings.Replace(body, "rounded-|lg", "rounded-lg bg-|", 1))
	c.change(uri, 2, typed)
	list := c.completion(uri, tpos)
	if list == nil || len(list.Items) == 0 {
		t.Fatalf("no completion in a constant's class string right after an edit")
	}

	// The finding is at the constant, with the class's range.
	broken := strings.ReplaceAll(strings.Replace(text, "rounded-lg", "rounded-xxl", 1), "|", "")
	c.change(uri, 3, broken)
	// The run of version 2 (its "bg-" is unknown too) may publish after
	// this change: wait for the run of this text, whose one finding is
	// rounded-xxl's.
	doc := &lspDoc{text: broken}
	diags := c.diagnostics(uri, func(d []lspDiagnostic) bool {
		return len(d) == 1 && broken[doc.offset(d[0].Range.Start):doc.offset(d[0].Range.End)] == "rounded-xxl"
	})
	if got := broken[doc.offset(diags[0].Range.Start):doc.offset(diags[0].Range.End)]; got != "rounded-xxl" {
		t.Errorf("the finding selects %q, want rounded-xxl in the constant", got)
	}
	if diags[0].Range.Start.Line != strings.Count(lspBufferHead, "\n") {
		t.Errorf("the finding is on line %d, want the constant's", diags[0].Range.Start.Line)
	}
}

// Aliases that another package of the module declares are the buffer's.
func TestLSPAliasesOfAnotherPackage(t *testing.T) {
	needGo(t) // it lints, which runs go list
	// The go command's "..." skips testdata folders: the module of the
	// fixture is its folder.
	fixtures, err := filepath.Abs(filepath.Join("testdata", "lspctx"))
	if err != nil {
		t.Fatal(err)
	}
	lspModuleContext = func(string) []string { return []string{filepath.ToSlash(fixtures) + "/..."} }
	t.Cleanup(func() { lspModuleContext = moduleContext })
	c := startLSP(t)
	c.initialize()
	dir, err := filepath.Abs(filepath.Join("testdata", "lspctx", "ui"))
	if err != nil {
		t.Fatal(err)
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	uri := fileURI(filepath.Join(dir, "ui.go"))
	text := `package ui

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func Card() tela.Node { return Div(Class("ctx-card flex")) }
`
	c.open(uri, text)
	if d := c.diagnostics(uri, func([]lspDiagnostic) bool { return true }); len(d) != 0 {
		t.Fatalf("diagnostics = %+v, want none: ctx-card is an alias that package theme declares", d)
	}
	hov, pos := withCursor(t, strings.Replace(text, "ctx-card", "ctx-|card", 1))
	c.change(uri, 2, hov)
	if md := c.hover(uri, pos); !strings.Contains(md, "is an alias for `rounded-xl border p-4`") {
		t.Errorf("hover over an alias of another package:\n%s", md)
	}
}

func TestModuleContext(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "ui", "cards")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := moduleContext(sub); got != nil {
		t.Errorf("without a go.mod: %v, want none", got)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := moduleContext(sub); len(got) != 1 || got[0] != "../../..." {
		t.Errorf("from ui/cards: %v, want ../../...", got)
	}
	if got := moduleContext(root); len(got) != 1 || got[0] != "./..." {
		t.Errorf("from the root: %v, want ./...", got)
	}
}

func TestShiftSpan(t *testing.T) {
	const old = "a := \"flex p-4\"\nb := 1\n"
	// The literal is old[5:15].
	tests := []struct {
		name       string
		cur        string
		start, end int
		ok         bool
	}{
		{"unchanged", old, 5, 15, true},
		{"typed inside", "a := \"flex p-4 gap-\"\nb := 1\n", 5, 20, true},
		{"deleted inside", "a := \"flex\"\nb := 1\n", 5, 11, true},
		{"inserted above", "// x\n" + old, 10, 20, true},
		{"edited below", old + "c := 2\n", 5, 15, true},
		{"quote deleted", "a := flex p-4\"\nb := 1\n", 0, 0, false},
		{"closing quote deleted", "a := \"flex p-4\nb := 1\n", 0, 0, false},
		{"inserted right before", "a := x\"flex p-4\"\nb := 1\n", 6, 16, true},
	}
	for _, tc := range tests {
		s, e, ok := shiftSpan(old, tc.cur, 5, 15)
		if ok != tc.ok || (ok && (s != tc.start || e != tc.end)) {
			t.Errorf("%s: got %d,%d,%v want %d,%d,%v", tc.name, s, e, ok, tc.start, tc.end, tc.ok)
		}
		if ok && (tc.cur[s] != '"' || tc.cur[e-1] != '"') {
			t.Errorf("%s: the span %q is not a literal", tc.name, tc.cur[s:e])
		}
	}
}
