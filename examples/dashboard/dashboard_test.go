package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1440, 900

// mount opens App's window at a size; dev mode is on, so any dev message
// (an unknown class, a hook misuse) fails the test.
func mount(t *testing.T, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(w, h)}, opts...)
	return telatest.MountApp(t, App, opts...)
}

// mountPreview opens a preview in App's window at 1440x900.
func mountPreview(t *testing.T, name string, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := App
	app.Root = Previews[name].Root
	opts = append([]telatest.Option{telatest.Size(windowW, windowH)}, opts...)
	return telatest.MountApp(t, app, opts...)
}

// scheme returns the options of a scheme by name.
func scheme(name string) []telatest.Option {
	if name == "dark" {
		return []telatest.Option{telatest.Dark()}
	}
	return nil
}

var schemes = []string{"light", "dark"}

// goldenPreviews are the previews the goldens open in both schemes
// (TestGolden, TestGoldenStates); TestGoldenStates also opens the page at
// windowW x windowH in both.
var goldenPreviews = map[string]bool{"1 Overview": true, "2 Board": true, "4 Command palette": true, "5 Report dialog": true, "6 Toast": true}

// TestNoDevMessages opens the page at several window sizes, up to 4K, and
// every preview, in both schemes. Every mount of the package is in dev
// mode, so what the goldens open already passed: this opens the rest (on
// the Frosted graphite page a mount is costly, mostly at 4K).
func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{1920, 1080}, {1280, 800}, {3840, 2160}} {
		mount(t, s[0], s[1])
		mount(t, s[0], s[1], telatest.Dark())
	}
	for _, name := range Previews.Names() {
		if goldenPreviews[name] {
			continue
		}
		for _, sc := range schemes {
			mountPreview(t, name, scheme(sc)...)
		}
	}
}

// TestGolden is the overview (the "1 Overview" preview, with the peak's
// tooltip open) in light and dark, at scale 1 and 2.
func TestGolden(t *testing.T) {
	for _, sc := range schemes {
		for _, scale := range []float64{1, 2} {
			t.Run(fmt.Sprintf("%s_x%g", sc, scale), func(t *testing.T) {
				a := mountPreview(t, "1 Overview", append(scheme(sc), telatest.Scale(scale))...)
				a.ExpectCount(telatest.ByRole("tooltip", "$7,710 · peak"), 1)
				a.Golden(fmt.Sprintf("overview_%s_x%g.png", sc, scale))
			})
		}
	}
}

// TestGoldenStates are the other states in both schemes: the board, the
// command palette, the report dialog, a toast, and the overview scrolled
// under its top bar.
func TestGoldenStates(t *testing.T) {
	for _, sc := range schemes {
		t.Run(sc, func(t *testing.T) {
			mountPreview(t, "2 Board", scheme(sc)...).Golden("board_" + sc + ".png")

			a := mountPreview(t, "4 Command palette", scheme(sc)...)
			a.ExpectCount(telatest.ByRole("dialog", "Command palette"), 1)
			a.Golden("palette_" + sc + ".png")

			a = mountPreview(t, "5 Report dialog", scheme(sc)...)
			a.ExpectCount(telatest.ByRole("dialog", "New report"), 1)
			a.Golden("dialog_" + sc + ".png")

			a = mountPreview(t, "6 Toast", scheme(sc)...)
			a.ExpectText("Report queued: Q3 revenue summary")
			a.Golden("toast_" + sc + ".png")

			a = mount(t, windowW, windowH, append(scheme(sc), telatest.InstantScroll())...)
			a.Scroll(telatest.ByRole("heading", "Subscribers"), 0, 320)
			a.Golden("scrolled_" + sc + ".png")
		})
	}
}

// TestRange picks a period: the numbers count to the new period's values,
// and the frames of the springs build nothing.
func TestRange(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.ExpectText("$45,231.89")
	a.Expect(telatest.ByRole("radio", "30D"), telatest.Checked(true))
	before := a.Stats()
	a.Click(telatest.ByRole("radio", "90D"))
	a.ExpectText("$128,904.12")
	a.ExpectText("6,718")
	a.ExpectText("last 90 days")
	a.Expect(telatest.ByRole("radio", "90D"), telatest.Checked(true))
	a.Expect(telatest.ByRole("radio", "30D"), telatest.Checked(false))
	// The springs (the number, 48 bars, 4 figures and 96 sparkline bars)
	// move through layout and paint closures: the frames of the animation
	// build nothing, so the builds are those of the change itself.
	if got := a.Stats().Builds - before.Builds; got > 260 {
		t.Errorf("changing the period built %d components, want the components that read it, once each", got)
	}
}

// TestMetricTabs moves the headline's tab: one amber line, under the
// selected tab, and the number follows the metric.
func TestMetricTabs(t *testing.T) {
	a := mount(t, windowW, windowH)
	line := telatest.ByClass("h-0.5", "bg-primary")
	a.ExpectCount(line, 1)
	a.Expect(telatest.ByRole("tab", "Revenue"), telatest.Selected(true))
	a.Click(telatest.ByRole("tab", "Orders"))
	a.Expect(telatest.ByRole("tab", "Orders"), telatest.Selected(true))
	a.ExpectCount(line, 1)
	l, tab := a.Bounds(line), a.Bounds(telatest.ByRole("tab", "Orders"))
	if l.X < tab.X || l.Right() > tab.Right() {
		t.Errorf("the line spans %v to %v, outside the Orders tab (%v to %v)", l.X, l.Right(), tab.X, tab.Right())
	}
	a.ExpectNoText("$45,231.89")
	a.ExpectCount(telatest.ByText("12,234", telatest.Exact()), 2) // the headline and the Orders figure
	a.Press("ArrowRight")
	a.Expect(telatest.ByRole("tab", "Visitors"), telatest.Selected(true))
	a.ExpectText("169,917")
}

// TestNavigation moves between the views with the rail, the keyboard
// shortcuts and the overview's link.
func TestNavigation(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.ExpectRole("heading", "Overview")
	a.Click(telatest.ByRole("button", "Tasks"))
	a.ExpectRole("heading", "Tasks")
	a.ExpectText("12 tasks")
	a.ExpectCount(telatest.ByRole("complementary", "Pulse"), 0)
	a.Press("Control+3")
	a.ExpectRole("heading", "Orders")
	a.ExpectText("240 orders")
	a.Click(telatest.ByRole("radio", "Failed"))
	a.ExpectText("24 orders")
	a.ExpectNoText("#4891") // a paid order
	a.Press("Control+1")
	a.ExpectRole("heading", "Overview")
	a.Click(telatest.ByRole("button", "View all"))
	a.ExpectRole("heading", "Orders")
}

// TestTasksTable sorts the table by its headers: the rows are laid out at
// their new places at once and drawn sliding there (AnimateLayout).
func TestTasksTable(t *testing.T) {
	a := mountPreview(t, "3 Tasks", telatest.NoSettle())
	a.Advance(2 * time.Second)
	row := func(title string) float32 { return a.Bounds(telatest.ByText(title, telatest.Exact())).Y }
	if row("Fix failed invoice retries") >= row("Ship the CSV export") {
		t.Fatalf("by priority, Fix failed invoice retries comes first")
	}
	before := string(a.Screenshot().Pix)
	a.Click(telatest.ByRole("button", "Task"))
	a.Advance(40 * time.Millisecond)
	mid := string(a.Screenshot().Pix)
	a.Advance(2 * time.Second)
	end := string(a.Screenshot().Pix)
	if mid == before || mid == end {
		t.Errorf("the rows did not slide: the frame 40 ms in equals the first one (%v) or the last one (%v)", mid == before, mid == end)
	}
	a.Expect(telatest.ByRole("columnheader", "Task"), telatest.Sort("ascending"))
	if !(row("Audit color contrast") < row("Fix failed invoice retries") && row("Fix failed invoice retries") < row("Ship the CSV export")) {
		t.Errorf("A-Z should order Audit..., Fix..., Ship...")
	}
	// Finished tasks stay last whatever the sort.
	if row("Ship the CSV export") >= row("Set up the staging pipeline") {
		t.Errorf("a finished task is above an open one")
	}
	a.Click(telatest.ByRole("button", "Due"))
	a.Advance(2 * time.Second)
	if row("Fix failed invoice retries") >= row("Review the onboarding emails") {
		t.Errorf("by due date, Today comes before Wed")
	}
}

// TestFinishTask checks a task off in the table: it sinks, the sprint
// counts it, and its card is in Done on the board.
func TestFinishTask(t *testing.T) {
	a := mountPreview(t, "3 Tasks")
	title := telatest.ByText("Fix failed invoice retries", telatest.Exact())
	y0 := a.Bounds(title).Y
	a.ExpectText("4 of 12 done")
	a.Click(telatest.ByRole("checkbox", "Fix failed invoice retries"))
	if y1 := a.Bounds(title).Y; y1 <= y0 {
		t.Errorf("the finished task is at %v, it was at %v: it should have sunk", y1, y0)
	}
	a.Expect(telatest.ByRole("checkbox", "Fix failed invoice retries"), telatest.Checked(true))
	a.ExpectText("5 of 12 done")
	a.Click(telatest.ByRole("radio", "Board"))
	if got := laneOf(a, "Fix failed invoice retries"); got != done {
		t.Errorf("the finished task's card is in lane %d, want Done", got)
	}
	// And back: unchecking it puts it at the end of To do.
	a.Click(telatest.ByRole("radio", "Table"))
	a.Click(telatest.ByRole("checkbox", "Fix failed invoice retries"))
	a.ExpectText("4 of 12 done")
}

// TestNewTask writes a task from the sprint's button: it needs a title,
// and it lands at the end of To do with a toast.
func TestNewTask(t *testing.T) {
	a := mountPreview(t, "2 Board")
	a.Click(telatest.ByRole("button", "New task"))
	a.ExpectRole("dialog", "New task")
	a.Expect(telatest.ByRole("button", "Create task"), telatest.Disabled(true))
	a.ExpectText("Give the task a title.")
	a.Fill(telatest.ByRole("textbox", "Title"), "Draft the Q4 plan")
	a.Click(telatest.ByRole("button", "Create task"))
	a.ExpectText("Task created")
	a.ExpectText("TEL-137")
	if got := order(a, todo); len(got) != 5 || got[4] != "Draft the Q4 plan" {
		t.Errorf("To do is %v", got)
	}
	a.ExpectText("13 tasks")
	// A column's own button opens the dialog for that column, empty again.
	a.Click(telatest.ByRole("button", "New task in Done"))
	a.ExpectValue(telatest.ByRole("textbox", "Title"), "")
	a.ExpectText("It goes to the end of Done.")
	a.Press("Escape")
}

// TestReportDialog opens the report dialog from the palette and from the
// weekly report, closes it with Escape and with the scrim, and checks that
// its form works.
func TestReportDialog(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Press("Control+k")
	a.Click(telatest.ByRole("option", "New report"))
	a.ExpectCount(telatest.ByRole("dialog", "New report"), 1)
	a.Expect(telatest.ByRole("radio", "PDF"), telatest.Checked(true))
	a.Click(telatest.ByRole("radio", "CSV"))
	a.Expect(telatest.ByRole("radio", "CSV"), telatest.Checked(true))
	a.Expect(telatest.ByRole("radio", "PDF"), telatest.Checked(false))
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("dialog", "New report"), 0)

	a.Click(telatest.ByRole("button", "Edit the weekly report"))
	a.ExpectCount(telatest.ByRole("dialog", "Weekly report"), 1)
	a.ClickAt(40, 450) // the scrim
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)

	a.Click(telatest.ByRole("button", "Edit the weekly report"))
	a.Click(telatest.ByRole("switch", "Send every Monday"))
	a.Click(telatest.ByRole("button", "Save"))
	a.ExpectCount(telatest.ByRole("dialog", ""), 0)
	a.ExpectText("Weekly report saved")
	a.ExpectText("Not scheduled")
}

// toastRegion is the pixels the toast covers, as a string to compare. The
// queries of telatest let the app settle, which would run the toast's
// countdown to its end, so a toast in the middle of its life is looked at
// as pixels.
func toastRegion(a *telatest.App) string {
	img := a.Screenshot()
	var sb strings.Builder
	for y := 810; y < 890; y++ {
		sb.Write(img.Pix[img.PixOffset(540, y):img.PixOffset(900, y)])
	}
	return sb.String()
}

// waitToast advances the clock in 50 ms steps until the toast's pixels
// differ from the page's (shown) or equal them (gone), at most 8 s.
func waitToast(t *testing.T, a *telatest.App, page string, shown bool) time.Duration {
	t.Helper()
	for range 160 {
		if (toastRegion(a) != page) == shown {
			return a.Now()
		}
		a.Advance(50 * time.Millisecond)
	}
	t.Fatalf("the toast was never %v", map[bool]string{true: "shown", false: "gone"}[shown])
	return 0
}

// TestToast exports from the palette: the toast's countdown (a tween on
// the frame clock) dismisses it by itself after about 4 s.
func TestToast(t *testing.T) {
	a := mount(t, windowW, windowH, telatest.NoSettle())
	a.Advance(3 * time.Second) // the numbers and bars have counted up
	page := toastRegion(a)
	a.Click(telatest.ByRole("button", "Search"))
	a.Advance(300 * time.Millisecond)
	a.Click(telatest.ByRole("option", "Export orders as CSV"))
	shown := waitToast(t, a, page, true)
	gone := waitToast(t, a, page, false)
	if life := gone - shown; life < 3500*time.Millisecond || life > 4600*time.Millisecond {
		t.Errorf("the toast lived %v: its countdown is 4 s", life)
	}
}

// TestThemeSwitch switches the scheme from the top bar and back: the
// wallpaper and the glass go dark.
func TestThemeSwitch(t *testing.T) {
	a := mount(t, windowW, windowH)
	light := a.Screenshot().RGBAAt(700, 600)
	a.Click(telatest.ByRole("button", "Switch theme"))
	dark := a.Screenshot().RGBAAt(700, 600)
	if dark.R > light.R/2 || dark.G > light.G/2 {
		t.Errorf("the page did not go dark: %v before, %v after", light, dark)
	}
	a.Click(telatest.ByRole("button", "Switch theme"))
	if got := a.Screenshot().RGBAAt(700, 600); got != light {
		t.Errorf("switching back gives %v, want %v", got, light)
	}
}

// TestBell opens the notifications and marks them read: the dot goes.
func TestBell(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(telatest.ByRole("button", "Notifications, 3 unread"))
	a.ExpectRole("dialog", "Notifications")
	a.ExpectText("Invoice #4881 paid")
	a.Click(telatest.ByRole("button", "Mark all as read"))
	a.Expect(telatest.ByRole("button", "Mark all as read"), telatest.Disabled(true))
	a.Press("Escape")
	a.ExpectCount(telatest.ByRole("button", "Notifications"), 1)
}

// TestPanesStayPut scrolls the main pane: only its content moves; the
// rail, the top bar and the Pulse pane stay where they are.
func TestPanesStayPut(t *testing.T) {
	a := mount(t, windowW, 680, telatest.InstantScroll()) // short, so the overview scrolls far
	at := func(l telatest.Locator) [2]float32 { r := a.Bounds(l); return [2]float32{r.X, r.Y} }
	period, rail, edit := telatest.ByRole("radiogroup", "Period"), telatest.ByRole("button", "Orders"), telatest.ByRole("button", "Edit the weekly report")
	p0, r0, e0 := at(period), at(rail), at(edit)
	y0 := a.Bounds(telatest.ByRole("heading", "Subscribers")).Y
	a.Scroll(telatest.ByRole("heading", "Subscribers"), 0, 300)
	if got := at(period); got != p0 {
		t.Errorf("the top bar moved from %v to %v", p0, got)
	}
	if got := at(rail); got != r0 {
		t.Errorf("the rail moved from %v to %v", r0, got)
	}
	if got := at(edit); got != e0 {
		t.Errorf("the Pulse pane moved from %v to %v", e0, got)
	}
	if y := a.Bounds(telatest.ByRole("heading", "Subscribers")).Y; y > y0-100 {
		t.Errorf("the content did not scroll: Subscribers went from y=%v to y=%v", y0, y)
	}
}

// TestIdle checks that the page at rest asks for no frames once every
// interaction has ended: no infinite animation is left running.
func TestIdle(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Click(telatest.ByRole("radio", "7D"))
	a.Click(telatest.ByRole("button", "Edit the weekly report"))
	a.Click(telatest.ByRole("button", "Save"))
	a.Advance(6 * time.Second) // the toast counts down and leaves
	frames := a.Stats().Frames
	a.Advance(5 * time.Second)
	if got := a.Stats().Frames; got != frames {
		t.Errorf("the page at rest ran %d frames in 5 s", got-frames)
	}
}
