package telatest_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func TestScrollAt(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("h-[200px] w-[300px] overflow-y-auto"),
			Map([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}, func(i int) tela.Node {
				return Div(Key(i), Class("h-[50px]"), Textf("Row %d", i))
			}))
	}, telatest.Size(300, 200))
	row := telatest.ByText("Row 10", telatest.Exact())
	y0 := app.Bounds(row).Y
	app.ScrollAt(150, 100, 0, 120)
	if y1 := app.Bounds(row).Y; y1 != y0-120 {
		t.Errorf("after ScrollAt(150, 100, 0, 120) Row 10 is at %v, want %v", y1, y0-120)
	}
}

// TestNodeOf gives the identity of the element a locator finds, the one
// the snapshot prints.
func TestNodeOf(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, nodesScreen)
	if got, want := app.NodeOf(telatest.ByRole("button", "Toggle")), nodeOf(t, app, `- button "Toggle"`); got != want {
		t.Errorf("NodeOf(Toggle) = %d, want %d", got, want)
	}
	mustFail(t, func(tb testing.TB) {
		telatest.MountFunc(tb, nodesScreen).NodeOf(telatest.ByRole("button", ""))
	}, "matches")
}

// TestScrollables lists the scroll containers that can move, the largest
// first, and leaves out one whose content fits.
func TestScrollables(t *testing.T) {
	t.Parallel()
	rows := func(n int) []tela.Child {
		var out []tela.Child
		for i := range n {
			out = append(out, Div(Key(i), Class("h-[50px]"), Textf("Row %d", i)))
		}
		return out
	}
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("flex gap-2"),
			Div(Class("h-[100px] w-[80px] overflow-y-auto"), TestID("small"), Fragment(rows(10)...)),
			Div(Class("h-[180px] w-[200px] overflow-y-auto"), TestID("big"), Fragment(rows(10)...)),
			Div(Class("h-[180px] w-[100px] overflow-y-auto"), TestID("fits"), Fragment(rows(1)...)),
		)
	}, telatest.Size(400, 200))
	got := app.Scrollables()
	if len(got) != 2 {
		t.Fatalf("Scrollables() = %v, want the two that overflow", got)
	}
	for i, want := range []string{"big", "small"} {
		if b, w := app.Bounds(app.NodeLocator(got[i])), app.Bounds(telatest.ByTestID(want)); b != w {
			t.Errorf("Scrollables()[%d] is at %v, want %q at %v", i, b, want, w)
		}
	}
}

// nodesScreen has a counter whose name changes, and a toggle that inserts
// a button before it and removes one after it.
func nodesScreen() tela.Node {
	n, setN := tela.UseState(0)
	extra, setExtra := tela.UseState(false)
	return Div(Class("flex flex-col gap-2 p-4"),
		If(extra, Button(Text("Inserted"))),
		Button(OnClick(func() { setN.Set(n + 1) }), Textf("Count %d", n)),
		Button(OnClick(func() { setExtra.Set(!extra) }), Text("Toggle")),
		If(!extra, Button(Text("Temporary"))),
	)
}

func nodeOf(t *testing.T, app *telatest.App, line string) uint64 {
	t.Helper()
	yaml, ids := app.AriaSnapshotNodes()
	lines := strings.Split(yaml, "\n")
	if len(ids) != len(lines) {
		t.Fatalf("%d ids for %d lines:\n%s", len(ids), len(lines), yaml)
	}
	for i, l := range lines {
		if l == line && ids[i] != 0 {
			return ids[i]
		}
	}
	t.Fatalf("no line %q with an id in:\n%s", line, yaml)
	return 0
}

// TestNodeLocatorFollowsTheElement checks that an element keeps its
// identity when its name changes and siblings come and go, and that its
// locator matches nothing once it is unmounted.
func TestNodeLocatorFollowsTheElement(t *testing.T) {
	t.Parallel()
	app := telatest.MountFunc(t, nodesScreen, telatest.Size(300, 300))
	count := nodeOf(t, app, `- button "Count 0"`)
	toggle := nodeOf(t, app, `- button "Toggle"`)
	temp := nodeOf(t, app, `- button "Temporary"`)
	if count == toggle || count == temp {
		t.Fatalf("identities are not distinct: %d %d %d", count, toggle, temp)
	}

	app.Click(app.NodeLocator(count))
	if got := nodeOf(t, app, `- button "Count 1"`); got != count {
		t.Errorf("the counter's identity changed with its name: %d, was %d", got, count)
	}
	app.Click(app.NodeLocator(toggle))
	// A button was inserted before the counter and another removed after.
	if got := nodeOf(t, app, `- button "Count 1"`); got != count {
		t.Errorf("the counter's identity changed when a sibling was inserted: %d, was %d", got, count)
	}
	if got := nodeOf(t, app, `- button "Toggle"`); got != toggle {
		t.Errorf("the toggle's identity changed: %d, was %d", got, toggle)
	}
	app.Click(app.NodeLocator(count))
	app.ExpectText("Count 2")
	if n := app.Count(app.NodeLocator(temp)); n != 0 {
		t.Errorf("the unmounted button still matches %d element(s)", n)
	}
	if n := app.Count(app.NodeLocator(count)); n != 1 {
		t.Errorf("the mounted counter matches %d elements, want 1", n)
	}
	if s := app.NodeLocator(count).String(); !strings.HasPrefix(s, "getByNodeId(") {
		t.Errorf("String() = %q", s)
	}
}
