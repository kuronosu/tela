package core

import (
	"slices"
	"sync/atomic"
	"time"
)

// The element list of an instrumented tree as changes (ElementsSince), so
// that a client that follows a big tree (tela devtools after every frame
// that built) receives what changed instead of the whole list.
//
// The tree keeps the last few lists it gave (maxElemSnaps), each with a
// version: a client gives back the version of the list it has, and gets
// the elements new or changed since (by value: a text that changed, a
// build count, bounds that moved) and, when the order of the IDs changed
// (an element mounted, unmounted or moved), the whole order as IDs. A
// version the tree no longer has (another client asked many times since,
// a new process) gets the whole list. Versions come from one counter
// started at the process's start time, so a version of an earlier child
// of tela dev is not one of this process. The lists are built and compared
// on the UI thread when asked: the instrumentation adds nothing to frames
// for them.

// maxElemSnaps bounds the lists kept: enough for a few clients asking in
// turns (tela devtools and tela mcp --attach), each about the size of the
// list (a 10,000-element tree's is a few megabytes).
const maxElemSnaps = 3

// elemVersion numbers the lists of every tree of the process.
var elemVersion atomic.Uint64

func nextElemVersion() uint64 {
	if elemVersion.Load() == 0 {
		elemVersion.CompareAndSwap(0, uint64(time.Now().UnixNano()))
	}
	return elemVersion.Add(1)
}

// elemSnap is a list given out, and its version.
type elemSnap struct {
	version uint64
	els     []ElementInfo
}

// ElementsDiff is the element list of a tree as a change from an earlier
// one (Tree.ElementsSince).
type ElementsDiff struct {
	// Version names the list; give it back to get the next change.
	Version uint64 `json:"version"`
	// Full reports that Elements is the whole list, in tree order (the
	// version given was 0 or unknown).
	Full bool `json:"full,omitempty"`
	// Elements is the whole list when Full, else the elements new or
	// changed since the version given (without details: as Elements(false)
	// lists them).
	Elements []ElementInfo `json:"elements"`
	// Order is the IDs of the whole list in tree order when that changed
	// since the version given; nil when it did not (and when Full). The
	// elements not in it are gone.
	Order []ElementID `json:"order,omitempty"`
}

// ElementsSince returns the element list as a change from the list of
// version since (0: none, the whole list). The zero ElementsDiff while the
// instrumentation is off. Call it on the UI thread.
func (t *Tree) ElementsSince(since uint64) ElementsDiff {
	ins := t.ins
	if ins == nil {
		return ElementsDiff{}
	}
	cur := t.Elements(false)
	if cur == nil {
		cur = []ElementInfo{}
	}
	var last *elemSnap
	if n := len(ins.snaps); n > 0 {
		last = &ins.snaps[n-1]
	}
	version := uint64(0)
	if last != nil && sameElements(last.els, cur) {
		version = last.version // nothing changed since the last list
	} else {
		version = nextElemVersion()
		ins.snaps = append(ins.snaps, elemSnap{version, cur})
		if len(ins.snaps) > maxElemSnaps {
			ins.snaps = slices.Delete(ins.snaps, 0, len(ins.snaps)-maxElemSnaps)
		}
	}
	i := slices.IndexFunc(ins.snaps, func(s elemSnap) bool { return s.version == since })
	if since == 0 || i < 0 {
		return ElementsDiff{Version: version, Full: true, Elements: cur}
	}
	return diffElements(ins.snaps[i].els, cur, version)
}

// diffElements is the change from old to cur.
func diffElements(old, cur []ElementInfo, version uint64) ElementsDiff {
	d := ElementsDiff{Version: version, Elements: []ElementInfo{}}
	byID := make(map[ElementID]int, len(old))
	for i := range old {
		byID[old[i].ID] = i
	}
	order := len(old) != len(cur)
	for i := range cur {
		if !order && old[i].ID != cur[i].ID {
			order = true
		}
		if j, ok := byID[cur[i].ID]; !ok || !sameElement(&old[j], &cur[i]) {
			d.Elements = append(d.Elements, cur[i])
		}
	}
	if order {
		d.Order = make([]ElementID, len(cur))
		for i := range cur {
			d.Order[i] = cur[i].ID
		}
	}
	return d
}

// sameElements reports whether two lists are equal.
func sameElements(a, b []ElementInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameElement(&a[i], &b[i]) {
			return false
		}
	}
	return true
}

// sameElement compares two entries of Elements(false) (no details).
func sameElement(a, b *ElementInfo) bool {
	if a.ID != b.ID || a.Parent != b.Parent || a.Depth != b.Depth || a.Kind != b.Kind || a.Component != b.Component ||
		a.Key != b.Key || a.Text != b.Text || a.Classes != b.Classes || a.Site != b.Site || a.Node != b.Node || a.Builds != b.Builds {
		return false
	}
	if (a.Bounds == nil) != (b.Bounds == nil) {
		return false
	}
	return a.Bounds == nil || *a.Bounds == *b.Bounds
}
