// Package wine tells tests whether a Windows test binary runs under Wine
// instead of on real Windows, so the tests that check what Wine does not
// implement can skip there (and only there). It is used by tests only.
//
// CI runs the Windows tests twice: under Wine on Linux (ARCHITECTURE §18)
// and on a real windows-latest runner. A test that fails under Wine because
// of a Wine limitation (no AF_UNIX sockets, other directory change
// notification semantics, no junctions) calls [Skip] with the limitation;
// the same test still runs on real Windows.
package wine

import "testing"

// Skip skips the test when it runs under Wine. limitation names what Wine
// does not do ("no AF_UNIX sockets"); it becomes the skip reason, so the
// log says why the test did not run.
func Skip(t testing.TB, limitation string) {
	t.Helper()
	if Detected() {
		t.Skip("Wine limitation: " + limitation + " (the test runs on real Windows)")
	}
}
