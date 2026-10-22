//go:build !teladev

package core

// Without the teladev build tag the dev-loop state (devstate.go, ADR-113)
// is not compiled: devStateEnabled is false, so the compiler removes every
// hook point guarded by it, and these stubs are never called. Release
// builds do not change (ADR-111).

const devStateEnabled = false

func devSeed(*Element, int) bool                        { return false }
func (t *Tree) devFirstBuild(_ *Element, out Node) Node { return out }
func (t *Tree) devAdoptSeed()                           {}
func (t *Tree) devMounted()                             {}
func (t *Tree) devFrame(full bool) bool                 { return full }
func (t *Tree) devNoteScheme(ColorScheme)               {}
func (t *Tree) devNoteTheme(Theme)                      {}
func devFocusWaits(*Element) bool                       { return false }
