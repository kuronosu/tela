package router

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/kuronosu/tela"
)

// Route is one entry of the route table given to [Routes]: a path pattern
// and the element it renders (React Router's route objects).
//
// A Path is made of segments: static text ("notes"), a parameter
// (":id") that takes one segment of any text, and a trailing splat ("*",
// "files/*") that takes the rest of the path. "*" alone matches anything,
// so it is the fallback ("not found") route. Several routes can match a
// path; the best ranks first, as in React Router: static segments beat
// parameters, parameters beat a splat, and longer paths beat shorter ones,
// whatever their order in the table. Matching ignores case and trailing
// slashes.
type Route struct {
	// Path is the pattern, relative to the parent route's ("new", ":id"),
	// or absolute ("/notes/:id") when it starts with the parent's whole
	// path. A route without a Path (and not an Index) is a layout: it
	// renders its Element around the matching child and has no URL of its
	// own.
	Path string
	// Index marks the route that renders at the parent's own path, in the
	// parent's [Outlet]: the "home" of a layout. An index route has no Path
	// and no Children.
	Index bool
	// Element is what the route renders. A route without an Element renders
	// its [Outlet], so a layout route that only groups paths needs none.
	Element tela.Node
	// Children are the routes nested in this one; the matching child
	// renders where the Element puts [Outlet].
	Children []Route
}

// Params are the values the matched path gave to the parameters of its
// routes: Params{"id": "3"} for "/notes/3" matched by "/notes/:id". The
// splat is under "*". Values are percent-decoded. [UseParams] returns them;
// do not modify the map.
type Params map[string]string

// match is one matched route of a branch.
type match struct {
	route *Route
	// id is the route's identity among the table (the indexes of it and
	// its parents), the key of its Presence child: the same route with
	// other parameters keeps its element.
	id string
	// base is the pathname the route and its parents matched, without the
	// splat and trailing slashes (React Router's pathnameBase); consumed the
	// number of segments of it, counted from the root of the table's
	// pathname.
	base     string
	consumed int
}

// routeMeta is a route of a branch while the table is flattened.
type routeMeta struct {
	rel      string // the path relative to the parent
	childIdx int
	route    *Route
}

// branch is the flattened path from the table's root to one matchable
// route: React Router's RouteBranch.
type branch struct {
	path  string
	score int
	metas []routeMeta
}

// Scores of the segments, from React Router.
const (
	dynamicSegmentValue = 3
	indexRouteValue     = 2
	emptySegmentValue   = 1
	staticSegmentValue  = 10
	splatPenalty        = -2
)

// flatten adds the branches of routes to out, after React Router's
// flattenRoutes. Mistakes in the table panic: they are programmer errors
// that would otherwise hide as a route that never matches.
func flatten(routes []Route, out *[]branch, parents []routeMeta, parentPath string) {
	for i := range routes {
		r := &routes[i]
		if r.Index && (r.Path != "" || len(r.Children) > 0) {
			panic(fmt.Sprintf("router: an index route has no Path and no Children (got Path %q and %d children); put Index: true on a route of its own, next to its siblings", r.Path, len(r.Children)))
		}
		meta := routeMeta{rel: r.Path, childIdx: i, route: r}
		if strings.HasPrefix(meta.rel, "/") {
			if !strings.HasPrefix(meta.rel, parentPath) {
				panic(fmt.Sprintf("router: the absolute route path %q is nested under %q; an absolute child path must start with the combined path of its parents (or make it relative: %q)",
					meta.rel, parentPath, strings.TrimPrefix(meta.rel, "/")))
			}
			meta.rel = meta.rel[len(parentPath):]
		}
		path := joinPaths(parentPath, meta.rel)
		metas := append(slices.Clone(parents), meta)
		if len(r.Children) > 0 {
			flatten(r.Children, out, metas, path)
		}
		if r.Path == "" && !r.Index {
			continue // a layout matches only through its children
		}
		*out = append(*out, branch{path: path, score: computeScore(path, r.Index), metas: metas})
	}
}

// computeScore ranks a path: the higher, the better (React Router's
// computeScore).
func computeScore(path string, index bool) int {
	segs := strings.Split(path, "/")
	score := len(segs)
	if slices.Contains(segs, "*") {
		score += splatPenalty
	}
	if index {
		score += indexRouteValue
	}
	for _, s := range segs {
		switch {
		case s == "*":
		case isParam(s):
			score += dynamicSegmentValue
		case s == "":
			score += emptySegmentValue
		default:
			score += staticSegmentValue
		}
	}
	return score
}

// isParam reports whether a pattern segment is ":name".
func isParam(s string) bool {
	if len(s) < 2 || s[0] != ':' {
		return false
	}
	for _, c := range s[1:] {
		if c != '-' && c != '_' && (c < '0' || c > '9') && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

// rank sorts the branches best first, like React Router's
// rankRouteBranches: by score, and among the routes of one parent, by
// their order in the table.
func rank(bs []branch) {
	slices.SortStableFunc(bs, func(a, b branch) int {
		if a.score != b.score {
			return b.score - a.score
		}
		if siblings(a, b) {
			return a.metas[len(a.metas)-1].childIdx - b.metas[len(b.metas)-1].childIdx
		}
		return 0
	})
}

// siblings reports whether two branches differ only in their last route.
func siblings(a, b branch) bool {
	if len(a.metas) != len(b.metas) {
		return false
	}
	for i := 0; i < len(a.metas)-1; i++ {
		if a.metas[i].childIdx != b.metas[i].childIdx {
			return false
		}
	}
	return true
}

// matchRoutes returns the routes of the best branch that matches pathname,
// root first, and the parameters it gave, or nil when none matches
// (React Router's matchRoutes). pathname is relative to nothing: nested
// tables pass the rest of the path after their parent's.
func matchRoutes(routes []Route, pathname string) ([]match, Params) {
	var branches []branch
	flatten(routes, &branches, nil, "")
	rank(branches)
	segs := pathSegments(pathname)
	for _, b := range branches {
		if ms, ps, ok := matchBranch(b, segs); ok {
			return ms, ps
		}
	}
	return nil, nil
}

// matchBranch matches the routes of b against the segments of a pathname.
func matchBranch(b branch, segs []string) ([]match, Params, bool) {
	params := Params{}
	var out []match
	consumed := 0
	id := ""
	for i, m := range b.metas {
		end := i == len(b.metas)-1
		used, ok := matchPattern(m.rel, segs[consumed:], end, params)
		if !ok {
			return nil, nil, false
		}
		consumed += used
		if id != "" {
			id += "/"
		}
		id += strconv.Itoa(m.childIdx)
		out = append(out, match{route: m.route, id: id, base: "/" + strings.Join(segs[:consumed], "/"), consumed: consumed})
	}
	return out, params, true
}

// matchPattern matches pattern, a route's relative path, against segs, the
// rest of the pathname, adding its parameters to params, and returns how
// many segments it matched, a splat's not counted. With end the pattern
// must match all of segs (unless it has a splat); without, it matches a
// prefix of whole segments.
func matchPattern(pattern string, segs []string, end bool, params Params) (int, bool) {
	splat := strings.HasSuffix(pattern, "*")
	if splat {
		pattern = strings.TrimSuffix(pattern, "*")
	}
	pat := pathSegments(pattern)
	if len(segs) < len(pat) || end && !splat && len(segs) != len(pat) {
		return 0, false
	}
	for i, p := range pat {
		switch {
		case isParam(p):
			params[p[1:]] = decode(segs[i])
		case !strings.EqualFold(p, segs[i]):
			return 0, false
		}
	}
	if splat {
		rest := make([]string, 0, len(segs)-len(pat))
		for _, s := range segs[len(pat):] {
			rest = append(rest, decode(s))
		}
		params["*"] = strings.Join(rest, "/")
	}
	return len(pat), true
}

// decode percent-decodes a path value, leaving it as it is when it does
// not decode.
func decode(s string) string {
	if d, err := url.PathUnescape(s); err == nil {
		return d
	}
	return s
}

// routePathnames returns the pathnames of the matches up to and including
// level, as relative paths resolve against them (React Router drops the
// matches of index routes and of routes that add nothing to the path).
func routePathnames(ms []match, level int) []string {
	var out []string
	for i := 0; i <= level && i < len(ms); i++ {
		if i == 0 || !ms[i].route.Index && ms[i].base != ms[i-1].base {
			out = append(out, ms[i].base)
		}
	}
	return out
}
