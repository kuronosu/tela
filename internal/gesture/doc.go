// Package gesture is Tela's gesture arena (ARCHITECTURE section 14), after
// Flutter's: the recognizers interested in a pointer sequence (a press and
// what follows until the release) compete in that pointer's arena, and one
// wins.
//
// gesture sits below core (ARCHITECTURE section 5) and knows neither
// Elements nor platform events: core feeds it pointer events of its own
// (Event) and creates the recognizers of a press, with callbacks that
// dispatch the high-level handlers (OnClick, OnDrag, OnLongPress).
//
// # The arena
//
// Arena is Flutter's GestureArenaManager. Every recognizer of a press is
// added to the pointer's arena, which closes once the press has been
// delivered to all of them. A member resolves itself: it accepts (it
// recognized its gesture: a drag past its slop, a long press past its
// timeout) or rejects (it cannot recognize it any more). The first member
// to accept after the arena closed wins and every other one is rejected;
// one that accepts while it is still open wins when it closes. When all
// members but one have rejected, the last one wins; a closed arena with a
// single member resolves to it at once. On release, Sweep gives a press
// that nothing resolved to its first member (Flutter's sweep).
//
// Manager routes the events of each pointer to its recognizers and runs
// the arena: Down adds the recognizers and closes the arena, Move and Up
// route, Up then sweeps, Cancel makes every recognizer give up.
//
// # Recognizers
//
//   - Tap feeds click. It never rejects itself because the pointer moved
//     (DOM click semantics: a press and a release over the same element
//     click, wherever the pointer went in between); it loses only when a
//     drag or long-press recognizer wins.
//   - Drag accepts when the pointer moves farther than its slop from the
//     press, and reports start, updates and end with offsets from the
//     press position and a velocity (VelocityTracker). It rejects itself
//     when the pointer is released before. It can measure the slop along
//     one axis only (a vertical scroller's pan, as Flutter's
//     VerticalDragGestureRecognizer), or accept eagerly at the press (a
//     scrollbar thumb).
//   - LongPress accepts when the pointer stays within its slop for its
//     duration (LongPressTimeout by default) and rejects itself when the
//     pointer moves farther or is released before.
//   - ClickCounter is the double tap: it counts consecutive presses close
//     in time and space (the DOM's click count, MouseEvent.detail). It does
//     not take part in the arena, so single clicks are never delayed:
//     a double click is the second click of a sequence.
//
// # Time
//
// The package has no clock and starts no goroutines: events carry their
// time (the backend clock), a recognizer waiting for a timeout reports it
// through Manager.NextDeadline, and the caller calls Manager.Tick when the
// clock reaches it. core arms a single wake-up for the earliest deadline,
// and only while a press waits for one, so an idle app uses no CPU.
//
// # Values
//
// The slops and timeouts are Flutter's for touch and the desktop
// conventions for the mouse; see the constants. Distances are in logical
// pixels. Comparisons round products explicitly, so that the same events
// give the same decisions on every architecture (ADR-041).
package gesture
