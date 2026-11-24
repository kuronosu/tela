package core

import "context"

// Fence tells when the pixels of a submitted frame are finished
// (TextureFrame.Fence): usually the application's GPU work that draws them.
type Fence interface {
	// Wait blocks until the pixels are finished, and returns nil; it
	// returns an error when they never will be, and the error of ctx soon
	// after ctx is done. The frame is dropped on an error. Tela calls it
	// once per frame, on a goroutine of its own, and cancels ctx when
	// another frame replaced this one.
	Wait(ctx context.Context) error
}

// fenceCloser is a Fence that holds something of the system: closeFence
// frees it, once the fence will not be waited for (again).
type fenceCloser interface{ closeFence() }

// closeFence frees f, nil included.
func closeFence(f Fence) {
	if c, ok := f.(fenceCloser); ok {
		c.closeFence()
	}
}

// SyncFile returns the Fence of a Linux sync file (what
// VK_KHR_external_fence_fd and EGL_ANDROID_native_fence_sync export): it
// signals when the GPU work the file stands for ends, and fails when that
// work failed. A negative fd is work that already ended, as in Vulkan.
//
// It takes the descriptor over: it is closed, once, when the wait ends, or
// when the frame the fence was submitted with is dropped or refused. Use the
// fence for one frame. On other systems its Wait returns an error that wraps
// ErrTextureUnsupported.
func SyncFile(fd int) Fence { return newSyncFile(fd) }
