package devtools

import (
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
)

// fakeView loads the made-up session synchronously.
func fakeView(t *testing.T, p Preload) View {
	t.Helper()
	m := NewModel(NewFakeSource())
	m.Preload(p)
	return m.View()
}

func kinds(v View, rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, v.Els[r.I].Kind)
	}
	return out
}

func TestTreeRowsFolding(t *testing.T) {
	v := fakeView(t, Preload{})
	all := TreeRows(v, TreeOptions{})
	if len(all) != len(v.Els) {
		t.Fatalf("%d rows for %d elements", len(all), len(v.Els))
	}
	// Folding the sidebar hides everything under it and nothing else.
	sidebar := v.Index[5]
	m := NewModel(NewFakeSource())
	m.Preload(Preload{})
	m.ToggleCollapsed(v.Paths[sidebar])
	v = m.View()
	rows := TreeRows(v, TreeOptions{})
	if len(rows) != len(all)-22 {
		t.Errorf("%d rows after folding the sidebar, want %d", len(rows), len(all)-22)
	}
	if i := RowIndex(rows, 5); i < 0 || !rows[i].HasKids || rows[i].Open {
		t.Errorf("the folded row: %+v", rows[i])
	}
	if RowIndex(rows, 6) >= 0 || RowIndex(rows, 28) < 0 {
		t.Errorf("the nav is still shown, or the content is gone")
	}
}

func TestTreeRowsComponents(t *testing.T) {
	v := fakeView(t, Preload{})
	rows := TreeRows(v, TreeOptions{Components: true})
	for _, k := range kinds(v, rows) {
		if k == "div" || k == "button" || k == "text" {
			t.Errorf("a primitive among the components: %s", k)
		}
	}
	// A component under primitives is indented by the components above it.
	i := RowIndex(rows, FakeSaveBtn)
	if i < 0 || rows[i].Depth != 3 { // root, the provider, main.Settings, then kit.Button: the div and footer are hidden
		t.Errorf("kit.Button row %d: %+v", i, rows[i])
	}
}

func TestTreeRowsSearch(t *testing.T) {
	v := fakeView(t, Preload{})
	rows := TreeRows(v, TreeOptions{Search: "PROGRESS"})
	got := kinds(v, rows)
	want := []string{"root", "Context[tela.Theme].Provider", "main.Settings", "div", "main", "main.Section", "kit.Card", "div", "kit.Progress"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows for \"progress\": %v, want %v", got, want)
	}
	if rows[len(rows)-1].Dim || !rows[0].Dim {
		t.Errorf("the match is dim, or its ancestors are not")
	}
	// Text and classes match too.
	if got := TreeRows(v, TreeOptions{Search: "previews do not"}); len(got) == 0 {
		t.Errorf("no row for a text")
	}
	if got := TreeRows(v, TreeOptions{Search: "nothing like this"}); len(got) != 0 {
		t.Errorf("%d rows for a search that matches nothing", len(got))
	}
}

func TestPaths(t *testing.T) {
	v := fakeView(t, Preload{})
	if got := v.Paths[v.Index[8]]; got != "root[0]/Context[tela.Theme].Provider[0]/main.Settings[0]/div[0]/main.Sidebar[0]/nav[0]/main.NavItem#general" {
		t.Errorf("path of the first nav item: %s", got)
	}
	seen := map[string]bool{}
	for _, p := range v.Paths {
		if seen[p] {
			t.Errorf("path %s is not unique", p)
		}
		seen[p] = true
	}
}

func TestParseSemantics(t *testing.T) {
	s := Semantics{
		YAML:     "- navigation \"Sections\":\n  - text: Darkroom\n  - button \"Save \\\"all\\\"\" [disabled]\n- heading \"Account\" [level=1]\n- textbox \"E-mail\": ana@x.app\n- list:\n  - /url: /a\n",
		Elements: []core.ElementID{4, 0, 9, 11, 12, 0, 0},
	}
	got := ParseSemantics(s)
	want := []SemRow{
		{Depth: 0, Role: "navigation", Name: "Sections", Element: 4},
		{Depth: 1, Role: "text", Value: "Darkroom"},
		{Depth: 1, Role: "button", Name: `Save "all"`, States: []string{"disabled"}, Element: 9},
		{Depth: 0, Role: "heading", Name: "Account", States: []string{"level=1"}, Element: 11},
		{Depth: 0, Role: "textbox", Name: "E-mail", Value: "ana@x.app", Element: 12},
		{Depth: 0, Role: "list"},
		{Depth: 1, Role: "/url", Value: "/a"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows:\n got  %+v\n want %+v", got, want)
	}
}

func TestSummarize(t *testing.T) {
	f := NewFakeSource().Frames
	ptrs := make([]*core.FrameProfile, len(f))
	for i := range f {
		ptrs[i] = &f[i]
	}
	s := Summarize(ptrs)
	if s.Frames != 140 || s.OverBudget != 2 || s.Slowest != core.PhaseRaster || s.Max < 24*time.Millisecond {
		t.Errorf("summary %+v", s)
	}
	if got := Summarize(nil); got.Frames != 0 {
		t.Errorf("summary of nothing: %+v", got)
	}
}

func TestLogFilter(t *testing.T) {
	all := fakeView(t, Preload{}).Logs
	if got := (LogFilter{Min: slog.LevelWarn}).Filter(all); len(got) != 4 {
		t.Errorf("%d warnings and errors, want 4", len(got))
	}
	if got := (LogFilter{Source: "app"}).Filter(all); len(got) != 3 {
		t.Errorf("%d app entries, want 3", len(got))
	}
	if got := (LogFilter{Search: "PERMISSION"}).Filter(all); len(got) != 1 || got[0].Seq != 6 {
		t.Errorf("search: %+v", got)
	}
}

// A frame notification that built elements fetches the tree again, and one
// that built the selection fetches its details: the panels follow the app.
func TestModelFollowsFrames(t *testing.T) {
	fake := NewFakeSource()
	m := NewModel(fake)
	m.Preload(Preload{Select: FakeSettings})
	m.OnChange(func() {})
	v := m.View()
	rev := v.ElsRev
	fake.mu.Lock()
	fake.Els = append(fake.Els, core.ElementInfo{ID: 99, Parent: 2, Depth: 2, Kind: "main.Banner", Component: true})
	d := fake.Details[FakeSettings]
	d.Recent = append(d.Recent, core.BuildRecord{Frame: 500, Reasons: []core.BuildReason{{Cause: core.CauseState, Hook: 0, HookKind: "UseState"}}})
	fake.Details[FakeSettings] = d
	fake.mu.Unlock()
	m.frames(Frames{Window: 1, Profiles: []core.FrameProfile{{Frame: 500, Built: 2, BuiltIDs: []core.ElementID{FakeSettings}}}})
	deadline := time.Now().Add(5 * time.Second)
	for {
		v = m.View()
		if v.ElsRev > rev && len(v.Detail.Recent) == 5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the model did not follow: tree rev %d (was %d), %d builds", v.ElsRev, rev, len(v.Detail.Recent))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok := v.Index[99]; !ok {
		t.Errorf("the new element is not in the tree")
	}
	m.Close()
}
