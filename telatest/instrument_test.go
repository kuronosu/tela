package telatest_test

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// insLine returns the line of its caller.
func insLine() int {
	_, _, line, _ := runtime.Caller(1)
	return line
}

var insHeadingLine int

func insScreen() tela.Node {
	n, setN := tela.UseState(0)
	insHeadingLine = insLine() + 1
	return Div(Class("p-4"), H1(Text("Title")),
		Button(OnClick(func() { setN.Set(n + 1) }), Textf("Count %d", n)))
}

func TestAriaSnapshotSources(t *testing.T) {
	app := telatest.MountFunc(t, insScreen, telatest.Instrument())
	plain := app.AriaSnapshot()
	if strings.Contains(plain, "src=") {
		t.Errorf("AriaSnapshot changed with Instrument:\n%s", plain)
	}
	got, nodes := app.AriaSnapshotSources()
	lines := strings.Split(got, "\n")
	if len(lines) != len(nodes) {
		t.Fatalf("%d nodes for %d lines:\n%s", len(nodes), len(lines), got)
	}
	want := fmt.Sprintf(`- heading "Title" [level=1] [src=instrument_test.go:%d]`, insHeadingLine)
	if !strings.Contains(got, want) {
		t.Errorf("snapshot lacks %q:\n%s", want, got)
	}
	if !strings.Contains(got, `- button "Count 0" [src=instrument_test.go:`) {
		t.Errorf("the button has no source:\n%s", got)
	}
	// Without the sources, the lines are the plain snapshot's.
	var stripped []string
	for _, l := range lines {
		if i := strings.Index(l, " [src="); i >= 0 {
			l = l[:i] + l[strings.Index(l[i:], "]")+i+1:]
		}
		stripped = append(stripped, l)
	}
	if strings.Join(stripped, "\n") != plain {
		t.Errorf("sources changed more than the line ends:\n%s\nvs\n%s", got, plain)
	}
	// Not instrumented: the plain snapshot.
	app2 := telatest.MountFunc(t, insScreen)
	if y, _ := app2.AriaSnapshotSources(); y != plain {
		t.Errorf("not instrumented: %q, want %q", y, plain)
	}
	if app2.Components() != nil || app2.Frames(0) != nil || strings.Contains(string(app2.LayoutJSON()), `"src"`) {
		t.Error("an app without Instrument reports DevTools data")
	}
	if !strings.Contains(string(app.LayoutJSON()), `"src": "instrument_test.go:`) {
		t.Errorf("the layout lacks src:\n%s", app.LayoutJSON())
	}
}

func TestComponentsAndWhyTheyRendered(t *testing.T) {
	app := telatest.MountFunc(t, insScreen, telatest.Instrument())
	app.Click(telatest.ByRole("button", "Count 0"))
	yaml, nodes := app.AriaSnapshotNodes()
	var id uint64
	for i, l := range strings.Split(yaml, "\n") {
		if strings.HasPrefix(l, `- button "Count 1"`) {
			id = nodes[i]
		}
	}
	c, ok := app.ComponentOf(id)
	if !ok {
		t.Fatalf("no component of the button:\n%s", yaml)
	}
	if len(c.Hooks) != 1 || c.Hooks[0].Kind != "UseState" || c.Hooks[0].Value != "1" || !strings.HasPrefix(c.Hooks[0].Source, "instrument_test.go:") {
		t.Errorf("hooks %+v, want UseState holding 1 at this file", c.Hooks)
	}
	if c.Builds != 2 || len(c.Renders) != 2 {
		t.Fatalf("builds %d, renders %+v", c.Builds, c.Renders)
	}
	if r := c.Renders[0].Reasons; len(r) != 1 || r[0] != "mounted" {
		t.Errorf("first render %v", r)
	}
	if r := c.Renders[1].Reasons; len(r) != 1 || !strings.HasPrefix(r[0], "UseState #1 at instrument_test.go:") || !strings.HasSuffix(r[0], "was set") {
		t.Errorf("second render %v, want the UseState setter", r)
	}
	if len(app.Components()) == 0 {
		t.Error("Components is empty")
	}
}

func TestFrames(t *testing.T) {
	app := telatest.MountFunc(t, insScreen, telatest.Instrument())
	before := app.Stats().Frames
	app.Click(telatest.ByRole("button", "Count 0"))
	fs := app.Frames(before)
	if len(fs) == 0 {
		t.Fatalf("no frames after a click (frame %d, all %d)", before, len(app.Frames(0)))
	}
	built := 0
	for _, f := range fs {
		if f.Number <= before {
			t.Errorf("frame %d is not after %d", f.Number, before)
		}
		built += f.Built
		if f.Phases["layout"] < 0 || f.Duration < 0 {
			t.Errorf("frame %+v", f)
		}
	}
	if built == 0 {
		t.Errorf("the click built nothing: %+v", fs)
	}
	if len(app.Frames(app.Stats().Frames)) != 0 {
		t.Error("frames after the last one")
	}
}
