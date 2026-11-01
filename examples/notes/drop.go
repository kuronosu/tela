package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
)

// Dropping files and text on the main window (drag and drop, ADR-209 and
// ADR-211). The whole window is the drop target: files become notes as
// Open makes them, and text becomes a note of its own. While a drag that
// carries either is over the window, the notes pane shows an overlay that
// says so; any other drag (an image, a custom type) gets no overlay and is
// refused (DropNone).

// acceptsDrag reports whether the drag carries what Notes imports: files or
// text. Before the drop only the types are known, so a file that Notes will
// not open (a photo) is accepted here and reported when it is dropped.
func acceptsDrag(dt *tela.DataTransfer) bool {
	return dt.Has("Files") || dt.Has("text/plain")
}

// dropState is what useDrop returns.
type dropState struct {
	// Over is true while an acceptable drag is over the window.
	Over bool
	// Props go on the element that is the drop target.
	Props tela.Prop
}

// useDrop makes the element that gets Props a drop target that calls onDrop
// with the data of an acceptable drag. HTML's enter and leave events
// bubble and come in pairs as the drag moves between the element's
// children (the new one's enter before the old one's leave), so Over counts
// them: enter adds one, leave takes one off, and it is over while the count
// is above zero (see tela.DragEvent).
func useDrop(onDrop func(*tela.DataTransfer)) dropState {
	depth, setDepth := tela.UseState(0)
	return dropState{
		Over: depth > 0,
		Props: tela.MergeProps(
			OnDragEnter(func(e tela.DragEvent) {
				if acceptsDrag(e.DataTransfer) {
					setDepth.Update(func(n int) int { return n + 1 })
				}
			}),
			OnDragOver(func(e tela.DragEvent) {
				if acceptsDrag(e.DataTransfer) {
					e.PreventDefault() // accept: the drop comes here
				}
			}),
			OnDragLeave(func(e tela.DragEvent) {
				if acceptsDrag(e.DataTransfer) {
					setDepth.Update(func(n int) int { return max(n-1, 0) })
				}
			}),
			OnDrop(func(e tela.DragEvent) {
				setDepth.Set(0)
				onDrop(e.DataTransfer)
			}),
		),
	}
}

// dropOverlay covers the notes pane while a drag that Notes takes is over
// the window: a dashed ring in the accent color, and what dropping does.
// It lets the pointer through, so it does not take the drag from the
// elements under it.
type dropOverlay struct{}

func (dropOverlay) Build() tela.Node {
	return Div(Role("status"), AriaLabel("Drop to import"),
		Class("pointer-events-none absolute inset-3 z-10 flex flex-col items-center justify-center gap-4",
			"rounded-2xl border-2 border-dashed border-gold/70 bg-background/95"),
		Span(Class("flex size-12 items-center justify-center rounded-full border border-gold/30 bg-card text-gold"),
			Icon(icons.Download, Class("size-5"), AriaHidden(true))),
		Div(Class("flex flex-col items-center gap-1"),
			Span(Class("text-lg font-semibold tracking-tight"), Text("Drop to import")),
			Span(Class("text-[13px] text-muted-foreground"), Text("Text and Markdown files, or text, become new notes.")),
		),
	)
}
