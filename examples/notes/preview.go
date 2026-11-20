package main

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// The Markdown preview of a note (kit.Markdown) edits the note through its
// source and links to other notes inside the app.

// taskItem matches a task-list item's marker at the start of a line: a list
// bullet or number, optionally inside block quotes, then [ ] or [x].
var taskItem = regexp.MustCompile(`^((?:[ \t]*>)*[ \t]*(?:[-*+]|[0-9]{1,9}[.)])[ \t]+\[)([ xX])(\](?:[ \t]|$))`)

// fenceLine matches the line that opens or closes a fenced code block,
// where a task-looking line is only text.
var fenceLine = regexp.MustCompile("^ {0,3}(```|~~~)")

// toggleTask checks or unchecks the task of the document with that index
// (kit.Markdown counts the tasks from 0, in document order) by changing its
// marker in the source. A source with no such task is returned unchanged.
func toggleTask(source string, index int, checked bool) string {
	lines := strings.Split(source, "\n")
	n, fenced := 0, false
	for i, line := range lines {
		if fenceLine.MatchString(line) {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		m := taskItem.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		if n == index {
			mark := " "
			if checked {
				mark = "x"
			}
			lines[i] = line[:m[4]] + mark + line[m[5]:]
			return strings.Join(lines, "\n")
		}
		n++
	}
	return source
}

// noteRef is what a link in a note names of another note, or "" when the
// link is not one: "notes:3", "notes://3", "notes:Groceries" and a
// relative link ("3", "./3", "/notes/3", "Groceries") are references to a
// note, by its number or its title; any link with another scheme, and "#..."
// links, are not.
func noteRef(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	var ref string
	switch {
	case strings.EqualFold(u.Scheme, "notes"):
		ref = u.Host + u.Opaque + u.Path
	case u.Scheme == "" && u.Host == "" && u.Path != "":
		ref = u.Path
	default:
		return ""
	}
	ref = strings.TrimPrefix(strings.Trim(ref, "/"), "notes/")
	ref = strings.TrimPrefix(ref, "./")
	return strings.TrimSpace(ref)
}

// findRef returns the note a reference names: its number, or else a note
// whose heading is the reference, ignoring case.
func findRef(ns []Note, ref string) (Note, bool) {
	if id, err := strconv.Atoi(ref); err == nil {
		return find(ns, id)
	}
	for _, n := range ns {
		if !n.Hidden() && strings.EqualFold(n.Heading(), ref) {
			return n, true
		}
	}
	return Note{}, false
}
