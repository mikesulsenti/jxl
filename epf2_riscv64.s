//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

#define ABS(V) VFSGNJXVV V, V, V

#define NEIGH(O)                \
	ADD     O, X11, X28     \
	VLE32V  (X28), V9       \
	ADD     O, X12, X28     \
	VLE32V  (X28), V10      \
	ADD     O, X13, X28     \
	VLE32V  (X28), V11      \
	                        \
	VFSUBVV V1, V9, V12     \
	ABS(V12)                \
	VFMULVF F0, V12, V12    \
	VFSUBVV V2, V10, V13    \
	ABS(V13)                \
	VFMACCVF V13, F1, V12   \
	VFSUBVV V3, V11, V13    \
	ABS(V13)                \
	VFMACCVF V13, F2, V12   \
	VFMVVF  F3, V13         \
	VFMACCVV V12, V8, V13   \
	VFMAXVV V15, V13, V12   \
	VFADDVV V12, V7, V7     \
	VFMACCVV V12, V9, V4    \
	VFMACCVV V12, V10, V5   \
	VFMACCVV V12, V11, V6

// func epf2RVV(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
//	k *[6][8]float32, y, x0, x1 int)
//
// One row of the third EPF pass over [x0, x1). The three channel scales and
// the one live in floating-point registers; only the eight border multipliers
// need a vector, because they differ per lane.
TEXT ·epf2RVV(SB), NOSPLIT, $0-72
	MOV  stride+16(FP), X14
	SLLI $2, X14

	MOV  y+48(FP), X28
	ADD  $-1, X28
	MUL  X14, X28, X28
	MOV  x0+56(FP), X29
	SLLI $2, X29
	ADD  X29, X28

	MOV src+8(FP), X30
	MOV (X30), X11
	ADD X28, X11
	MOV 8(X30), X12
	ADD X28, X12
	MOV 16(X30), X13
	ADD X28, X13

	ADD X14, X28

	MOV dst+0(FP), X30
	MOV (X30), X5
	ADD X28, X5
	MOV 8(X30), X6
	ADD X28, X6
	MOV 16(X30), X7
	ADD X28, X7

	MOV  y+48(FP), X28
	SRAI $3, X28
	MOV  sigmaStride+32(FP), X29
	MUL  X29, X28, X28
	MOV  x0+56(FP), X29
	SRAI $3, X29
	ADD  X29, X28
	SLLI $2, X28
	MOV  sigma+24(FP), X10
	ADD  X28, X10

	MOV  x1+64(FP), X21
	MOV  x0+56(FP), X28
	SUB  X28, X21
	SRLI $3, X21
	BEQZ X21, done

	SLLI $1, X14, X22

	MOV  k+40(FP), X23
	MOVF 0(X23), F0
	MOVF 32(X23), F1
	MOVF 64(X23), F2
	MOVF 96(X23), F3

	ADD $160, X23, X24

loop:
	MOVWU (X10), X28
	MOV   $0xc079ef80, X29
	BLTU  X29, X28, copy

	MOV $8, X25
	MOV X24, X26

inner:
	VSETVLI X25, E32, M1, TA, MA, X15

	VMVVI $0, V15

	VLE32V  (X26), V14
	MOVF    (X10), F4
	VFMULVF F4, V14, V8

	ADD    X14, X11, X28
	VLE32V (X28), V1
	ADD    X14, X12, X28
	VLE32V (X28), V2
	ADD    X14, X13, X28
	VLE32V (X28), V3

	VMVVV  V1, V4
	VMVVV  V2, V5
	VMVVV  V3, V6
	VFMVVF F3, V7

	NEIGH(X0)
	ADD $-4, X14, X16
	NEIGH(X16)
	ADD $4, X14, X16
	NEIGH(X16)
	NEIGH(X22)

	VFDIVVV V7, V4, V4
	VSE32V  V4, (X5)
	VFDIVVV V7, V5, V5
	VSE32V  V5, (X6)
	VFDIVVV V7, V6, V6
	VSE32V  V6, (X7)

	SLLI $2, X15, X28
	ADD  X28, X5
	ADD  X28, X6
	ADD  X28, X7
	ADD  X28, X11
	ADD  X28, X12
	ADD  X28, X13
	ADD  X28, X26
	SUB  X15, X25
	BNEZ X25, inner

	JMP next

copy:
	MOV $8, X25

copyinner:
	VSETVLI X25, E32, M1, TA, MA, X15

	ADD    X14, X11, X28
	VLE32V (X28), V1
	VSE32V V1, (X5)
	ADD    X14, X12, X28
	VLE32V (X28), V1
	VSE32V V1, (X6)
	ADD    X14, X13, X28
	VLE32V (X28), V1
	VSE32V V1, (X7)

	SLLI $2, X15, X28
	ADD  X28, X5
	ADD  X28, X6
	ADD  X28, X7
	ADD  X28, X11
	ADD  X28, X12
	ADD  X28, X13
	SUB  X15, X25
	BNEZ X25, copyinner

next:
	ADD  $4, X10
	ADD  $-1, X21
	BNEZ X21, loop

done:
	RET
