//go:build telagpu && (linux || windows)

package vk

import (
	"sync"

	"github.com/ebitengine/purego"
)

// The call layer: call1 to call10 call the C function at fn with that
// many integer or pointer arguments and return its integer result (the
// low 32 bits are the VkResult of the commands that have one, see
// [result]). Every wrapper of the bindings goes through them.
//
// They are marked uintptrescapes, like syscall.SyscallN and
// purego.SyscallN: when a pointer is converted to uintptr in the argument
// list of a call to one of them, the compiler moves the object it points
// to to the heap, where the garbage collector never moves it, and keeps
// it alive until the call returns. So a wrapper writes
//
//	call2(fn, uintptr(handle), uintptr(unsafe.Pointer(info)))
//
// with the conversion in the argument list and nowhere else: a uintptr
// computed before the call would point into a stack that may move.
//
// The arguments travel in a pooled array instead of the variadic slice of
// purego.SyscallN, which the same directive makes a heap allocation on
// every call: recording a command allocates nothing (BenchmarkCall).
//
// Floating-point arguments are not supported (they go in other
// registers), and neither are struct arguments by value.

// maxArgs is the largest number of arguments of a bound command
// (vkCmdPipelineBarrier).
const maxArgs = 10

// argBlock carries the arguments of one call.
type argBlock [maxArgs]uintptr

var argBlocks = sync.Pool{New: func() any { return new(argBlock) }}

// invoke calls fn with the first n arguments of a block from the pool.
func invoke(fn uintptr, b *argBlock, n int) uintptr {
	r, _, _ := purego.SyscallN(fn, b[:n]...)
	argBlocks.Put(b)
	return r
}

//go:uintptrescapes
func call1(fn, a0 uintptr) uintptr {
	b := argBlocks.Get().(*argBlock)
	b[0] = a0
	return invoke(fn, b, 1)
}

//go:uintptrescapes
func call2(fn, a0, a1 uintptr) uintptr {
	b := argBlocks.Get().(*argBlock)
	b[0], b[1] = a0, a1
	return invoke(fn, b, 2)
}

//go:uintptrescapes
func call3(fn, a0, a1, a2 uintptr) uintptr {
	b := argBlocks.Get().(*argBlock)
	b[0], b[1], b[2] = a0, a1, a2
	return invoke(fn, b, 3)
}

//go:uintptrescapes
func call4(fn, a0, a1, a2, a3 uintptr) uintptr {
	b := argBlocks.Get().(*argBlock)
	b[0], b[1], b[2], b[3] = a0, a1, a2, a3
	return invoke(fn, b, 4)
}

//go:uintptrescapes
func call5(fn, a0, a1, a2, a3, a4 uintptr) uintptr {
	b := argBlocks.Get().(*argBlock)
	b[0], b[1], b[2], b[3], b[4] = a0, a1, a2, a3, a4
	return invoke(fn, b, 5)
}

//go:uintptrescapes
func call6(fn, a0, a1, a2, a3, a4, a5 uintptr) uintptr {
	b := argBlocks.Get().(*argBlock)
	b[0], b[1], b[2], b[3], b[4], b[5] = a0, a1, a2, a3, a4, a5
	return invoke(fn, b, 6)
}

//go:uintptrescapes
func call7(fn, a0, a1, a2, a3, a4, a5, a6 uintptr) uintptr {
	b := argBlocks.Get().(*argBlock)
	b[0], b[1], b[2], b[3], b[4], b[5], b[6] = a0, a1, a2, a3, a4, a5, a6
	return invoke(fn, b, 7)
}

//go:uintptrescapes
func call8(fn, a0, a1, a2, a3, a4, a5, a6, a7 uintptr) uintptr {
	b := argBlocks.Get().(*argBlock)
	b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7] = a0, a1, a2, a3, a4, a5, a6, a7
	return invoke(fn, b, 8)
}

//go:uintptrescapes
func call10(fn, a0, a1, a2, a3, a4, a5, a6, a7, a8, a9 uintptr) uintptr {
	b := argBlocks.Get().(*argBlock)
	b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7], b[8], b[9] = a0, a1, a2, a3, a4, a5, a6, a7, a8, a9
	return invoke(fn, b, 10)
}
