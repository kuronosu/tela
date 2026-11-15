package remote

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// TestWindowsDoNotPresentDmabufs: the child's windows send their frames
// to the host through shared memory, and the protocol carries no dmabuf,
// so they do not claim the capability (contract v13): a renderer in a
// `tela dev` child presents with Present.
func TestWindowsDoNotPresentDmabufs(t *testing.T) {
	if _, ok := platform.Window(&Window{}).(platform.DmabufPresenter); ok {
		t.Error("a remote window implements platform.DmabufPresenter")
	}
}
