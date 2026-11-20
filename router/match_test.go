package router

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// matched returns the paths of the matched routes, "a > b" for a nested
// match, and the params.
func matched(routes []Route, pathname string) (string, Params) {
	ms, ps := matchRoutes(routes, pathname)
	if ms == nil {
		return "", nil
	}
	var names []string
	for _, m := range ms {
		name := m.route.Path
		if m.route.Index {
			name = "(index)"
		}
		if name == "" {
			name = "(layout)"
		}
		names = append(names, name)
	}
	return strings.Join(names, " > "), ps
}

func TestMatchRanking(t *testing.T) {
	// The order of the table does not matter: static beats dynamic beats
	// splat, whatever comes first.
	tables := [][]Route{
		{{Path: "*"}, {Path: "/notes/:id"}, {Path: "/notes/new"}},
		{{Path: "/notes/new"}, {Path: "/notes/:id"}, {Path: "*"}},
		{{Path: "/notes/:id"}, {Path: "*"}, {Path: "/notes/new"}},
	}
	for i, routes := range tables {
		for _, c := range []struct {
			path, want string
			params     Params
		}{
			{"/notes/new", "/notes/new", Params{}},
			{"/notes/7", "/notes/:id", Params{"id": "7"}},
			{"/notes/7/edit", "*", Params{"*": "notes/7/edit"}},
			{"/other", "*", Params{"*": "other"}},
			{"/", "*", Params{"*": ""}},
		} {
			got, ps := matched(routes, c.path)
			if got != c.want || !reflect.DeepEqual(ps, c.params) {
				t.Errorf("table %d: %q matched %q %v, want %q %v", i, c.path, got, ps, c.want, c.params)
			}
		}
	}
}

func TestMatchLongerPathsBeatShorterOnes(t *testing.T) {
	routes := []Route{{Path: "/a/*"}, {Path: "/a/b/*"}, {Path: "/a/b/c"}, {Path: "/:x/b/c"}}
	for path, want := range map[string]string{
		"/a/b/c":   "/a/b/c",
		"/z/b/c":   "/:x/b/c",
		"/a/b/d":   "/a/b/*",
		"/a/q":     "/a/*",
		"/a/b/c/d": "/a/b/*",
	} {
		if got, _ := matched(routes, path); got != want {
			t.Errorf("%q matched %q, want %q", path, got, want)
		}
	}
}

func TestMatchIgnoresCaseAndTrailingSlashes(t *testing.T) {
	routes := []Route{{Path: "/Notes/:id"}}
	for _, p := range []string{"/notes/1", "/NOTES/1", "/notes/1/", "//notes//1"} {
		if got, ps := matched(routes, p); got != "/Notes/:id" || ps["id"] != "1" {
			t.Errorf("%q matched %q %v", p, got, ps)
		}
	}
	for _, p := range []string{"/notes", "/notes/1/2", "/xnotes/1"} {
		if got, _ := matched(routes, p); got != "" {
			t.Errorf("%q matched %q", p, got)
		}
	}
}

func TestMatchParamsAreDecoded(t *testing.T) {
	_, ps := matched([]Route{{Path: "/f/:name/*"}}, "/f/a%20b%2Fc/x%20y/z%")
	if ps["name"] != "a b/c" || ps["*"] != "x y/z%" {
		t.Errorf("params %q", ps)
	}
}

func TestMatchNestedRoutes(t *testing.T) {
	routes := []Route{
		{Path: "/", Children: []Route{
			{Index: true},
			{Path: "about"},
			{Path: "notes", Children: []Route{
				{Index: true},
				{Path: ":id"},
				{Path: "new"},
			}},
		}},
		{Path: "*"},
	}
	for _, c := range []struct {
		path, want string
		params     Params
	}{
		{"/", "/ > (index)", Params{}},
		{"/about", "/ > about", Params{}},
		{"/notes", "/ > notes > (index)", Params{}},
		{"/notes/9", "/ > notes > :id", Params{"id": "9"}},
		{"/notes/new", "/ > notes > new", Params{}},
		{"/notes/9/x", "*", Params{"*": "notes/9/x"}},
	} {
		got, ps := matched(routes, c.path)
		if got != c.want || !reflect.DeepEqual(ps, c.params) {
			t.Errorf("%q matched %q %v, want %q %v", c.path, got, ps, c.want, c.params)
		}
	}
}

func TestMatchLayoutRoutesHaveNoPathOfTheirOwn(t *testing.T) {
	routes := []Route{
		{Children: []Route{{Path: "/login"}, {Path: "/signup"}}}, // a layout without a path
		{Path: "/app", Children: []Route{{Path: "inbox"}}},
	}
	for path, want := range map[string]string{
		"/login":     "(layout) > /login",
		"/signup":    "(layout) > /signup",
		"/app/inbox": "/app > inbox",
		"/app":       "/app", // a route with a path matches by itself, even with children
		"/":          "",
	} {
		if got, _ := matched(routes, path); got != want {
			t.Errorf("%q matched %q, want %q", path, got, want)
		}
	}
}

func TestMatchAbsoluteChildPaths(t *testing.T) {
	routes := []Route{{Path: "/notes", Children: []Route{{Path: "/notes/:id"}}}}
	if got, ps := matched(routes, "/notes/4"); got != "/notes > /notes/:id" || ps["id"] != "4" {
		t.Errorf("matched %q %v", got, ps)
	}
}

func TestMatchSiblingsKeepTheirOrderOnATie(t *testing.T) {
	routes := []Route{{Path: "/:a"}, {Path: "/:b"}}
	if got, ps := matched(routes, "/x"); got != "/:a" || ps["a"] != "x" {
		t.Errorf("matched %q %v: the first of equal routes wins", got, ps)
	}
}

func TestMatchBases(t *testing.T) {
	ms, _ := matchRoutes([]Route{{Path: "/notes", Children: []Route{{Path: ":id/*"}}}}, "/Notes/3/a/b")
	var bases []string
	for _, m := range ms {
		bases = append(bases, m.base)
	}
	if want := []string{"/Notes", "/Notes/3"}; !reflect.DeepEqual(bases, want) {
		t.Errorf("bases %q, want %q (the splat is not in a base)", bases, want)
	}
}

func TestBadRoutesPanicWithTheFix(t *testing.T) {
	for name, c := range map[string]struct {
		routes []Route
		want   string
	}{
		"index with a path":     {[]Route{{Index: true, Path: "x"}}, "index route has no Path"},
		"index with children":   {[]Route{{Index: true, Children: []Route{{Path: "x"}}}}, "index route has no Path"},
		"absolute outside path": {[]Route{{Path: "/a", Children: []Route{{Path: "/b/c"}}}}, `"/b/c" is nested under "/a"`},
	} {
		func() {
			defer func() {
				msg := fmt.Sprint(recover())
				if !strings.Contains(msg, c.want) {
					t.Errorf("%s: panic %q, want it to say %q", name, msg, c.want)
				}
			}()
			matchRoutes(c.routes, "/")
		}()
	}
}

func TestResolveTo(t *testing.T) {
	// The matches of /app/notes/3: pathnames of the routes up to here.
	routes := []string{"/", "/app", "/app/notes", "/app/notes/3"}
	cur := Location{Path: "/app/notes/3", Query: "x=1"}
	for _, c := range []struct {
		to, want string
		routes   []string
	}{
		{"/abs?q=1#h", "/abs?q=1#h", routes},
		{"edit", "/app/notes/3/edit", routes},
		{"./edit", "/app/notes/3/edit", routes},
		{"..", "/app/notes", routes},
		{"../4", "/app/notes/4", routes},
		{"../..", "/app", routes},
		{"../../x", "/app/x", routes},
		{"../../../../x", "/x", routes},
		{"", "/app/notes/3", routes},
		{"?page=2", "/app/notes/3?page=2", routes},
		{"#top", "/app/notes/3#top", routes},
		{"sub/", "/app/notes/3/sub/", routes},
		{"x/../y", "/app/notes/3/y", routes},
		{"edit", "/edit", nil},
		{"..", "/", []string{"/"}},
	} {
		if got := resolveTo(c.to, c.routes, cur).String(); got != c.want {
			t.Errorf("resolveTo(%q, %v) = %q, want %q", c.to, c.routes, got, c.want)
		}
	}
	if got := resolveTo("", []string{"/a"}, Location{Path: "/a/"}).String(); got != "/a/" {
		t.Errorf("an empty path keeps the trailing slash of the location: %q", got)
	}
}

func TestRoutePathnamesSkipIndexRoutesAndRepeats(t *testing.T) {
	ms, _ := matchRoutes([]Route{{Path: "/", Children: []Route{{Index: true}}}}, "/")
	if got := routePathnames(ms, 1); !reflect.DeepEqual(got, []string{"/"}) {
		t.Errorf("pathnames %q: the index route adds none", got)
	}
}

func TestParseLocation(t *testing.T) {
	for in, want := range map[string]Location{
		"/a":           {Path: "/a"},
		"/a?b=1":       {Path: "/a", Query: "b=1"},
		"/a#h":         {Path: "/a", Hash: "h"},
		"/a?b=1#h?x":   {Path: "/a", Query: "b=1", Hash: "h?x"},
		"?b=1":         {Query: "b=1"},
		"":             {},
		"/a?":          {Path: "/a"},
		"/a?b=1&b=2#z": {Path: "/a", Query: "b=1&b=2", Hash: "z"},
	} {
		if got := parseLocation(in); got != want {
			t.Errorf("parseLocation(%q) = %+v, want %+v", in, got, want)
		}
	}
	if s := (Location{Path: "/a", Query: "q", Hash: "h"}).String(); s != "/a?q#h" {
		t.Errorf("String %q", s)
	}
}

func TestDeepLinkLocation(t *testing.T) {
	for _, c := range []struct {
		link string
		want string
		ok   bool
	}{
		{"notes://notes/3", "/notes/3", true},
		{"notes:/notes/3", "/notes/3", true},
		{"notes:notes/3", "/notes/3", true},
		{"Notes:open?id=3", "/open?id=3", true},
		{"notes://notes/3?a=b#c", "/notes/3?a=b#c", true},
		{"notes:", "/", true},
		{"notes", "", false},
		{"notebook://x", "", false},
		{"https://notes/3", "", false},
		{"xnotes:/a", "", false},
	} {
		got, ok := deepLinkLocation("notes", c.link)
		if ok != c.ok || ok && got.String() != c.want {
			t.Errorf("deepLinkLocation(%q) = %q, %v; want %q, %v", c.link, got, ok, c.want, c.ok)
		}
	}
}

func TestHistory(t *testing.T) {
	h := newHistory("")
	if h.current().String() != "/" || len(h.Entries) != 1 {
		t.Fatalf("new history %+v", h)
	}
	h = h.push(Location{Path: "/a"}).push(Location{Path: "/b"})
	h = h.move(-1)
	if h.current().Path != "/a" {
		t.Errorf("after back at %v", h.current())
	}
	h = h.push(Location{Path: "/c"}) // drops /b
	if !reflect.DeepEqual(h.Entries, []string{"/", "/a", "/c"}) || h.Index != 2 {
		t.Errorf("history %+v", h)
	}
	if h2 := h.move(5).move(-9); !reflect.DeepEqual(h2, h) {
		t.Errorf("moving beyond the ends changed it: %+v", h2)
	}
	r := h.replace(Location{Path: "/d"})
	if !reflect.DeepEqual(r.Entries, []string{"/", "/a", "/d"}) || !reflect.DeepEqual(h.Entries, []string{"/", "/a", "/c"}) {
		t.Errorf("replace %+v changed the old history %+v", r, h)
	}
	if got := newHistory("notes?x=1").current().String(); got != "/notes?x=1" {
		t.Errorf("an initial without a leading slash: %q", got)
	}
}
