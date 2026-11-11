package text

import "sync/atomic"

// defaultFonts is the constructor RegisterDefaultFonts registered.
var defaultFonts atomic.Pointer[func() *Collection]

// RegisterDefaultFonts registers the constructor of the default font set,
// the collection NewDefaultCollection returns: internal/stdfonts registers
// the deterministic set of tests (Inter, JetBrains Mono, Noto Sans Arabic,
// Noto Color Emoji) from its init, and only tests (and telatest) link it,
// so an app's binary carries no font file (ADR-244). The last call wins.
func RegisterDefaultFonts(newCollection func() *Collection) {
	defaultFonts.Store(&newCollection)
}

// HasDefaultFonts reports whether a package registered a default font
// set: the binary is a test, telatest or the tela command, whose output
// must not depend on the machine.
func HasDefaultFonts() bool { return defaultFonts.Load() != nil }

// NewDefaultCollection returns a new collection of the default font set
// that a package registered with RegisterDefaultFonts, or nil when none
// did: a program that links no font package has no default set, and
// internal/core then uses the system's fonts (NewSystemCollection).
func NewDefaultCollection() *Collection {
	if f := defaultFonts.Load(); f != nil {
		return (*f)()
	}
	return nil
}
