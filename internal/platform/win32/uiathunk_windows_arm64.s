#include "textflag.h"

// Thunks for the UIA methods that take doubles (uiacall_windows_arm64.go).
// They run on UIA's threads before any Go code: they only move registers
// and branch, so the Go callback returns straight to UIA (LR is kept).
// R1-R3 are argument registers and R16 the intra-procedure scratch
// register IP0, all free to clobber here.

// ElementProviderFromPoint(this X0, x D0, y D1, ret X1) and
// ITextProvider::RangeFromPoint(this X0, UiaPoint D0 D1, ret X1) become
// (this, x bits, y bits, ret).
TEXT uiaFromPointThunk<>(SB),NOSPLIT|NOFRAME,$0-0
	MOVD	R1, R3
	FMOVD	F0, R1
	FMOVD	F1, R2
	MOVD	$·uiaFromPointTarget(SB), R16
	MOVD	(R16), R16
	B	(R16)

TEXT uiaTextFromPointThunk<>(SB),NOSPLIT|NOFRAME,$0-0
	MOVD	R1, R3
	FMOVD	F0, R1
	FMOVD	F1, R2
	MOVD	$·uiaTextFromPointTarget(SB), R16
	MOVD	(R16), R16
	B	(R16)

// IRangeValueProvider::SetValue(this X0, value D0).
TEXT uiaRangeSetValueThunk<>(SB),NOSPLIT|NOFRAME,$0-0
	FMOVD	F0, R1
	MOVD	$·uiaRangeSetValueTarget(SB), R16
	MOVD	(R16), R16
	B	(R16)

GLOBL	·uiaFromPointThunkAddr(SB), RODATA, $8
DATA	·uiaFromPointThunkAddr(SB)/8, $uiaFromPointThunk<>(SB)
GLOBL	·uiaTextFromPointThunkAddr(SB), RODATA, $8
DATA	·uiaTextFromPointThunkAddr(SB)/8, $uiaTextFromPointThunk<>(SB)
GLOBL	·uiaRangeSetValueThunkAddr(SB), RODATA, $8
DATA	·uiaRangeSetValueThunkAddr(SB)/8, $uiaRangeSetValueThunk<>(SB)
