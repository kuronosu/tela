package remote

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/platform/platformtest"
)

// TestConformance runs the backend-agnostic platform suite on a child
// Backend whose host is a real-time headless backend: every subtest,
// window control, hiding, parent and modal windows and the desktop
// services included (ADR-184).
func TestConformance(t *testing.T) {
	platformtest.Run(t, platformtest.Config{
		New: func(t *testing.T) platform.Backend {
			p := newPair(t, HostOptions{}, headless.Options{})
			b, err := Dial(p.host.Addr(), DialOptions{Tag: t.Name()})
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			t.Cleanup(func() { b.Close() })
			return b
		},
	})
}
