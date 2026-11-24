//go:build darwin

package darwin

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego/objc"
)

// The functions of MainDisplayMetalRegistryID, bound on first use and
// independently of Load: nothing here needs AppKit.
var (
	metalOnce                             sync.Once
	metalErr                              error
	cgMainDisplayID                       func() uint32                // CGDirectDisplayID CGMainDisplayID(void)
	cgDirectDisplayCopyCurrentMetalDevice func(display uint32) uintptr // id<MTLDevice>, +1 (macOS 10.11)
	metalPoolPush                         func() uintptr
	metalPoolPop                          func(pool uintptr)

	selRegistryID = objc.RegisterName("registryID")
)

// MainDisplayMetalRegistryID returns the registryID of the Metal device that
// drives the main display, 0 without one (no window server session, no GPU).
// Any thread. It may change when the system switches GPUs.
//
// It does not ask MTLCreateSystemDefaultDevice, whose device is the same one
// wherever it has no side effect: on a Mac that switches GPUs by itself,
// that call makes it switch to the high-power one (MTLDevice.h).
func MainDisplayMetalRegistryID() uint64 {
	metalOnce.Do(func() {
		metalErr = loadOptional(libObjC, func(lib uintptr) {
			bind(&metalPoolPush, lib, "objc_autoreleasePoolPush")
			bind(&metalPoolPop, lib, "objc_autoreleasePoolPop")
		})
		if metalErr != nil {
			return
		}
		metalErr = loadOptional(frameworkCG, func(lib uintptr) {
			bind(&cgMainDisplayID, lib, "CGMainDisplayID")
			bind(&cgDirectDisplayCopyCurrentMetalDevice, lib, "CGDirectDisplayCopyCurrentMetalDevice")
		})
	})
	if metalErr != nil {
		return 0
	}
	runtime.LockOSThread() // the pool is the thread's
	defer runtime.UnlockOSThread()
	pool := metalPoolPush()
	defer metalPoolPop(pool)
	dev := objc.ID(cgDirectDisplayCopyCurrentMetalDevice(cgMainDisplayID()))
	if dev == 0 {
		return 0
	}
	id := uint64(dev.Send(selRegistryID))
	dev.Send(selRelease)
	return id
}
