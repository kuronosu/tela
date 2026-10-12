// Package wrongtype is a test package of cmd/tela whose Preview has the wrong
// type.
package wrongtype

// Preview should return a tela.Node.
func Preview() int { return 1 }
