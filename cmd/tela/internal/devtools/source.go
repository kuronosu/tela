// Package devtools is the UI of tela devtools (ADR-299): a Tela app, built
// with kit, that shows the running app of a tela dev session, its element
// tree, accessibility tree, frame profile and log.
//
// The package knows no transport. It reads a [Source], the calls of the
// session's tools socket as Go methods, which cmd/tela implements over its
// client and the tests implement with [FakeSource]. A [Model] holds what the
// panels show and fetches it on goroutines, so the UI thread never waits for
// the session.
package devtools

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/kuronosu/tela/internal/core"
)

// Source is a tela dev session as the DevTools see it: the methods of its
// tools socket (cmd/tela/dev_tools.go). Calls are safe from any goroutine.
// window is a window ID of Windows; 0 is the app's first window.
type Source interface {
	Info(ctx context.Context) (Info, error)
	Windows(ctx context.Context) ([]Window, error)
	// Subscribe and Unsubscribe choose the notifications of Notes: picked,
	// frames, logs, reload and windows.
	Subscribe(ctx context.Context, events ...string) error
	Unsubscribe(ctx context.Context, events ...string) error
	Elements(ctx context.Context, window uint32, detail bool) ([]core.ElementInfo, error)
	Element(ctx context.Context, window uint32, id core.ElementID) (core.ElementInfo, error)
	Inspect(ctx context.Context, window uint32, id core.ElementID) (core.InspectNode, error)
	Semantics(ctx context.Context, window uint32) (Semantics, error)
	Profiles(ctx context.Context, window uint32, since int) ([]core.FrameProfile, error)
	Logs(ctx context.Context, since uint64) ([]LogEntry, uint64, error)
	Pick(ctx context.Context, window uint32, on bool) error
	Highlight(ctx context.Context, window uint32, id core.ElementID) error
	// Notes delivers the notifications in order; it is closed after Done.
	Notes() <-chan Note
	// Done is closed when the session ended.
	Done() <-chan struct{}
	Close() error
}

// Info is what the session says about itself.
type Info struct {
	Pkg      string    `json:"pkg"`
	Dir      string    `json:"dir,omitempty"`
	PID      int       `json:"pid"`
	Started  time.Time `json:"started"`
	Build    int       `json:"build"`
	Protocol int       `json:"protocol"`
}

// Window is a window of the app.
type Window struct {
	ID      uint32  `json:"id"`
	Title   string  `json:"title"`
	Width   float32 `json:"width"`
	Height  float32 `json:"height"`
	Scale   float64 `json:"scale"`
	Focused bool    `json:"focused"`
	Visible bool    `json:"visible"`
	// Active: the running app shows it.
	Active bool `json:"active"`
}

// Semantics is the aria snapshot of a window with, per line, its node and
// its element (0 for text and /url lines).
type Semantics struct {
	YAML     string           `json:"yaml"`
	Nodes    []uint64         `json:"nodes"`
	Elements []core.ElementID `json:"elements"`
}

// LogEntry is an entry of the session's log. Source is "stderr" (what the
// app printed), "app" (the dev warnings and errors of its trees) or "tela
// dev" (the session's own lines: building, a failed build).
type LogEntry struct {
	Seq    uint64     `json:"seq"`
	At     time.Time  `json:"at"`
	Level  slog.Level `json:"level"`
	Msg    string     `json:"msg"`
	Source string     `json:"source"`
	Window uint32     `json:"window,omitempty"`
	Build  int        `json:"build,omitempty"`
}

// Note is a notification: its event and parameters.
type Note struct {
	Method string
	Params json.RawMessage
}

// Picked is the parameters of a picked notification: the element a click
// picked in pick mode, or ID 0 when Escape ended pick mode.
type Picked struct {
	Window  uint32            `json:"window"`
	ID      core.ElementID    `json:"id"`
	Element *core.ElementInfo `json:"element"`
}

// Frames is the parameters of a frames notification.
type Frames struct {
	Window   uint32              `json:"window"`
	Profiles []core.FrameProfile `json:"profiles"`
}

// Reload is the parameters of a reload notification.
type Reload struct {
	Build int `json:"build"`
}
