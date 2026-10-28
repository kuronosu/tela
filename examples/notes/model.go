package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Note is one note of the library. The library is the list of notes the
// app keeps in notes.json (desktop.DataDir): every edit is stored there, so
// nothing typed is lost even if the app is killed. A note can also be tied
// to a file on disk (Path), which Open and Save do.
type Note struct {
	ID int `json:"id"`
	// Text is what the editor shows.
	Text string `json:"text"`
	// Saved is the text the note had when it was last read from or
	// written to its file (or when it was created): the note is dirty
	// while Text differs from it.
	Saved string `json:"saved"`
	// Path is the file the note was opened from or saved to; empty for a
	// note that only lives in the library.
	Path string `json:"path,omitempty"`
}

// Dirty reports whether the note has changes that were not saved to a file.
func (n Note) Dirty() bool { return n.Text != n.Saved }

// Title is what the sidebar and the title bar call the note: the name of
// its file, else its first line of text.
func (n Note) Title() string {
	if n.Path != "" {
		return filepath.Base(n.Path)
	}
	for line := range strings.SplitSeq(n.Text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return clip(line, 40)
		}
	}
	return "Untitled note"
}

// Preview is the muted line under the title in the sidebar: the text after
// the title line.
func (n Note) Preview() string {
	seenTitle := n.Path != ""
	for line := range strings.SplitSeq(n.Text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case !seenTitle:
			seenTitle = true
		default:
			return clip(line, 60)
		}
	}
	return "No additional text"
}

// FileName is the name Save suggests for a note with no file yet: its
// title, made safe for every file system, with the extension of text files.
func (n Note) FileName() string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '-'
		}
		return r
	}, n.Title())
	name = strings.Trim(strings.TrimSpace(name), ".")
	if name == "" {
		name = "Untitled note"
	}
	return name + ".txt"
}

// Words and Characters count what the status line shows.
func (n Note) Words() int      { return len(strings.Fields(n.Text)) }
func (n Note) Characters() int { return utf8.RuneCountInString(n.Text) }

// clip shortens s to at most n characters, ending with an ellipsis.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// welcomeNotes is the library of the first run. It is a function so that
// there is no package-level slice for the code to modify by mistake;
// desktop.UseStoreSetting itself decodes the stored library over a copy of
// the default, so the default is never changed.
func welcomeNotes() []Note { return []Note{{ID: 1, Text: welcomeText, Saved: welcomeText}} }

const welcomeText = `Welcome to Notes
Everything you type is kept in your library, even if you close the app without saving.

Use Open to read a text file into a new note, and Save to write a note to a file. Preferences has the font size, word wrap, and what closing the window does: it can keep Notes running in the tray.`

// nextID is an ID no note of ns has.
func nextID(ns []Note) int {
	id := 0
	for _, n := range ns {
		id = max(id, n.ID)
	}
	return id + 1
}

// find returns the note with the ID, or the first note when none has it
// (the library has a note selected whenever it has any).
func find(ns []Note, id int) (Note, bool) {
	for _, n := range ns {
		if n.ID == id {
			return n, true
		}
	}
	if len(ns) > 0 {
		return ns[0], true
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
