package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// mount opens a root with the settings of App (size, theme, root class).
// Dev mode is on, so any dev message (an unknown class, a hook misuse)
// fails the test.
func mount(t *testing.T, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.Mount(t, root, append([]telatest.Option{telatest.FromApp(App)}, opts...)...)
}

// The elements the tests use, by role and accessible name, as a user of a
// screen reader finds them.
var (
	projectBox    = telatest.ByRole("combobox", "Project")
	projectToggle = telatest.ByRole("button", "Show all projects")
	bell          = telatest.ByRole("button", "Notifications")
	viewButton    = telatest.ByRole("button", "View")
	actionsButton = telatest.ByRole("button", "Actions")
	newDeployment = telatest.ByRole("button", "New deployment")
	overviewTab   = telatest.ByRole("tab", "Overview")
	activityTab   = telatest.ByRole("tab", "Activity")
	alertsTab     = telatest.ByRole("tab", "Alerts")
)

// tab presses Tab n times: the screen is reached with the keyboard only.
func tab(a *telatest.App, n int) {
	for i := 0; i < n; i++ {
		a.Press("Tab")
	}
}

// section returns the part of an aria snapshot that starts at its first line
// whose text, indentation aside, begins with start (such as `- listbox "Project":`)
// and holds the lines indented deeper than it, without that indentation: the
// snapshot of one widget out of the whole window's.
func section(snapshot, start string) string {
	lines := strings.Split(snapshot, "\n")
	for i, l := range lines {
		text := strings.TrimLeft(l, " ")
		if !strings.HasPrefix(text, start) {
			continue
		}
		indent := len(l) - len(text)
		end := i + 1
		for end < len(lines) && len(lines[end])-len(strings.TrimLeft(lines[end], " ")) > indent {
			end++
		}
		out := lines[i:end]
		for j, o := range out {
			out[j] = o[indent:]
		}
		return strings.Join(out, "\n")
	}
	return ""
}

// optionsOf counts the options of the listbox with the given name in the
// window's aria snapshot.
func optionsOf(a *telatest.App, listbox string) int {
	return strings.Count(section(a.AriaSnapshot(), `- listbox "`+listbox+`"`), "- option")
}

// dedent trims the blank lines around a raw string and the indentation that
// all its other lines share, so that it can be written inside the code.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	indent := -1
	for _, l := range lines {
		if n := len(l) - len(strings.TrimLeft(l, " \t")); indent < 0 || n < indent {
			indent = n
		}
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight(l[min(indent, len(l)):], " \t")
	}
	return strings.Join(lines, "\n")
}

// expectSection checks one part of the window's aria snapshot against a raw
// string. The snapshot settles the app first.
func expectSection(t *testing.T, a *telatest.App, start, want string) {
	t.Helper()
	if got := section(a.AriaSnapshot(), start); got != dedent(want) {
		t.Errorf("the aria snapshot from %q is\n%s\nwant\n%s", start, got, dedent(want))
	}
}

// TestNoDevMessages mounts every preview at several window sizes: a class
// typo, a missing key or a hook misuse is reported in dev mode, which fails
// the test.
func TestNoDevMessages(t *testing.T) {
	for _, name := range Previews.Names() {
		for _, s := range [][2]float32{{1120, 760}, {720, 600}, {1600, 900}} {
			t.Run(fmt.Sprintf("%s/%gx%g", name, s[0], s[1]), func(t *testing.T) {
				mount(t, Previews[name], telatest.Size(s[0], s[1]))
			})
		}
	}
}
