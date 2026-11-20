package devtools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
)

// FakeSource is a [Source] over made-up data: a settings screen's element
// tree with details, layout, semantics, a profile and a log. The previews
// of the package and its tests use it; a test changes its fields, calls
// Notify to push notifications and reads Calls.
type FakeSource struct {
	mu sync.Mutex

	InfoValue Info
	Win       []Window
	// Els is the tree. Details holds the elements with their hooks, props and
	// builds, Boxes their layout; the others answer with what Els has.
	Els     []core.ElementInfo
	Details map[core.ElementID]core.ElementInfo
	Boxes   map[core.ElementID]core.InspectNode
	Sem     Semantics
	Frames  []core.FrameProfile
	Log     []LogEntry
	// Err, when set, is what every call that reads the tree answers.
	Err error

	calls []string
	notes chan Note
	done  chan struct{}
	once  sync.Once
}

// NewFakeSource returns a source with the settings screen.
func NewFakeSource() *FakeSource {
	f := &FakeSource{notes: make(chan Note, 64), done: make(chan struct{})}
	f.InfoValue = Info{Pkg: "./examples/settings", PID: 4242, Started: time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC), Build: 3, Protocol: 1}
	f.Win = []Window{
		{ID: 1, Title: "Darkroom Preferences", Width: 1100, Height: 760, Scale: 1, Focused: true, Visible: true, Active: true},
		{ID: 2, Title: "About Darkroom", Width: 420, Height: 320, Scale: 1, Visible: true, Active: true},
	}
	f.Els, f.Details, f.Boxes = fakeTree()
	f.Sem = fakeSemantics(f.Els)
	f.Frames = fakeFrames(f.Els)
	f.Log = fakeLog()
	return f
}

// Calls returns the calls that changed something in the session, in order:
// "pick on", "pick off", "highlight 12".
func (f *FakeSource) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *FakeSource) record(format string, args ...any) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	f.mu.Unlock()
}

// Notify pushes a notification: method is the event, params its value.
func (f *FakeSource) Notify(method string, params any) {
	b, _ := json.Marshal(params)
	f.notes <- Note{Method: method, Params: b}
}

// End ends the session.
func (f *FakeSource) End() { f.once.Do(func() { close(f.done); close(f.notes) }) }

func (f *FakeSource) Info(context.Context) (Info, error) { return f.InfoValue, nil }
func (f *FakeSource) Windows(context.Context) ([]Window, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Window(nil), f.Win...), nil
}
func (f *FakeSource) Subscribe(context.Context, ...string) error   { return nil }
func (f *FakeSource) Unsubscribe(context.Context, ...string) error { return nil }

func (f *FakeSource) Elements(_ context.Context, _ uint32, _ bool) ([]core.ElementInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	return append([]core.ElementInfo(nil), f.Els...), nil
}

func (f *FakeSource) Element(_ context.Context, _ uint32, id core.ElementID) (core.ElementInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if d, ok := f.Details[id]; ok {
		return d, nil
	}
	for _, el := range f.Els {
		if el.ID == id {
			// The builds the frames recorded for it, with a cause by turns.
			for _, p := range f.Frames {
				if !slices.Contains(p.BuiltIDs, id) {
					continue
				}
				r := core.BuildReason{Cause: core.CauseParent, Hook: -1}
				switch (int(id) + p.Frame) % 4 {
				case 0:
					r = core.BuildReason{Cause: core.CauseProps, Hook: -1, Fields: []string{"Class", "Children"}}
				case 1:
					r = core.BuildReason{Cause: core.CauseState, Hook: 0, HookKind: "UseState", Site: site(el.Kind+".Build", "settings.go", 44)}
				}
				if !el.Component {
					r = core.BuildReason{Cause: core.CauseParent, Hook: -1}
				}
				el.Recent = append(el.Recent, core.BuildRecord{Frame: p.Frame, Reasons: []core.BuildReason{r}})
			}
			return el, nil
		}
	}
	return core.ElementInfo{}, fmt.Errorf("no element %d (it was unmounted)", id)
}

func (f *FakeSource) Inspect(_ context.Context, _ uint32, id core.ElementID) (core.InspectNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.Boxes[id]; ok {
		return b, nil
	}
	// A component has no box: the session answers with its parent's.
	owner := id
	for _, el := range f.Els {
		if el.ID == id && el.Component {
			owner = el.Parent
		}
	}
	return core.InspectNode{Kind: "div", Element: owner, Rect: gfx.RectXYWH(0, 0, 120, 40),
		Style: core.StyleSummary{Display: "block", Color: "#d5d9df", Font: "400 14px/20 Inter", Opacity: 1}}, nil
}

func (f *FakeSource) Semantics(context.Context, uint32) (Semantics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Sem, nil
}

func (f *FakeSource) Profiles(_ context.Context, _ uint32, since int) ([]core.FrameProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []core.FrameProfile
	for _, p := range f.Frames {
		if p.Frame > since {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *FakeSource) Logs(_ context.Context, since uint64) ([]LogEntry, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []LogEntry
	var last uint64
	for _, e := range f.Log {
		if e.Seq > since {
			out = append(out, e)
		}
		last = max(last, e.Seq)
	}
	return out, last, nil
}

func (f *FakeSource) Pick(_ context.Context, _ uint32, on bool) error {
	if on {
		f.record("pick on")
	} else {
		f.record("pick off")
	}
	return nil
}

func (f *FakeSource) Highlight(_ context.Context, _ uint32, id core.ElementID) error {
	f.record("highlight %d", id)
	return nil
}

func (f *FakeSource) Notes() <-chan Note    { return f.notes }
func (f *FakeSource) Done() <-chan struct{} { return f.done }
func (f *FakeSource) Close() error          { return nil }

var _ Source = (*FakeSource)(nil)

// --- the made-up screen ---

type treeBuilder struct {
	els   []core.ElementInfo
	depth map[core.ElementID]int
}

func (b *treeBuilder) add(parent core.ElementID, kind string, set func(*core.ElementInfo)) core.ElementID {
	el := core.ElementInfo{ID: core.ElementID(len(b.els) + 1), Parent: parent, Kind: kind}
	if parent != 0 {
		el.Depth = b.depth[parent] + 1
	}
	if set != nil {
		set(&el)
	}
	b.depth[el.ID] = el.Depth
	b.els = append(b.els, el)
	return el.ID
}

func site(fn, file string, line int) core.SourceSite {
	return core.SourceSite{Func: fn, File: "/home/ana/darkroom/" + file, Line: line}
}

func comp(file string, line int, fn string) func(*core.ElementInfo) {
	return func(e *core.ElementInfo) { e.Component = true; e.Site = site(fn, file, line) }
}

func prim(classes string, file string, line int) func(*core.ElementInfo) {
	return func(e *core.ElementInfo) { e.Classes = classes; e.Site = site("main.Settings.Build", file, line) }
}

func text(s string) func(*core.ElementInfo) { return func(e *core.ElementInfo) { e.Text = s } }

func keyed(key string, set func(*core.ElementInfo)) func(*core.ElementInfo) {
	return func(e *core.ElementInfo) {
		e.Key = key
		if set != nil {
			set(e)
		}
	}
}

// Element IDs of the made-up tree that the tests and previews refer to:
// the main.Settings component, the Save kit.Button (its primitive is the
// next ID) and the input of the Name field.
const (
	FakeSettings  core.ElementID = 3
	FakeSaveBtn   core.ElementID = 57
	FakeNameField core.ElementID = 42
)

func fakeTree() ([]core.ElementInfo, map[core.ElementID]core.ElementInfo, map[core.ElementID]core.InspectNode) {
	b := &treeBuilder{depth: map[core.ElementID]int{}}
	root := b.add(0, "root", nil)
	prov := b.add(root, "Context[tela.Theme].Provider", func(e *core.ElementInfo) { e.Component = true })
	settings := b.add(prov, "main.Settings", comp("settings.go", 42, "main.Settings.Build"))
	shell := b.add(settings, "div", prim("flex h-full bg-background text-foreground", "settings.go", 58))
	side := b.add(shell, "main.Sidebar", comp("sidebar.go", 18, "main.Sidebar.Build"))
	nav := b.add(side, "nav", prim("flex w-56 flex-col gap-1 border-r p-3", "sidebar.go", 24))
	b.add(nav, "span", prim("px-2 pb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground", "sidebar.go", 26))
	for i, name := range []string{"General", "Develop", "Export", "Safelight", "Account"} {
		item := b.add(nav, "main.NavItem", keyed(strings.ToLower(name), comp("sidebar.go", 33, "main.NavItem.Build")))
		btn := b.add(item, "kit.Button", comp("button.go", 165, "kit.Button.Build"))
		el := b.add(btn, "button", prim("inline-flex h-8 items-center gap-2 rounded-md px-2 text-sm hover:bg-accent", "button.go", 190))
		b.add(el, "text", text(name))
		_ = i
	}
	content := b.add(shell, "main", prim("flex min-w-0 flex-1 flex-col gap-6 overflow-y-auto p-8", "settings.go", 66))
	b.add(content, "h1", prim("text-2xl font-semibold tracking-tight", "settings.go", 67))
	b.add(b.els[len(b.els)-1].ID, "text", text("Account"))
	profile := b.add(content, "main.Section", keyed("profile", comp("section.go", 21, "main.Section.Build")))
	card := b.add(profile, "kit.Card", comp("card.go", 88, "kit.Card.Build"))
	cardEl := b.add(card, "div", prim("flex flex-col gap-6 rounded-xl border bg-card py-6 shadow-sm", "card.go", 96))
	b.add(cardEl, "h2", prim("px-6 font-semibold leading-none", "card.go", 101))
	b.add(b.els[len(b.els)-1].ID, "text", text("Profile"))
	form := b.add(cardEl, "div", prim("grid gap-4 px-6", "section.go", 34))
	for _, f := range []struct{ label, value string }{{"Name", "Ana Ruiz"}, {"E-mail", "ana.ruiz@darkroom.app"}} {
		field := b.add(form, "kit.Field", comp("field.go", 112, "kit.Field.Build"))
		wrap := b.add(field, "div", prim("grid gap-2", "field.go", 120))
		lbl := b.add(wrap, "label", prim("text-sm font-medium", "field.go", 122))
		b.add(lbl, "text", text(f.label))
		in := b.add(wrap, "kit.Input", comp("input.go", 178, "kit.Input.Build"))
		b.add(in, "input", prim("h-9 w-full rounded-md border bg-transparent px-3 text-sm", "input.go", 201))
	}
	plan := b.add(content, "main.Section", keyed("plan", comp("section.go", 21, "main.Section.Build")))
	pcard := b.add(plan, "kit.Card", comp("card.go", 88, "kit.Card.Build"))
	pel := b.add(pcard, "div", prim("flex flex-col gap-6 rounded-xl border bg-card py-6 shadow-sm", "card.go", 96))
	b.add(pel, "kit.Progress", comp("progress.go", 54, "kit.Progress.Build"))
	b.add(pel, "span", prim("px-6 text-sm text-muted-foreground", "settings.go", 112))
	b.add(b.els[len(b.els)-1].ID, "text", text("58.8 GB free. Originals count; previews do not."))
	foot := b.add(shell, "footer", prim("flex items-center justify-end gap-2 border-t px-8 py-3", "settings.go", 130))
	b.add(foot, "kit.Button", keyed("discard", comp("button.go", 165, "kit.Button.Build")))
	save := b.add(foot, "kit.Button", keyed("save", comp("button.go", 165, "kit.Button.Build")))
	sb := b.add(save, "button", prim("inline-flex h-9 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground", "button.go", 190))
	b.add(sb, "text", text("Save"))
	// Portals of an open menu: the app's overlay layer.
	b.add(root, "Portal", nil)

	els := b.els
	// Bounds and builds.
	y := float32(0)
	for i := range els {
		els[i].Builds = 1 + i%4
		if els[i].Kind == "root" || els[i].Component {
			continue
		}
		r := gfx.RectXYWH(0, y, 1100, 28)
		els[i].Bounds = &r
		y += 18
	}
	details := map[core.ElementID]core.ElementInfo{}
	boxes := map[core.ElementID]core.InspectNode{}

	d := els[settings-1]
	d.Builds = 6
	d.Props = []core.PropInfo{{Name: "Title", Value: `"Darkroom Preferences"`}, {Name: "Sections", Value: `[General Develop Export Safelight Account]`}, {Name: "OnSave", Value: "func()"}}
	d.Hooks = []core.HookInfo{
		{Index: 0, Kind: "UseState", Site: site("main.Settings.Build", "settings.go", 44), Value: `"account"`},
		{Index: 1, Kind: "UseState", Site: site("main.Settings.Build", "settings.go", 45), Value: `{Name:"Ana Ruiz" Email:"ana.ruiz@darkroom.app"}`},
		{Index: 2, Kind: "UseMemo", Site: site("main.Settings.Build", "settings.go", 47), Value: `[5 sections]`},
		{Index: 3, Kind: "UseEffect", Site: site("main.Settings.Build", "settings.go", 49), Value: "deps [account]"},
		{Index: 4, Kind: "UseContext", Site: site("main.Settings.Build", "settings.go", 53), Value: "tela.Theme"},
	}
	d.Recent = []core.BuildRecord{
		{Frame: 41, Reasons: []core.BuildReason{{Cause: core.CauseMount, Hook: -1}}},
		{Frame: 96, Reasons: []core.BuildReason{{Cause: core.CauseState, Hook: 0, HookKind: "UseState", Site: site("main.Settings.Build", "settings.go", 44)}}},
		{Frame: 118, Reasons: []core.BuildReason{
			{Cause: core.CauseState, Hook: 1, HookKind: "UseState", Site: site("main.Settings.Build", "settings.go", 45)},
			{Cause: core.CauseContext, Hook: 4, HookKind: "UseContext", Context: "tela.Theme", Element: prov},
		}},
		{Frame: 131, Reasons: []core.BuildReason{{Cause: core.CauseProps, Hook: -1, Fields: []string{"Title", "Sections"}, Uncomparable: []string{"OnSave"}}}},
	}
	details[settings] = d

	s := els[save-1]
	sd := s
	sd.Props = []core.PropInfo{{Name: "Variant", Value: `""`}, {Name: "Size", Value: `""`}, {Name: "Disabled", Value: "false"}, {Name: "OnClick", Value: "func()"}, {Name: "Children", Value: "<text>"}}
	sd.Hooks = []core.HookInfo{{Index: 0, Kind: "UseContext", Site: site("kit.Button.Build", "button.go", 166), Value: "kit.Style"}, {Index: 1, Kind: "UsePress", Site: site("kit.Button.Build", "button.go", 172), Value: "{pressed:false}"}}
	sd.Builds = 3
	sd.Recent = []core.BuildRecord{
		{Frame: 41, Reasons: []core.BuildReason{{Cause: core.CauseMount, Hook: -1}}},
		{Frame: 118, Reasons: []core.BuildReason{{Cause: core.CauseParent, Hook: -1}}},
		{Frame: 131, Reasons: []core.BuildReason{{Cause: core.CauseProps, Hook: -1, Fields: []string{"Disabled"}}}},
	}
	details[save] = sd
	// The save button's primitive, with a padded box.
	btnID := save + 1
	bd := els[btnID-1]
	bd.Builds = 3
	bd.Recent = []core.BuildRecord{{Frame: 131, Reasons: []core.BuildReason{{Cause: core.CauseParent, Hook: -1}}}}
	details[btnID] = bd
	boxes[btnID] = core.InspectNode{Kind: "button", Element: btnID, Rect: gfx.RectXYWH(980, 710, 72, 36),
		Margin:  core.Edges{Top: 0, Right: 8, Bottom: 0, Left: 8},
		Border:  core.Edges{Top: 1, Right: 1, Bottom: 1, Left: 1},
		Padding: core.Edges{Top: 8, Right: 16, Bottom: 8, Left: 16},
		Style: core.StyleSummary{Classes: "inline-flex h-9 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground",
			Display: "inline-flex", Position: "static", Width: "auto", Height: "36px", Flex: "row justify center items center",
			FlexItem: "grow 0 shrink 1 basis auto", Color: "#0b1220", Background: "#62a8ff", Font: "500 14px/20 Inter", TextAlign: "center", Opacity: 1}}
	return els, details, boxes
}

func fakeSemantics(els []core.ElementInfo) Semantics {
	type line struct {
		text string
		el   core.ElementID
	}
	find := func(kind, txt string) core.ElementID {
		for _, e := range els {
			if e.Kind == kind && (txt == "" || e.Text == txt) {
				return e.ID
			}
		}
		return 0
	}
	btn := func(name string) core.ElementID {
		for _, e := range els {
			if e.Kind == "text" && e.Text == name {
				return e.Parent
			}
		}
		return 0
	}
	lines := []line{
		{`- navigation "Preferences sections":`, find("nav", "")},
		{`  - text: Darkroom Preferences`, 0},
		{`  - button "General"`, btn("General")},
		{`  - button "Develop"`, btn("Develop")},
		{`  - button "Export"`, btn("Export")},
		{`  - button "Safelight"`, btn("Safelight")},
		{`  - button "Account" [current]`, btn("Account")},
		{`- main:`, find("main", "")},
		{`  - heading "Account" [level=1]`, find("h1", "")},
		{`  - heading "Profile" [level=2]`, find("h2", "")},
		{`  - group:`, 0},
		{`    - textbox "Name": Ana Ruiz`, FakeNameField},
		{`    - textbox "E-mail": ana.ruiz@darkroom.app`, FakeNameField + 6},
		{`  - progressbar "Storage used"`, find("kit.Progress", "")},
		{`  - paragraph: 58.8 GB free. Originals count; previews do not.`, 0},
		{`- contentinfo:`, find("footer", "")},
		{`  - button "Discard" [disabled]`, 0},
		{`  - button "Save"`, btn("Save")},
	}
	var s Semantics
	var sb strings.Builder
	for i, l := range lines {
		sb.WriteString(l.text + "\n")
		s.Nodes = append(s.Nodes, uint64(i+1))
		s.Elements = append(s.Elements, l.el)
	}
	s.YAML = sb.String()
	return s
}

// fakeFrames makes a profile of 140 frames: a quiet app with a few slow
// frames (a layout after the content grew, a build of the whole screen).
func fakeFrames(els []core.ElementInfo) []core.FrameProfile {
	ms := func(f float64) time.Duration { return time.Duration(f * float64(time.Millisecond)) }
	var out []core.FrameProfile
	seed := uint32(7)
	rnd := func() float64 {
		seed = seed*1664525 + 1013904223
		return float64(seed>>8) / float64(1<<24)
	}
	for n := 1; n <= 140; n++ {
		scale := 0.6 + rnd()*0.8
		var p [core.NumPhases]time.Duration
		p[core.PhaseInput] = ms(0.05 * scale)
		p[core.PhaseAnimate] = ms(0.15 * scale)
		p[core.PhaseBuild] = ms(0.9 * scale)
		p[core.PhaseStyle] = ms(0.5 * scale)
		p[core.PhaseLayout] = ms(1.1 * scale)
		p[core.PhasePaint] = ms(0.8 * scale)
		p[core.PhaseSemantics] = ms(0.2 * scale)
		p[core.PhaseRaster] = ms(2.1 * scale)
		p[core.PhasePresent] = ms(0.4 * scale)
		p[core.PhaseEffects] = ms(0.05 * scale)
		built := 2 + int(rnd()*6)
		switch {
		case n == 37:
			p[core.PhaseBuild], p[core.PhaseLayout], p[core.PhaseStyle] = ms(9.5), ms(7.2), ms(3.1)
			built = 61
		case n == 88:
			p[core.PhaseRaster], p[core.PhasePaint] = ms(14.0), ms(4.2)
			built = 4
		case n == 121:
			p[core.PhaseBuild], p[core.PhaseLayout] = ms(5.6), ms(4.8)
			built = 23
		case n%29 == 0:
			p[core.PhaseRaster] = ms(8.5)
		}
		fp := core.FrameProfile{Frame: n + 100, Start: time.Duration(n) * ms(16.7), Built: built,
			Styled: built * 3, LaidOut: built * 4, Layers: 2 + n%3, Damage: int64(1100*760) / int64(1+n%7)}
		fp.Phases = p
		for i, d := range p {
			if core.Phase(i) != core.PhaseInput {
				fp.Duration += d
			}
		}
		for i := 0; i < min(built, 40); i++ {
			fp.BuiltIDs = append(fp.BuiltIDs, els[(n*7+i*3)%len(els)].ID)
		}
		fp.BuiltMore = built - len(fp.BuiltIDs)
		out = append(out, fp)
	}
	return out
}

func fakeLog() []LogEntry {
	t0 := time.Date(2026, 11, 20, 9, 30, 12, 0, time.UTC)
	e := func(i int, lvl slog.Level, src, msg string) LogEntry {
		return LogEntry{Seq: uint64(i), At: t0.Add(time.Duration(i) * 1300 * time.Millisecond), Level: lvl, Msg: msg, Source: src, Build: 3}
	}
	return []LogEntry{
		e(1, slog.LevelInfo, "stderr", "darkroom: opening catalog /home/ana/Pictures/darkroom.db"),
		e(2, slog.LevelInfo, "stderr", "darkroom: 4,218 images, 12 collections"),
		e(3, slog.LevelWarn, "app", `Class "rounded-xxl" is unknown; did you mean "rounded-2xl"? (settings.go:112)`),
		e(4, slog.LevelDebug, "stderr", "darkroom: thumbnail cache warm in 82ms"),
		e(5, slog.LevelWarn, "app", "UseState #3 of main.Settings holds a func, which tela dev cannot keep across reloads (settings.go:47)"),
		e(6, slog.LevelError, "stderr", "darkroom: export preset \"web-2048\" failed: open /mnt/share/web: permission denied"),
		e(7, slog.LevelInfo, "stderr", "darkroom: saved preferences"),
		e(8, slog.LevelError, "app", "the key \"profile\" is used by two siblings of main.Section (settings.go:90): the second one is dropped"),
	}
}
