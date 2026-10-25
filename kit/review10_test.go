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

// Tests of the phase-10 review fixes (ADR-152).

// An expanded Toaster only fans the stack out: the timers keep running
// (sonner's `expand`); only the pointer over the stack pauses them.
func TestToastExpandDoesNotPauseTheTimers(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{Expand: true}, telatest.NoSettle())
	handle.Show("Expanded")
	app.Advance(500 * time.Millisecond)
	app.ExpectText("Expanded")
	app.Advance(5 * time.Second)
	app.Advance(time.Second)
	app.ExpectNoText("Expanded")
}

// ToastReplace with an id no toast has keeps the automatic ids from
// reaching it later.
func TestToastReplaceOfAnUnknownIDReservesIt(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{Duration: kit.ToastForever})
	_ = app
	custom := handle.Show("Custom", kit.ToastReplace(kit.ToastID(2)))
	if custom != 2 {
		t.Fatalf("Show with ToastReplace(2) returned %d, want 2", custom)
	}
	seen := map[kit.ToastID]bool{custom: true}
	for i := 0; i < 4; i++ {
		id := handle.Show("Automatic")
		if seen[id] {
			t.Fatalf("automatic id %d collides with an earlier one (%v)", id, seen)
		}
		seen[id] = true
	}
}

// A toast is as tall as its body plus the padding and border its recipe
// gives, not a fixed amount.
func TestToastHeightFollowsTheItemRecipe(t *testing.T) {
	height := func(item func(string) string) float32 {
		s := kit.DefaultStyle()
		s.Toast.Item = item(s.Toast.Item)
		var handle kit.ToastHandle
		app := telatest.Mount(t, kit.StyleProvider{Style: &s, Children: toastApp{&handle, kit.Toaster{Duration: kit.ToastForever}}},
			telatest.Size(480, 360), telatest.Theme(kit.Theme()), telatest.RootClass("bg-background text-foreground"))
		handle.Show("Event created", kit.ToastDescription("Sunday at 9:00"))
		app.Advance(time.Second)
		return app.Bounds(telatest.ByRole("status", "")).Height
	}
	base := height(func(s string) string { return s })
	padded := height(func(s string) string { return strings.Replace(s, " p-4 ", " p-8 ", 1) })
	thick := height(func(s string) string {
		return strings.Replace(s, " border border-border ", " border-4 border-border ", 1)
	})
	if d := padded - base; d < 31.5 || d > 32.5 {
		t.Errorf("p-8 made the toast %v px taller than p-4 (%v vs %v), want 32", d, padded, base)
	}
	if d := thick - base; d < 5.5 || d > 6.5 {
		t.Errorf("border-4 made the toast %v px taller than border (%v vs %v), want 6", d, thick, base)
	}
}

var _ tela.Component = kit.Toaster{}

// A DefaultValue of 0 is a start like any other: a slider from -50 to 50
// that begins at 0 has its knob in the middle, and one without a
// DefaultValue begins at Min.
func TestSliderDefaultValueZero(t *testing.T) {
	knobAt := func(s kit.Slider) (knob, root tela.Rect) {
		s.AriaLabel, s.Props, s.Min, s.Max = "Offset", h.TestID("s"), -50, 50
		app := telatest.MountFunc(t, func() tela.Node { return h.Div(h.Class("w-[300px] p-4"), s) })
		return app.Bounds(telatest.ByRole("slider", "Offset")), app.Bounds(telatest.ByTestID("s"))
	}
	knob, root := knobAt(kit.Slider{DefaultValue: new(0.0)})
	if mid := root.X + root.Width/2; knob.X+knob.Width/2 < mid-1 || knob.X+knob.Width/2 > mid+1 {
		t.Errorf("a slider from -50 to 50 with DefaultValue 0 has its knob centered at %v, want the middle %v", knob.X+knob.Width/2, mid)
	}
	knob, root = knobAt(kit.Slider{})
	if knob.X != root.X {
		t.Errorf("a slider without DefaultValue has its knob at %v, want Min at %v", knob.X, root.X)
	}
}

// logTB collects what a test logs: the dev warnings of a mount.
type logTB struct {
	testing.TB
	logs *[]string
}

func (l logTB) Logf(format string, args ...any) {
	*l.logs = append(*l.logs, fmt.Sprintf(format, args...))
}
func (l logTB) Log(args ...any) { *l.logs = append(*l.logs, fmt.Sprint(args...)) }

// A group name, or the run of ungrouped items, that comes back after another
// group does not repeat the keys of its separator and heading.
func TestSelectRepeatedGroupsHaveDistinctKeys(t *testing.T) {
	items := []kit.SelectItem[string]{
		{Value: "a1", Label: "A one", Group: "A"},
		{Value: "b1", Label: "B one", Group: "B"},
		{Value: "a2", Label: "A two", Group: "A"},
		{Value: "x1", Label: "Loose one"},
		{Value: "b2", Label: "B two", Group: "B"},
		{Value: "x2", Label: "Loose two"},
	}
	var logs []string
	app := telatest.Mount(logTB{t, &logs}, kit.Select[string]{Items: items, DefaultOpen: true, Placeholder: "Pick"},
		telatest.Size(320, 400))
	app.ExpectText("A two")
	for _, l := range logs {
		if strings.Contains(l, "duplicate key") {
			t.Errorf("dev warning: %s", l)
		}
	}
}
