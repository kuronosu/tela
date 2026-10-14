package main

import "slices"

// State is everything the explorer remembers: where it is, the history on
// both sides, what is selected, the search text and the sort order. It is
// replaced, never mutated, by reduce.
type State struct {
	Path    Path
	Back    []Path
	Forward []Path
	Sel     string // name of the selected entry of Path, "" for none
	Query   string
	Sort    Column
	Desc    bool
	Msg     string // shown in the status bar instead of the counts, until the next action
}

// Action is what the UI asks of reduce.
type Action struct {
	Kind  string // "open", "back", "forward", "up", "select", "move", "first", "last", "activate", "query", "sort"
	Path  Path
	Name  string
	Delta int
	Query string
	Col   Column
}

func (s State) visit(p Path) State {
	if p == s.Path {
		return s
	}
	s.Back = append(slices.Clone(s.Back), s.Path)
	s.Forward = nil
	s.Path, s.Sel, s.Query, s.Msg = p, "", "", ""
	return s
}

// reduce is the explorer's reducer.
func reduce(s State, a Action) State {
	s.Msg = ""
	switch a.Kind {
	case "open":
		if e := Lookup(a.Path); e != nil && e.IsFolder() {
			return s.visit(a.Path)
		}
	case "up":
		return s.visit(Parent(s.Path))
	case "back":
		if n := len(s.Back); n > 0 {
			s.Forward = append(slices.Clone(s.Forward), s.Path)
			s.Path, s.Back = s.Back[n-1], slices.Clone(s.Back[:n-1])
			s.Sel, s.Query = "", ""
		}
	case "forward":
		if n := len(s.Forward); n > 0 {
			s.Back = append(slices.Clone(s.Back), s.Path)
			s.Path, s.Forward = s.Forward[n-1], slices.Clone(s.Forward[:n-1])
			s.Sel, s.Query = "", ""
		}
	case "select":
		s.Sel = a.Name
	case "first", "last":
		if dir := Lookup(s.Path); dir != nil {
			if entries := Sorted(dir, s.Sort, s.Desc, s.Query); len(entries) > 0 {
				s.Sel = entries[0].Name
				if a.Kind == "last" {
					s.Sel = entries[len(entries)-1].Name
				}
			}
		}
	case "move":
		dir := Lookup(s.Path)
		if dir == nil {
			return s
		}
		entries := Sorted(dir, s.Sort, s.Desc, s.Query)
		if len(entries) == 0 {
			return s
		}
		i := -1
		for j, e := range entries {
			if e.Name == s.Sel {
				i = j
			}
		}
		switch {
		case i < 0 && a.Delta > 0:
			i = 0
		case i < 0:
			i = len(entries) - 1
		default:
			i = min(max(i+a.Delta, 0), len(entries)-1)
		}
		s.Sel = entries[i].Name
	case "activate":
		if s.Sel == "" {
			return s
		}
		e := Lookup(Join(s.Path, s.Sel))
		switch {
		case e == nil:
		case e.IsFolder():
			return s.visit(Join(s.Path, s.Sel))
		default:
			s.Msg = "Opening " + e.Name + "…"
		}
	case "query":
		s.Query = a.Query
		if s.Sel != "" {
			// Keep the selection only while it still matches.
			dir := Lookup(s.Path)
			if dir != nil && !slices.ContainsFunc(Sorted(dir, s.Sort, s.Desc, s.Query), func(e *Entry) bool { return e.Name == s.Sel }) {
				s.Sel = ""
			}
		}
	case "sort":
		if s.Sort == a.Col {
			s.Desc = !s.Desc
		} else {
			s.Sort, s.Desc = a.Col, false
		}
	}
	return s
}
