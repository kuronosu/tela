//go:build teladev

package core

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kuronosu/tela/internal/devloop"
	"github.com/kuronosu/tela/internal/platform/remote"
	"github.com/kuronosu/tela/internal/semantics"
)

// The DevTools side of the child runtime (ADR-299): the handlers of the
// tela.dev.tools.* requests the supervisor relays from its tools socket,
// over the instrumentation of the windows' trees (instrument.go), and the
// pushes of what the instrumentation records: the elements picked, and the
// frame profiles and log entries, batched (devloop.ToolsBatch). The kinds
// and their JSON payloads are devloop's.
//
// Everything runs on the UI thread: the handlers (remote.Backend.Handle),
// the hook after each frame (devToolsFrame) and the flush that a timer
// posts to the main tree.

// devTools is the DevTools state of the child; UI thread only.
var devTools struct {
	h  *Host
	rb *remote.Backend
	on bool
	// frames and logs are the last frame and log entry pushed per tree.
	frames map[*Tree]int
	logs   map[*Tree]uint64
	// armed: a flush is due (its timer runs); last is when the last one
	// ran.
	armed bool
	last  time.Time
	// gen counts the times the instrumentation turned on, so a flush
	// armed before it turned off and on again does nothing.
	gen int
}

// devToolsRegister registers the tools handlers on the connection to the
// supervisor.
func devToolsRegister(rb *remote.Backend, h *Host) {
	devTools.h, devTools.rb = h, rb
	handle := func(kind string, needOn bool, fn func(h *Host, t *Tree, p devloop.ToolsParams) (any, error)) {
		rb.Handle(kind, func(payload []byte) ([]byte, error) {
			var p devloop.ToolsParams
			if len(payload) > 0 {
				if err := json.Unmarshal(payload, &p); err != nil {
					return nil, fmt.Errorf("bad parameters: %v", err)
				}
			}
			if needOn && !devTools.on {
				return nil, fmt.Errorf("the instrumentation is off")
			}
			var t *Tree
			if kind != devloop.KindToolsInstrument && kind != devloop.KindToolsLogs {
				var err error
				if t, err = devToolsTree(h, p.Window); err != nil {
					return nil, err
				}
			}
			res, err := fn(h, t, p)
			if err != nil {
				return nil, err
			}
			return json.Marshal(res)
		})
	}
	handle(devloop.KindToolsInstrument, false, devToolsInstrument)
	handle(devloop.KindToolsElements, true, func(_ *Host, t *Tree, p devloop.ToolsParams) (any, error) {
		els := t.Elements(p.Detail)
		if els == nil {
			els = []ElementInfo{}
		}
		return map[string]any{"elements": els}, nil
	})
	handle(devloop.KindToolsElement, true, func(_ *Host, t *Tree, p devloop.ToolsParams) (any, error) {
		info, ok := t.Element(ElementID(p.ID))
		if !ok {
			return nil, devToolsNoElement(p)
		}
		return info, nil
	})
	handle(devloop.KindToolsInspect, true, func(_ *Host, t *Tree, p devloop.ToolsParams) (any, error) {
		if p.ID == 0 {
			return t.Inspect(), nil
		}
		if _, ok := t.Element(ElementID(p.ID)); !ok {
			return nil, devToolsNoElement(p)
		}
		return t.InspectElementID(ElementID(p.ID))
	})
	handle(devloop.KindToolsSemantics, true, func(_ *Host, t *Tree, _ devloop.ToolsParams) (any, error) {
		yaml, ids := semantics.SnapshotIDs(t.Semantics())
		nodes := make([]uint64, len(ids))
		els := make([]uint64, len(ids))
		srcs := make([]string, len(ids))
		for i, id := range ids {
			nodes[i] = uint64(id)
			if id != 0 {
				el := t.ElementOfNode(id)
				els[i] = uint64(el)
				srcs[i] = t.AppSite(el).Rel() // "settings.go:42", as the app's working directory shows it
			}
		}
		return map[string]any{"yaml": yaml, "nodes": nodes, "elements": els, "srcs": srcs}, nil
	})
	handle(devloop.KindToolsProfiles, true, func(_ *Host, t *Tree, p devloop.ToolsParams) (any, error) {
		ps := t.FrameProfiles(p.Since)
		if ps == nil {
			ps = []FrameProfile{}
		}
		return map[string]any{"profiles": ps}, nil
	})
	handle(devloop.KindToolsLogs, true, func(h *Host, _ *Tree, _ devloop.ToolsParams) (any, error) {
		return map[string]any{"logs": devToolsNewLogs(h)}, nil
	})
	handle(devloop.KindToolsPick, true, func(_ *Host, t *Tree, p devloop.ToolsParams) (any, error) {
		if !p.On {
			t.SetPick(false, nil)
			return struct{}{}, nil
		}
		win := devToolsWindowID(t)
		t.SetPick(true, func(id ElementID) {
			msg := map[string]any{"window": win, "id": id}
			if info, ok := t.Element(id); ok && id != 0 {
				msg["element"] = info
			}
			devToolsNotify(devloop.KindToolsPicked, msg)
		})
		return struct{}{}, nil
	})
	handle(devloop.KindToolsHighlight, true, func(_ *Host, t *Tree, p devloop.ToolsParams) (any, error) {
		if p.ID != 0 {
			if _, ok := t.Element(ElementID(p.ID)); !ok {
				return nil, devToolsNoElement(p)
			}
		}
		t.Highlight(ElementID(p.ID))
		return struct{}{}, nil
	})
}

func devToolsNoElement(p devloop.ToolsParams) error {
	return fmt.Errorf("no element %d in window %d (it unmounted, or the IDs started again with a reload)", p.ID, p.Window)
}

// devToolsInstrument turns the instrumentation of every window on or off.
func devToolsInstrument(h *Host, _ *Tree, p devloop.ToolsParams) (any, error) {
	if p.On != devTools.on {
		h.SetInstrument(p.On)
		devTools.on = p.On
		devTools.gen++
		devTools.frames = map[*Tree]int{}
		devTools.logs = map[*Tree]uint64{}
		devTools.armed = false
	}
	return map[string]any{"on": devTools.on}, nil
}

// devToolsTree returns the tree of the window with remote ID id, the main
// window's for 0.
func devToolsTree(h *Host, id uint32) (*Tree, error) {
	if id == 0 {
		return h.tree, nil
	}
	for _, v := range h.views {
		if rw, ok := v.win.(*remote.Window); ok && rw.ID() == id {
			return v.tree, nil
		}
	}
	return nil, fmt.Errorf("no window %d", id)
}

// devToolsWindowID returns the remote ID of the window of tree t (0 if it
// has none).
func devToolsWindowID(t *Tree) uint32 {
	for _, v := range devTools.h.views {
		if v.tree == t {
			if rw, ok := v.win.(*remote.Window); ok {
				return rw.ID()
			}
		}
	}
	return 0
}

// devToolsNotify pushes a notification to the supervisor.
func devToolsNotify(kind string, msg any) {
	b, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tela dev: %s: %v\n", kind, err)
		return
	}
	devTools.rb.Notify(kind, b)
}

// devToolsFrame runs after each frame of an instrumented window: the
// frames recorded and the log entries are pushed at most every
// devloop.ToolsBatch, from a flush posted to the main tree.
func devToolsFrame(h *Host) {
	if !devTools.on || devTools.armed || devTools.rb == nil {
		return
	}
	devTools.armed = true
	gen := devTools.gen
	delay := max(devloop.ToolsBatch-time.Since(devTools.last), 0)
	time.AfterFunc(delay, func() {
		h.tree.enqueue(update{post: func() {
			if devTools.gen == gen && devTools.armed {
				devToolsFlush(h)
			}
		}})
	})
}

// devToolsFlush pushes the frames recorded and the log entries since the
// last push.
func devToolsFlush(h *Host) {
	devTools.armed = false
	devTools.last = time.Now()
	for _, v := range h.views {
		t := v.tree
		ps := t.FrameProfiles(devTools.frames[t])
		if len(ps) == 0 {
			continue
		}
		devTools.frames[t] = ps[len(ps)-1].Frame
		devToolsNotify(devloop.KindToolsFrames, map[string]any{"window": devToolsWindowID(t), "profiles": ps})
	}
	for _, b := range devToolsNewLogs(h) {
		devToolsNotify(devloop.KindToolsLogs, b)
	}
}

// devToolsNewLogs returns the log entries of every window not pushed yet,
// and counts them as pushed.
func devToolsNewLogs(h *Host) []devloop.ToolsLogBatch {
	out := []devloop.ToolsLogBatch{}
	for _, v := range h.views {
		t := v.tree
		es := t.Logs(devTools.logs[t])
		if len(es) == 0 {
			continue
		}
		devTools.logs[t] = es[len(es)-1].Seq
		b := devloop.ToolsLogBatch{Window: devToolsWindowID(t)}
		for _, e := range es {
			raw, err := json.Marshal(e)
			if err == nil {
				b.Entries = append(b.Entries, raw)
			}
		}
		out = append(out, b)
	}
	return out
}
