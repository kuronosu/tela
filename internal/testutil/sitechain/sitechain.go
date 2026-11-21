// Package sitechain makes a node at the end of a deep chain of helper
// frames that are not the app's, for the tests of internal/core's creation
// sites (instrument_site.go): its frames are in a Tela package, so the
// rules of creation sites skip them, as they skip a library's helpers. It
// imports nothing of Tela, so core's own tests can use it. It is used by
// tests only.
package sitechain

// Call calls f(kind, children...) under depth frames of Call, and returns
// what it returns: with f a node constructor (core.Primitive), the
// constructor's caller is depth frames above the caller of Call.
//
//go:noinline
func Call[K, C, R any](depth int, f func(K, ...C) R, kind K, children ...C) R {
	if depth <= 0 {
		return f(kind, children...)
	}
	return Call(depth-1, f, kind, children...)
}
