//go:build race

package xkb

// raceEnabled reports whether the tests run under the race detector,
// whose instrumentation makes timing checks meaningless.
const raceEnabled = true
