//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

#define BIAS(C, FB)             \
	VLE32V    (C), V1       \
	VFCVTFXV  V1, V2        \
	VFRDIVVF  F3, V2, V3    \
	VFSUBVV   V3, V2, V4    \
	VFMULVF   FB, V2, V5    \
	VMSEQVI   $1, V1, V0    \
	VMERGEVVM V5, V4, V0, V4 \
	VMSEQVI   $-1, V1, V0   \
	VMERGEVVM V5, V4, V0, V4 \
	VMSEQVI   $0, V1, V0    \
	VMERGEVIM $0, V4, V0, V4

#define SIDE(C, FB, M, FS, FC, O) \
	BIAS(C, FB)               \
	VLE32V   (M), V7          \
	VFMULVV  V7, V4, V4       \
	VFMULVF  FS, V4, V4       \
	VFMACCVF V6, FC, V4       \
	VSE32V   V4, (O)

// func dequantRVV(a *dequantArgs)
//
// The dequantiser inner loop. adjustQuantBias is three merges rather than four
// branches: the general case is computed for every lane, which divides by zero
// where q is, and the result is merged away again along with the two cases
// that |q| == 1 covers, where bias * float32(q) gives both signs at once.
TEXT ·dequantRVV(SB), NOSPLIT, $0-8
	MOV a+0(FP), X21

	MOV 0(X21), X10
	MOV 8(X21), X11
	MOV 16(X21), X12
	MOV 24(X21), X13
	MOV 32(X21), X14
	MOV 40(X21), X15
	MOV 48(X21), X16
	MOV 56(X21), X17
	MOV 64(X21), X18
	MOV 72(X21), X19

	BEQZ X19, done

	MOVF 80(X21), F0
	MOVF 84(X21), F1
	MOVF 88(X21), F2
	MOVF 92(X21), F3
	MOVF 96(X21), F4
	MOVF 100(X21), F5
	MOVF 104(X21), F6
	MOVF 108(X21), F7
	MOVF 112(X21), F8

loop:
	VSETVLI X19, E32, M1, TA, MA, X20

	BIAS(X14, F1)
	VLE32V  (X17), V7
	VFMULVV V7, V4, V6
	VFMULVF F5, V6, V6
	VSE32V  V6, (X11)

	SIDE(X13, F0, X16, F4, F7, X10)
	SIDE(X15, F2, X18, F6, F8, X12)

	SLLI $2, X20, X22
	ADD  X22, X10
	ADD  X22, X11
	ADD  X22, X12
	ADD  X22, X13
	ADD  X22, X14
	ADD  X22, X15
	ADD  X22, X16
	ADD  X22, X17
	ADD  X22, X18
	SUB  X20, X19
	BNEZ X19, loop

done:
	RET
