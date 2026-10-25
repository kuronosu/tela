package main

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

func start(t *testing.T, id string, s tela.ColorScheme, path Path) *telatest.App {
	return mount(t, Explorer{Look: id, Scheme: s, Path: path})
}

// TestClickSelects: a click selects one row and a click on another moves the
// selection.
func TestClickSelects(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents")
		a.Click(cell("Resume.pdf"))
		a.Expect(row("Resume.pdf"), telatest.Selected(true))
		a.Click(cell("todo.md"))
		a.Expect(row("todo.md"), telatest.Selected(true))
		a.Expect(row("Resume.pdf"), telatest.Selected(false))
		a.ExpectText("todo.md")
	})
}

// TestNavigation: a double click enters a folder; back, forward and up.
func TestNavigation(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents")
		a.Expect(btn("Back"), telatest.Disabled(true)) // nothing to go back to

		a.DoubleClick(cell("Projects"))
		a.ExpectCount(cell("tela"), 1)
		a.ExpectCount(cell("website"), 1)
		a.ExpectCount(cell("Resume.pdf"), 0)
		a.ExpectCount(btn("Go to Projects"), 1)

		a.Click(btn("Up"))
		a.ExpectCount(cell("Resume.pdf"), 1)
		a.ExpectCount(btn("Go to Projects"), 0)

		a.Click(btn("Back")) // back to Projects
		a.ExpectCount(cell("tela"), 1)
		a.Click(btn("Back")) // back to Documents
		a.ExpectCount(cell("Resume.pdf"), 1)
		a.Click(btn("Forward"))
		a.ExpectCount(cell("tela"), 1)

		// Entering a folder drops the forward history.
		a.DoubleClick(cell("tela"))
		a.ExpectCount(cell("README.md"), 1)
		a.ExpectCount(cell("main.go"), 1)
		a.ExpectCount(btn("Go to tela"), 1)
	})
}

// TestSidebarAndPathBar: places and crumbs navigate, and the place of the
// open folder is the one that is on.
func TestSidebarAndPathBar(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents/Projects/tela")
		a.Click(place("Downloads"))
		a.ExpectCount(cell("wallpaper.jpg"), 1)
		a.ExpectCount(cell("main.go"), 0)
		a.Expect(place("Downloads"), telatest.Checked(true))
		a.Expect(place("Documents"), telatest.Checked(false))

		a.Click(place("Videos"))
		a.ExpectCount(cell("demo-reel.mp4"), 1)
		a.Expect(place("Downloads"), telatest.Checked(false))

		// The path bar: click a parent crumb.
		a = start(t, id, s, "Documents/Projects/tela")
		a.Click(btn("Go to Documents"))
		a.ExpectCount(cell("Resume.pdf"), 1)
		a.Click(btn("Go to " + Looks[id].Root))
		a.ExpectCount(cell("Desktop"), 1)
		a.ExpectCount(cell("hello.go"), 1)
		a.ExpectCount(cell("Trash"), 0) // hidden: reachable from the sidebar only
	})
}

// TestSearchFilters: typing narrows the list, clearing restores it, and
// navigating clears the search.
func TestSearchFilters(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents")
		a.ExpectCount(telatest.ByRole("row", ""), 10) // the header and nine entries

		a.Fill(search(), "doc")
		a.ExpectValue(search(), "doc")
		a.ExpectCount(telatest.ByRole("row", ""), 3) // Contract draft.docx, Quarterly review.docx
		a.ExpectCount(cell("Contract draft.docx"), 1)

		a.Fill(search(), "ZZZ")
		a.ExpectCount(telatest.ByRole("row", ""), 2)
		a.ExpectText("No results")

		a.Fill(search(), "")
		a.ExpectCount(telatest.ByRole("row", ""), 10)

		a.Fill(search(), "re")
		a.ExpectCount(cell("Reports"), 1)
		a.Click(place("Downloads"))
		a.ExpectValue(search(), "")
		a.ExpectCount(cell("wallpaper.jpg"), 1)
	})
}

// TestKeyboard: Down and Up move the selection, Enter opens a folder,
// Backspace goes back.
func TestKeyboard(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents")
		a.Press("ArrowDown")
		a.Expect(row("Projects"), telatest.Selected(true)) // the first entry
		a.Press("ArrowDown")
		a.Press("ArrowDown")
		a.Expect(row("Taxes 2025"), telatest.Selected(true))
		a.Press("ArrowUp")
		a.Expect(row("Reports"), telatest.Selected(true))
		a.Press("ArrowUp")
		a.Press("ArrowUp") // stays on the first entry
		a.Expect(row("Projects"), telatest.Selected(true))

		a.Press("Enter") // Projects
		a.ExpectCount(cell("tela"), 1)
		a.Press("ArrowDown")
		a.Press("Enter") // tela
		a.ExpectCount(cell("README.md"), 1)

		a.Press("Backspace")
		a.ExpectCount(cell("website"), 1)
		a.Press("Backspace")
		a.ExpectCount(cell("Resume.pdf"), 1)

		a.Press("End")
		a.Expect(row("todo.md"), telatest.Selected(true)) // the last entry
		a.Press("Enter")                                  // a file: it only says so
		a.ExpectText("Opening todo.md")

		a.Press("Alt+ArrowUp")
		a.ExpectCount(cell("Desktop"), 1)
	})
}

// TestSortByColumn: pressing a header sorts, pressing it again reverses;
// folders stay first.
func TestSortByColumn(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents")
		order := func() []string {
			var names []string
			for _, line := range strings.Split(a.AriaSnapshot(), "\n") {
				if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "- cell \""); ok {
					if name := rest[:strings.Index(rest, "\"")]; Lookup(name) {
						names = append(names, name)
					}
				}
			}
			return names
		}
		if got := order(); got[0] != "Projects" || got[3] != "Budget 2026.xlsx" {
			t.Fatalf("default order: %v", got)
		}
		a.Expect(telatest.ByRole("columnheader", "Name"), telatest.Sort("ascending"))
		a.Click(btn("Size"))
		a.Expect(telatest.ByRole("columnheader", "Size"), telatest.Sort("ascending"))
		got := order()
		if got[3] != "todo.md" || got[len(got)-1] != "Resume.pdf" {
			t.Fatalf("by size: %v", got)
		}
		a.Click(btn("Size"))
		a.Expect(telatest.ByRole("columnheader", "Size"), telatest.Sort("descending"))
		got = order()
		if got[3] != "Resume.pdf" || got[len(got)-1] != "todo.md" {
			t.Fatalf("by size, descending: %v", got)
		}
		a.Click(btn("Size")) // the third press goes back to the name
		a.Expect(telatest.ByRole("columnheader", "Name"), telatest.Sort("ascending"))
	})
}

// Lookup reports whether name is an entry of the Documents folder.
func Lookup(name string) bool { return root.Find(Join("Documents", name)) != nil }

// TestHoverAndSelectionPixels: hovering a row paints it, and the selected
// row's color differs from the rest, in every look.
func TestHoverAndSelectionPixels(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := mount(t, shown(id, s))
		r := a.Bounds(cell("Resume.pdf"))
		px, py := int(r.X+r.Width/2), int(r.Y+r.Height/2)
		rest := a.Screenshot().RGBAAt(px, py)
		a.Hover(cell("Resume.pdf"))
		hover := a.Screenshot().RGBAAt(px, py)
		if hover == rest {
			t.Errorf("hover did not change the row: %v", rest)
		}
		c := a.Bounds(cell("Contract draft.docx"))
		sel := a.Screenshot().RGBAAt(int(c.X+c.Width/2), int(c.Y+c.Height/2))
		if sel == rest || sel == hover {
			t.Errorf("selected row %v looks like an idle %v or hovered %v row", sel, rest, hover)
		}
	})
}

// TestEmptyFolder shows the placeholder.
func TestEmptyFolder(t *testing.T) {
	a := start(t, "breeze", tela.Light, "Documents/Taxes 2025")
	a.ExpectText("This folder is empty")
	a.ExpectText("0 Folders, 0 Files")
}

// TestSwitchLook: the Look group of the menu swaps the style and the theme
// of the window, with the state kept.
func TestSwitchLook(t *testing.T) {
	a := mount(t, shown("win11", tela.Light))
	before := a.Screenshot()
	a.Click(btn("See more"))
	a.Click(telatest.ByRole("menuitemradio", "KDE"))
	a.Advance(500e6)
	a.ExpectText("Date Modified") // KDE's header
	a.Expect(row("Contract draft.docx"), telatest.Selected(true))
	if same(before, a.Screenshot()) {
		t.Fatal("the window did not change")
	}
	// The same pixels as a window that started as KDE.
	want := mount(t, shown("breeze", tela.Light)).Screenshot()
	if !same(want, a.Screenshot()) {
		t.Error("switching to KDE draws something else than starting as KDE")
	}
}

func TestFlags(t *testing.T) {
	for in, want := range map[string]string{"win11": "win11", "adwaita": "adwaita", "BREEZE": "breeze"} {
		if got, err := parseTheme(in); err != nil || got != want {
			t.Errorf("parseTheme(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if got, err := parseTheme("auto"); err != nil || (got != "adwaita" && got != "win11") {
		t.Errorf("parseTheme(auto) = %q, %v", got, err)
	}
	if _, err := parseTheme("aqua"); err == nil || !strings.Contains(err.Error(), "win11") {
		t.Errorf("parseTheme(aqua) error = %v; want one that lists the looks", err)
	}
	for in, want := range map[string]tela.ColorScheme{"auto": tela.System, "true": tela.Dark, "false": tela.Light} {
		if got, err := parseMode(in); err != nil || got != want {
			t.Errorf("parseMode(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := parseMode("maybe"); err == nil {
		t.Error("parseMode(maybe) did not fail")
	}
}

// TestPreviews: every preview builds.
func TestPreviews(t *testing.T) {
	if len(Previews) != 6 {
		t.Fatalf("%d previews, want 6", len(Previews))
	}
	for _, name := range Previews.Names() {
		telatest.Mount(t, Previews[name], telatest.Size(windowW, windowH), telatest.Theme(Looks[LookIDs[0]].Theme))
	}
}
