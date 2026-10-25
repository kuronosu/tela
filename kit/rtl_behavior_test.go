package kit_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// What direction changes in the kit beyond the mirrored layout (ADR-151):
// where a switch's thumb sits, which way a slider's value grows, which arrow
// goes to the next tab or item, where a sticky column sticks, a dialog's
// close button, a toast's action.

// rtlScenarios are the two ways a component learns that it is right to left:
// a DirectionProvider above it, and its own Direction field (the field is
// applied by the function, which gets the direction).
var rtlScenarios = []string{"provider", "field"}

// mountRTL mounts the component a scenario builds: with the provider the
// component is built without a direction, with the field it is given "rtl".
func mountRTL(t *testing.T, scenario string, build func(direction string) tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.MountFunc(t, func() tela.Node {
		if scenario == "provider" {
			return h.Div(h.Class("w-[300px] p-4"), kit.DirectionProvider{Direction: "rtl", Children: build("")})
		}
		return h.Div(h.Class("w-[300px] p-4"), build("rtl"))
	}, append([]telatest.Option{telatest.Theme(kit.Theme())}, opts...)...)
}

func TestRTLSwitchThumbSide(t *testing.T) {
	thumb := telatest.ByClass("pointer-events-none", "block", "rounded-full")
	for _, c := range []struct {
		dir string
		on  bool
	}{{"ltr", false}, {"ltr", true}, {"rtl", false}, {"rtl", true}} {
		t.Run(fmt.Sprintf("%s-on=%t", c.dir, c.on), func(t *testing.T) {
			app := telatest.MountFunc(t, func() tela.Node {
				return h.Div(h.Dir(c.dir), h.Class("p-4"), kit.Switch{AriaLabel: "Wi-Fi", DefaultChecked: c.on})
			}, telatest.Theme(kit.Theme()))
			track, knob := app.Bounds(telatest.ByRole("switch", "Wi-Fi")), app.Bounds(thumb)
			// The thumb is inside the track's 1 px border: at its start when off,
			// at its end when on; the start is the right in rtl.
			atLeft := c.on == (c.dir == "rtl")
			if atLeft && !near(knob.X, track.X+1) {
				t.Errorf("the thumb is at x=%v, want the left end of the track %+v", knob.X, track)
			}
			if !atLeft && !near(knob.X+knob.Width, track.X+track.Width-1) {
				t.Errorf("the thumb ends at x=%v, want the right end of the track %+v", knob.X+knob.Width, track)
			}
		})
	}
}

func TestRTLSliderKeys(t *testing.T) {
	for _, sc := range rtlScenarios {
		t.Run(sc, func(t *testing.T) {
			app := mountRTL(t, sc, func(dir string) tela.Component {
				return kit.Slider{AriaLabel: "Volume", DefaultValue: 50, Step: 5, Direction: dir}
			})
			vol := sliderRole("Volume")
			app.Focus(vol)
			app.Press("ArrowLeft")
			app.Expect(vol, telatest.HasValue("55"))
			app.Press("ArrowRight")
			app.Press("ArrowRight")
			app.Expect(vol, telatest.HasValue("45"))
			app.Press("ArrowUp")
			app.Expect(vol, telatest.HasValue("50"))
			// Home goes to the minimum, which is at the right.
			app.Press("Home")
			root := app.Bounds(telatest.ByRole("slider", "Volume"))
			track := app.Bounds(telatest.ByClass("relative", "grow"))
			if !near(root.X+root.Width, track.X+track.Width) {
				t.Errorf("at 0 the thumb ends at %v, want the right end of the track %+v", root.X+root.Width, track)
			}
			app.Press("End")
			if root = app.Bounds(telatest.ByRole("slider", "Volume")); !near(root.X, track.X) {
				t.Errorf("at 100 the thumb is at %v, want the left end of the track %+v", root.X, track)
			}
		})
	}
}

func TestRTLSliderPointer(t *testing.T) {
	for _, sc := range rtlScenarios {
		t.Run(sc, func(t *testing.T) {
			var committed []float64
			app := mountRTL(t, sc, func(dir string) tela.Component {
				return kit.Slider{AriaLabel: "Volume", Props: h.TestID("s"), Direction: dir,
					OnValueCommit: func(v []float64) { committed = v }}
			})
			root := app.Bounds(telatest.ByTestID("s"))
			y := root.Y + root.Height/2
			// A press a quarter of the way from the left is at three quarters of
			// the value, and the drag follows the pointer back.
			app.MouseMove(root.X+root.Width/4, y)
			app.MouseDown()
			app.Expect(sliderRole("Volume"), telatest.HasValue("75"))
			app.MouseMove(root.X+root.Width*3/4, y)
			app.MouseUp()
			app.Expect(sliderRole("Volume"), telatest.HasValue("25"))
			if len(committed) != 1 || committed[0] != 25 {
				t.Errorf("OnValueCommit got %v, want [25]", committed)
			}
		})
	}
}

// A range fills between its knobs and a one-knob slider from the minimum's
// end: the pixel near the right end of a 30% slider is filled, the left is
// not.
func TestRTLSliderFill(t *testing.T) {
	app := mountRTL(t, "field", func(dir string) tela.Component {
		return kit.Slider{AriaLabel: "Volume", DefaultValue: 30, Props: h.TestID("s"), Direction: dir}
	})
	root := app.Bounds(telatest.ByTestID("s"))
	y := root.Y + root.Height/2
	filled, empty := app.PixelAt(root.X+root.Width-12, y), app.PixelAt(root.X+12, y)
	if filled == empty {
		t.Errorf("the track looks the same at both ends (%v): a 30%% slider is filled from the right in rtl", filled)
	}
	if filled.R > 100 || empty.R < 200 {
		t.Errorf("the right end is %v and the left %v, want primary at the right and muted at the left", filled, empty)
	}
}

func TestRTLSliderVerticalUnchanged(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("h-[200px] p-4"), kit.DirectionProvider{Direction: "rtl", Children: kit.Slider{
			AriaLabel: "Volume", Orientation: kit.Vertical, DefaultValue: 50}})
	}, telatest.Theme(kit.Theme()))
	vol := sliderRole("Volume")
	app.Focus(vol)
	app.Press("ArrowUp")
	app.Expect(vol, telatest.HasValue("51"))
	app.Press("ArrowRight")
	app.Expect(vol, telatest.HasValue("52"))
}

func TestRTLToggleGroupKeys(t *testing.T) {
	for _, sc := range rtlScenarios {
		t.Run(sc, func(t *testing.T) {
			app := mountRTL(t, sc, func(dir string) tela.Component {
				return kit.ToggleGroup[string]{Items: tgViews, AriaLabel: "View", DefaultValue: new("list"), Direction: dir}
			})
			app.Focus(tgRadio("List"))
			// List, Grid, (Map is disabled), Table are laid out from the right.
			list, grid := app.Bounds(tgRadio("List")), app.Bounds(tgRadio("Grid"))
			if list.X < grid.X {
				t.Fatalf("List is at x=%v, Grid at x=%v: the group is not mirrored", list.X, grid.X)
			}
			app.Press("ArrowLeft")
			app.Expect(tgRadio("Grid"), telatest.Focused(true))
			app.Press("ArrowRight")
			app.Expect(tgRadio("List"), telatest.Focused(true))
			app.Press("ArrowRight") // wraps to the last
			app.Expect(tgRadio("Table"), telatest.Focused(true))
		})
	}
}

func TestRTLTabsKeys(t *testing.T) {
	for _, sc := range rtlScenarios {
		t.Run(sc, func(t *testing.T) {
			app := mountRTL(t, sc, func(dir string) tela.Component {
				return kit.Tabs[string]{Tabs: demoTabs(), AriaLabel: "Settings", Direction: dir}
			})
			app.Focus(telatest.ByRole("tab", "Account"))
			app.Press("ArrowLeft")
			selectedTab(app, "Password")
			app.Press("ArrowRight")
			selectedTab(app, "Account")
			app.Press("ArrowRight") // wraps
			selectedTab(app, "Team")
		})
	}
}

func TestRTLRadioGroupKeys(t *testing.T) {
	for _, sc := range rtlScenarios {
		t.Run(sc, func(t *testing.T) {
			app := mountRTL(t, sc, func(dir string) tela.Component {
				return kit.RadioGroup[string]{Orientation: kit.Horizontal, AriaLabel: "Size", Direction: dir, DefaultValue: new("a"),
					Items: []kit.RadioItem[string]{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}, {Value: "c", Label: "C"}}}
			})
			app.Focus(telatest.ByRole("radio", "A"))
			app.Press("ArrowLeft")
			app.Expect(telatest.ByRole("radio", "B"), telatest.Focused(true), telatest.Checked(true))
			app.Press("ArrowRight")
			app.Expect(telatest.ByRole("radio", "A"), telatest.Focused(true), telatest.Checked(true))
		})
	}
}

func TestRTLMenubarKeys(t *testing.T) {
	app := mountBar(t, appBar(func(m *kit.Menubar[string]) { m.Direction = "rtl" }))
	app.Focus(barItem("File"))
	app.Press("ArrowLeft")
	app.ExpectFocused(barItem("Edit"))
	app.Press("ArrowRight")
	app.ExpectFocused(barItem("File"))
}

func TestRTLColorPickerKeys(t *testing.T) {
	for _, sc := range rtlScenarios {
		t.Run(sc, func(t *testing.T) {
			app := mountRTL(t, sc, func(dir string) tela.Component {
				return kit.ColorPicker{DefaultValue: tela.Hex("#ff0000"), Direction: dir}
			})
			hue := telatest.ByRole("slider", "Hue")
			app.Focus(hue)
			app.Press("ArrowLeft") // the hue grows toward the left
			app.Expect(hue, telatest.HasValue("Hue 1°"))
			knob := app.Bounds(hue)
			track := app.Bounds(telatest.ByClass("relative", "h-3", "grow"))
			if knob.X+knob.Width < track.X+track.Width-4 {
				t.Errorf("the hue minimum's thumb is at %+v, want it at the right end of the track %+v", knob, track)
			}
		})
	}
}

func TestRTLCalendarFollowsTheProvider(t *testing.T) {
	app := calMount(t, kit.DirectionProvider{Direction: "rtl", Children: kit.Calendar{
		Today: calToday, DefaultValue: calDate(2026, time.October, 14), AriaLabel: "Dates"}})
	app.Focus(calCell(calDate(2026, time.October, 14)))
	app.Press("ArrowLeft") // the next day is at the left
	app.ExpectFocused(calCell(calDate(2026, time.October, 15)))
}

func TestRTLTableStickyColumns(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		t.Run(dir, func(t *testing.T) {
			app := mountSticky(t, h.Div(h.Dir(dir), stickyTable(20, nil)))
			head := func(c int) tela.Rect { return app.Bounds(telatest.ByTestID(fmt.Sprintf("s-h%d", c))) }
			root := app.Bounds(telatest.ByTestID("s-table"))
			left, right := root.X+1, root.X+root.Width-1 // inside the border
			// edge is the sticky edge of a column: its start.
			edge := func(r tela.Rect) float32 {
				if dir == "rtl" {
					return right - (r.X + r.Width)
				}
				return r.X - left
			}
			if edge(head(0)) != 0 || edge(head(1)) != 100 {
				t.Fatalf("at rest the sticky columns are %v and %v px from the start edge, want 0 and 100", edge(head(0)), edge(head(1)))
			}
			// Scrolling toward the end (right in ltr, left in rtl).
			dx := float32(200)
			if dir == "rtl" {
				dx = -200
			}
			app.Scroll(telatest.ByTestID("s-table"), dx, 0)
			if edge(head(0)) != 0 || edge(head(1)) != 100 {
				t.Errorf("scrolled, the sticky columns are %v and %v px from the start edge, want 0 and 100", edge(head(0)), edge(head(1)))
			}
			if e := edge(head(5)); e > 100+120+140*3-200+1 {
				t.Errorf("scrolled, the last column is %v px from the start edge: it did not scroll", e)
			}
		})
	}
}

func TestRTLDialogCloseAndHeader(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		t.Run(dir, func(t *testing.T) {
			app := telatest.MountFunc(t, func() tela.Node {
				return h.Div(h.Dir(dir), pageUnder(editDialog(kit.Dialog{DefaultOpen: true, ShowClose: true}))())
			}, telatest.Theme(kit.Theme()), telatest.Size(640, 480))
			dlg, closeBtn, title := app.Bounds(profileDlg), app.Bounds(closeButton), app.Bounds(telatest.ByRole("heading", "Edit profile"))
			// The close button is 16 px (top-4 end-4) from the end corner; the
			// title starts at the start edge, inside the 24 px padding.
			if dir == "rtl" {
				if !near(closeBtn.X-dlg.X, 17) || !near(dlg.X+dlg.Width-(title.X+title.Width), 25) {
					t.Errorf("rtl: close %+v and title %+v in the dialog %+v: want the close button at the left corner and the title at the right", closeBtn, title, dlg)
				}
			} else if !near(dlg.X+dlg.Width-(closeBtn.X+closeBtn.Width), 17) || !near(title.X-dlg.X, 25) {
				t.Errorf("ltr: close %+v and title %+v in the dialog %+v: want the close button at the right corner and the title at the left", closeBtn, title, dlg)
			}
		})
	}
}

func TestRTLMenuIndicatorAndShortcut(t *testing.T) {
	items := []kit.MenuItem[string]{
		{Key: "bar", Label: "Status bar", Kind: kit.MenuCheckbox, DefaultChecked: true},
		{Key: "open", Label: "Open", Shortcut: "\u2318O"},
	}
	// The shortcut is drawn in pure red, to find where it is.
	style := kit.DefaultStyle()
	style.DropdownMenu.Shortcut += " text-[#ff0000]"
	for _, dir := range []string{"ltr", "rtl"} {
		t.Run(dir, func(t *testing.T) {
			app := telatest.MountFunc(t, func() tela.Node {
				return h.Div(h.Dir(dir), h.Class("w-[300px] p-4"), kit.StyleProvider{Style: &style,
					Children: kit.StaticDropdownMenu(kit.DropdownMenu[string]{Items: items, Class: "w-56"})})
			}, telatest.Theme(kit.Theme()))
			row := app.Bounds(telatest.ByRole("menuitemcheckbox", "Status bar"))
			ind := app.Bounds(telatest.ByClass("pointer-events-none", "absolute", "start-2"))
			open := app.Bounds(telatest.ByRole("menuitem", "Open"))
			// The red pixels of the Open row: the shortcut's extent.
			img := app.Screenshot()
			first, last := float32(-1), float32(-1)
			for x := int(open.X); x < int(open.X+open.Width); x++ {
				for y := int(open.Y); y < int(open.Y+open.Height); y++ {
					if p := img.RGBAAt(x, y); p.R > 180 && p.G < 90 && p.B < 90 {
						if first < 0 {
							first = float32(x)
						}
						last = float32(x + 1)
					}
				}
			}
			if first < 0 {
				t.Fatal("no red pixel of the shortcut in the row of Open")
			}
			// The row has 8 px of padding; the glyphs have a little side bearing.
			if dir == "rtl" {
				if !near(row.X+row.Width-(ind.X+ind.Width), 8) || first-open.X < 7 || first-open.X > 12 {
					t.Errorf("rtl: the check %+v is not 8 px from the right of the row %+v, or the shortcut (x %v to %v) from the left of %+v", ind, row, first, last, open)
				}
			} else if !near(ind.X-row.X, 8) || open.X+open.Width-last < 7 || open.X+open.Width-last > 12 {
				t.Errorf("ltr: the check %+v is not 8 px from the left of the row %+v, or the shortcut (x %v to %v) from the right of %+v", ind, row, first, last, open)
			}
		})
	}
}

func TestRTLToastActionAndPosition(t *testing.T) {
	var handle kit.ToastHandle
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Dir("rtl"), toastApp{&handle, kit.Toaster{Duration: kit.ToastForever, Position: kit.ToastBottomRight}})
	}, telatest.Size(480, 360), telatest.Theme(kit.Theme()), telatest.RootClass("bg-background text-foreground"))
	handle.Show("Archived", kit.ToastAction("Undo", func() {}))
	text, action := app.Bounds(telatest.ByText("Archived", telatest.Exact())), app.Bounds(telatest.ByRole("button", "Undo"))
	toast := app.Bounds(telatest.ByRole("status", ""))
	// Positions are physical, as sonner's: bottom-right stays at the right.
	if !near(toast.X+toast.Width, 480-24) {
		t.Errorf("the bottom-right toast ends at x=%v, want 456", toast.X+toast.Width)
	}
	// The text is at the start (right) and the action at the end (left).
	if !(action.X < text.X) || !near(action.X-toast.X, 16+1) {
		t.Errorf("the action %+v and the text %+v in the toast %+v: want the action at the left end, 16 px in", action, text, toast)
	}
}

func TestDirectionProviderHasNoBox(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("flex w-[300px] gap-2 p-4"), h.Div(h.TestID("a"), h.Class("size-10")),
			kit.DirectionProvider{Direction: "RTL", Children: tela.C(func() tela.Node {
				return h.Div(h.TestID("b"), h.Class("size-10"))
			})})
	})
	a, b := app.Bounds(telatest.ByTestID("a")), app.Bounds(telatest.ByTestID("b"))
	if b.X != a.X+a.Width+8 {
		t.Errorf("b is at x=%v, want it next to a as a flex item of the same row (%v)", b.X, a.X+a.Width+8)
	}
}

func TestDirectionProviderRejectsOtherValues(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("DirectionProvider{Direction: \"up\"} did not panic in dev mode")
		}
		if s := fmt.Sprint(r); !strings.Contains(s, `"ltr" or "rtl"`) {
			t.Errorf("the panic %q does not say what to use", s)
		}
	}()
	telatest.Mount(t, kit.DirectionProvider{Direction: "up", Children: h.Div()})
}

// A menu at the start of its trigger starts where the text does: at the
// trigger's left edge, or at its right edge in a right-to-left page, as
// Floating UI's start alignment does.
func TestRTLMenuAlignsToTheTriggersStart(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		t.Run(dir, func(t *testing.T) {
			var log []string
			d := actionsMenu(&log, func(d *kit.DropdownMenu[string]) { d.Align = "start" })
			app := telatest.MountFunc(t, func() tela.Node {
				return h.Div(h.Dir(dir), h.Class("flex justify-center p-6"), d)
			}, telatest.Theme(kit.Theme()), telatest.Size(600, 400))
			trigger := app.Bounds(ddTrigger)
			app.Click(ddTrigger)
			menu := app.Bounds(ddMenu)
			if dir == "rtl" && !near(menu.X+menu.Width, trigger.X+trigger.Width) {
				t.Errorf("rtl: the menu %+v does not end where its trigger %+v does", menu, trigger)
			}
			if dir == "ltr" && !near(menu.X, trigger.X) {
				t.Errorf("ltr: the menu %+v does not start where its trigger %+v does", menu, trigger)
			}
		})
	}
}
