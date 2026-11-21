package main

import (
	"bytes"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/devloop"
)

// The log a tela dev session serves on its tools socket (ADR-299): the
// lines the children write on their standard error (source "stderr",
// where log/slog's default handler writes, so the app's own log, which the
// child cannot capture itself: see internal/core/instrument_log.go), the
// dev warnings and errors their instrumented trees keep (source "app",
// pushed by the child while the instrumentation is on), and the
// supervisor's own lines (source "tela dev": building, build failed,
// reloaded). Entries get the session's own sequence numbers, across
// reloads.
//
// A dev warning is printed on the child's standard error and kept by its
// tree, so it reaches the supervisor twice, through the pipe and through
// the control channel, in either order. The "app" entry is the one kept
// (it has the level and the window): a line of standard error waits
// toolsLogGrace before it is added, and is dropped when an "app" entry of
// the same build has that line by then; an "app" line not seen on
// standard error yet is remembered as long, so that a later twin there is
// dropped too. A line that waited may come after entries that arrived
// after it (its sequence number is given when it is added); a logs call
// adds what waits at once, after the child's entries not pushed yet.

// toolsLogMax bounds the entries kept.
const toolsLogMax = 2000

// toolsLineMax bounds a line of standard error kept (the rest is cut).
const toolsLineMax = 4 << 10

// toolsLogGrace is how long a line of standard error waits for its twin
// among the "app" entries, and an "app" line for its twin on standard
// error. The child pushes its entries at most every devloop.ToolsBatch
// after they were logged, so three batches leave room for a busy machine;
// past it both are kept.
const toolsLogGrace = 3 * devloop.ToolsBatch

// toolsLogEntry is an entry of the log.
type toolsLogEntry struct {
	Seq   uint64     `json:"seq"`
	At    time.Time  `json:"at"`
	Level slog.Level `json:"level"`
	Msg   string     `json:"msg"`
	// Source is "stderr", "app" or "tela dev".
	Source string `json:"source"`
	// Window is the window of an "app" entry.
	Window uint32 `json:"window,omitempty"`
	// Build is the build generation of the child that wrote it.
	Build int `json:"build,omitempty"`
}

// toolsLog is the ring of entries; safe from any goroutine.
type toolsLog struct {
	mu   sync.Mutex
	buf  []toolsLogEntry
	next int // the oldest entry once buf is full
	seq  uint64
	// added is called with the entries just added, outside the lock.
	added func([]toolsLogEntry)

	// held are the lines of standard error waiting for their twin, in
	// order; apps the lines of "app" entries whose twin has not come.
	// timer adds the held lines whose wait ended.
	held  []twinLine
	apps  []twinLine
	timer *time.Timer
}

// twinLine is a line waiting for its twin until due (for an "app" line,
// only the entry's Build and Msg are set).
type twinLine struct {
	e   toolsLogEntry
	due time.Time
}

// add appends entries (their Seq is set here) and reports them.
func (l *toolsLog) add(es []toolsLogEntry) {
	if len(es) == 0 {
		return
	}
	l.mu.Lock()
	l.commitLocked(es)
	fn := l.added
	l.mu.Unlock()
	if fn != nil {
		fn(es)
	}
}

// commitLocked gives es their sequence numbers and keeps them. l.mu is
// held.
func (l *toolsLog) commitLocked(es []toolsLogEntry) {
	for i := range es {
		l.seq++
		es[i].Seq = l.seq
		if len(l.buf) < toolsLogMax {
			l.buf = append(l.buf, es[i])
		} else {
			l.buf[l.next] = es[i]
			l.next = (l.next + 1) % toolsLogMax
		}
	}
}

// addStderr adds lines of a child's standard error: each waits for its
// twin among the "app" entries, or is dropped when that came first.
func (l *toolsLog) addStderr(es []toolsLogEntry) {
	if len(es) == 0 {
		return
	}
	now := time.Now()
	l.mu.Lock()
	l.pruneLocked(now)
	for _, e := range es {
		if i := slices.IndexFunc(l.apps, func(h twinLine) bool { return h.e.Build == e.Build && h.e.Msg == e.Msg }); i >= 0 {
			l.apps = slices.Delete(l.apps, i, i+1)
			continue
		}
		l.held = append(l.held, twinLine{e, now.Add(toolsLogGrace)})
	}
	l.armLocked(now)
	l.mu.Unlock()
}

// addApp adds the "app" entries of a child's trees, dropping the lines of
// standard error that are their twins (a message of several lines has a
// twin per line).
func (l *toolsLog) addApp(es []toolsLogEntry) {
	if len(es) == 0 {
		return
	}
	now := time.Now()
	l.mu.Lock()
	l.pruneLocked(now)
	for _, e := range es {
		for line := range strings.SplitSeq(e.Msg, "\n") {
			line = strings.TrimRight(line, "\r")
			if line == "" {
				continue
			}
			if i := slices.IndexFunc(l.held, func(h twinLine) bool { return h.e.Build == e.Build && h.e.Msg == line }); i >= 0 {
				l.held = slices.Delete(l.held, i, i+1)
				continue
			}
			l.apps = append(l.apps, twinLine{toolsLogEntry{Build: e.Build, Msg: line}, now.Add(toolsLogGrace)})
		}
	}
	l.commitLocked(es)
	fn := l.added
	l.mu.Unlock()
	if fn != nil {
		fn(es)
	}
}

// flush adds the lines of standard error that wait, at once.
func (l *toolsLog) flush() { l.release(time.Time{}) }

// release adds the held lines due by now (all of them for the zero time)
// and forgets the "app" lines whose wait ended.
func (l *toolsLog) release(now time.Time) {
	l.mu.Lock()
	var out []toolsLogEntry
	keep := l.held[:0]
	for _, h := range l.held {
		if now.IsZero() || !h.due.After(now) {
			out = append(out, h.e)
		} else {
			keep = append(keep, h)
		}
	}
	clear(l.held[len(keep):])
	l.held = keep
	if now.IsZero() {
		now = time.Now()
	}
	l.pruneLocked(now)
	l.commitLocked(out)
	l.armLocked(now)
	fn := l.added
	l.mu.Unlock()
	if fn != nil && len(out) > 0 {
		fn(out)
	}
}

// pruneLocked forgets the "app" lines whose wait ended. l.mu is held.
func (l *toolsLog) pruneLocked(now time.Time) {
	l.apps = slices.DeleteFunc(l.apps, func(h twinLine) bool { return !h.due.After(now) })
}

// armLocked starts the timer of the first held line when none runs. l.mu
// is held.
func (l *toolsLog) armLocked(now time.Time) {
	if len(l.held) == 0 || l.timer != nil {
		return
	}
	l.timer = time.AfterFunc(l.held[0].due.Sub(now), func() {
		l.mu.Lock()
		l.timer = nil
		l.mu.Unlock()
		l.release(time.Now())
	})
}

// since returns the entries whose Seq is above since, oldest first, and
// the last Seq given.
func (l *toolsLog) since(since uint64) ([]toolsLogEntry, uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []toolsLogEntry{}
	for _, part := range [][]toolsLogEntry{l.buf[l.next:], l.buf[:l.next]} {
		for _, e := range part {
			if e.Seq > since {
				out = append(out, e)
			}
		}
	}
	return out, l.seq
}

// stderr returns the writer of a child's standard error: it writes to w
// (nil: nowhere) and keeps each line in the log.
func (l *toolsLog) stderr(build int, w io.Writer) io.Writer {
	return &stderrTee{log: l, build: build, w: w}
}

// stderrTee keeps the lines written to it; the copy of one pipe writes
// to it, so it needs no lock.
type stderrTee struct {
	log     *toolsLog
	build   int
	w       io.Writer
	partial []byte
}

func (t *stderrTee) Write(p []byte) (int, error) {
	var es []toolsLogEntry
	now := time.Now()
	rest := p
	for len(rest) > 0 {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			if room := toolsLineMax - len(t.partial); room > 0 {
				t.partial = append(t.partial, rest[:min(room, len(rest))]...)
			}
			break
		}
		line := rest[:i]
		rest = rest[i+1:]
		if len(t.partial) > 0 {
			line = append(t.partial, line[:min(len(line), max(toolsLineMax-len(t.partial), 0))]...)
			t.partial = nil
		} else if len(line) > toolsLineMax {
			line = line[:toolsLineMax]
		}
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 {
			continue
		}
		msg := string(line)
		es = append(es, toolsLogEntry{At: now, Level: stderrLevel(msg), Msg: msg, Source: "stderr", Build: t.build})
	}
	t.log.addStderr(es)
	if t.w == nil {
		return len(p), nil
	}
	return t.w.Write(p)
}

// slogLevel finds the level of a line of log/slog's default handler
// ("2026/11/20 10:04:05 WARN msg") or of its text handler ("time=...
// level=WARN msg=...").
var slogLevel = regexp.MustCompile(`^(?:\d{4}/\d\d/\d\d \d\d:\d\d:\d\d(?:\.\d+)? (DEBUG|INFO|WARN|ERROR)[ +-]|(?:time=\S+ )?level=(DEBUG|INFO|WARN|ERROR)[ +-])`)

// stderrLevel is the level of a line of standard error: the one slog
// printed, Error for a panic or a fatal error, else Info.
func stderrLevel(line string) slog.Level {
	if m := slogLevel.FindStringSubmatch(line); m != nil {
		name := m[1] + m[2]
		var lv slog.Level
		if lv.UnmarshalText([]byte(name)) == nil {
			return lv
		}
	}
	if bytes.HasPrefix([]byte(line), []byte("panic: ")) || bytes.HasPrefix([]byte(line), []byte("fatal error: ")) {
		return slog.LevelError
	}
	return slog.LevelInfo
}
