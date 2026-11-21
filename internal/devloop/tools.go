package devloop

import (
	"encoding/json"
	"time"
)

// DevTools kinds (ADR-299). The supervisor relays the calls of its tools socket that are about a
// window's tree to the active child with the kinds below, and the child
// pushes what happens there with the notifications below. Payloads are
// JSON (encoding/json): a request's payload is a [ToolsParams], and its
// reply a JSON object documented per kind, made of the types of
// internal/core's instrumentation (ElementInfo, InspectNode, FrameProfile,
// LogEntry), which the supervisor passes through to its clients without
// decoding. Every request takes a window: the ID of a window of the child
// (remote.Window.ID; 0 is its main window). An unknown window, an unknown
// element and a request while the instrumentation is off are errors of the
// request.
//
// Requests, host → child, handled on the child's UI thread:
//
//   - [KindToolsInstrument]: turn the instrumentation of every window on
//     or off (ToolsParams.On; core.Host.SetInstrument). Reply {"on": bool}.
//     The supervisor turns it on while a client of its tools socket holds
//     it, and off when the last one let go or disconnected; turned off,
//     pick mode ends and the highlight goes. While it is on, the child
//     pushes [KindToolsFrames] and [KindToolsLogs].
//   - [KindToolsElements]: the mounted elements of the window's tree, in
//     tree order (ToolsParams.Detail adds hooks, props and builds). Reply
//     {"elements": [ElementInfo...]}. With ToolsParams.Diff, the change
//     from the list of version ToolsParams.Version
//     (core.Tree.ElementsSince): reply an ElementsDiff, {"version",
//     "full", "elements", "order"}.
//   - [KindToolsElement]: one element with its details (ToolsParams.ID).
//     Reply an ElementInfo.
//   - [KindToolsInspect]: the layout of the element ToolsParams.ID, or of
//     the whole tree for 0, as tela inspect --layout gives it, down to
//     ToolsParams.Depth levels of children (the nodes of the last level
//     count the ones left out in "more"). Reply an InspectNode.
//   - [KindToolsSemantics]: the aria snapshot of the window (the YAML of
//     tela inspect) with, per line, the semantics node it prints, that
//     node's element (0 for text and /url lines) and the element's
//     creation site in the app ("settings.go:42", "" when unknown). Reply
//     {"yaml": string, "nodes": [uint64...], "elements": [uint64...],
//     "srcs": [string...]}.
//   - [KindToolsProfiles]: the frame profiles recorded after frame
//     ToolsParams.Since (0 for all, at most the last 600). Reply
//     {"profiles": [FrameProfile...]}.
//   - [KindToolsLogs]: the log entries of every window that were not
//     pushed yet (the window is ignored); they count as pushed. Reply a
//     [ToolsLogBatch] list: {"logs": [{"window", "entries"}...]}.
//   - [KindToolsPick]: pick mode on or off in the window (ToolsParams.On;
//     core.Tree.SetPick). The element picked comes as [KindToolsPicked].
//     Reply {}.
//   - [KindToolsHighlight]: highlight element ToolsParams.ID in the
//     window, 0 for none (core.Tree.Highlight). Reply {}.
//   - [KindToolsHit]: what hit testing finds at the point ToolsParams.X,
//     Y of the window (core.Tree.HitElement, as the last frame painted
//     it; pick mode plays no part), and whether the point reaches the
//     element ToolsParams.ID. Reply {"id": the element hit (0 for none),
//     "node": its semantics node or its nearest ancestor's, "within":
//     bool}.
//   - [KindToolsReveal]: scroll element ToolsParams.ID into view
//     (core.Tree.RevealElement). Reply {"visible": the part of its box
//     visible once the scroll ends, as a rect, absent when none is;
//     "moved": bool, a scroll is under way: wait for the frames and ask
//     again}.
//
// Request, child → host, once, before the first build:
//
//   - [KindToolsStart]: whether a client of the tools socket holds the
//     instrumentation, so that the child starts with it on (a reload's
//     new child, whose first builds then record their creation sites,
//     reasons and profiles). Payload empty. Reply {"on": bool}. A host
//     without a handler, or that fails, means off.
//
// Notifications, child → host:
//
//   - [KindToolsPicked]: a click picked an element in pick mode, or Escape
//     ended it (ID 0). Payload {"window", "id", "element": ElementInfo
//     with details, absent for 0}.
//   - [KindToolsFrames]: the frames recorded since the last such
//     notification of the window, batched: at most one per window every
//     [ToolsBatch]. Payload {"window", "profiles": [FrameProfile...]}.
//   - [KindToolsLogs]: the log entries of a window since the last ones
//     sent, batched like the frames. Payload a ToolsLogBatch.
const (
	KindToolsInstrument = "tela.dev.tools.instrument"
	KindToolsElements   = "tela.dev.tools.elements"
	KindToolsElement    = "tela.dev.tools.element"
	KindToolsInspect    = "tela.dev.tools.inspect"
	KindToolsSemantics  = "tela.dev.tools.semantics"
	KindToolsProfiles   = "tela.dev.tools.profiles"
	KindToolsLogs       = "tela.dev.tools.logs"
	KindToolsPick       = "tela.dev.tools.pick"
	KindToolsHighlight  = "tela.dev.tools.highlight"
	KindToolsHit        = "tela.dev.tools.hit"
	KindToolsReveal     = "tela.dev.tools.reveal"
	KindToolsStart      = "tela.dev.tools.start"
	KindToolsPicked     = "tela.dev.tools.picked"
	KindToolsFrames     = "tela.dev.tools.frames"
)

// ToolsBatch bounds how often the child pushes frames and logs.
const ToolsBatch = 100 * time.Millisecond

// ToolsParams is the payload of the tools requests; each kind reads the
// fields it needs.
type ToolsParams struct {
	// Window is the child's window ID; 0 is its main window.
	Window uint32 `json:"window,omitempty"`
	// ID is an element of the window's tree.
	ID uint64 `json:"id,omitempty"`
	// Detail asks for the details of every element.
	Detail bool `json:"detail,omitempty"`
	// Since is the last frame the caller has.
	Since int `json:"since,omitempty"`
	// On turns the instrumentation or pick mode on.
	On bool `json:"on,omitempty"`
	// X and Y are a point of the window in logical pixels.
	X float32 `json:"x,omitempty"`
	Y float32 `json:"y,omitempty"`
	// Diff asks KindToolsElements for the change from the list of
	// version Version (0: none) instead of the list.
	Diff    bool   `json:"diff,omitempty"`
	Version uint64 `json:"version,omitempty"`
	// Depth bounds the levels of children of KindToolsInspect (0: the box
	// alone); nil for all.
	Depth *int `json:"depth,omitempty"`
}

// ToolsLogBatch is the log entries of one window (the JSON of
// core.LogEntry: seq, at, level, msg).
type ToolsLogBatch struct {
	Window  uint32            `json:"window"`
	Entries []json.RawMessage `json:"entries"`
}
