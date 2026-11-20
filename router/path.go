package router

import (
	"net/url"
	"strings"
)

// Location is where the router is: the pathname, the query string and the
// fragment of the current history entry (React Router's pathname, search
// and hash). It is comparable.
type Location struct {
	// Path is the pathname, always starting with "/": "/notes/3".
	Path string
	// Query is the query string without the leading "?": "sort=asc&page=2".
	Query string
	// Hash is the fragment without the leading "#".
	Hash string
}

// String returns the location as the path with its query and fragment:
// "/notes/3?sort=asc#top".
func (l Location) String() string {
	s := l.Path
	if l.Query != "" {
		s += "?" + l.Query
	}
	if l.Hash != "" {
		s += "#" + l.Hash
	}
	return s
}

// Values returns the parsed query: "?tag=a&tag=b" gives {"tag": {"a", "b"}}.
// A query that does not parse gives the pairs that did.
func (l Location) Values() url.Values {
	v, _ := url.ParseQuery(l.Query)
	return v
}

// parseLocation splits "path?query#hash" (React Router's parsePath). The
// path of a location without one is empty.
func parseLocation(s string) Location {
	var l Location
	if i := strings.IndexByte(s, '#'); i >= 0 {
		l.Hash = strings.TrimPrefix(s[i:], "#")
		s = s[:i]
	}
	if i := strings.IndexByte(s, '?'); i >= 0 {
		l.Query = strings.TrimPrefix(s[i:], "?")
		s = s[:i]
	}
	l.Path = s
	return l
}

// absolute returns the location of an absolute entry: the path starts with
// "/" and has no empty segments at the start.
func (l Location) absolute() Location {
	if !strings.HasPrefix(l.Path, "/") {
		l.Path = "/" + l.Path
	}
	return l
}

// joinPaths joins path pieces with "/" and collapses repeated slashes
// (React Router's joinPaths).
func joinPaths(paths ...string) string {
	s := strings.Join(paths, "/")
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	return s
}

// resolveTo resolves to, a path as a Link or Navigator is given it, like
// React Router's resolveTo: an absolute path stays, a relative one is
// resolved from the pathname of the route the caller renders in (".." goes
// up one route, not one segment), and a path without a pathname ("?page=2",
// "#top", "") is the current location's with the new query or fragment.
// routePathnames are the pathnames of the matched routes up to the
// caller's, root first (see routePathnames), and current the location.
func resolveTo(to string, routePathnames []string, current Location) Location {
	l := parseLocation(to)
	isEmpty := to == ""
	toPathname := l.Path
	if isEmpty {
		toPathname = "/"
	}
	var from string
	if toPathname == "" {
		// "?q" and "#h": the current pathname.
		from = current.Path
	} else {
		idx := len(routePathnames) - 1
		if strings.HasPrefix(toPathname, "..") {
			segs := strings.Split(toPathname, "/")
			for len(segs) > 0 && segs[0] == ".." {
				segs = segs[1:]
				idx--
			}
			l.Path = strings.Join(segs, "/")
		}
		from = "/"
		if idx >= 0 {
			from = routePathnames[idx]
		}
	}
	out := Location{Query: l.Query, Hash: l.Hash}
	switch {
	case l.Path == "":
		out.Path = from
	case strings.HasPrefix(l.Path, "/"):
		out.Path = l.Path
	default:
		out.Path = resolvePathname(l.Path, from)
	}
	hasExplicitSlash := toPathname != "" && toPathname != "/" && strings.HasSuffix(toPathname, "/")
	hasCurrentSlash := (isEmpty || toPathname == ".") && strings.HasSuffix(current.Path, "/")
	if !strings.HasSuffix(out.Path, "/") && (hasExplicitSlash || hasCurrentSlash) {
		out.Path += "/"
	}
	return out
}

// resolvePathname resolves a relative path against from, handling "." and
// ".." segments (React Router's resolvePathname).
func resolvePathname(rel, from string) string {
	segs := strings.Split(strings.TrimRight(from, "/"), "/")
	for _, s := range strings.Split(rel, "/") {
		switch s {
		case "..":
			if len(segs) > 1 {
				segs = segs[:len(segs)-1]
			}
		case ".":
		default:
			segs = append(segs, s)
		}
	}
	if len(segs) > 1 {
		return strings.Join(segs, "/")
	}
	return "/"
}

// pathSegments splits a pathname into its non-empty segments.
func pathSegments(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
