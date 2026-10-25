package tela

import "github.com/kuronosu/tela/internal/core"

// DevMode reports whether dev mode is on where it is called: in a build,
// the mode of the tree being built (App.Dev, telatest.NoDev); elsewhere the
// process default, on unless the program is a release build, with
// TELA_DEV=1 or TELA_DEV=0 overriding both (ADR-085, ADR-093). Component
// libraries use it to check their own props the way Tela checks classes
// and roles: panic with the fix in dev mode, fall back quietly otherwise.
// It is a lookup and costs nothing.
func DevMode() bool { return core.DevNow() }
