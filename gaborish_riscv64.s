//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

#define TAPS(P, L, C, R)  \
	ADD    $-4, P, X17 \
	VLE32V (X17), L    \
	VLE32V (P), C      \
	ADD    $4, P, X17  \
	VLE32V (X17), R

// func gaborishRVV(dst, top, mid, bot *float32, n int, k *[3][8]float32)
//
// The Gaborish three by three over n samples of one row. Every pointer is at
// least one sample inside the row, so there is no clamp to apply here.
TEXT ·gaborishRVV(SB), NOSPLIT, $0-48
	MOV dst+0(FP), X10
	MOV top+8(FP), X11
	MOV mid+16(FP), X12
	MOV bot+24(FP), X13
	MOV n+32(FP), X14
	MOV k+40(FP), X15

	BEQZ X14, done

	MOVF 0(X15), F0
	MOVF 32(X15), F1
	MOVF 64(X15), F2

loop:
	VSETVLI X14, E32, M1, TA, MA, X16

	TAPS(X11, V2, V1, V3)
	TAPS(X12, V5, V4, V6)
	TAPS(X13, V8, V7, V9)

	VFADDVV V5, V1, V10
	VFADDVV V7, V10, V10
	VFADDVV V6, V10, V10

	VFADDVV V3, V2, V11
	VFADDVV V8, V11, V11
	VFADDVV V9, V11, V11

	VFMULVF  F0, V4, V12
	VFMACCVF V10, F1, V12
	VFMACCVF V11, F2, V12

	VSE32V V12, (X10)

	SLLI $2, X16, X17
	ADD  X17, X10
	ADD  X17, X11
	ADD  X17, X12
	ADD  X17, X13
	SUB  X16, X14
	BNEZ X14, loop

done:
	RET
