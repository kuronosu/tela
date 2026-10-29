package semantics

import (
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/editdist"
)

// The role vocabulary and the role groups below are WAI-ARIA 1.2's, as
// Playwright 1.61's roleUtils.ts lists them (validRoles,
// allowsNameFromContent, the roles that prohibit naming, and the roles
// that support each state).

// roles are the non-abstract ARIA roles an author may give, sorted.
var roles = []string{
	"alert", "alertdialog", "application", "article", "banner", "blockquote", "button", "caption", "cell",
	"checkbox", "code", "columnheader", "combobox", "complementary", "contentinfo", "definition", "deletion",
	"dialog", "directory", "document", "emphasis", "feed", "figure", "form", "generic", "grid", "gridcell",
	"group", "heading", "img", "insertion", "link", "list", "listbox", "listitem", "log", "main", "mark",
	"marquee", "math", "menu", "menubar", "menuitem", "menuitemcheckbox", "menuitemradio", "meter",
	"navigation", "none", "note", "option", "paragraph", "presentation", "progressbar", "radio",
	"radiogroup", "region", "row", "rowgroup", "rowheader", "scrollbar", "search", "searchbox", "separator",
	"slider", "spinbutton", "status", "strong", "subscript", "superscript", "switch", "tab", "table",
	"tablist", "tabpanel", "term", "textbox", "time", "timer", "toolbar", "tooltip", "tree", "treegrid",
	"treeitem",
}

// ValidRole reports whether role is an ARIA role an author may give
// (lowercase, as ARIA requires).
func ValidRole(role string) bool {
	_, ok := slices.BinarySearch(roles, role)
	return ok
}

// ParseRole returns the role of a role attribute value: the first valid
// role of its space-separated tokens (ARIA's fallback list), or "" when
// none is. bad is the first token that is not a valid role, if any.
func ParseRole(spec string) (role, bad string) {
	for _, tok := range strings.Fields(spec) {
		if ValidRole(tok) {
			if role == "" {
				role = tok
			}
		} else if bad == "" {
			bad = tok
		}
	}
	return role, bad
}

// SuggestRole returns the valid role closest to name by edit distance
// (case-insensitive), or "" when none is close.
func SuggestRole(name string) string {
	lower := strings.ToLower(name)
	if ValidRole(lower) {
		return lower
	}
	best, bestD := "", 3+len(lower)/5
	for _, r := range roles {
		if d := editdist.Distance(lower, r); d < bestD {
			best, bestD = r, d
		}
	}
	return best
}

var (
	// nameFromContent are the roles named from their content (ARIA's
	// "name from: contents", as Chromium and Playwright apply it).
	nameFromContent = []string{"button", "cell", "checkbox", "columnheader", "gridcell", "heading", "link",
		"menuitem", "menuitemcheckbox", "menuitemradio", "option", "radio", "row", "rowheader", "switch", "tab",
		"tooltip", "treeitem"}
	// nameFromContentDescendant are the roles whose content contributes
	// to the name of an ancestor named from content ("" is an element
	// without a role).
	nameFromContentDescendant = []string{"", "caption", "code", "contentinfo", "definition", "deletion",
		"emphasis", "insertion", "list", "listitem", "mark", "none", "paragraph", "presentation", "region",
		"row", "rowgroup", "section", "strong", "subscript", "superscript", "table", "term", "time"}
	// nameProhibited are the roles that cannot be named.
	nameProhibited = []string{"caption", "code", "definition", "deletion", "emphasis", "generic", "insertion",
		"mark", "paragraph", "presentation", "strong", "subscript", "suggestion", "superscript", "term", "time"}
	// labelProhibited are the roles on which aria-label and
	// aria-labelledby are not global attributes (they do not resolve a
	// presentational role conflict).
	labelProhibited = []string{"caption", "code", "deletion", "emphasis", "generic", "insertion", "paragraph",
		"presentation", "strong", "subscript", "superscript"}
	// rangeRoles are the range widgets, whose value names them inside an
	// ancestor's name.
	rangeRoles = []string{"meter", "progressbar", "scrollbar", "slider", "spinbutton"}

	// The roles that support each state in a snapshot.
	checkedRoles  = []string{"checkbox", "menuitemcheckbox", "option", "radio", "switch", "menuitemradio", "treeitem"}
	pressedRoles  = []string{"button"}
	selectedRoles = []string{"gridcell", "option", "row", "tab", "rowheader", "columnheader", "treeitem"}
	expandedRoles = []string{"application", "button", "checkbox", "combobox", "gridcell", "link", "listbox",
		"menuitem", "row", "rowheader", "tab", "treeitem", "columnheader", "menuitemcheckbox", "menuitemradio",
		"switch"}
	invalidRoles = []string{"application", "checkbox", "columnheader", "combobox", "gridcell", "listbox",
		"radiogroup", "rowheader", "searchbox", "slider", "spinbutton", "switch", "textbox", "tree"}
	levelRoles    = []string{"heading", "listitem", "row", "treeitem"}
	disabledRoles = []string{"application", "button", "composite", "gridcell", "group", "input", "link",
		"menuitem", "scrollbar", "separator", "tab", "checkbox", "columnheader", "combobox", "grid", "listbox",
		"menu", "menubar", "menuitemcheckbox", "menuitemradio", "option", "radio", "radiogroup", "row",
		"rowheader", "searchbox", "select", "slider", "spinbutton", "switch", "tablist", "textbox", "toolbar",
		"tree", "treegrid", "treeitem"}
)

// NameFromContent reports whether an element of role (as computed, ""
// for none) takes its name from its content: always for the roles of
// ARIA's "name from: contents", and also, when descendant is set (the
// element is inside the element being named), for the roles whose text
// still contributes.
func NameFromContent(role string, descendant bool) bool {
	return slices.Contains(nameFromContent, role) || descendant && slices.Contains(nameFromContentDescendant, role)
}

// NameProhibited reports whether an element of role cannot be named.
func NameProhibited(role string) bool { return slices.Contains(nameProhibited, role) }

// LabelProhibited reports whether aria-label and aria-labelledby are not
// global attributes on role, so they do not keep an element with role
// "none" or "presentation" exposed.
func LabelProhibited(role string) bool { return slices.Contains(labelProhibited, role) }

// RangeRole reports whether role is a range widget.
func RangeRole(role string) bool { return slices.Contains(rangeRoles, role) }
