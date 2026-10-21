package main

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

func start(t *testing.T, id string, m Mode, path Path) *telatest.App {
	return mount(t, Props{Theme: id, Mode: m, Path: path})
}

// TestClickSelects: a click selects one row and a click on another moves the
// selection.
func TestClickSelects(t *testing.T) {
	eachTheme(t, func(t *testing.T, id string, m Mode) {
		a := start(t, id, m, "Documents")
		a.Click(opt("Resume.pdf"))
		if !isSelected(a, "Resume.pdf") {
			t.Fatal("Resume.pdf is not selected after a click")
		}
		a.Click(opt("todo.md"))
		if isSelected(a, "Resume.pdf") || !isSelected(a, "todo.md") {
			t.Fatal("the selection did not move to todo.md")
		}
		a.ExpectText("todo.md")
	})
}

// TestDoubleClickEntersFolder and back, forward and up.
func TestNavigation(t *testing.T) {
	eachTheme(t, func(t *testing.T, id string, m Mode) {
		a := start(t, id, m, "Documents")
		if !strings.Contains(a.AriaSnapshot(), `button "Back" [disabled]`) {
			t.Fatal("Back is enabled before any navigation")
		}

		a.DoubleClick(opt("Projects"))
		a.ExpectCount(opt("tela"), 1)
		a.ExpectCount(opt("website"), 1)
		a.ExpectCount(opt("Resume.pdf"), 0)
		a.ExpectCount(btn("Go to Projects"), 1)

		a.Click(btn("Up"))
		a.ExpectCount(opt("Resume.pdf"), 1)
		a.ExpectCount(btn("Go to Projects"), 0)

		a.Click(btn("Back")) // back to Projects
		a.ExpectCount(opt("tela"), 1)
		a.Click(btn("Back")) // back to Documents
		a.ExpectCount(opt("Resume.pdf"), 1)
		a.Click(btn("Forward"))
		a.ExpectCount(opt("tela"), 1)

		// Entering a folder drops the forward history.
		a.DoubleClick(opt("tela"))
		a.ExpectCount(opt("README.md"), 1)
		a.ExpectCount(opt("main.go"), 1)
		a.ExpectCount(btn("Go to tela"), 1)
	})
}

// TestSidebarAndPathBar: places and crumbs navigate.
func TestSidebarAndPathBar(t *testing.T) {
	eachTheme(t, func(t *testing.T, id string, m Mode) {
		a := start(t, id, m, "Documents/Projects/tela")
		a.Click(btn("Downloads"))
		a.ExpectCount(opt("wallpaper.jpg"), 1)
		a.ExpectCount(opt("main.go"), 0)

		a.Click(btn("Videos"))
		a.ExpectCount(opt("demo-reel.mp4"), 1)

		// The path bar: click a parent crumb.
		a = start(t, id, m, "Documents/Projects/tela")
		a.Click(btn("Go to Documents"))
		a.ExpectCount(opt("Resume.pdf"), 1)
		a.Click(btn("Go to " + Themes[id].Root))
		a.ExpectCount(opt("Desktop"), 1)
		a.ExpectCount(opt("hello.go"), 1)
		a.ExpectCount(opt("Trash"), 0) // hidden: reachable from the sidebar only
	})
}

// TestSearchFilters: typing narrows the list, clearing restores it, and
// navigating clears the search.
func TestSearchFilters(t *testing.T) {
	eachTheme(t, func(t *testing.T, id string, m Mode) {
		a := start(t, id, m, "Documents")
		a.ExpectCount(telatest.ByRole("option", ""), 9)

		a.Fill(search(), "doc")
		a.ExpectValue(search(), "doc")
		a.ExpectCount(telatest.ByRole("option", ""), 2) // Contract draft.docx, Quarterly review.docx
		a.ExpectCount(opt("Contract draft.docx"), 1)

		a.Fill(search(), "ZZZ")
		a.ExpectCount(telatest.ByRole("option", ""), 0)
		a.ExpectText("No results")

		a.Fill(search(), "")
		a.ExpectCount(telatest.ByRole("option", ""), 9)

		a.Fill(search(), "re")
		a.ExpectCount(opt("Reports"), 1)
		a.Click(btn("Downloads"))
		a.ExpectValue(search(), "")
		a.ExpectCount(opt("wallpaper.jpg"), 1)
	})
}

// TestKeyboard: Down and Up move the selection, Enter opens a folder,
// Backspace goes back.
func TestKeyboard(t *testing.T) {
	eachTheme(t, func(t *testing.T, id string, m Mode) {
		a := start(t, id, m, "Documents")
		a.Press("ArrowDown")
		if !isSelected(a, "Projects") {
			t.Fatalf("first ArrowDown did not select the first entry:\n%s", a.AriaSnapshot())
		}
		a.Press("ArrowDown")
		a.Press("ArrowDown")
		if !isSelected(a, "Taxes 2025") {
			t.Fatal("Taxes 2025 is not selected after three ArrowDown")
		}
		a.Press("ArrowUp")
		if !isSelected(a, "Reports") {
			t.Fatal("Reports is not selected after ArrowUp")
		}
		a.Press("ArrowUp")
		a.Press("ArrowUp") // stays on the first entry
		if !isSelected(a, "Projects") {
			t.Fatal("selection left the first entry")
		}

		a.Press("Enter") // Projects
		a.ExpectCount(opt("tela"), 1)
		a.Press("ArrowDown")
		a.Press("Enter") // tela
		a.ExpectCount(opt("README.md"), 1)

		a.Press("Backspace")
		a.ExpectCount(opt("website"), 1)
		a.Press("Backspace")
		a.ExpectCount(opt("Resume.pdf"), 1)

		a.Press("End")
		if !isSelected(a, "todo.md") {
			t.Fatal("End did not select the last entry")
		}
		a.Press("Enter") // a file: it only says so
		a.ExpectText("Opening todo.md")

		a.Press("Alt+ArrowUp")
		a.ExpectCount(opt("Desktop"), 1)
	})
}

// TestSortByColumn: clicking a header sorts, twice reverses; folders stay first.
func TestSortByColumn(t *testing.T) {
	eachTheme(t, func(t *testing.T, id string, m Mode) {
		a := start(t, id, m, "Documents")
		order := func() []string {
			var names []string
			for _, line := range strings.Split(a.AriaSnapshot(), "\n") {
				if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "- option \""); ok {
					names = append(names, rest[:strings.Index(rest, "\"")])
				}
			}
			return names
		}
		if got := order(); got[0] != "Projects" || got[3] != "Budget 2026.xlsx" {
			t.Fatalf("default order: %v", got)
		}
		a.Click(btn("Sort by Size"))
		got := order()
		if got[3] != "todo.md" || got[len(got)-1] != "Resume.pdf" {
			t.Fatalf("by size: %v", got)
		}
		a.Click(btn("Sort by Size"))
		got = order()
		if got[3] != "Resume.pdf" || got[len(got)-1] != "todo.md" {
			t.Fatalf("by size, descending: %v", got)
		}
	})
}

// TestHoverAndSelectionPixels: hovering a row paints it, and the selected
// row's color differs from the rest, in every theme.
func TestHoverAndSelectionPixels(t *testing.T) {
	eachTheme(t, func(t *testing.T, id string, m Mode) {
		a := mount(t, shown(id, m))
		xBox := a.Bounds(opt("Resume.pdf"))
		x, y, w, h := xBox.X, xBox.Y, xBox.Width, xBox.Height
		px := int(x + w - 4)
		py := int(y + h/2)
		rest := a.Screenshot().RGBAAt(px, py)
		a.Hover(opt("Resume.pdf"))
		hover := a.Screenshot().RGBAAt(px, py)
		if hover == rest {
			t.Errorf("hover did not change the row: %v", rest)
		}
		sxBox := a.Bounds(opt("Contract draft.docx"))
		sx, sy, sw, sh := sxBox.X, sxBox.Y, sxBox.Width, sxBox.Height
		sel := a.Screenshot().RGBAAt(int(sx+sw-4), int(sy+sh/2))
		if sel == rest || sel == hover {
			t.Errorf("selected row %v looks like an idle %v or hovered %v row", sel, rest, hover)
		}
	})
}

// TestEmptyFolder shows the placeholder.
func TestEmptyFolder(t *testing.T) {
	a := start(t, "breeze", Light, "Documents/Taxes 2025")
	a.ExpectText("This folder is empty")
	a.ExpectText("0 Folders, 0 Files")
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
		t.Errorf("parseTheme(aqua) error = %v; want one that lists the themes", err)
	}
	for in, want := range map[string]Mode{"auto": Auto, "true": Dark, "false": Light} {
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
		telatest.Mount(t, Previews[name], telatest.Size(windowW, windowH), telatest.Dev())
	}
}
