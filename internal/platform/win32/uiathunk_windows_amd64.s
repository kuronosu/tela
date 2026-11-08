#include "textflag.h"

// Thunks for the UIA methods that take doubles (uiacall_windows_amd64.go).
// They run on UIA's threads before any Go code: they only move registers
// and jump, so the Go callback returns straight to UIA. RAX, RDX and R8
// are volatile in the x64 convention.

// ElementProviderFromPoint(this RCX, x XMM1, y XMM2, ret R9).
TEXT uiaFromPointThunk<>(SB),NOSPLIT|NOFRAME,$0-0
	MOVQ	X1, DX
	MOVQ	X2, R8
	MOVQ	·uiaFromPointTarget(SB), AX
	JMP	AX

// IRangeValueProvider::SetValue(this RCX, value XMM1).
TEXT uiaRangeSetValueThunk<>(SB),NOSPLIT|NOFRAME,$0-0
	MOVQ	X1, DX
	MOVQ	·uiaRangeSetValueTarget(SB), AX
	JMP	AX

GLOBL	·uiaFromPointThunkAddr(SB), RODATA, $8
DATA	·uiaFromPointThunkAddr(SB)/8, $uiaFromPointThunk<>(SB)
GLOBL	·uiaRangeSetValueThunkAddr(SB), RODATA, $8
DATA	·uiaRangeSetValueThunkAddr(SB)/8, $uiaRangeSetValueThunk<>(SB)
