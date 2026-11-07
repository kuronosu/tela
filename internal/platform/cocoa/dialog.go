package cocoa

import (
	"path"
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the file dialogs and message boxes (dialog_darwin.go
// shows them), tested on every OS: how the contract's file filters become
// an NSOpenPanel's or NSSavePanel's allowed content types, and how a
// message box becomes an NSAlert.
//
// Filters. A pattern "*.ext" (one extension, no other glob character) is
// the UTType of the extension (+[UTType typeWithFilenameExtension:], a
// dynamic type when the system declares none), which is what the panels
// filter by: an open panel enables only the files of those types, a save
// panel appends the first type's extension and offers to switch to the
// others. "*" is all files. Any other pattern ("README*", "*.tar.gz",
// "photo?.png") cannot be a content type, so:
//
//   - in an open panel, a filter with such a pattern filters through the
//     panel's delegate instead (panel:shouldEnableURL:, every pattern of
//     the filter matched against the file name without regard to case, as
//     the default macOS file system compares names; folders stay enabled
//     so the user can browse);
//   - in a save panel, such patterns are left out: the user types the
//     name, which a pattern cannot complete. A filter left with no
//     extension allows any name.
//
// Several filters get a "Format:" popup in the panel's accessory view, as
// Chromium does, with the first one selected; choosing another one
// changes the panel's types (and the delegate's filter).

// panelFilter is a FileFilter as the panels use it.
type panelFilter struct {
	label string
	exts  []string // "*.ext" patterns: lower-case extensions without the dot, unique, in order
	globs []string // other patterns, lower case
	all   bool     // a "*" pattern: every file
}

// panelFilters converts the contract's filters (already checked: no
// pattern is empty or holds ';' or NUL). A filter without a name is
// labeled with its patterns.
func panelFilters(filters []platform.FileFilter) []panelFilter {
	out := make([]panelFilter, len(filters))
	for i, f := range filters {
		pf := panelFilter{label: f.Name}
		if pf.label == "" {
			pf.label = strings.Join(f.Patterns, ", ")
		}
		for _, p := range f.Patterns {
			p = strings.ToLower(p)
			if p == "*" || p == "*.*" {
				pf.all = true
			} else if ext, ok := simpleExtension(p); ok {
				if !slices.Contains(pf.exts, ext) {
					pf.exts = append(pf.exts, ext)
				}
			} else if !slices.Contains(pf.globs, p) {
				pf.globs = append(pf.globs, p)
			}
		}
		out[i] = pf
	}
	return out
}

// simpleExtension returns the extension of a pattern "*.ext" whose
// extension is one name component without glob characters.
func simpleExtension(p string) (string, bool) {
	ext, ok := strings.CutPrefix(p, "*.")
	if !ok || ext == "" || strings.ContainsAny(ext, "*?[]\\/.") {
		return "", false
	}
	return ext, true
}

// openTypes returns the extensions an open panel filters by for f, and
// whether the panel's delegate filters instead (useDelegate) or nothing
// filters (both zero: every file).
func (f panelFilter) openTypes() (exts []string, useDelegate bool) {
	switch {
	case f.all:
		return nil, false
	case len(f.globs) > 0:
		return nil, true
	}
	return f.exts, false
}

// saveTypes returns the extensions a save panel allows for f; nil allows
// any name.
func (f panelFilter) saveTypes() []string {
	if f.all {
		return nil
	}
	return f.exts
}

// match reports whether the file name passes f in an open panel whose
// delegate filters (folders are enabled before this is asked).
func (f panelFilter) match(name string) bool {
	if f.all {
		return true
	}
	name = strings.ToLower(name)
	for _, ext := range f.exts {
		if strings.HasSuffix(name, "."+ext) && len(name) > len(ext)+1 {
			return true
		}
	}
	for _, g := range f.globs {
		if ok, err := path.Match(g, name); err == nil && ok {
			return true
		}
	}
	return false
}

// NSAlert and NSModalResponse values the mapping below answers with (the
// darwin package has the same constants, which exist only on darwin).
const (
	alertFirstButtonReturn  = 1000
	alertStyleWarning       = 0
	alertStyleInformational = 1
	alertStyleCritical      = 2
)

// alertButton is one button of an NSAlert, in the order it is added: the
// first is the default button (Return, rightmost), the next ones go to its
// left.
type alertButton struct {
	title  string
	answer platform.MessageAnswer
	// escape gives the button the Escape key. NSAlert gives it to a button
	// titled "Cancel" by itself; the No of a Yes/No box gets it here, so
	// Escape answers the set's DismissAnswer.
	escape bool
}

// alertButtons returns the buttons of a message box with the set b, in
// the order the macOS guidelines give them (and NSAlert's own Save
// boxes): the affirmative button at the right, Cancel at its left, and a
// destructive No ("Don't Save") at the far left.
func alertButtons(b platform.MessageButtons) []alertButton {
	switch b {
	case platform.ButtonsOKCancel:
		return []alertButton{{"OK", platform.AnswerOK, false}, {"Cancel", platform.AnswerCancel, true}}
	case platform.ButtonsYesNo:
		return []alertButton{{"Yes", platform.AnswerYes, false}, {"No", platform.AnswerNo, true}}
	case platform.ButtonsYesNoCancel:
		return []alertButton{{"Yes", platform.AnswerYes, false}, {"Cancel", platform.AnswerCancel, true}, {"No", platform.AnswerNo, false}}
	}
	return []alertButton{{"OK", platform.AnswerOK, false}}
}

// alertButtonResponse is the response of the alert button at index i (in
// the order added), as -[NSAlert runModal] and a sheet's completion
// handler report it; a detached alert's buttons answer with it.
func alertButtonResponse(i int) int { return alertFirstButtonReturn + i }

// alertAnswer returns the answer of an alert with the set b that ended
// with response: the button at response - NSAlertFirstButtonReturn, or
// the set's DismissAnswer for anything else (a sheet ended because its
// window closed).
func alertAnswer(b platform.MessageButtons, response int) platform.MessageAnswer {
	buttons := alertButtons(b)
	if i := response - alertFirstButtonReturn; i >= 0 && i < len(buttons) {
		return buttons[i].answer
	}
	return b.DismissAnswer()
}

// alertStyle returns the NSAlertStyle of a kind. Warning and
// informational alerts both show the app's icon; a critical one (an
// error) shows a caution sign badged with it, which macOS keeps for
// errors and destructive questions. A question is informational.
func alertStyle(k platform.MessageKind) int {
	switch k {
	case platform.MessageWarning:
		return alertStyleWarning
	case platform.MessageError:
		return alertStyleCritical
	}
	return alertStyleInformational
}

// alertText returns the alert's message (bold, at the top) and
// informative text: the title and the text, or the text alone as the
// message when there is no title (an empty bold line would show).
func alertText(title, text string) (message, informative string) {
	if title == "" {
		return text, ""
	}
	return title, text
}
