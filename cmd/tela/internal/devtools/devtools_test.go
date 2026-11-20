package devtools

import (
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
