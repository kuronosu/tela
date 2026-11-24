package core

import (
	"context"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// The mailbox of a texture (ADR-315): which of the submitted frames wait,
// which one is shown next, and which are dropped. The latest finished frame
// wins. It is a plain state machine: it runs nothing, waits for nothing and
// releases nothing. Its owner (Texture, under its lock) starts the waits,
// and lets go of the frames the methods return.
//
// It holds at most four frames:
//
//	waiting  at most two, whose pixels are not finished (a Fence, or the
//	         copy of mapped memory), oldest first;
//	ready    at most one, finished, for the next frame of a window;
//	shown    at most one, the last one a window took.
//
// The rules:
//
//   - A frame submitted unfinished waits. When two wait already, it
//     replaces the newer of the two: the older one is always left to
//     finish, so a producer faster than its fences never starves the
//     screen.
//   - A frame that becomes ready (submitted finished, or its wait ends)
//     replaces the ready one and the waiting ones submitted before it.
//   - A window's frame takes the ready one, which replaces the shown one.
//   - A frame whose wait fails is dropped.
//
// So the frames are shown in the order they were submitted, some of them
// missing, and never one older than another shown before it.

// texFrameState is where a frame is in its life.
type texFrameState uint8

const (
	texNew     texFrameState = iota // not submitted
	texWaiting                      // for its Fence, or the copy of its pixels
	texReady                        // for a window's frame
	texShown                        // a window took it: it is, or was, the shown one
	texDropped                      // replaced before a window took it, or its wait failed
)

// texMaxWaiting bounds the waiting frames.
const texMaxWaiting = 2

// texFrame is one frame submitted to a texture. The mailbox owns seq and
// state; the rest is the Texture's (texture.go), under its lock.
type texFrame struct {
	seq   uint64 // the submission number, from 1
	state texFrameState

	w, h   int
	opaque bool
	// img draws the frame; nil until its pixels are finished.
	img *gfx.Image
	// buf is the imported buffer of a frame made by Submit, nil for
	// SubmitImage; pix are pixels of the Texture's own, given back to its
	// spares with the frame (a copy).
	buf *TextureBuffer
	pix []byte
	// release is the application's Release, nil for none; at is when the
	// frame was submitted.
	release func()
	at      time.Time
	// refs counts who may read the frame: the mailbox, while it holds it,
	// and the windows whose lists draw it (texView). reading reports that a
	// goroutine copies the buffer's pixels, which holds it too.
	refs    int
	reading bool
	// done: the frame was let go (its Release is queued).
	done bool
	// presented: a window presented it (its age is counted once).
	presented bool
	// cancel ends the wait of an unfinished frame.
	cancel context.CancelFunc
}

// texMailbox is the mailbox of a texture.
type texMailbox struct {
	shown, ready *texFrame
	waiting      []*texFrame // oldest first
	// submitted, shownN and droppedN count the frames: submitted is the
	// sum of the other two and of those waiting or ready.
	submitted, shownN, droppedN uint64
	out                         []*texFrame // what the last call returned
}

// submit adds frame f, finished or not, and returns the frames it
// replaces (valid until the next call).
func (m *texMailbox) submit(f *texFrame, finished bool) []*texFrame {
	m.out = m.out[:0]
	m.submitted++
	f.seq = m.submitted
	if finished {
		m.becomeReady(f)
		return m.out
	}
	f.state = texWaiting
	if len(m.waiting) == texMaxWaiting {
		m.drop(m.waiting[texMaxWaiting-1])
		m.waiting = m.waiting[:texMaxWaiting-1]
	}
	m.waiting = append(m.waiting, f)
	return m.out
}

// finish makes waiting frame f ready and returns the frames it replaces
// (valid until the next call); ok is false, and nothing changes, when f
// was dropped meanwhile.
func (m *texMailbox) finish(f *texFrame) (replaced []*texFrame, ok bool) {
	m.out = m.out[:0]
	if f.state != texWaiting {
		return nil, false
	}
	m.becomeReady(f)
	return m.out, true
}

// fail drops waiting frame f, whose wait failed; it reports false when f
// was dropped meanwhile.
func (m *texMailbox) fail(f *texFrame) bool {
	m.out = m.out[:0]
	if f.state != texWaiting {
		return false
	}
	m.unwait(func(w *texFrame) bool { return w == f })
	return true
}

// take makes the ready frame the shown one, for a window's frame, and
// returns it with the one it replaces; nil when none is ready.
func (m *texMailbox) take() (f, replaced *texFrame) {
	if m.ready == nil {
		return nil, nil
	}
	f, replaced = m.ready, m.shown
	m.ready, m.shown = nil, f
	f.state = texShown
	m.shownN++
	return f, replaced
}

// close empties the mailbox. It returns the waiting and ready frames,
// which are dropped, and the shown one (nil for none), which was shown.
func (m *texMailbox) close() (dropped []*texFrame, shown *texFrame) {
	m.out = m.out[:0]
	m.unwait(func(*texFrame) bool { return true })
	if f := m.ready; f != nil {
		m.ready = nil
		m.drop(f)
	}
	shown, m.shown = m.shown, nil
	return m.out, shown
}

// pending counts the frames that wait or are ready.
func (m *texMailbox) pending() int {
	n := len(m.waiting)
	if m.ready != nil {
		n++
	}
	return n
}

// becomeReady makes f the ready frame: the ready one and the waiting ones
// submitted before f are dropped.
func (m *texMailbox) becomeReady(f *texFrame) {
	if old := m.ready; old != nil {
		m.drop(old)
	}
	f.state = texReady
	m.ready = f
	m.unwait(func(w *texFrame) bool { return w.seq <= f.seq })
}

// unwait takes the waiting frames that match out of the list; those still
// waiting (not the one that became ready) are dropped.
func (m *texMailbox) unwait(match func(*texFrame) bool) {
	keep := m.waiting[:0]
	for _, w := range m.waiting {
		switch {
		case !match(w):
			keep = append(keep, w)
		case w.state == texWaiting:
			m.drop(w)
		}
	}
	clear(m.waiting[len(keep):])
	m.waiting = keep
}

// drop counts frame f as dropped and returns it to the caller.
func (m *texMailbox) drop(f *texFrame) {
	f.state = texDropped
	m.droppedN++
	m.out = append(m.out, f)
}
