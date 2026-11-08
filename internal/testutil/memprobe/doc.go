// Package memprobe measures where a Tela example's memory goes. It is the
// test-side half of the memory measurement of ADR-240: the TestMemory tests
// of the examples (memory_test.go) mount a screen, hand a [Case] to [Run] and
// [Run] reports one JSON line, prefixed with [ResultPrefix], on stdout. The
// driver, internal/testutil/memreport, builds the test binaries, runs one
// process per case and mode, and prints the tables. See that command for how
// to run it and how to read the numbers.
//
// The package imports only the standard library, so it can sit below every
// layer: the Tela-specific mounting stays in the examples' test files. It is
// used from tests only and is not part of the public API.
//
// # Modes
//
// The environment variable [EnvVar] (TELA_MEM) turns the measurement on; its
// value picks the mode:
//
//   - "rss" (or any other non-empty value): the runtime's default sampling.
//     The result has the heap after a GC, its peak, the bytes allocated and
//     the process's RSS; nothing inflates the numbers.
//   - "profile": runtime.MemProfileRate is 1, so every allocation is recorded
//     and the live heap can be split by subsystem ([Breakdown]). The recording
//     costs memory, so the RSS of this mode is not the app's.
//
// MemProfileRate must be set before the allocations it should see: the
// embedded fonts are parsed on the first use of a text collection
// (text.embeddedTypefaces, a sync.Once), after the test starts, but the
// driver sets GODEBUG=memprofilerate=1 for the profile mode anyway, which
// the runtime applies before any package initializes, and this package's
// init sets the rate too for runs without the driver.
package memprobe
