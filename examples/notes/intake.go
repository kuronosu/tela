package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// What comes into the library from outside, besides the Open button: files
// dropped on the window or named on the command line, text dropped on it,
// and tela-notes: links (link.go). They all end in the same place, so they
// behave alike: a file that is already open selects its note, any other
// text file becomes a new note, and what cannot be read is a message, not a
// failure of the rest.

// isTextFile reports whether Notes opens the file: the extensions of the
// Open dialog's filter.
func isTextFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md":
		return true
	}
	return false
}

// problem is a file that was not imported, and why.
type problem struct{ Name, Reason string }

// intake is the result of importing files.
type intake struct {
	// Fresh are the new notes, to add to the library, in order.
	Fresh []Note
	// Open is the ID of the note to show: the last file's, new or already
	// open; 0 when no file could be used.
	Open int
	// Problems are the files that were left out.
	Problems []problem
}

// importFiles reads the text files at paths into new notes made from place
// (the folder or the pin of where the user is), with newID giving each its
// ID. A file that a note of ns is tied to, or that an earlier path of the
// same call made a note of, selects that note instead of making a second
// one. Files that cannot be read as text are left out and named in
// Problems, and so are the ones that are not .txt or .md, unless anyFile:
// the user chose them (Open, a command line), while a drop can be anything.
func importFiles(ns []Note, paths []string, place Note, newID func() int, anyFile bool) intake {
	var in intake
	for _, path := range paths {
		name := filepath.Base(path)
		if n, ok := byPath(ns, path); ok {
			in.Open = n.ID
			continue
		}
		if n, ok := byPath(in.Fresh, path); ok {
			in.Open = n.ID
			continue
		}
		if !anyFile && !isTextFile(path) {
			in.Problems = append(in.Problems, problem{name, "Notes opens .txt and .md files."})
			continue
		}
		text, err := readTextFile(path)
		if err != nil {
			in.Problems = append(in.Problems, problem{name, err.Error()})
			continue
		}
		n := place
		n.ID = newID()
		n.Title, n.Text, n.Saved, n.Path = fileTitle(path), text, text, path
		in.Fresh = append(in.Fresh, n)
		in.Open = n.ID
	}
	return in
}

// plural is "1 note" or "3 notes".
func plural(n int, thing string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, thing)
	}
	return fmt.Sprintf("%d %ss", n, thing)
}
