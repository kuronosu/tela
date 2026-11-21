package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuronosu/tela/internal/lint"
)

// Quick fixes, and the helpers that let the server know more class strings
// than the syntax shows: the module's other packages as context, and spans
// of constants that follow an edit.

// lspCodeAction is a code action of the protocol.
type lspCodeAction struct {
	Title       string            `json:"title"`
	Kind        string            `json:"kind"`
	IsPreferred bool              `json:"isPreferred,omitempty"`
	Diagnostics []lspDiagnostic   `json:"diagnostics,omitempty"`
	Edit        *lspWorkspaceEdit `json:"edit,omitempty"`
}

type lspWorkspaceEdit struct {
	Changes map[string][]lspTextEdit `json:"changes"`
}

// classSuggestion returns the class a finding of the class rule proposes in
// place of the one written, "" for any other finding: an unknown class or an
// invalid value, with the suggestion of its message ("did you mean"). The
// suggestion of an unknown variant is the variant, not a class.
func classSuggestion(d lint.Diagnostic) string {
	if d.Rule != lint.RuleClass || d.Suggestion == "" {
		return ""
	}
	if !strings.HasPrefix(d.Message, "unknown class ") && !strings.HasPrefix(d.Message, "invalid class ") {
		return ""
	}
	return d.Suggestion
}

// codeAction serves textDocument/codeAction: one quick fix for each finding
// of the last run that the range touches, when the run read the text the
// editor has now.
func (s *lspServer) codeAction(params json.RawMessage) (any, error) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Range   lspRange `json:"range"`
		Context struct {
			Only []string `json:"only"`
		} `json:"context"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, badParams(err)
	}
	actions := []lspCodeAction{}
	if len(p.Context.Only) > 0 && !wantsKind(p.Context.Only, "quickfix") {
		return actions, nil
	}
	s.mu.Lock()
	doc := s.docs[p.TextDocument.URI]
	fixes := s.fixes[p.TextDocument.URI]
	s.mu.Unlock()
	if doc == nil || fixes.version != doc.version {
		return actions, nil
	}
	from, to := doc.offset(p.Range.Start), doc.offset(p.Range.End)
	for _, f := range fixes.list {
		start, end := doc.offset(f.diag.Range.Start), doc.offset(f.diag.Range.End)
		// The cursor at either end of the class counts as on it.
		if to < start || from > end {
			continue
		}
		actions = append(actions, lspCodeAction{
			Title: "Replace with " + f.suggestion, Kind: "quickfix", IsPreferred: true,
			Diagnostics: []lspDiagnostic{f.diag},
			Edit: &lspWorkspaceEdit{Changes: map[string][]lspTextEdit{
				doc.uri: {{Range: f.diag.Range, NewText: f.suggestion}},
			}},
		})
	}
	return actions, nil
}

// wantsKind reports whether a client that asks for the kinds only wants
// kind: it names it or one of its parents ("" is every kind).
func wantsKind(only []string, kind string) bool {
	for _, k := range only {
		if k == "" || k == kind || strings.HasPrefix(kind, k+".") {
			return true
		}
	}
	return false
}

// lspModuleContext is moduleContext; a test points it at a folder of
// fixtures, which the go command's "..." skips as testdata.
var lspModuleContext = moduleContext

// moduleContext returns the context patterns of a lint run of dir: the
// whole module the directory is in, found by the go.mod above it, as a
// pattern relative to dir. It is nil when there is no go.mod.
func moduleContext(dir string) []string {
	root := dir
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return nil
		}
		root = parent
	}
	rel, err := filepath.Rel(dir, root)
	if err != nil {
		return nil
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return []string{"./..."}
	}
	return []string{rel + "/..."}
}

// shiftSpan maps a span of the text the last lint run read (old) to the text
// the editor has now (cur): spans outside the edited region keep their
// text, the ones that hold it grow or shrink with it. ok is false when the
// edit overlaps an end of the span, so that it may not be the same string.
func shiftSpan(old, cur string, start, end int) (int, int, bool) {
	prefix := 0
	for prefix < len(old) && prefix < len(cur) && old[prefix] == cur[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(cur)-prefix && old[len(old)-1-suffix] == cur[len(cur)-1-suffix] {
		suffix++
	}
	delta := len(cur) - len(old)
	// The edit replaced old[prefix:len(old)-suffix].
	editEnd := len(old) - suffix
	// The first character of the literal and the one before its end must
	// be outside the edited region.
	mapOff := func(off int, isEnd bool) (int, bool) {
		if isEnd {
			switch {
			case off <= prefix:
				return off, true
			case off > editEnd:
				return off + delta, true
			}
			return 0, false
		}
		switch {
		case off < prefix:
			return off, true
		case off >= editEnd:
			return off + delta, true
		}
		return 0, false
	}
	ns, ok1 := mapOff(start, false)
	ne, ok2 := mapOff(end, true)
	if !ok1 || !ok2 || ne < ns || ne > len(cur) {
		return 0, 0, false
	}
	// A shifted literal must still be one: it starts with its quote.
	if ns >= len(cur) || (cur[ns] != '"' && cur[ns] != '`') {
		return 0, 0, false
	}
	return ns, ne, true
}
