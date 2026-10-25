package main

import (
	"fmt"
	"slices"
	"strings"
)

// Clip is what Cut and Copy put on the clipboard: an entry of a folder.
type Clip struct {
	Path Path // the folder it is in
	Name string
	Cut  bool
}

// State is everything the explorer remembers: the file system, where it is,
// the history on both sides, what is selected, the search text, the sort
// order and the clipboard. It is replaced, never mutated, by reduce (the
// file system is a persistent tree: see Entry.Edit).
type State struct {
	FS      *Entry
	Path    Path
	Back    []Path
	Forward []Path
	Sel     string // name of the selected entry of Path, "" for none
	Query   string
	Sort    Column
	Desc    bool
	Clip    *Clip
	Msg     string // shown in the status bar instead of the counts, until the next action
}

// Action is what the UI asks of reduce.
type Action struct {
	Kind  string // "open", "back", "forward", "up", "select", "move", "first", "last", "activate", "query", "sort", "rename", "delete", "newfolder", "cut", "copy", "paste", "msg"
	Path  Path
	Name  string
	To    string // the new name of a "rename"
	Delta int
	Query string
	Col   Column
	Desc  bool
}

// Dir returns the folder the explorer shows. A folder that was deleted
// while it was in the history shows as an empty one.
func (s State) Dir() *Entry {
	if d := s.FS.Find(s.Path); d != nil {
		return d
	}
	return &Entry{Name: Base(s.Path), Kind: KindFolder}
}

// Entries returns what the list shows: the folder's listed children that
// match the search, in the sort order.
func (s State) Entries() []*Entry { return Sorted(s.Dir(), s.Sort, s.Desc, s.Query) }

// Selected returns the selected entry, or nil.
func (s State) Selected() *Entry {
	for _, e := range s.Entries() {
		if e.Name == s.Sel {
			return e
		}
	}
	return nil
}

func (s State) visit(p Path) State {
	if p == s.Path {
		return s
	}
	s.Back = append(slices.Clone(s.Back), s.Path)
	s.Forward = nil
	s.Path, s.Sel, s.Query = p, "", ""
	return s
}

// exists reports whether the folder at p is still there.
func (s State) exists(p Path) bool {
	e := s.FS.Find(p)
	return e != nil && e.IsFolder()
}

// reduce is the explorer's reducer.
func reduce(s State, a Action) State {
	s.Msg = ""
	switch a.Kind {
	case "open":
		if s.exists(a.Path) {
			return s.visit(a.Path)
		}
	case "up":
		return s.visit(Parent(s.Path))
	case "back":
		// Folders that were deleted since are skipped.
		for len(s.Back) > 0 {
			n := len(s.Back)
			target := s.Back[n-1]
			s.Back = slices.Clone(s.Back[:n-1])
			if s.exists(target) {
				s.Forward = append(slices.Clone(s.Forward), s.Path)
				s.Path, s.Sel, s.Query = target, "", ""
				break
			}
		}
	case "forward":
		for len(s.Forward) > 0 {
			n := len(s.Forward)
			target := s.Forward[n-1]
			s.Forward = slices.Clone(s.Forward[:n-1])
			if s.exists(target) {
				s.Back = append(slices.Clone(s.Back), s.Path)
				s.Path, s.Sel, s.Query = target, "", ""
				break
			}
		}
	case "select":
		s.Sel = a.Name
	case "first", "last":
		if entries := s.Entries(); len(entries) > 0 {
			s.Sel = entries[0].Name
			if a.Kind == "last" {
				s.Sel = entries[len(entries)-1].Name
			}
		}
	case "move":
		entries := s.Entries()
		if len(entries) == 0 {
			return s
		}
		i := slices.IndexFunc(entries, func(e *Entry) bool { return e.Name == s.Sel })
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
		e := s.FS.Find(Join(s.Path, s.Sel))
		switch {
		case e == nil:
		case e.IsFolder():
			return s.visit(Join(s.Path, s.Sel))
		default:
			s.Msg = "Opening " + e.Name + "…"
		}
	case "query":
		s.Query = a.Query
		if s.Sel != "" && s.Selected() == nil {
			// Keep the selection only while it still matches.
			s.Sel = ""
		}
	case "sort":
		s.Sort, s.Desc = a.Col, a.Desc
	case "rename":
		return s.rename(a.Name, a.To)
	case "delete":
		s.FS = s.FS.Edit(s.Path, func(c []*Entry) []*Entry {
			return slices.DeleteFunc(c, func(e *Entry) bool { return e.Name == a.Name })
		})
		s.Sel = ""
		s.Msg = "Deleted “" + a.Name + "”"
	case "newfolder":
		name := ""
		s.FS = s.FS.Edit(s.Path, func(c []*Entry) []*Entry {
			name = UniqueName(c, "New folder")
			return append(c, &Entry{Name: name, Kind: KindFolder, Modified: now})
		})
		s.Sel, s.Query = name, ""
	case "cut", "copy":
		if s.FS.Find(Join(s.Path, a.Name)) != nil {
			s.Clip = &Clip{Path: s.Path, Name: a.Name, Cut: a.Kind == "cut"}
			if a.Kind == "cut" {
				s.Msg = "Cut “" + a.Name + "”"
			} else {
				s.Msg = "Copied “" + a.Name + "”"
			}
		}
	case "paste":
		return s.paste()
	case "msg":
		s.Msg = a.Name
	}
	return s
}

// rename gives the entry called from of the current folder the name to.
// An empty name, the same name or a name another entry has changes nothing
// and says why in the status bar.
func (s State) rename(from, to string) State {
	to = strings.TrimSpace(to)
	switch {
	case to == "" || to == from:
		return s
	case strings.Contains(to, "/"):
		s.Msg = "A name cannot contain “/”"
		return s
	}
	dir := s.Dir()
	if slices.ContainsFunc(dir.Children, func(e *Entry) bool { return e.Name == to }) {
		s.Msg = "“" + to + "” already exists"
		return s
	}
	s.FS = s.FS.Edit(s.Path, func(c []*Entry) []*Entry {
		for i, e := range c {
			if e.Name == from {
				r := *e
				r.Name = to
				c[i] = &r
			}
		}
		return c
	})
	// The history points at the folder by its path.
	was, now := Join(s.Path, from), Join(s.Path, to)
	for _, list := range []*[]Path{&s.Back, &s.Forward} {
		*list = slices.Clone(*list)
		for i, p := range *list {
			(*list)[i] = rebase(p, was, now)
		}
	}
	if s.Clip != nil && s.Clip.Path == s.Path && s.Clip.Name == from {
		c := *s.Clip
		c.Name = to
		s.Clip = &c
	}
	if s.Sel == from {
		s.Sel = to
	}
	s.Msg = "Renamed to “" + to + "”"
	return s
}

// rebase turns a path inside the folder from into the same path inside to.
func rebase(p, from, to Path) Path {
	switch {
	case p == from:
		return to
	case strings.HasPrefix(p, from+"/"):
		return to + p[len(from):]
	}
	return p
}

// paste puts the clipboard's entry in the current folder: a copy with a
// name of its own, or the entry itself after a cut.
func (s State) paste() State {
	if s.Clip == nil {
		return s
	}
	src := s.FS.Find(Join(s.Clip.Path, s.Clip.Name))
	if src == nil {
		s.Clip = nil
		return s
	}
	if s.Clip.Cut && s.Clip.Path == s.Path {
		return s // moving an entry into its own folder
	}
	if src.IsFolder() && (Join(s.Clip.Path, s.Clip.Name) == s.Path || strings.HasPrefix(s.Path, Join(s.Clip.Path, s.Clip.Name)+"/")) {
		s.Msg = "A folder cannot be put inside itself"
		return s
	}
	name := ""
	if s.Clip.Cut {
		s.FS = s.FS.Edit(s.Clip.Path, func(c []*Entry) []*Entry {
			return slices.DeleteFunc(c, func(e *Entry) bool { return e == src })
		})
	}
	s.FS = s.FS.Edit(s.Path, func(c []*Entry) []*Entry {
		name = UniqueName(c, src.Name)
		cp := *src
		cp.Name = name
		return append(c, &cp)
	})
	if s.Clip.Cut {
		s.Clip = nil
	}
	s.Sel, s.Query = name, ""
	s.Msg = fmt.Sprintf("Pasted “%s”", name)
	return s
}
