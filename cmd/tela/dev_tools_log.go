package main

import (
	"bytes"
	"io"
	"log/slog"
	"regexp"
	"sync"
	"time"
)

// The log a tela dev session serves on its tools socket (ADR-299): the
// lines the children write on their standard error (source "stderr",
// where log/slog's default handler writes, so the app's own log, which the
// child cannot capture itself: see internal/core/instrument_log.go), and
// the dev warnings and errors their instrumented trees keep (source "app",
// pushed by the child while the instrumentation is on). A warning that the
// app also prints on its standard error is in both, once per source.
// Entries get the session's own sequence numbers, across reloads.

// toolsLogMax bounds the entries kept.
const toolsLogMax = 2000

// toolsLineMax bounds a line of standard error kept (the rest is cut).
const toolsLineMax = 4 << 10

// toolsLogEntry is an entry of the log.
type toolsLogEntry struct {
	Seq   uint64     `json:"seq"`
	At    time.Time  `json:"at"`
	Level slog.Level `json:"level"`
	Msg   string     `json:"msg"`
	// Source is "stderr" or "app".
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
}

// add appends entries (their Seq is set here) and reports them.
func (l *toolsLog) add(es []toolsLogEntry) {
	if len(es) == 0 {
		return
	}
	l.mu.Lock()
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
	fn := l.added
	l.mu.Unlock()
	if fn != nil {
		fn(es)
	}
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
	t.log.add(es)
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
