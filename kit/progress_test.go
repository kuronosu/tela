package kit_test

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's Progress (new-york-v4/ui/progress.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnProgress          = "relative h-2 w-full overflow-hidden rounded-full bg-primary/20"
	shadcnProgressIndicator = "h-full w-full flex-1 bg-primary transition-all"
)

func progressCell(name string, p kit.Progress) cell {
	p.Props = h.TestID(name + "-bar")
	state := "loading"
	if p.Value >= 100 {
		state = "complete"
	}
	html := fmt.Sprintf(`<div role="progressbar" aria-valuemax="100" aria-valuemin="0" aria-valuenow="%g" aria-valuetext="%g%%" data-state=%q data-value="%g" data-max="100" data-slot="progress" data-probe="%s-bar" class="%s"><div data-state=%q data-value="%g" data-max="100" data-slot="progress-indicator" class="%s" style="transform: translateX(-%g%%);"></div></div>`,
		p.Value, p.Value, state, p.Value, name, cn(shadcnProgress, p.Class), state, p.Value, cn(shadcnProgressIndicator, p.IndicatorClass), 100-p.Value)
	return cell{name: name, width: 300, probes: []string{name + "-bar"}, tela: p, html: html}
}

func progressSheet() *sheet {
	return &sheet{name: "progress", slot: 320, cells: []cell{
		progressCell("zero", kit.Progress{Value: 0, AriaLabel: "a"}),
		progressCell("quarter", kit.Progress{Value: 25, AriaLabel: "b"}),
		progressCell("two-thirds", kit.Progress{Value: 66, AriaLabel: "c"}),
		progressCell("done", kit.Progress{Value: 100, AriaLabel: "d"}),
		progressCell("tall", kit.Progress{Value: 40, AriaLabel: "e", Class: "h-4"}),
		progressCell("indicator-class", kit.Progress{Value: 70, AriaLabel: "f", IndicatorClass: "bg-emerald-500"}),
	}}
}

func TestProgressGolden(t *testing.T)   { progressSheet().golden(t) }
func TestChromiumProgress(t *testing.T) { progressSheet().chromium(t) }

func progressbar(name string) telatest.Locator { return telatest.ByRole("progressbar", name) }

// movingProgress shows a value the test changes.
type movingProgress struct{}

func (movingProgress) Build() tela.Node {
	v, setV := tela.UseState(10.0)
	return h.Div(h.Class("w-[200px]"),
		kit.Progress{Value: v, AriaLabel: "Upload", Props: h.TestID("bar"), IndicatorClass: "duration-300"},
		h.Button(h.TestID("more"), h.OnClick(func() { setV.Set(60) }), h.Text("More")))
}

func TestProgressValueAndTransition(t *testing.T) {
	app := telatest.Mount(t, movingProgress{}, telatest.NoSettle())
	ind := telatest.ByClass("h-full", "flex-1")
	bar := app.Bounds(telatest.ByTestID("bar"))
	if bar.Width != 200 {
		t.Fatalf("the bar is %g wide, want 200", bar.Width)
	}
	// 10%: the indicator is slid 90% of its width to the left.
	if got := app.Bounds(ind); got.X+got.Width-bar.X < 19.9 || got.X+got.Width-bar.X > 20.1 {
		t.Errorf("at 10%% the indicator shows %g px of the bar, want 20", got.X+got.Width-bar.X)
	}
	app.Click(telatest.ByTestID("more"))
	// A transition: the bar has not reached 100 px yet 60 ms after the
	// change, and has reached 120 px later.
	at := func() color.RGBA { return app.PixelAt(bar.X+100, bar.Y+bar.Height/2) }
	app.Advance(60 * time.Millisecond)
	mid := at()
	app.Advance(time.Second)
	end := at()
	if mid == end {
		t.Errorf("the bar has no transition: the pixel at 100 px is %v both during and after", mid)
	}
	if shown := app.Bounds(ind); shown.X+shown.Width-bar.X < 119.9 || shown.X+shown.Width-bar.X > 120.1 {
		t.Errorf("at 60%% the indicator shows %g px, want 120", shown.X+shown.Width-bar.X)
	}
}

func TestProgressSemantics(t *testing.T) {
	got, err := telatest.AriaSnapshot(h.Div(
		kit.Progress{Value: 40, AriaLabel: "Upload"},
		kit.Progress{Value: 512, Max: 2048, AriaLabel: "Disk", FormatValue: func(v float64) string { return fmt.Sprintf("%.0f MB", v) }},
		kit.Progress{Indeterminate: true, AriaLabel: "Loading"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`progressbar "Upload"`, `progressbar "Disk"`, `progressbar "Loading"`} {
		if !strings.Contains(got, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, got)
		}
	}
}

func TestProgressMaxAndClamp(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-[200px]"), kit.Progress{Value: 500, Max: 1000, AriaLabel: "Half", Props: h.TestID("half")},
			kit.Progress{Value: 150, AriaLabel: "Over", Props: h.TestID("over")})
	})
	for i, want := range []float32{100, 200} { // 500 of 1000; 150 is clamped to the whole bar
		bar := app.Bounds(telatest.ByTestID([]string{"half", "over"}[i]))
		ind := app.Bounds(telatest.ByClass("h-full", "flex-1").Nth(i))
		if shown := ind.X + ind.Width - bar.X; shown < want-0.1 || shown > want+0.1 {
			t.Errorf("bar %d shows %g px, want %g", i, shown, want)
		}
	}
	app.Expect(progressbar("Half"), telatest.HasName("Half"))
}

func ExampleProgress() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("w-64 p-6"), kit.Progress{Value: 40, AriaLabel: "Upload"})
	}
	app := telatest.MountFunc(t, page)
	app.ExpectRole("progressbar", "Upload")
	fmt.Println("progress shown")
	// Output: progress shown
}
