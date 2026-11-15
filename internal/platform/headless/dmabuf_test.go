package headless

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// TestWindowsDoNotPresentDmabufs: a headless window has no compositor to
// hand a dmabuf to, so it does not claim the capability (contract v13):
// a renderer that asks falls back to Present, which is what tests and
// snapshots read.
func TestWindowsDoNotPresentDmabufs(t *testing.T) {
	b := New(Options{Size: platform.Size{W: 100, H: 80}})
	w := newWindow(t, b, platform.WindowOptions{Title: "w"})
	if _, ok := platform.Window(w).(platform.DmabufPresenter); ok {
		t.Error("a headless window implements platform.DmabufPresenter")
	}
}
