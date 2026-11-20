package devtools

import (
	"strings"

	"github.com/kuronosu/tela/internal/core"
)

// Row is a visible row of the element tree.
type Row struct {
	ID core.ElementID
	// I is the element's index in View.Els.
	I int
	// Depth is the indentation, in levels shown.
	Depth int
	// HasKids: the element has children (shown or not); Open: they are
	// shown.
	HasKids, Open bool
	// Dim: the row is only here because a match is below it.
	Dim bool
}

// TreeOptions choose which elements of the tree show.
type TreeOptions struct {
	// Components hides the primitives and the text: what remains is the
	// component tree.
	Components bool
	// Search shows the elements whose kind, key, text or classes contain
	// it, and their ancestors (case-insensitive).
	Search string
}

// TreeRows returns the rows the tree shows: the elements in tree order, without
// those under a folded row, filtered by o.
func TreeRows(v View, o TreeOptions) []Row {
	els := v.Els
	n := len(els)
	keep := func(i int) bool { return !o.Components || els[i].Component || i == 0 }
	// hasKids: a row has children shown (the nearest kept ancestor of an
	// element has it).
	hasKids := make([]bool, n)
	for i := 1; i < n; i++ {
		if !keep(i) {
			continue
		}
		for p, ok := v.Index[els[i].Parent]; ok; p, ok = v.Index[els[p].Parent] {
			if keep(p) {
				hasKids[p] = true
				break
			}
		}
	}
	var match []bool
	search := strings.ToLower(strings.TrimSpace(o.Search))
	if search != "" {
		match = make([]bool, n)
		for i := n - 1; i >= 0; i-- {
			el := &els[i]
			if keep(i) && matches(el, search) {
				match[i] = true
			}
			// A match makes its ancestors visible.
			if match[i] {
				if p, ok := v.Index[el.Parent]; ok && el.Parent != 0 {
					match[p] = true
				}
			}
		}
	}
	rows := make([]Row, 0, n)
	depth := make([]int, n) // the depth shown of each element
	hidden := -1            // the depth (of Els) under which rows are hidden
	for i := range els {
		el := &els[i]
		if hidden >= 0 {
			if el.Depth > hidden {
				continue
			}
			hidden = -1
		}
		d := 0
		if p, ok := v.Index[el.Parent]; ok && el.Parent != 0 {
			d = depth[p]
			if keep(p) {
				d++
			}
		}
		depth[i] = d
		if !keep(i) {
			continue
		}
		dim := false
		if search != "" {
			if !match[i] {
				continue
			}
			dim = !matches(el, search)
		}
		open := !v.Collapsed[v.Paths[i]]
		rows = append(rows, Row{ID: el.ID, I: i, Depth: d, HasKids: hasKids[i] && search == "", Open: open || search != "", Dim: dim})
		if !open && hasKids[i] && search == "" {
			hidden = el.Depth
		}
	}
	return rows
}

func matches(el *core.ElementInfo, lower string) bool {
	for _, s := range []string{el.Kind, el.Key, el.Text, el.Classes} {
		if s != "" && strings.Contains(strings.ToLower(s), lower) {
			return true
		}
	}
	return false
}

// RowIndex returns the position of element id among rows, or -1.
func RowIndex(rows []Row, id core.ElementID) int {
	for i, r := range rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}
