package main

import (
	"encoding/json"
	"slices"
	"testing"
	"time"
)

func TestParseTags(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"", nil},
		{"no tags here", nil},
		{"#travel", []string{"travel"}},
		{"Trip #travel and #2026", []string{"travel", "2026"}},
		{"line one\n#idea\n\t#Other-Thing_2", []string{"idea", "other-thing_2"}},
		{"#Travel #travel #TRAVEL", []string{"travel"}},
		// Not tags: headings, words with a # inside, anchors, bare hashes.
		{"# Title\n## Section\n###No space", nil},
		{"C# and a#b and https://x.test/#top", nil},
		{"# #", nil},
		{"#-", nil},
		{"#trailing- and #under_ and #ok", []string{"trailing", "under", "ok"}},
		{"#café #日本", []string{"café", "日本"}},
		{"(#paren) #real,", []string{"real"}},
	} {
		if got := parseTags(tc.text); !slices.Equal(got, tc.want) {
			t.Errorf("parseTags(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestWhenLabel(t *testing.T) {
	at := time.Date(2026, time.October, 30, 9, 41, 0, 0, time.UTC) // a Friday
	for _, tc := range []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, ""},
		{at, "Today, 09:41"},
		{time.Date(2026, 10, 30, 0, 5, 0, 0, time.UTC), "Today, 00:05"},
		{time.Date(2026, 10, 29, 22, 3, 0, 0, time.UTC), "Yesterday, 22:03"},
		{time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC), "Wed"},
		{time.Date(2026, 10, 26, 18, 30, 0, 0, time.UTC), "Mon"},
		{time.Date(2026, 10, 23, 18, 30, 0, 0, time.UTC), "Oct 23"}, // 7 days ago is not the week any more
		{time.Date(2026, 10, 12, 11, 20, 0, 0, time.UTC), "Oct 12"},
		{time.Date(2026, 1, 3, 8, 0, 0, 0, time.UTC), "Jan 3"},
		{time.Date(2025, 12, 31, 8, 0, 0, 0, time.UTC), "Dec 31, 2025"},
		// A time of another zone is read in the clock's zone.
		{time.Date(2026, 10, 30, 1, 0, 0, 0, time.FixedZone("far", 14*3600)), "Yesterday, 11:00"},
	} {
		if got := whenLabel(tc.t, at); got != tc.want {
			t.Errorf("whenLabel(%v) = %q, want %q", tc.t, got, tc.want)
		}
	}
	// The six days before a Friday are the week: Saturday to Thursday.
	if got := whenLabel(time.Date(2026, 10, 24, 18, 30, 0, 0, time.UTC), at); got != "Sat" {
		t.Errorf("six days ago = %q, want Sat", got)
	}
}

func TestHeadingAndSnippet(t *testing.T) {
	for _, tc := range []struct {
		name     string
		note     Note
		heading  string
		snippet  string
		fileName string
	}{
		{"title", Note{Title: " Plan ", Text: "first\nsecond"}, "Plan", "first second", "Plan.txt"},
		{"first line", Note{Text: "\n  Groceries \nmilk\n\neggs"}, "Groceries", "milk eggs", "Groceries.txt"},
		{"only a heading", Note{Text: "Groceries"}, "Groceries", "No additional text", "Groceries.txt"},
		{"file", Note{Path: "/tmp/todo.md", Text: "ship it"}, "todo.md", "ship it", "todo.md.txt"},
		{"title wins over file", Note{Title: "todo", Path: "/tmp/todo.md", Text: "ship it"}, "todo", "ship it", "todo.txt"},
		{"empty", Note{}, "Untitled note", "No additional text", "Untitled note.txt"},
		{"unsafe name", Note{Title: `a/b: c?`}, `a/b: c?`, "No additional text", "a-b- c-.txt"},
	} {
		if got := tc.note.Heading(); got != tc.heading {
			t.Errorf("%s: Heading() = %q, want %q", tc.name, got, tc.heading)
		}
		if got := tc.note.Snippet(100); got != tc.snippet {
			t.Errorf("%s: Snippet() = %q, want %q", tc.name, got, tc.snippet)
		}
		if got := tc.note.FileName(); got != tc.fileName {
			t.Errorf("%s: FileName() = %q, want %q", tc.name, got, tc.fileName)
		}
	}
	if got := (Note{Title: "T", Text: "one two three four"}).Snippet(10); got != "one two t…" {
		t.Errorf("Snippet(10) = %q, want it clipped with an ellipsis", got)
	}
}

// library is a small library of notes, for the filter tests.
func testLibrary() []Note {
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	return []Note{
		{ID: 1, Title: "Trip", Text: "Packing list #travel #2026", Saved: "Packing list #travel #2026", Folder: "Journal", Modified: at.Add(-time.Hour)},
		{ID: 2, Title: "Agenda", Text: "Q4 goals #meeting", Saved: "old", Folder: "Work", Modified: at.Add(-2 * time.Hour)},
		{ID: 3, Title: "Reading", Text: "Piranesi #books", Saved: "Piranesi #books", Folder: "Journal", Pinned: true, Modified: at.Add(-24 * time.Hour)},
		{ID: 4, Text: "Loose thought", Saved: "Loose thought", Modified: at},
		{ID: 5, Title: "Old note", Text: "from before", Saved: "from before"}, // no date: an earlier version's note
	}
}

func ids(ns []Note) []int {
	var out []int
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

func TestFilters(t *testing.T) {
	ns := testLibrary()
	for _, tc := range []struct {
		name string
		f    filter
		want []int
	}{
		// Pinned first, then the newest; a note with no date goes last.
		{"all", viewAll, []int{3, 4, 1, 2, 5}},
		{"pinned", viewPinned, []int{3}},
		{"unsaved", viewUnsaved, []int{2}},
		{"folder", filter{kindFolder, "Journal"}, []int{3, 1}},
		{"folder with no notes", filter{kindFolder, "Nope"}, nil},
		{"tag", filter{kindTag, "travel"}, []int{1}},
		{"tag 2026", filter{kindTag, "2026"}, []int{1}},
		{"tag with no notes", filter{kindTag, "nope"}, nil},
	} {
		got := ids(visible(ns, tc.f, ""))
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: visible = %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := folders(ns); !slices.Equal(got, []string{"Journal", "Work"}) {
		t.Errorf("folders = %q", got)
	}
	if got := tagsOf(ns); !slices.Equal(got, []string{"2026", "books", "meeting", "travel"}) {
		t.Errorf("tags = %q", got)
	}
	// The input is not reordered.
	if got := ids(ns); !slices.Equal(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("visible changed the library's order: %v", got)
	}
}

func TestSearch(t *testing.T) {
	ns := testLibrary()
	for _, tc := range []struct {
		query string
		f     filter
		want  []int
	}{
		{"trip", viewAll, []int{1}},     // the title
		{"PIRANESI", viewAll, []int{3}}, // the text, ignoring case
		{"  loose ", viewAll, []int{4}}, // the heading of a note with no title, trimmed
		{"goals", filter{kindFolder, "Work"}, []int{2}},
		{"goals", filter{kindFolder, "Journal"}, nil}, // the filter and the search both apply
		{"", viewAll, []int{3, 4, 1, 2, 5}},
		{"zzz", viewAll, nil},
	} {
		got := ids(visible(ns, tc.f, tc.query))
		if !slices.Equal(got, tc.want) {
			t.Errorf("search %q in %v = %v, want %v", tc.query, tc.f, got, tc.want)
		}
	}
}

func TestFolderNamed(t *testing.T) {
	ns := testLibrary()
	for in, want := range map[string]string{"journal": "Journal", "  WORK ": "Work", " Recipes ": "Recipes", "": ""} {
		if got := folderNamed(ns, in); got != want {
			t.Errorf("folderNamed(%q) = %q, want %q", in, got, want)
		}
	}
}

// A library written by the version of Notes before titles, folders, pins
// and dates still loads, and its notes read well.
func TestOldNotesJSONLoads(t *testing.T) {
	old := `[{"id":1,"text":"Groceries\nmilk and eggs","saved":"Groceries\nmilk and eggs"},
		{"id":2,"text":"Todo\n- ship it\n","saved":"Todo\n- ship it\n","path":"/home/ana/todo.md"},
		{"id":3,"text":"draft #idea","saved":""}]`
	var ns []Note
	if err := json.Unmarshal([]byte(old), &ns); err != nil {
		t.Fatal(err)
	}
	if len(ns) != 3 {
		t.Fatalf("decoded %d notes, want 3", len(ns))
	}
	n := ns[0]
	if n.Heading() != "Groceries" || n.Snippet(50) != "milk and eggs" || n.Folder != "" || n.Pinned || n.When() != "" || n.Dirty() {
		t.Errorf("old note = heading %q, snippet %q, folder %q, pinned %v, when %q, dirty %v",
			n.Heading(), n.Snippet(50), n.Folder, n.Pinned, n.When(), n.Dirty())
	}
	if n := ns[1]; n.Heading() != "todo.md" || n.Snippet(50) != "Todo - ship it" {
		t.Errorf("old note with a file = heading %q, snippet %q", n.Heading(), n.Snippet(50))
	}
	if n := ns[2]; !n.Dirty() || !slices.Equal(n.Tags(), []string{"idea"}) {
		t.Errorf("old draft = dirty %v, tags %q", n.Dirty(), n.Tags())
	}
	// All of them are in the list, the newest created first.
	if got := ids(visible(ns, viewAll, "")); !slices.Equal(got, []int{3, 2, 1}) {
		t.Errorf("visible = %v, want [3 2 1]", got)
	}
	// What it writes reads back the same.
	data, err := json.Marshal(ns)
	if err != nil {
		t.Fatal(err)
	}
	var back []Note
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(ns, back, func(a, b Note) bool { return a == b }) {
		t.Errorf("the library does not read back as it was written:\n%+v\n%+v", ns, back)
	}
}
