package main

import "github.com/kuronosu/tela"

// Offset is how far the dragged card is from where it started, in pixels.
type Offset struct{ X, Y float32 }

// DragState is the state of the drag in progress, made of Values so that
// each piece costs only what reads it:
//
//   - Offset changes on every pointer move and is read only inside the
//     TranslateFn of the dragged card: a move repaints that card and
//     builds nothing.
//   - Dragging (the card's ID, 0 when idle) and Over (where the card would
//     land) are read in component bodies: the board's columns and the cards
//     whose props changed rebuild when they change, which is rare (the
//     start and end of the drag, and when the pointer crosses a card).
//
// It is created once per board by useDrag and is a pointer, so it is
// comparable and can sit in the props of CP components.
type DragState struct {
	Dragging *tela.Value[int]
	Offset   *tela.Value[Offset]
	Over     *tela.Value[Target]
}

// useDrag is a custom hook (it calls hooks): call it from a component body.
func useDrag() *DragState {
	d := tela.UseMemo(func() *DragState { return &DragState{} }, nil)
	d.Dragging = tela.UseValue(0)
	d.Offset = tela.UseValue(Offset{})
	d.Over = tela.UseValue(Target{})
	return d
}

// Active reports whether a card is being dragged.
func (d *DragState) Active() bool { return d.Dragging.Get() != 0 }

// Start begins dragging card id, which sits in column col: until the
// pointer reaches another card or column the target is the card itself,
// which Move treats as "no change".
func (d *DragState) Start(id, col int) {
	d.Offset.Set(Offset{})
	d.Over.Set(Target{Col: col, Before: id, Valid: true})
	d.Dragging.Set(id)
}

// Drop ends the drag: the card goes to where it was dropped and the drag
// state is cleared. It runs through tela.Post so that it sees the Over
// value set by the last pointer move even if no frame ran in between: Post
// and Value updates share one queue and apply in order.
func (d *DragState) Drop(id int, set tela.Setter[Board]) {
	tela.Post(func() {
		target := d.Over.Get()
		d.Dragging.Set(0)
		d.Offset.Set(Offset{})
		d.Over.Set(Target{})
		set.Update(func(b Board) Board { return b.Move(id, target) })
	})
}
