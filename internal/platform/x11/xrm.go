//go:build linux

package x11

import (
	"math"
	"strconv"
	"strings"
)

// The X resource database (RESOURCE_MANAGER on the root window, written by
// xrdb and by GTK/Qt settings daemons) holds lines "spec: value". A spec is
// a sequence of components joined by bindings: "." (tight, the next level)
// or "*" (loose, any number of levels); "?" matches any single component.
// Tela only needs one resource, Xft.dpi (name "Xft.dpi", class "Xft.Dpi"),
// which is what Gio queries through XrmGetResource and winit through its
// resource-database crate.

// defaultDPI is the DPI of scale 1 on X11, as in every toolkit.
const defaultDPI = 96

// scaleFromResources returns Xft.dpi / 96 from a resource database string,
// or 1 if Xft.dpi is absent or not a usable number (it must give a scale in
// [0.25, 16]).
func scaleFromResources(db string) float64 {
	dpi, ok := lookupXftDPI(db)
	if !ok {
		return 1
	}
	s := dpi / defaultDPI
	if math.IsNaN(s) || s < 0.25 || s > 16 {
		return 1
	}
	return s
}

// lookupXftDPI returns the value of the best Xft.dpi entry in db.
func lookupXftDPI(db string) (float64, bool) {
	name := [2]string{"Xft", "dpi"}
	class := [2]string{"Xft", "Dpi"}
	var best [2]int
	var value string
	found := false
	for _, line := range strings.Split(db, "\n") {
		line = strings.TrimLeft(line, " \t")
		if line == "" || line[0] == '!' || line[0] == '#' {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		spec := strings.TrimRight(line[:colon], " \t")
		score, ok := matchSpec(spec, name, class)
		// Later entries win ties, like a database built line by line.
		if !ok || (found && better(best, score)) {
			continue
		}
		best, value, found = score, strings.TrimSpace(line[colon+1:]), true
	}
	if !found {
		return 0, false
	}
	dpi, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return dpi, true
}

// Per-level match quality, compared level by level from the first one
// (the Xrm precedence rules: a match by name beats one by class, which
// beats "?", which beats skipping the level through a loose binding; at
// equal quality a tight binding beats a loose one).
const (
	matchSkipped = iota
	matchLooseAny
	matchTightAny
	matchLooseClass
	matchTightClass
	matchLooseName
	matchTightName
)

// better reports whether a beats b strictly.
func better(a, b [2]int) bool {
	if a[0] != b[0] {
		return a[0] > b[0]
	}
	return a[1] > b[1]
}

// matchSpec reports whether a resource spec matches the two-level resource
// (name, class), and how well at each level.
func matchSpec(spec string, name, class [2]string) (score [2]int, ok bool) {
	type comp struct {
		loose bool
		s     string
	}
	var comps []comp
	loose := false
	start := 0
	flush := func(end int) {
		if end > start {
			comps = append(comps, comp{loose, spec[start:end]})
			loose = false
		}
	}
	for i := 0; i < len(spec); i++ {
		switch spec[i] {
		case '.', '*':
			flush(i)
			if spec[i] == '*' {
				loose = true
			}
			start = i + 1
		}
	}
	flush(len(spec))
	if len(comps) == 0 {
		return score, false
	}
	var rec func(ci, level int, sc [2]int) ([2]int, bool)
	rec = func(ci, level int, sc [2]int) ([2]int, bool) {
		if ci == len(comps) {
			return sc, level == len(name)
		}
		if level == len(name) {
			return sc, false
		}
		c := comps[ci]
		var bestSc [2]int
		found := false
		try := func(s [2]int, ok bool) {
			if ok && (!found || better(s, bestSc)) {
				bestSc, found = s, true
			}
		}
		q := 0
		switch {
		case c.s == name[level]:
			q = matchLooseName
		case c.s == class[level]:
			q = matchLooseClass
		case c.s == "?":
			q = matchLooseAny
		}
		if q != 0 {
			if !c.loose {
				q++ // tight variant
			}
			s := sc
			s[level] = q
			try(rec(ci+1, level+1, s))
		}
		if c.loose {
			// A loose binding may skip this level.
			s := sc
			s[level] = matchSkipped
			try(rec(ci, level+1, s))
		}
		return bestSc, found
	}
	return rec(0, 0, score)
}
