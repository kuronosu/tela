//go:build darwin

package darwin

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Block is an Objective-C block: a completion handler made from a Go
// function with NewBlock, or one that the system passes to a delegate
// method (an ID there, called with CallBlock).
type Block = objc.Block

// NewBlock returns a block (+1: Release it once the method it is passed to
// has copied it, which methods taking a completion handler do) that calls
// fn. fn takes the block itself first, then the block's arguments: integer,
// pointer, BOOL (bool) and object (ID) arguments only. Each distinct
// function type costs one purego callback, once per process; blocks of a
// type already seen cost none.
func NewBlock(fn any) Block { return objc.NewBlock(fn) }

// CallBlock calls the block b, which the system made (the completion
// handler of a UNUserNotificationCenter delegate method), with integer or
// pointer arguments; nil does nothing. A block made by NewBlock is called
// the same way. The block's layout is the ABI's Block_literal (isa, flags
// and reserved, invoke, descriptor), whose invoke takes the block first.
func CallBlock(b ID, args ...uintptr) {
	if b == 0 {
		return
	}
	p := uintptr(b) + 16 // invoke, after isa (8) and flags + reserved (4 + 4)
	// p points into the block, C memory outside the Go heap; the double
	// indirection keeps vet's unsafeptr check quiet about the conversion.
	invoke := *(*uintptr)(*(*unsafe.Pointer)(unsafe.Pointer(&p)))
	if invoke == 0 {
		return
	}
	all := append([]uintptr{uintptr(b)}, args...)
	purego.SyscallN(invoke, all...)
}

var (
	performOnce  sync.Once
	performBlock func(rl, mode uintptr, block Block)
)

// RunLoopPerformBlock makes the run loop rl run fn once, on its thread, in
// mode (CFRunLoopCommonModes runs it in the modal panel and event tracking
// modes too), at its next iteration: CFRunLoopPerformBlock, followed by
// CFRunLoopWakeUp so a sleeping loop runs it at once. Safe from any
// thread. The block runs between the run loop's sources and timers,
// never inside one of their callbacks.
func RunLoopPerformBlock(rl, mode uintptr, fn func()) {
	performOnce.Do(func() {
		if loadOptional(frameworkCF, func(lib uintptr) { bind(&performBlock, lib, "CFRunLoopPerformBlock") }) != nil {
			performBlock = nil
		}
	})
	if performBlock == nil {
		return
	}
	b := NewBlock(func(Block) { fn() })
	performBlock(rl, mode, b) // copies the block
	b.Release()
	CFRunLoopWakeUp(rl)
}
