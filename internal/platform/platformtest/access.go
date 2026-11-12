package platformtest

import (
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/kuronosu/tela/internal/platform"
)

// AccessScript makes random accessibility updates for the tests of the
// bridges' incremental views (ADR-277): a first update with a whole tree
// of mixed roles (ignored containers, texts, text-level elements, leaves
// whose children are presentational, text fields, tables, menus, lists,
// live regions, aria-controls), then updates as core sends them for
// random insertions, removals, moves, reorders and changes of roles,
// names, states and references: each holds the nodes that changed,
// a parent whose children changed among them.
type AccessScript struct {
	r      *rand.Rand
	nodes  map[platform.AccessID]*platform.AccessNode
	parent map[platform.AccessID]platform.AccessID
	next   platform.AccessID
	focus  platform.AccessID
}

// accessRoles are the roles the script gives elements ("generic" twice:
// ignored containers are common).
var accessRoles = []string{"generic", "generic", "none", "paragraph", "heading", "button", "checkbox", "strong",
	"emphasis", "code", "link", "textbox", "list", "listitem", "table", "row", "cell", "menu", "menuitem",
	"listbox", "option", "img", "dialog", "group", "tree", "treeitem", "tablist", "tab", "slider"}

// NewAccessScript returns a script of about 80 nodes from seed.
func NewAccessScript(seed uint64) *AccessScript {
	s := &AccessScript{r: rand.New(rand.NewPCG(seed, 11)), nodes: map[platform.AccessID]*platform.AccessNode{},
		parent: map[platform.AccessID]platform.AccessID{}, next: 1, focus: 1}
	s.nodes[1] = &platform.AccessNode{ID: 1, Kind: platform.AccessRoot, Bounds: platform.Rect{W: 800, H: 600}}
	s.next = 2
	for range 80 {
		s.add(s.pickParent())
	}
	return s
}

// First returns the update with the whole tree.
func (s *AccessScript) First() platform.AccessUpdate {
	u := platform.AccessUpdate{Root: 1, Focus: s.focus}
	for _, id := range s.ids() {
		u.Nodes = append(u.Nodes, s.nodes[id].Clone())
	}
	return u
}

// ids returns the nodes' IDs in order.
func (s *AccessScript) ids() []platform.AccessID {
	ids := make([]platform.AccessID, 0, len(s.nodes))
	for id := range s.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// pick returns a random node.
func (s *AccessScript) pick() platform.AccessID {
	ids := s.ids()
	return ids[s.r.IntN(len(ids))]
}

// pickParent returns a random node that may have children (not a text).
func (s *AccessScript) pickParent() platform.AccessID {
	for {
		if id := s.pick(); s.nodes[id].Kind != platform.AccessText {
			return id
		}
	}
}

// fill gives element n random values.
func (s *AccessScript) fill(n *platform.AccessNode) {
	n.Role = accessRoles[s.r.IntN(len(accessRoles))]
	n.Name, n.Description, n.Value = "", "", ""
	if s.r.IntN(3) == 0 {
		n.Name = fmt.Sprint("name ", s.r.IntN(5))
	}
	if n.Role == "textbox" {
		n.Value = fmt.Sprint("value ", s.r.IntN(5))
	}
	n.States = 0
	if s.r.IntN(5) == 0 {
		n.States |= platform.AccessFocusable
	}
	if s.r.IntN(4) == 0 {
		n.States |= platform.AccessInline
	}
	n.Selected = platform.AccessToggle(s.r.IntN(3))
	n.Expanded = platform.AccessToggle(s.r.IntN(3))
	n.Live = ""
	if s.r.IntN(10) == 0 {
		n.Live = "polite"
	}
	n.Controls = 0
	if s.r.IntN(8) == 0 {
		n.Controls = s.pick()
	}
	n.Bounds = platform.Rect{X: float32(s.r.IntN(700)), Y: float32(s.r.IntN(500)), W: float32(1 + s.r.IntN(100)), H: float32(1 + s.r.IntN(40))}
}

// add inserts a new node at a random place of parent's children and
// returns it.
func (s *AccessScript) add(parent platform.AccessID) *platform.AccessNode {
	n := &platform.AccessNode{ID: s.next}
	s.next++
	if s.r.IntN(3) == 0 {
		n.Kind, n.Text = platform.AccessText, fmt.Sprint("text ", s.r.IntN(9))
		n.States = platform.AccessInline
		n.Bounds = platform.Rect{X: float32(s.r.IntN(700)), Y: float32(s.r.IntN(500)), W: 30, H: 10}
	} else {
		n.Kind = platform.AccessElement
		s.fill(n)
	}
	s.nodes[n.ID] = n
	p := s.nodes[parent]
	p.Children = slices.Insert(slices.Clone(p.Children), s.r.IntN(len(p.Children)+1), n.ID)
	s.parent[n.ID] = parent
	return n
}

// inside reports whether id is a or below it.
func (s *AccessScript) inside(id, a platform.AccessID) bool {
	for ; id != 0; id = s.parent[id] {
		if id == a {
			return true
		}
	}
	return false
}

func (s *AccessScript) removeSubtree(id platform.AccessID) {
	for _, c := range s.nodes[id].Children {
		s.removeSubtree(c)
	}
	delete(s.nodes, id)
	delete(s.parent, id)
}

// unlink takes id out of its parent's children and returns the parent.
func (s *AccessScript) unlink(id platform.AccessID) platform.AccessID {
	p := s.parent[id]
	pn := s.nodes[p]
	pn.Children = slices.DeleteFunc(slices.Clone(pn.Children), func(c platform.AccessID) bool { return c == id })
	return p
}

// Next changes the tree at random and returns the update.
func (s *AccessScript) Next() platform.AccessUpdate {
	var changed []platform.AccessID
	for range 1 + s.r.IntN(3) {
		switch k := s.r.IntN(7); {
		case k == 0: // new nodes
			p := s.pickParent()
			n := s.add(p)
			changed = append(changed, p, n.ID)
			for range s.r.IntN(3) {
				if n.Kind == platform.AccessText {
					break
				}
				changed = append(changed, s.add(n.ID).ID)
			}
			changed = append(changed, n.ID)
		case k == 1: // a removal
			id := s.pick()
			if id == 1 || s.inside(s.focus, id) {
				continue
			}
			changed = append(changed, s.unlink(id))
			s.removeSubtree(id)
		case k == 2: // a move
			id, to := s.pick(), s.pickParent()
			if id == 1 || s.inside(to, id) {
				continue
			}
			changed = append(changed, s.unlink(id), to)
			tn := s.nodes[to]
			tn.Children = slices.Insert(slices.Clone(tn.Children), s.r.IntN(len(tn.Children)+1), id)
			s.parent[id] = to
		case k == 3: // a reorder
			p := s.nodes[s.pickParent()]
			if len(p.Children) > 1 {
				p.Children = slices.Clone(p.Children)
				s.r.Shuffle(len(p.Children), func(i, j int) { p.Children[i], p.Children[j] = p.Children[j], p.Children[i] })
				changed = append(changed, p.ID)
			}
		case k == 4: // a focus move
			s.focus = s.pick()
		default: // new values
			n := s.nodes[s.pick()]
			switch n.Kind {
			case platform.AccessText:
				n.Text += "+"
			case platform.AccessElement:
				if s.r.IntN(2) == 0 {
					s.fill(n)
				} else {
					n.Name += "'"
					n.Bounds.X++
				}
			default:
				n.Bounds.W++
			}
			changed = append(changed, n.ID)
		}
	}
	u := platform.AccessUpdate{Root: 1, Focus: s.focus}
	seen := map[platform.AccessID]bool{}
	for _, id := range changed {
		if n := s.nodes[id]; n != nil && !seen[id] {
			seen[id] = true
			u.Nodes = append(u.Nodes, n.Clone())
		}
	}
	return u
}
