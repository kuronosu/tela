//go:build linux

package protocols_test

import (
	"os"
	"testing"
	"time"

	. "github.com/kuronosu/tela/internal/platform/wayland/protocols"
	"github.com/kuronosu/tela/internal/platform/wayland/wire"
)

// TestLiveCompositor talks to the real compositor. It only runs with
// TELA_LIVE=1 and a WAYLAND_DISPLAY (or WAYLAND_SOCKET), and it never opens a
// window.
func TestLiveCompositor(t *testing.T) {
	if os.Getenv("TELA_LIVE") != "1" {
		t.Skip("set TELA_LIVE=1 to run against the real compositor")
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("WAYLAND_SOCKET") == "" {
		t.Skip("WAYLAND_DISPLAY is not set")
	}

	conn, err := wire.Dial()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	display := NewWlDisplay(conn)
	registry := display.GetRegistry()
	globals := map[string]WlRegistryGlobalEvent{}
	registry.SetGlobalHandler(func(e WlRegistryGlobalEvent) { globals[e.Interface] = e })

	if err := conn.RoundtripTimeout(5 * time.Second); err != nil {
		t.Fatalf("first roundtrip: %v", err)
	}
	for _, name := range []string{WlCompositorInterfaceName, WlShmInterfaceName, XdgWmBaseInterfaceName} {
		if _, ok := globals[name]; !ok {
			t.Errorf("compositor does not advertise %s (got %d globals)", name, len(globals))
		}
	}
	t.Logf("compositor advertises %d globals", len(globals))

	g := globals[WlCompositorInterfaceName]
	version := min(g.Version, WlCompositorVersion)
	compositor := WlRegistryBind[WlCompositor](registry, g.Name, version)
	if compositor.Version() != version || compositor.Destroyed() {
		t.Errorf("compositor proxy = %v version %d", compositor, compositor.Version())
	}

	// A request on the bound object, then another roundtrip: no protocol error.
	callback := display.Sync()
	done := false
	callback.SetDoneHandler(func(WlCallbackDoneEvent) { done = true })
	if err := conn.RoundtripTimeout(5 * time.Second); err != nil {
		t.Fatalf("second roundtrip: %v", err)
	}
	if !done {
		t.Error("wl_callback.done was not delivered by the roundtrip")
	}
	if err := conn.Err(); err != nil {
		t.Fatalf("connection error: %v", err)
	}
}
