package devtools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/core"
)

// Conn is the state of the connection to the session.
type Conn uint8

const (
	// Connecting: the first calls are in flight.
	Connecting Conn = iota
	// Connected: the session answers.
	Connected
	// Ended: the session ended; nothing more will come.
	Ended
)

// Limits of what the model keeps.
const (
	maxFrames   = core.ProfileFrames // profile frames of a recording
	maxLogs     = 5000
	maxFrameEls = 60 // elements listed for a frame
	// refreshDelay is how long the tree waits for the app to settle before
	// it is fetched again after frames that built.
	refreshDelay = 250 * time.Millisecond
)

// View is what the panels show: a snapshot of the model, safe to read on
// any goroutine. Slices and maps are never changed after the snapshot is
// taken, so a View can be kept and compared.
type View struct {
	Conn Conn
	// Err is the last error of a call, shown in the banner; "" when the
	// last calls worked.
	Err   string
	Info  Info
	Build int

	Windows []Window
	Window  uint32

	// Els is the element tree in tree order (each element after its
	// parent), Paths the stable path of each (kind and key or position among
	// its siblings, from the root), ElsRev a number that changes with them.
	Els    []core.ElementInfo
	Paths  []string
	Index  map[core.ElementID]int
	ElsRev int
	// Collapsed holds the paths of the rows folded, CollapsedRev a number
	// that changes with it.
	Collapsed    map[string]bool
	CollapsedRev int

	Selected core.ElementID
	// Detail is the selected element with its hooks, props and builds, Box
	// its layout; nil until they arrive (or when the element is gone).
	Detail *core.ElementInfo
	Box    *core.InspectNode
	// RevealRev changes when the selection should be scrolled into view (a
	// pick, a reload that moved it).
	RevealRev int
	Picking   bool

	Sem    Semantics
	SemRev int

	Recording bool
	Frames    []*core.FrameProfile
	// Frame is the number of the frame selected in the profile (0 for
	// none) and FrameEls the elements it built that are still mounted, with
	// their builds; FrameMore counts the ones not listed.
	Frame     int
	FrameEls  []core.ElementInfo
	FrameMore int

	Logs []LogEntry
}

// Model fetches what the panels show from a [Source] and keeps it. Its
// methods may be called from any goroutine; the ones that call the source
// return at once and do that on goroutines of their own. OnChange is called
// after every change.
type Model struct {
	src    Source
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	v        View
	onChange func()
	tab      string
	selPath  string // the path of the selection, to find it after a reload
	selSeq   int    // the latest selection's number: older answers are dropped
	frameSeq int
	treeSeq  int // the latest tree fetch's number: older answers are dropped
	logSeq   uint64
	timer    *time.Timer // the pending tree refresh
	started  bool

	hlMu   sync.Mutex
	hlWant core.ElementID
	hlSig  chan struct{}
}

// NewModel returns a model of src; call Start to connect.
func NewModel(src Source) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	return &Model{src: src, ctx: ctx, cancel: cancel, hlSig: make(chan struct{}, 1),
		v: View{Collapsed: map[string]bool{}, Index: map[core.ElementID]int{}}}
}

// OnChange sets the function called, on a goroutine of the model, after
// the view changed.
func (m *Model) OnChange(f func()) {
	m.mu.Lock()
	m.onChange = f
	m.mu.Unlock()
}

// View returns the current snapshot.
func (m *Model) View() View {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.v
}

// Start connects and keeps the view up to date until Close. Calling it
// again does nothing.
func (m *Model) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()
	go m.run()
	go m.highlighter()
}

// Close stops the model and closes the source.
func (m *Model) Close() {
	m.cancel()
	m.mu.Lock()
	if m.timer != nil {
		m.timer.Stop()
	}
	m.mu.Unlock()
	m.src.Close()
}

// update changes the view under the lock and tells the UI.
func (m *Model) update(f func(v *View)) {
	m.mu.Lock()
	f(&m.v)
	cb := m.onChange
	m.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// fail records the error of a call, unless the model is closing.
func (m *Model) fail(what string, err error) {
	if err == nil || m.ctx.Err() != nil {
		return
	}
	m.update(func(v *View) { v.Err = what + ": " + err.Error() })
}

func (m *Model) run() {
	ctx := m.ctx
	if info, err := m.src.Info(ctx); err == nil {
		m.update(func(v *View) { v.Info, v.Build = info, info.Build })
	} else {
		m.fail("info", err)
	}
	if err := m.src.Subscribe(ctx, "picked", "frames", "logs", "reload", "windows"); err != nil {
		m.fail("subscribe", err)
	}
	m.loadAll()
	if es, last, err := m.src.Logs(ctx, 0); err == nil {
		m.addLogs(es, last)
	} else {
		m.fail("logs", err)
	}
	notes, done := m.src.Notes(), m.src.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case n, ok := <-notes:
			if !ok {
				notes = nil
				continue
			}
			m.note(n)
		case <-done:
			// Drain what was queued before the end.
			for n := range m.src.Notes() {
				m.note(n)
			}
			m.update(func(v *View) { v.Conn = Ended; v.Picking = false })
			return
		}
	}
}

// loadAll fetches the windows and the tree of the current window.
func (m *Model) loadAll() {
	ws, err := m.src.Windows(m.ctx)
	if err != nil {
		m.fail("windows", err)
		return
	}
	m.setWindows(ws)
	m.refreshTree()
}

func (m *Model) setWindows(ws []Window) {
	m.update(func(v *View) {
		v.Windows = ws
		if !slices.ContainsFunc(ws, func(w Window) bool { return w.ID == v.Window }) || v.Window == 0 {
			v.Window = firstWindow(ws)
		}
	})
}

// firstWindow is the window shown first: the first the running app shows.
func firstWindow(ws []Window) uint32 {
	for _, w := range ws {
		if w.Active {
			return w.ID
		}
	}
	if len(ws) > 0 {
		return ws[0].ID
	}
	return 0
}

// note handles a notification.
func (m *Model) note(n Note) {
	switch n.Method {
	case "picked":
		var p Picked
		if json.Unmarshal(n.Params, &p) == nil {
			m.picked(p)
		}
	case "frames":
		var f Frames
		if json.Unmarshal(n.Params, &f) == nil {
			m.frames(f)
		}
	case "logs":
		var l struct {
			Entries []LogEntry `json:"entries"`
		}
		if json.Unmarshal(n.Params, &l) == nil {
			m.addLogs(l.Entries, 0)
		}
	case "reload":
		var r Reload
		json.Unmarshal(n.Params, &r)
		m.update(func(v *View) {
			v.Build = r.Build
			v.Err = ""
			v.Picking = false
			v.Frame, v.FrameEls, v.FrameMore = 0, nil, 0 // their IDs are the old build's
		})
		m.loadAll()
	case "windows":
		var w struct {
			Windows []Window `json:"windows"`
		}
		if json.Unmarshal(n.Params, &w) == nil {
			m.setWindows(w.Windows)
			m.scheduleRefresh()
		}
	}
}

// --- the tree ---

// refreshTree fetches the elements of the current window and keeps the
// selection by ID, or by path when the IDs started again (a reload).
func (m *Model) refreshTree() {
	m.mu.Lock()
	win := m.v.Window
	m.treeSeq++
	seq := m.treeSeq
	m.mu.Unlock()
	els, err := m.src.Elements(m.ctx, win, false)
	if err != nil {
		m.fail("elements", err)
		return
	}
	paths, index := pathsOf(els)
	var reselect core.ElementID
	var need bool
	m.update(func(v *View) {
		if seq != m.treeSeq {
			return // a newer fetch is on its way
		}
		v.Conn = Connected
		v.Err = ""
		v.Els, v.Paths, v.Index = els, paths, index
		v.ElsRev++
		if v.Selected == 0 {
			return
		}
		if i, ok := index[v.Selected]; ok {
			m.selPath = paths[i]
			return
		}
		if i := slices.Index(paths, m.selPath); i >= 0 && m.selPath != "" {
			v.Selected, reselect, need = els[i].ID, els[i].ID, true
			v.RevealRev++
			return
		}
		v.Selected, v.Detail, v.Box = 0, nil, nil
	})
	if need {
		m.fetchDetail(reselect)
	} else if v := m.View(); v.Selected != 0 && m.tabIs("elements") && v.Detail == nil {
		m.fetchDetail(v.Selected)
	}
	if m.tabIs("a11y") {
		m.loadSemantics()
	}
}

func (m *Model) tabIs(t string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tab == t
}

// scheduleRefresh fetches the tree once the app has been quiet for a moment.
func (m *Model) scheduleRefresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer != nil || m.ctx.Err() != nil {
		return
	}
	m.timer = time.AfterFunc(refreshDelay, func() {
		m.mu.Lock()
		m.timer = nil
		m.mu.Unlock()
		if m.ctx.Err() == nil {
			m.refreshTree()
		}
	})
}

// pathsOf returns the path of every element and the index of each ID. A
// path is the kinds from the root, each with its key or its position among
// the siblings of the same kind: "root/main.App/div[1]/main.Row#7".
func pathsOf(els []core.ElementInfo) ([]string, map[core.ElementID]int) {
	paths := make([]string, len(els))
	index := make(map[core.ElementID]int, len(els))
	type slot struct {
		parent core.ElementID
		kind   string
	}
	counts := map[slot]int{}
	for i, el := range els {
		index[el.ID] = i
		seg := el.Kind
		if el.Key != "" {
			seg += "#" + el.Key
		} else {
			s := slot{el.Parent, el.Kind}
			seg += "[" + strconv.Itoa(counts[s]) + "]"
			counts[s]++
		}
		if pi, ok := index[el.Parent]; ok && el.Parent != 0 {
			seg = paths[pi] + "/" + seg
		}
		paths[i] = seg
	}
	return paths, index
}

// --- selection ---

// SetTab tells the model which panel shows: "elements", "a11y",
// "profiler" or "logs". It fetches what that panel needs.
func (m *Model) SetTab(tab string) {
	m.mu.Lock()
	changed := m.tab != tab
	m.tab = tab
	v := m.v
	m.mu.Unlock()
	if !changed {
		return
	}
	switch tab {
	case "a11y":
		go m.loadSemantics()
		go m.refreshTree()
	case "elements":
		if v.Selected != 0 && v.Detail == nil {
			go m.fetchDetail(v.Selected)
		}
		go m.refreshTree()
	}
}

// Select selects an element: its details and layout are fetched. reveal
// scrolls the tree to it and unfolds its ancestors.
func (m *Model) Select(id core.ElementID, reveal bool) {
	m.selectWith(id, reveal, nil)
}

func (m *Model) selectWith(id core.ElementID, reveal bool, known *core.ElementInfo) {
	m.update(func(v *View) {
		if v.Selected != id {
			v.Detail, v.Box = nil, nil
		}
		if known != nil && known.ID == id && v.Detail == nil {
			v.Detail = known
		}
		v.Selected = id
		if i, ok := v.Index[id]; ok {
			m.selPath = v.Paths[i]
			if reveal {
				v.Collapsed = unfold(v, i)
				v.CollapsedRev++
				v.RevealRev++
			}
		} else if id == 0 {
			m.selPath = ""
		}
	})
	if id != 0 {
		go m.fetchDetail(id)
	}
}

// unfold returns v.Collapsed without the ancestors of element i.
func unfold(v *View, i int) map[string]bool {
	out := make(map[string]bool, len(v.Collapsed))
	for p, c := range v.Collapsed {
		out[p] = c
	}
	for p := v.Els[i].Parent; p != 0; {
		j, ok := v.Index[p]
		if !ok {
			break
		}
		delete(out, v.Paths[j])
		p = v.Els[j].Parent
	}
	return out
}

// fetchDetail loads the element's details and its layout.
func (m *Model) fetchDetail(id core.ElementID) {
	m.mu.Lock()
	m.selSeq++
	seq, win := m.selSeq, m.v.Window
	m.mu.Unlock()
	info, err := m.src.Element(m.ctx, win, id)
	var box *core.InspectNode
	if err == nil {
		if b, berr := m.src.Inspect(m.ctx, win, id); berr == nil {
			box = &b
		}
	}
	m.update(func(v *View) {
		if seq != m.selSeq || v.Selected != id {
			return
		}
		if err != nil {
			// The element is gone (a reload, a rebuild): clear the selection
			// unless the tree will say otherwise.
			if m.ctx.Err() == nil {
				v.Detail, v.Box = nil, nil
				v.Err = "element: " + err.Error()
			}
			return
		}
		v.Err = ""
		v.Detail, v.Box = &info, box
	})
}

// SelectWindow chooses the window the panels show.
func (m *Model) SelectWindow(id uint32) {
	m.update(func(v *View) {
		if v.Window == id {
			return
		}
		v.Window = id
		v.Selected, v.Detail, v.Box = 0, nil, nil
		v.Frames, v.Frame, v.FrameEls = nil, 0, nil
		m.selPath = ""
	})
	go m.refreshTree()
}

// ToggleCollapsed folds or unfolds the row at path.
func (m *Model) ToggleCollapsed(path string) {
	m.update(func(v *View) {
		out := make(map[string]bool, len(v.Collapsed)+1)
		for p, c := range v.Collapsed {
			out[p] = c
		}
		if out[path] {
			delete(out, path)
		} else {
			out[path] = true
		}
		v.Collapsed = out
		v.CollapsedRev++
	})
}

// SetAllCollapsed folds every row that has children, or unfolds all.
func (m *Model) SetAllCollapsed(collapsed bool) {
	m.update(func(v *View) {
		out := map[string]bool{}
		if collapsed {
			for i, el := range v.Els {
				if i+1 < len(v.Els) && v.Els[i+1].Parent == el.ID {
					out[v.Paths[i]] = true
				}
			}
		}
		v.Collapsed = out
		v.CollapsedRev++
	})
}

// Highlight highlights the element in the app (0 clears it). Calls are
// coalesced: only the latest wanted element is sent.
func (m *Model) Highlight(id core.ElementID) {
	m.hlMu.Lock()
	m.hlWant = id
	m.hlMu.Unlock()
	select {
	case m.hlSig <- struct{}{}:
	default:
	}
}

func (m *Model) highlighter() {
	var sent core.ElementID
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.hlSig:
		}
		m.hlMu.Lock()
		want := m.hlWant
		m.hlMu.Unlock()
		if want == sent {
			continue
		}
		win := m.View().Window
		if err := m.src.Highlight(m.ctx, win, want); err != nil {
			continue
		}
		sent = want
	}
}

// TogglePick turns pick mode of the window on or off; the element picked
// arrives as a notification.
func (m *Model) TogglePick() {
	var on bool
	var win uint32
	m.update(func(v *View) { v.Picking = !v.Picking; on, win = v.Picking, v.Window })
	go func() {
		if err := m.src.Pick(m.ctx, win, on); err != nil {
			m.update(func(v *View) { v.Picking = false })
			m.fail("pick", err)
		}
	}()
}

func (m *Model) picked(p Picked) {
	m.update(func(v *View) { v.Picking = false })
	if p.ID == 0 {
		return
	}
	if _, ok := m.View().Index[p.ID]; !ok {
		m.refreshTree()
	}
	m.selectWith(p.ID, true, p.Element)
}

// --- semantics ---

func (m *Model) loadSemantics() {
	win := m.View().Window
	s, err := m.src.Semantics(m.ctx, win)
	if err != nil {
		m.fail("semantics", err)
		return
	}
	m.update(func(v *View) { v.Sem = s; v.SemRev++ })
}

// --- profile ---

// SetRecording starts or stops recording frames. Starting clears the
// frames of the last recording.
func (m *Model) SetRecording(on bool) {
	m.update(func(v *View) {
		if on && !v.Recording {
			v.Frames, v.Frame, v.FrameEls, v.FrameMore = nil, 0, nil, 0
		}
		v.Recording = on
	})
}

// ClearFrames drops the recorded frames.
func (m *Model) ClearFrames() {
	m.update(func(v *View) { v.Frames, v.Frame, v.FrameEls, v.FrameMore = nil, 0, nil, 0 })
}

func (m *Model) frames(f Frames) {
	m.mu.Lock()
	v := &m.v
	if f.Window != 0 && f.Window != v.Window && v.Window != 0 {
		m.mu.Unlock()
		return
	}
	// The tree and the selection only matter while a panel that shows them
	// is open.
	shows := m.tab == "" || m.tab == "elements" || m.tab == "a11y"
	var tree, detail bool
	for _, p := range f.Profiles {
		if p.Built > 0 {
			tree = shows
			if v.Selected != 0 && shows && slices.Contains(p.BuiltIDs, v.Selected) {
				detail = true
			}
		}
	}
	changed := false
	if v.Recording {
		fs := slices.Clip(v.Frames)
		for i := range f.Profiles {
			fs = append(fs, &f.Profiles[i])
		}
		if len(fs) > maxFrames {
			fs = slices.Clone(fs[len(fs)-maxFrames:])
		}
		v.Frames = fs
		changed = true
	}
	selected, cb := v.Selected, m.onChange
	m.mu.Unlock()
	if changed && cb != nil {
		cb()
	}
	if tree {
		m.scheduleRefresh()
	}
	if detail {
		go m.fetchDetail(selected)
	}
}

// SelectFrame selects a frame of the profile: the elements it built that
// are still mounted are fetched with their builds.
func (m *Model) SelectFrame(n int) {
	m.update(func(v *View) { v.Frame, v.FrameEls, v.FrameMore = n, nil, 0 })
	go m.loadFrame(n)
}

// loadFrame fetches the elements frame n built.
func (m *Model) loadFrame(n int) {
	m.mu.Lock()
	m.frameSeq++
	seq := m.frameSeq
	win := m.v.Window
	var ids []core.ElementID
	var more int
	for _, p := range m.v.Frames {
		if p.Frame == n {
			ids, more = p.BuiltIDs, p.BuiltMore
		}
	}
	m.mu.Unlock()
	if len(ids) == 0 {
		return
	}
	els := []core.ElementInfo{}
	seen := map[core.ElementID]bool{}
	for _, id := range ids {
		if seen[id] {
			continue // built twice in the frame: listed once
		}
		seen[id] = true
		if len(els) >= maxFrameEls {
			break
		}
		el, err := m.src.Element(m.ctx, win, id)
		if err != nil {
			if m.ctx.Err() != nil {
				return
			}
			continue // unmounted since
		}
		els = append(els, el)
	}
	m.update(func(v *View) {
		if seq != m.frameSeq || v.Frame != n {
			return
		}
		v.FrameEls = els
		v.FrameMore = more + len(ids) - len(els)
	})
}

// --- logs ---

func (m *Model) addLogs(es []LogEntry, last uint64) {
	m.update(func(v *View) {
		var add []LogEntry
		for _, e := range es {
			if e.Seq > m.logSeq {
				add = append(add, e)
				m.logSeq = e.Seq
			}
		}
		m.logSeq = max(m.logSeq, last)
		if len(add) == 0 {
			return
		}
		logs := append(slices.Clip(v.Logs), add...)
		if len(logs) > maxLogs {
			logs = slices.Clone(logs[len(logs)-maxLogs:])
		}
		v.Logs = logs
	})
}

// ClearLogs hides the entries so far.
func (m *Model) ClearLogs() { m.update(func(v *View) { v.Logs = nil }) }

// Describe is the connection line shown in the toolbar.
func (v View) Describe() string {
	switch v.Conn {
	case Ended:
		return "session ended"
	case Connecting:
		return "connecting"
	}
	return fmt.Sprintf("build %d", v.Build)
}

// Refresh fetches the tree and the semantics again.
func (m *Model) Refresh() {
	go func() {
		m.refreshTree()
		m.loadSemantics()
	}()
}

// Preload is how a model is filled without goroutines, for previews: what
// the panels show once the first calls answered, and a selection.
type Preload struct {
	// Select is the element selected, with its layout revealed in the tree.
	Select core.ElementID
	// Frames fills the profile with the source's frames, as a recording
	// that was stopped, and Frame selects one of them (0: none).
	Frames bool
	Frame  int
}

// Preload runs the first calls of Start and the selections of p in the
// calling goroutine, and leaves the model without a listener: nothing
// follows the source afterwards. Use it before the first View.
func (m *Model) Preload(p Preload) {
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()
	ctx := m.ctx
	info, _ := m.src.Info(ctx)
	ws, _ := m.src.Windows(ctx)
	m.update(func(v *View) { v.Info, v.Build = info, info.Build })
	m.setWindows(ws)
	m.refreshTree()
	if es, last, err := m.src.Logs(ctx, 0); err == nil {
		m.addLogs(es, last)
	}
	m.loadSemantics()
	if p.Select != 0 {
		m.update(func(v *View) {
			v.Selected = p.Select
			if i, ok := v.Index[p.Select]; ok {
				m.selPath = v.Paths[i]
				v.Collapsed = unfold(v, i)
				v.CollapsedRev++
				v.RevealRev++
			}
		})
		m.fetchDetail(p.Select)
	}
	if p.Frames {
		fs, _ := m.src.Profiles(ctx, m.View().Window, 0)
		m.update(func(v *View) {
			for i := range fs {
				v.Frames = append(v.Frames, &fs[i])
			}
		})
		if p.Frame != 0 {
			m.update(func(v *View) { v.Frame = p.Frame })
			m.loadFrame(p.Frame)
		}
	}
}
