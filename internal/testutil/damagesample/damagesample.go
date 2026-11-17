// Package damagesample thins out telatest.CheckDamage under go test
// -short, for the tests of this repository: a test that calls [Short]
// keeps rasterizing every frame incrementally, but compares only every
// k-th frame with a full rendering (each comparison costs a full frame),
// and the last frame when the app is closed. The frames are those of the
// test's fixed script on the headless clock, so the subset is always the
// same. Without -short every frame is compared, as before.
//
// telatest reads the setting of the test it mounts an app for; nothing
// outside this module can reach it (telatest's public CheckDamage is
// unchanged).
package damagesample

import (
	"strings"
	"sync"
	"testing"
)

var (
	mu    sync.Mutex
	every = map[string]int{} // by test name; a subtest follows its parent
)

// Short makes the apps that tb and its subtests mount with
// telatest.CheckDamage compare every k-th frame with a full rendering, and
// their last frame, when the tests run with -short. It does nothing
// without -short or for k < 2. It holds until tb ends, and only for tb:
// tests that run in parallel keep their own setting.
func Short(tb testing.TB, k int) {
	tb.Helper()
	if !testing.Short() || k < 2 {
		return
	}
	name := tb.Name()
	mu.Lock()
	old, had := every[name]
	every[name] = k
	mu.Unlock()
	tb.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if had {
			every[name] = old
		} else {
			delete(every, name)
		}
	})
}

// Every is the k of [Short] in effect for tb, from tb or the closest of its
// parent tests that called it: 0 when every frame is compared.
func Every(tb testing.TB) int {
	mu.Lock()
	defer mu.Unlock()
	if len(every) == 0 {
		return 0
	}
	for name := tb.Name(); ; {
		if k, ok := every[name]; ok {
			return k
		}
		i := strings.LastIndexByte(name, '/')
		if i < 0 {
			return 0
		}
		name = name[:i]
	}
}
