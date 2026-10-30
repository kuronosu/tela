package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// now is the clock of the app: the tests replace it so that the dates of
// the goldens do not move.
var now = time.Now

// Note is one note of the library. The library is the list of notes the
// app keeps in notes.json (desktop.DataDir): every edit is stored there, so
// nothing typed is lost even if the app is killed. A note can also be tied
// to a file on disk (Path), which Open and Save do.
//
// notes.json files of earlier versions have no title, folder, pin or date:
// those fields read as empty, and the note still looks right (its heading
// is its first line, it has no folder and no date).
type Note struct {
	ID int `json:"id"`
	// Title is the text of the editor's title field. It is separate from
	// Text, which is the body and the only thing a file holds.
	Title string `json:"title"`
	// Text is the body of the note, as the editor shows it.
	Text string `json:"text"`
	// Saved is the text the note had when it was last read from or
	// written to its file (or when it was created): the note is dirty
	// while Text differs from it.
	Saved string `json:"saved"`
	// Path is the file the note was opened from or saved to; empty for a
	// note that only lives in the library.
	Path string `json:"path"`
	// Folder is the folder of the rail the note is in; empty for none.
	Folder string `json:"folder"`
	// Pinned notes come first in every list.
	Pinned bool `json:"pinned"`
	// Modified is when the title or the text last changed; zero for a
	// note of an earlier version.
	Modified time.Time `json:"modified"`
}

// Dirty reports whether the note has changes that were not saved to a file.
func (n Note) Dirty() bool { return n.Text != n.Saved }

// fromText reports whether the heading of the note is the first line of
// its text, which is so while it has neither a title nor a file.
func (n Note) fromText() bool { return strings.TrimSpace(n.Title) == "" && n.Path == "" }

// Heading is what the lists and the messages call the note: its title,
// else the name of its file, else its first line of text, else "Untitled
// note".
func (n Note) Heading() string {
	if t := strings.TrimSpace(n.Title); t != "" {
		return clip(t, 60)
	}
	if n.Path != "" {
		return filepath.Base(n.Path)
	}
	for line := range strings.SplitSeq(n.Text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return clip(line, 60)
		}
	}
	return "Untitled note"
}

// Snippet is the muted text of a card: the text after the heading, with
// its lines joined, at most limit characters. A note with no more text
// than its heading has "No additional text".
func (n Note) Snippet(limit int) string {
	skip := n.fromText()
	var words []string
	for line := range strings.SplitSeq(n.Text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case skip:
			skip = false
		default:
			words = append(words, line)
		}
	}
	if len(words) == 0 {
		return "No additional text"
	}
	return clip(strings.Join(words, " "), limit)
}

// FileName is the name Save suggests for a note with no file yet: its
// heading, made safe for every file system, with the extension of text
// files.
func (n Note) FileName() string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '-'
		}
		return r
	}, n.Heading())
	name = strings.Trim(strings.TrimSpace(name), ".")
	if name == "" {
		name = "Untitled note"
	}
	return name + ".txt"
}

// Words and Characters count what the status line shows: the text of the
// note.
func (n Note) Words() int      { return len(strings.Fields(n.Text)) }
func (n Note) Characters() int { return utf8.RuneCountInString(n.Text) }

// Tags are the #hashtags of the text, lowercase, without the #.
func (n Note) Tags() []string { return parseTags(n.Text) }

// When is the date of the card: see whenLabel.
func (n Note) When() string { return whenLabel(n.Modified, now()) }

// clip shortens s to at most n characters, ending with an ellipsis.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// parseTags returns the hashtags of text, in order of appearance and
// without repeats, lowercase and without the #. A tag is a # that starts the
// text or follows a space or a line break, followed by letters, digits, "-"
// and "_" (a trailing "-" or "_" is not part of it). So "# Title" (a
// heading), "C#", "a#b" and the anchor of "https://x.test/#top" are not
// tags, and "#travel" and "#2026" are.
func parseTags(text string) []string {
	var tags []string
	rs := []rune(text)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '#' || (i > 0 && !unicode.IsSpace(rs[i-1])) {
			continue
		}
		j := i + 1
		for j < len(rs) && isTagRune(rs[j]) {
			j++
		}
		tag := strings.ToLower(strings.TrimRight(string(rs[i+1:j]), "-_"))
		if tag != "" && !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
		i = j - 1
	}
	return tags
}

func isTagRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
}

// whenLabel is how a card tells when a note changed, relative to at: "Today,
// 09:41", "Yesterday, 22:03", the weekday ("Mon") for the rest of the last
// week, and the date ("Oct 12", with the year when it is not this one)
// before that. A note with no date (zero) has no label.
func whenLabel(t, at time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.In(at.Location())
	civil := func(x time.Time) time.Time {
		y, m, d := x.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	days := int(civil(at).Sub(civil(t)).Hours() / 24)
	switch {
	case days == 0:
		return "Today, " + t.Format("15:04")
	case days == 1:
		return "Yesterday, " + t.Format("15:04")
	case days >= 2 && days < 7:
		return t.Format("Mon")
	case t.Year() == at.Year():
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}

// filter is what the rail has chosen: a view, a folder or a tag.
type filter struct{ Kind, Name string }

// The kinds of filter.
const (
	kindView   = "view"
	kindFolder = "folder"
	kindTag    = "tag"
)

// The views of the rail.
var (
	viewAll     = filter{kindView, "all"}
	viewPinned  = filter{kindView, "pinned"}
	viewUnsaved = filter{kindView, "unsaved"}
)

// Title is the heading of the overview of the filter.
func (f filter) Title() string {
	switch {
	case f == viewPinned:
		return "Pinned"
	case f == viewUnsaved:
		return "Unsaved"
	case f.Kind == kindFolder:
		return f.Name
	case f.Kind == kindTag:
		return "#" + f.Name
	}
	return "All notes"
}

// Match reports whether the note belongs to the filter.
func (f filter) Match(n Note) bool {
	switch f.Kind {
	case kindFolder:
		return n.Folder == f.Name
	case kindTag:
		return slices.Contains(n.Tags(), f.Name)
	}
	switch f.Name {
	case "pinned":
		return n.Pinned
	case "unsaved":
		return n.Dirty()
	}
	return true
}

// matchesQuery reports whether the title or the text of the note has the
// query (ignoring case); an empty query matches every note.
func matchesQuery(n Note, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(n.Heading()+"\n"+n.Title+"\n"+n.Text), q)
}

// visible returns the notes of the filter that have the query, pinned notes
// first and then the most recently changed (a note with no date goes after
// the ones that have one, the newest created first). It is the order of
// every list and of the overview.
func visible(ns []Note, f filter, query string) []Note {
	var out []Note
	for _, n := range ns {
		if f.Match(n) && matchesQuery(n, query) {
			out = append(out, n)
		}
	}
	slices.SortStableFunc(out, func(a, b Note) int {
		if a.Pinned != b.Pinned {
			if a.Pinned {
				return -1
			}
			return 1
		}
		if c := b.Modified.Compare(a.Modified); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	})
	return out
}

// folders are the folders of the notes, in alphabetical order. A folder
// exists while a note is in it.
func folders(ns []Note) []string {
	var out []string
	for _, n := range ns {
		if n.Folder != "" && !slices.Contains(out, n.Folder) {
			out = append(out, n.Folder)
		}
	}
	slices.SortFunc(out, func(a, b string) int { return cmp.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return out
}

// tagsOf are the tags of all the notes, in alphabetical order.
func tagsOf(ns []Note) []string {
	var out []string
	for _, n := range ns {
		for _, t := range n.Tags() {
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	slices.Sort(out)
	return out
}

// folderNamed is the name a new folder gets: the spelling of an existing
// folder that differs only in case, else the name as typed (trimmed).
func folderNamed(ns []Note, name string) string {
	name = strings.TrimSpace(name)
	for _, f := range folders(ns) {
		if strings.EqualFold(f, name) {
			return f
		}
	}
	return name
}

// welcomeNotes is the library of the first run: a few short notes that
// show the folders, the tags and the pin. It is a function so that there is
// no package-level slice for the code to modify by mistake. at is the date of the
// first note, and the others go back in time from there.
func welcomeNotes(at time.Time) []Note {
	note := func(id int, title, folder string, ago time.Duration, text string) Note {
		return Note{ID: id, Title: title, Text: text, Saved: text, Folder: folder, Modified: at.Add(-ago)}
	}
	welcome := note(1, "Welcome to Notes", "Getting started", 0, welcomeText)
	welcome.Pinned = true
	return []Note{
		welcome,
		note(2, "Keyboard shortcuts", "Getting started", 86*time.Minute, shortcutsText),
		note(3, "Groceries", "Home", 11*time.Hour+38*time.Minute,
			"Coffee beans, oats, lemons, olive oil. Ask at the bakery if they still do the seeded rye.\n\n#errands"),
		note(4, "Weekend in the mountains", "Ideas", 4*24*time.Hour,
			"Leave on Friday after lunch, back on Sunday night. A cabin with a stove, a thermos, and no signal.\n\n#travel #ideas"),
	}
}

const welcomeText = `Everything you type is kept in your library, even if you close the app without saving.

Use Open to read a text file into a new note, and Save to write a note to a file. Folders and tags are in the rail: pick a folder at the top of a note, and tag it by writing a hash and a word anywhere in its text, like the one below.

#welcome`

const shortcutsText = `Ctrl+N makes a note, Ctrl+O opens a file, Ctrl+S saves the note to its file and Ctrl+, opens Preferences. Press Escape in the list to go back to all your notes.

#welcome`

// nextID is an ID no note of ns has.
func nextID(ns []Note) int {
	id := 0
	for _, n := range ns {
		id = max(id, n.ID)
	}
	return id + 1
}

// find returns the note with the ID.
func find(ns []Note, id int) (Note, bool) {
	for _, n := range ns {
		if n.ID == id {
			return n, true
		}
	}
	return Note{}, false
}

// byPath returns the note tied to the file, if there is one.
func byPath(ns []Note, path string) (Note, bool) {
	for _, n := range ns {
		if n.Path == path {
			return n, true
		}
	}
	return Note{}, false
}

// edit returns ns with the note id changed by f. It copies the slice: the
// previous value may still be in use by a render.
func edit(ns []Note, id int, f func(*Note)) []Note {
	out := append([]Note(nil), ns...)
	for i := range out {
		if out[i].ID == id {
			f(&out[i])
		}
	}
	return out
}

// editContent is edit for a change of the title or the text: the note is
// dated now.
func editContent(ns []Note, id int, f func(*Note)) []Note {
	return edit(ns, id, func(n *Note) {
		f(n)
		n.Modified = now()
	})
}

// without returns ns with the note id left out (an empty slice, not nil, when
// none is left: the library stores "[]").
func without(ns []Note, id int) []Note {
	out := make([]Note, 0, len(ns))
	for _, n := range ns {
		if n.ID != id {
			out = append(out, n)
		}
	}
	return out
}

// neighbor is the note to show after deleting the note id from the list:
// the one after it, else the one before it; false when it was the only one.
func neighbor(list []Note, id int) (Note, bool) {
	for i, n := range list {
		if n.ID != id {
			continue
		}
		switch {
		case i+1 < len(list):
			return list[i+1], true
		case i > 0:
			return list[i-1], true
		}
	}
	return Note{}, false
}

// dirty returns the notes with unsaved changes.
func dirty(ns []Note) []Note {
	var out []Note
	for _, n := range ns {
		if n.Dirty() {
			out = append(out, n)
		}
	}
	return out
}

// fileTitle is the title of a note opened from a file: its name without the
// extension.
func fileTitle(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// maxFileSize is the biggest file Open reads: this is a plain-text notes
// app, and the text field is not made for megabytes.
const maxFileSize = 2 << 20

// readTextFile reads a text file for Open: UTF-8 (with or without a byte
// order mark) and at most maxFileSize. Line endings become "\n".
func readTextFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("the file is %d MB; Notes opens text files up to %d MB", info.Size()>>20, maxFileSize>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", errors.New("the file is not UTF-8 text")
	}
	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	return strings.ReplaceAll(text, "\r\n", "\n"), nil
}

// writeTextFile writes a note's text for Save.
func writeTextFile(path, text string) error {
	return os.WriteFile(path, []byte(text), 0o644)
}
