// Package devloop holds what the tela dev supervisor (cmd/tela) and the
// child runtime (the //go:build teladev files of tela and internal/core)
// agree on: environment variables, the kinds of control messages they
// exchange over the remote backend's control channel
// (internal/platform/remote: Child.Request/Notify on the host,
// Backend.Request/Notify/Handle on the child), and the encodings of their
// payloads (ADR-111, ADR-115).
//
// Dev-loop code only: nothing in a release build imports this package. It
// imports only the standard library.
//
// # Environment
//
// The supervisor starts each child (a build of the app with -tags teladev)
// with these variables, besides the remote backend's own
// (remote.EnvAddr, TELA_REMOTE: the host's socket; remote.EnvTag,
// TELA_REMOTE_TAG: the build generation, "1", "2", ...):
//
//   - [EnvChild] (TELA_DEVLOOP=1): the process runs under tela dev. The
//     child runtime then uses remote.DialEnv as its backend instead of
//     platform/auto, whatever TELA_BACKEND says (the supervisor's own
//     TELA_BACKEND is inherited and chose the real window's backend).
//   - [EnvNoState] (TELA_DEVLOOP_NO_STATE=1, only with --no-state): the
//     child neither captures nor restores state. It sends no
//     [KindStateLoad] request (if it does, the reply is empty) and no
//     [KindStateReport]; the supervisor never sends [KindStateSave].
//   - TELA_DEV=1 unless the user's environment sets TELA_DEV: dev mode is
//     on even if the build flags make a binary look like a release build
//     (-trimpath, ADR-085).
//
// --size, --scale and --dark need nothing from the child: the supervisor
// creates the real window at the forced size, makes the child render at
// the forced scale, and forces DarkMode in the system settings the child
// receives (the welcome and every SettingsEvent), so the child's
// Settings().DarkMode is true and the app's own App.ColorScheme still wins.
//
// # Control messages
//
// Every kind starts with "tela.dev.". Requests get one reply (bytes or an
// error); notifications get none. A child with no handler for a host
// request answers with remote.ErrNoHandler, which the supervisor treats as
// "not supported".
//
//   - [KindStateSave], request, host → child. Payload: empty. Reply: the
//     child's state as opaque bytes (the supervisor does not look inside;
//     encoding/gob or anything else), or an error. Sent to the active
//     child right before a swap, with input frozen; the supervisor waits
//     [SaveTimeout] and proceeds without state on an error or a timeout.
//     The child keeps running normally afterwards: the swap may fail (the
//     new build crashes before its first frame) and the old child stays.
//   - [KindStateLoad], request, child → host. Payload: empty. Reply: the
//     bytes a [KindStateSave] returned to seed this child with, or empty
//     for a fresh start (the first child, after a crash, with R or
//     --no-state). The child sends it at startup, before mounting its
//     root, and only once; the host answers at once.
//   - [KindStateReport], notification, child → host, optional. Payload:
//     [StateReport.Encode]. Sent after the child restored state (before
//     its first Present), so the supervisor can print "state: 12 kept, 1
//     reset".
//   - [KindPanic], notification, child → host. Payload: UTF-8 text built
//     by [PanicPayload]: the line "panic: <value>" (the value formatted
//     with %v; it may span several lines) followed by a blank line and the
//     stack of the panicking goroutine. Sent with Backend.Notify
//     (which returns once it is written) right before the child exits
//     with a non-zero status. The child also prints the panic on its
//     standard error as Go would (the supervisor forwards that output and
//     prints only [PanicSummary] itself).
//   - [KindAssets], notification, child → host. Payload: [EncodeAssets] of
//     the absolute paths of every directory registered with tela.Assets as
//     an os.DirFS (directories the app reads at run time; the supervisor
//     rebuilds and reloads when a file under one changes). Each
//     notification carries the complete current list, which replaces the
//     previous one; send it again when the list grows.
//   - [KindQuit], notification, host → child. Payload: empty. The
//     supervisor stops a child that was replaced: the child should quit
//     its event loop and exit with status 0 promptly, without saving
//     anything. After [StopTimeout] the supervisor closes the connection
//     and kills the process. A child that exits with status 0 on its own
//     (the user closed the window) makes tela dev quit; one that exits
//     because of [KindQuit] does not.
package devloop

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Environment variables the supervisor sets for its children.
const (
	// EnvChild is "1" in a process started by tela dev.
	EnvChild = "TELA_DEVLOOP"
	// EnvNoState is "1" when tela dev runs with --no-state.
	EnvNoState = "TELA_DEVLOOP_NO_STATE"
)

// Control message kinds; see the package documentation.
const (
	KindStateSave   = "tela.dev.state.save"
	KindStateLoad   = "tela.dev.state.load"
	KindStateReport = "tela.dev.state.report"
	KindPanic       = "tela.dev.panic"
	KindAssets      = "tela.dev.assets"
	KindQuit        = "tela.dev.quit"
)

// Timeouts of the supervisor that the child runtime should respect.
const (
	// SaveTimeout bounds the reply to a KindStateSave request.
	SaveTimeout = time.Second
	// FirstFrameTimeout bounds how long a new child may take from its
	// start to its first Present before the swap is abandoned.
	FirstFrameTimeout = 15 * time.Second
	// StopTimeout is how long a child has to exit after KindQuit.
	StopTimeout = time.Second
)

// StateReport is the payload of KindStateReport: how many hook slots (and
// engine states) the child restored from the saved state, and how many it
// had to reset (a component whose hooks changed, a value that could not be
// decoded).
type StateReport struct {
	Kept, Reset int
}

// Encode returns the payload: two unsigned varints, Kept and Reset.
func (r StateReport) Encode() []byte {
	b := binary.AppendUvarint(nil, uint64(max(r.Kept, 0)))
	return binary.AppendUvarint(b, uint64(max(r.Reset, 0)))
}

// DecodeStateReport decodes a KindStateReport payload. Bytes after the two
// varints are ignored, so the report can grow.
func DecodeStateReport(p []byte) (StateReport, error) {
	kept, n := binary.Uvarint(p)
	if n <= 0 {
		return StateReport{}, errors.New("devloop: bad state report")
	}
	reset, m := binary.Uvarint(p[n:])
	if m <= 0 {
		return StateReport{}, errors.New("devloop: bad state report")
	}
	const limit = 1 << 31
	if kept >= limit || reset >= limit {
		return StateReport{}, errors.New("devloop: state report out of range")
	}
	return StateReport{Kept: int(kept), Reset: int(reset)}, nil
}

// EncodeAssets returns the payload of KindAssets: the paths separated by
// NUL bytes (a byte no path contains).
func EncodeAssets(dirs []string) []byte {
	return []byte(strings.Join(dirs, "\x00"))
}

// DecodeAssets decodes a KindAssets payload. Empty paths are dropped.
func DecodeAssets(p []byte) []string {
	var dirs []string
	for _, d := range bytes.Split(p, []byte{0}) {
		if len(d) > 0 {
			dirs = append(dirs, string(d))
		}
	}
	return dirs
}

// PanicPayload returns the payload of KindPanic for the recovered value v
// and the stack of the panicking goroutine (runtime/debug.Stack).
func PanicPayload(v any, stack []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "panic: %v\n\n", v)
	b.Write(bytes.TrimRight(stack, "\n"))
	return b.Bytes()
}

// PanicSummary returns the first line of a KindPanic payload.
func PanicSummary(p []byte) string {
	s, _, _ := strings.Cut(string(p), "\n")
	return strings.TrimSpace(s)
}
