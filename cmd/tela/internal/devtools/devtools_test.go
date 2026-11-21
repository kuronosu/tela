package devtools

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/telatest"
)

// mount mounts the DevTools over src in the window the previews use.
func mount(t *testing.T, src Source, tab string, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := telatest.Mount(t, Root{Source: src, Tab: tab}, append([]telatest.Option{telatest.FromApp(App)}, opts...)...)
	return app
}

func TestSelectShowsReasonsAndSource(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabElements)
	app.ExpectRole("treeitem", "main.Settings")
	app.Click(telatest.ByRole("treeitem", "main.Settings"))
	// Where it is in the source, and why it rendered, newest first.
	app.ExpectText("settings.go:42")
	app.ExpectText("Why it rendered")
	app.ExpectText("UseState #2 at settings.go:45 was set")
	app.ExpectText("Props: Title, Sections differ; OnSave cannot be compared")
	app.ExpectText("Mounted")
	if got := app.Count(telatest.ByRole("treeitem", "main.Settings")); got != 1 {
		t.Errorf("%d rows of main.Settings", got)
	}
	app.Expect(telatest.ByRole("treeitem", "main.Settings"), telatest.Selected(true))
}

func TestPickFlow(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabElements)
	app.ExpectRole("treeitem", "main.Settings")
	app.Click(telatest.ByRole("button", "Pick an element"))
	waitCall(t, fake, "pick on")
	// The app picks the Save button's primitive: it is selected and shown.
	picked := fake.Els[FakeSaveBtn]
	fake.Notify("picked", Picked{Window: 1, ID: picked.ID, Element: &picked})
	app.ExpectText("button.go:190")
	app.Expect(telatest.ByRole("treeitem", "button"), telatest.Selected(true))
	// Pick mode ended with the pick: the button picks again.
	app.Click(telatest.ByRole("button", "Pick an element"))
	waitCall(t, fake, "pick on", 2)
	// Escape in the app: pick mode ends, nothing is selected.
	fake.Notify("picked", Picked{Window: 1})
}

// After frames that built, the tree is fetched as a change from the list
// the model has, not whole; selecting an element fetches its box alone.
func TestTreeIncremental(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabElements)
	app.ExpectRole("treeitem", "main.Settings")
	waitCall(t, fake, "elements")
	time.Sleep(50 * time.Millisecond) // the fetches of the start (the load, the tab's) are in
	before := len(fake.Calls())
	fake.mu.Lock()
	i := slices.IndexFunc(fake.Els, func(e core.ElementInfo) bool { return e.Kind == "text" })
	fake.Els[i].Text = "Changed by a build"
	fake.mu.Unlock()
	fake.Notify("frames", Frames{Window: 1, Profiles: []core.FrameProfile{{Frame: 900, Built: 1}}})
	app.ExpectRole("treeitem", `text "Changed by a build"`)
	waitCall(t, fake, "elements since 1")
	for _, c := range fake.Calls()[before:] {
		if strings.HasPrefix(c, "elements") && c != "elements since 1" {
			t.Errorf("after the build: call %q, want only the change since version 1", c)
		}
	}
	app.Click(telatest.ByRole("treeitem", "main.Settings"))
	waitCall(t, fake, fmt.Sprintf("inspect %d depth 0", FakeSettings))
}

// The splitter between the tree and the details sets the pane's width by
// dragging and with the arrow keys, within its bounds, for both panels.
func TestSplitterResizesDetails(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabElements)
	app.ExpectRole("treeitem", "main.Settings")
	sep := telatest.ByRole("separator", "Details width")
	edge := func(l telatest.Locator) float32 { b := app.Bounds(l); return b.X + b.Width/2 } // the pane's left edge
	x0 := edge(sep)
	if want := float32(App.Width) - detailsDefault; x0 < want-8 || x0 > want+8 {
		t.Fatalf("the splitter is at x %v, want about %v (a %v px pane)", x0, want, detailsDefault)
	}
	app.DragBy(sep, -100, 0)
	x1 := edge(sep)
	if d := x0 - x1; d < 99 || d > 101 {
		t.Errorf("a drag 100 px to the left moved the splitter by %v", d)
	}
	app.Focus(sep)
	app.Press("ArrowRight")
	if d := edge(sep) - x1; d < 15 || d > 17 {
		t.Errorf("ArrowRight moved the splitter by %v, want 16", d)
	}
	app.Press("Home")
	if x := edge(sep); x < float32(App.Width)-detailsMin-2 || x > float32(App.Width)-detailsMin+2 {
		t.Errorf("Home put the splitter at %v, want the narrowest pane (%v px)", x, detailsMin)
	}
	app.DragBy(sep, 400, 0)
	if x := edge(sep); x < float32(App.Width)-detailsMin-2 {
		t.Errorf("a drag past the bound put the splitter at %v", x)
	}
	// The Accessibility panel's pane has the same width.
	app.Click(telatest.ByRole("tab", "Accessibility"))
	if x := edge(sep); x < float32(App.Width)-detailsMin-2 || x > float32(App.Width)-detailsMin+2 {
		t.Errorf("the Accessibility panel's splitter is at %v", x)
	}
}

// The details' copy menu puts the element's JSON, its source location and
// its aria snapshot line on the clipboard.
func TestCopyActions(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabElements)
	app.Click(telatest.ByRole("treeitem", "main.Settings"))
	app.ExpectText("Why it rendered") // the details arrived (the JSON has the hooks)
	copyItem := func(item string) string {
		t.Helper()
		app.Click(telatest.ByRole("button", "Copy"))
		app.Click(telatest.ByRole("menuitem", item))
		dt := app.Clipboard()
		if dt == nil {
			t.Fatalf("%s: nothing on the clipboard", item)
		}
		return dt.Text()
	}
	js := copyItem("Copy as JSON")
	var got struct {
		ID    core.ElementID `json:"id"`
		Kind  string         `json:"kind"`
		Hooks []core.HookInfo
	}
	if err := json.Unmarshal([]byte(js), &got); err != nil || got.ID != FakeSettings || got.Kind != "main.Settings" || len(got.Hooks) == 0 {
		t.Errorf("the JSON copied: %+v, %v\n%s", got, err, js)
	}
	if src := copyItem("Copy source location"); !strings.HasSuffix(src, "settings.go:42") || !strings.Contains(src, "/") {
		t.Errorf("the source location copied: %q, want the file's path and line", src)
	}
	// The aria line of the Save button: the menu fetches the snapshot when
	// it opens (the Elements panel has not).
	fake.Notify("picked", Picked{Window: 1, ID: FakeSaveBtn + 1})
	app.Expect(telatest.ByRole("treeitem", "button").Nth(0), telatest.Selected(false)) // settled
	app.Click(telatest.ByRole("button", "Copy"))
	app.Press("Escape")
	if line := copyItem("Copy aria snapshot line"); line != `- button "Save"` {
		t.Errorf("the aria line copied: %q", line)
	}
}

// Hovering a node of the Accessibility panel highlights its element in the
// app, as the Elements panel does; selecting one keeps it highlighted.
func TestA11yHighlights(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabA11y)
	row := telatest.ByRole("treeitem", "button Save")
	app.ExpectRole("treeitem", "button Save")
	app.Hover(row)
	waitCall(t, fake, fmt.Sprintf("highlight %d", FakeSaveBtn+1))
	app.Click(row)
	app.Hover(telatest.ByRole("treeitem", "heading Account"))
	heading := fake.Sem.Elements[slices.IndexFunc(strings.Split(fake.Sem.YAML, "\n"), func(l string) bool { return strings.Contains(l, `heading "Account"`) })]
	waitCall(t, fake, fmt.Sprintf("highlight %d", heading))
	// Off the rows, the selection shows again.
	app.Hover(telatest.ByRole("tab", "Logs"))
	waitCall(t, fake, fmt.Sprintf("highlight %d", FakeSaveBtn+1), 2)
}

// With no system preference the DevTools are dark; the toolbar's toggle
// switches the scheme.
func TestDarkWithoutPreference(t *testing.T) {
	a := app(NewFakeSource())
	t.Cleanup(func() { core.SetNoPreferenceScheme(core.ColorSchemeSystem) })
	ui := telatest.Mount(t, a.Root, telatest.FromApp(a)) // the headless system states no preference
	ui.ExpectRole("button", "Switch to the light scheme")
	ui.Click(telatest.ByRole("button", "Switch to the light scheme"))
	ui.ExpectRole("button", "Switch to the dark scheme")
	ui.Click(telatest.ByRole("button", "Switch to the dark scheme"))
	ui.ExpectRole("button", "Switch to the light scheme")
}

// The session dropped notifications for a client that did not read them:
// the model fetches again what they would have brought.
func TestDroppedResyncs(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabLogs)
	app.ExpectText("darkroom: saved preferences")
	fake.mu.Lock()
	last := fake.Log[len(fake.Log)-1]
	fake.Log = append(fake.Log, LogEntry{Seq: last.Seq + 1, At: last.At, Level: slog.LevelWarn, Msg: "a warning the queue dropped", Source: "app"})
	fake.Win = append(fake.Win, Window{ID: 3, Title: "Export", Width: 400, Height: 300, Scale: 1, Visible: true, Active: true})
	fake.mu.Unlock()
	fake.Notify("dropped", map[string]any{"events": map[string]int{"logs": 1, "windows": 1}})
	app.ExpectText("a warning the queue dropped")
}

func TestHoverHighlights(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabElements)
	app.ExpectRole("treeitem", "main.Sidebar")
	app.Hover(telatest.ByRole("treeitem", "main.Sidebar"))
	waitCall(t, fake, "highlight 5")
}

func TestProfilerFrameClick(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabProfiler)
	app.ExpectText("Nothing recorded")
	app.Click(telatest.ByRole("button", "Record frames"))
	app.ExpectText("Recording")
	fake.Notify("frames", Frames{Window: 1, Profiles: fake.Frames[30:45]})
	app.ExpectText("frames")
	app.Click(telatest.ByRole("button", "Frame 137, 24.35 ms"))
	// The breakdown by phase, the counts, and what it built with why.
	app.ExpectText("Frame 137")
	app.ExpectText("9.50 ms")
	app.ExpectText("over budget")
	app.ExpectText("Elements built")
	app.ExpectText("Its parent built")
	app.Click(telatest.ByRole("button", "Stop frames"))
	app.ExpectRole("button", "Record frames")
}

func TestReloadKeepsSelection(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabElements)
	app.ExpectRole("treeitem", "main.Settings")
	app.Click(telatest.ByRole("treeitem", "main.NavItem key export"))
	app.ExpectText("sidebar.go:33")
	// A new build: the same tree with other IDs.
	for i := range fake.Els {
		fake.Els[i].ID += 100
		if fake.Els[i].Parent != 0 {
			fake.Els[i].Parent += 100
		}
	}
	fake.Details = map[core.ElementID]core.ElementInfo{}
	fake.InfoValue.Build = 4
	fake.Notify("reload", Reload{Build: 4})
	app.ExpectText("build 4")
	app.Expect(telatest.ByRole("treeitem", "main.NavItem key export"), telatest.Selected(true))
	app.ExpectText("sidebar.go:33")

	// A build that numbers its elements in another order (as it builds
	// them): the old ID of the selection names another element now.
	n := len(fake.Els)
	ids := map[core.ElementID]core.ElementID{}
	for i := range fake.Els {
		ids[fake.Els[i].ID] = fake.Els[n-1-i].ID
	}
	for i := range fake.Els {
		fake.Els[i].ID = ids[fake.Els[i].ID]
		fake.Els[i].Parent = ids[fake.Els[i].Parent]
	}
	fake.InfoValue.Build = 5
	fake.Notify("reload", Reload{Build: 5})
	app.ExpectText("build 5")
	app.Expect(telatest.ByRole("treeitem", "main.NavItem key export"), telatest.Selected(true))
}

func TestLogsFilter(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabLogs)
	app.ExpectText("8 of 8 entries")
	app.Click(telatest.ByRole("radio", "Errors"))
	app.ExpectText("2 of 8 entries")
	app.ExpectText("permission denied")
	app.ExpectNoText("saved preferences")
	// A new entry arrives live.
	fake.Notify("logs", map[string]any{"entries": []LogEntry{{Seq: 9, At: time.Now(), Level: 8, Msg: "boom", Source: "stderr"}}})
	app.ExpectText("3 of 9 entries")
}

func TestSearchAndComponents(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabElements)
	app.ExpectRole("treeitem", "main.Settings")
	app.Click(telatest.ByRole("radio", "Components"))
	app.ExpectCount(telatest.ByRole("treeitem", "button"), 0)
	app.Click(telatest.ByRole("radio", "All"))
	app.Fill(telatest.ByRole("textbox", "Search elements"), "progress")
	app.ExpectRole("treeitem", "kit.Progress")
	app.ExpectCount(telatest.ByRole("treeitem", "main.Sidebar"), 0)
}

func TestSessionEnd(t *testing.T) {
	fake := NewFakeSource()
	app := mount(t, fake, TabElements)
	app.ExpectRole("treeitem", "main.Settings")
	fake.End()
	app.ExpectText("The tela dev session ended")
	app.Advance(2 * time.Second)
	app.ExpectQuit()
}

// waitCall waits until the source saw the call n times (once by default).
func waitCall(t *testing.T, f *FakeSource, call string, n ...int) {
	t.Helper()
	want := 1
	if len(n) > 0 {
		want = n[0]
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := 0
		for _, c := range f.Calls() {
			if c == call {
				got++
			}
		}
		if got >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the session never saw %q x%d; calls: %s", call, want, strings.Join(f.Calls(), ", "))
		}
		time.Sleep(2 * time.Millisecond)
	}
}

var _ = tela.Dark
