package main

import (
	"sort"
	"strings"
	"time"
)

// Kind is what an entry is, which decides its icon and its type name.
type Kind uint8

// The kinds of entry.
const (
	KindFolder Kind = iota
	KindText
	KindDoc
	KindSheet
	KindPDF
	KindImage
	KindAudio
	KindVideo
	KindArchive
	KindCode
	numKinds
)

// Entry is a file or a folder of the fake in-memory file system.
type Entry struct {
	Name     string
	Kind     Kind
	Size     int64 // bytes; unused for folders
	Modified time.Time
	Children []*Entry // folders only
	Hidden   bool     // reachable by path (the sidebar) but not listed in its parent
}

// Ext returns the extension of a file name without the dot, in lower case.
func (e *Entry) Ext() string {
	if i := strings.LastIndexByte(e.Name, '.'); i > 0 {
		return strings.ToLower(e.Name[i+1:])
	}
	return ""
}

// IsFolder reports whether the entry is a folder.
func (e *Entry) IsFolder() bool { return e.Kind == KindFolder }

// Visible returns the children of a folder that are listed.
func (e *Entry) Visible() []*Entry {
	out := make([]*Entry, 0, len(e.Children))
	for _, c := range e.Children {
		if !c.Hidden {
			out = append(out, c)
		}
	}
	return out
}

// Path is the location of a folder: its names joined with "/", the empty
// string being the home folder.
type Path = string

// Join returns the path of a child of the folder at p.
func Join(p Path, name string) Path {
	if p == "" {
		return name
	}
	return p + "/" + name
}

// Parent returns the path of the folder that holds p ("" for the home folder
// and for a first-level folder).
func Parent(p Path) Path {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return ""
}

// Base returns the last name of a path.
func Base(p Path) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// Segments returns the names of a path, from the first level down.
func Segments(p Path) []string {
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// Lookup returns the entry at path p, or nil.
func Lookup(p Path) *Entry {
	e := root
	for _, name := range Segments(p) {
		var next *Entry
		for _, c := range e.Children {
			if c.Name == name {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		e = next
	}
	return e
}

// Column is a column of the file list.
type Column uint8

// The columns of the file list.
const (
	ColName Column = iota
	ColModified
	ColType
	ColSize
)

// Sorted returns the listed children of a folder, folders first, ordered by
// the column. Names compare case-insensitively; ties fall back to the name.
func Sorted(dir *Entry, by Column, desc bool, query string) []*Entry {
	query = strings.ToLower(strings.TrimSpace(query))
	var out []*Entry
	for _, c := range dir.Visible() {
		if query == "" || strings.Contains(strings.ToLower(c.Name), query) {
			out = append(out, c)
		}
	}
	less := func(a, b *Entry) bool {
		switch by {
		case ColModified:
			if !a.Modified.Equal(b.Modified) {
				return a.Modified.Before(b.Modified)
			}
		case ColType:
			if a.Kind != b.Kind {
				return a.Kind < b.Kind
			}
		case ColSize:
			if a.Size != b.Size {
				return a.Size < b.Size
			}
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.IsFolder() != b.IsFolder() {
			return a.IsFolder()
		}
		if desc {
			return less(b, a)
		}
		return less(a, b)
	})
	return out
}

// Total returns the size of the files in a folder, not counting subfolders.
func Total(entries []*Entry) (files, folders int, size int64) {
	for _, e := range entries {
		if e.IsFolder() {
			folders++
		} else {
			files++
			size += e.Size
		}
	}
	return
}

func at(month time.Month, day, hour, min int) time.Time {
	return time.Date(2026, month, day, hour, min, 0, 0, time.UTC)
}

func dir(name string, mod time.Time, children ...*Entry) *Entry {
	return &Entry{Name: name, Kind: KindFolder, Modified: mod, Children: children}
}

func file(name string, kind Kind, size int64, mod time.Time) *Entry {
	return &Entry{Name: name, Kind: kind, Size: size, Modified: mod}
}

const (
	kb = int64(1024)
	mb = 1024 * kb
)

// root is the home folder of the fake file system.
var root = func() *Entry {
	trash := dir("Trash", at(10, 2, 9, 12),
		file("old-draft.txt", KindText, 3*kb, at(9, 30, 18, 4)),
		file("screenshot-0412.png", KindImage, 412*kb, at(9, 28, 11, 40)),
	)
	trash.Hidden = true
	data := dir("Data", at(10, 14, 8, 0),
		dir("Backups", at(10, 1, 3, 0)),
		dir("Archive 2025", at(1, 3, 10, 0)),
		file("disk-image.zip", KindArchive, 1840*mb, at(8, 12, 22, 7)),
	)
	data.Hidden = true
	return dir("", at(10, 14, 8, 0),
		dir("Desktop", at(10, 13, 17, 5),
			file("notes.txt", KindText, 2*kb, at(10, 13, 17, 5)),
			file("Screenshot 2026-10-13.png", KindImage, 1210*kb, at(10, 13, 16, 58)),
		),
		dir("Documents", at(10, 12, 15, 42),
			dir("Projects", at(10, 12, 15, 42),
				dir("tela", at(10, 12, 15, 42),
					file("README.md", KindText, 9*kb, at(10, 11, 9, 20)),
					file("main.go", KindCode, 2*kb, at(10, 12, 15, 42)),
					file("go.mod", KindCode, kb, at(10, 1, 12, 0)),
				),
				dir("website", at(9, 29, 10, 15)),
			),
			dir("Reports", at(10, 8, 9, 30),
				file("Q3 summary.pdf", KindPDF, 1480*kb, at(10, 8, 9, 30)),
				file("Q3 numbers.xlsx", KindSheet, 96*kb, at(10, 7, 17, 11)),
			),
			dir("Taxes 2025", at(4, 14, 18, 2)),
			file("Budget 2026.xlsx", KindSheet, 214*kb, at(10, 10, 11, 5)),
			file("Contract draft.docx", KindDoc, 58*kb, at(10, 9, 14, 22)),
			file("Meeting notes.txt", KindText, 4*kb, at(10, 12, 15, 40)),
			file("Resume.pdf", KindPDF, 732*kb, at(8, 19, 10, 3)),
			file("Quarterly review.docx", KindDoc, 131*kb, at(9, 25, 16, 48)),
			file("todo.md", KindText, 1*kb, at(10, 13, 8, 12)),
		),
		dir("Downloads", at(10, 14, 7, 31),
			file("invoice-0412.pdf", KindPDF, 181*kb, at(10, 14, 7, 31)),
			file("photos-export.zip", KindArchive, 48*mb, at(10, 11, 20, 14)),
			file("tela-0.5.1.tar.gz", KindArchive, 6*mb, at(10, 9, 13, 45)),
			file("wallpaper.jpg", KindImage, 3*mb, at(10, 5, 19, 2)),
			file("podcast-episode.mp3", KindAudio, 38*mb, at(9, 30, 8, 55)),
		),
		dir("Music", at(8, 2, 21, 17),
			dir("Albums", at(8, 2, 21, 17)),
			file("Morning light.flac", KindAudio, 29*mb, at(6, 14, 7, 0)),
			file("Night drive.mp3", KindAudio, 9*mb, at(6, 14, 7, 4)),
		),
		dir("Pictures", at(10, 10, 12, 50),
			dir("Screenshots", at(10, 13, 16, 58)),
			dir("Vacation 2025", at(8, 30, 20, 11)),
			file("harbor.jpg", KindImage, 4*mb, at(10, 10, 12, 50)),
			file("portrait.png", KindImage, 2*mb, at(9, 18, 9, 33)),
			file("sunrise.jpg", KindImage, 5*mb, at(7, 7, 6, 12)),
		),
		dir("Videos", at(9, 21, 19, 40),
			file("demo-reel.mp4", KindVideo, 412*mb, at(9, 21, 19, 40)),
			file("family-trip.mkv", KindVideo, 1210*mb, at(8, 30, 22, 5)),
		),
		trash,
		data,
		file("hello.go", KindCode, 1*kb, at(10, 3, 10, 5)),
	)
}()
