package core

import (
	"bytes"
	"io"
	"log/slog"
	"sync"
	"time"
)

// The log of an instrumented tree (ADR-299): a ring of the last maxLogs
// dev warnings and errors it reported (what dev mode prints: warnf,
// warnOnce, logf; and, for a Host's main tree, the Host's own errors),
// with their time. The messages still go where they went; the ring only
// keeps a copy. Messages come from any goroutine, so the ring has its own
// lock and the tree holds it in an atomic pointer (Tree.insLog), nil while
// the instrumentation is off.
//
// The app's own log/slog output is not captured here. Doing it in the
// process means slog.SetDefault with a handler that wraps the previous
// one, and when that one is slog's default handler it writes through the
// log package, whose output SetDefault has just pointed at the new
// handler: a loop, unless the format of every log line changes. Under tela
// dev the supervisor already reads the child's standard error, where
// slog's default handler writes, which is the place to tee it.

// maxLogs bounds the entries of the ring.
const maxLogs = 500

// LogEntry is a message of the log.
type LogEntry struct {
	// Seq numbers the entries of the tree from 1, so a reader asks for
	// the entries after the last one it saw (Logs).
	Seq   uint64     `json:"seq"`
	At    time.Time  `json:"at"`
	Level slog.Level `json:"level"`
	Msg   string     `json:"msg"`
}

// logRing is the ring of a tree's log.
type logRing struct {
	mu   sync.Mutex
	buf  []LogEntry
	next int // the oldest entry once buf is full
	seq  uint64
}

func newLogRing() *logRing { return &logRing{buf: make([]LogEntry, 0, 64)} }

func (r *logRing) add(level slog.Level, msg string) {
	at := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e := LogEntry{Seq: r.seq, At: at, Level: level, Msg: msg}
	if len(r.buf) < maxLogs {
		r.buf = append(r.buf, e)
		return
	}
	r.buf[r.next] = e
	r.next = (r.next + 1) % maxLogs
}

// noteLog keeps msg in the log of an instrumented tree. Safe from any
// goroutine.
func (t *Tree) noteLog(level slog.Level, msg string) {
	if r := t.insLog.Load(); r != nil {
		r.add(level, msg)
	}
}

// Logs returns the entries of the log whose Seq is above since (0 for
// all), oldest first: at most the last maxLogs. Nil while the
// instrumentation is off. Safe from any goroutine.
func (t *Tree) Logs(since uint64) []LogEntry {
	r := t.insLog.Load()
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []LogEntry
	add := func(es []LogEntry) {
		for _, e := range es {
			if e.Seq > since {
				out = append(out, e)
			}
		}
	}
	add(r.buf[r.next:])
	add(r.buf[:r.next])
	return out
}

// insHostLog is a Host's log: it writes to w and keeps each line in the
// log of the main tree while it is instrumented.
type insHostLog struct {
	w io.Writer
	t *Tree
}

func (l insHostLog) Write(p []byte) (int, error) {
	n, err := l.w.Write(p)
	if r := l.t.insLog.Load(); r != nil {
		for line := range bytes.SplitSeq(bytes.TrimRight(p, "\r\n"), []byte("\n")) {
			if len(line) > 0 {
				r.add(slog.LevelError, string(line))
			}
		}
	}
	return n, err
}
