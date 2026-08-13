//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

#define ABS(V) VFSGNJXVV V, V, V

#define LD(V, R, O)       \
	ADD    $O, R, X30 \
	VLE32V (X30), V

#define ROWS(P)          \
	MOV P, X5        \
	ADD X16, P, X6   \
	ADD X17, P, X7   \
	ADD X18, P, X25  \
	ADD X19, P, X26  \
	ADD X16, X26, X28 \
	ADD X17, X26, X29

#define DIR(ACC, B0, O0, B1, O1, B2, O2, B3, O3, B4, O4) \
	LD(V17, B0, O0)                                  \
	VFSUBVV V17, V12, V17                            \
	ABS(V17)                                         \
	LD(V18, B1, O1)                                  \
	VFSUBVV V18, V13, V18                            \
	ABS(V18)                                         \
	VFADDVV V18, V17, V17                            \
	LD(V18, B2, O2)                                  \
	VFSUBVV V18, V14, V18                            \
	ABS(V18)                                         \
	VFADDVV V18, V17, V17                            \
	LD(V18, B3, O3)                                  \
	VFSUBVV V18, V15, V18                            \
	ABS(V18)                                         \
	VFADDVV V18, V17, V17                            \
	LD(V18, B4, O4)                                  \
	VFSUBVV V18, V16, V18                            \
	ABS(V18)                                         \
	VFADDVV V18, V17, V17                            \
	VFMACCVF V17, F1, ACC

#define SADPLANE(P, SOFF)                                        \
	ROWS(P)                                                  \
	LD(V12, X7, 0)                                           \
	LD(V13, X25, -4)                                         \
	LD(V14, X25, 0)                                          \
	LD(V15, X25, 4)                                          \
	LD(V16, X26, 0)                                          \
	MOVF SOFF(X22), F1                                       \
	                                                         \
	DIR(V0, X5, 0, X6, -4, X6, 0, X6, 4, X7, 0)              \
	DIR(V1, X6, -4, X7, -8, X7, -4, X7, 0, X25, -4)          \
	DIR(V2, X6, 0, X7, -4, X7, 0, X7, 4, X25, 0)             \
	DIR(V3, X6, 4, X7, 0, X7, 4, X7, 8, X25, 4)              \
	DIR(V4, X7, -8, X25, -12, X25, -8, X25, -4, X26, -8)     \
	DIR(V5, X7, -4, X25, -8, X25, -4, X25, 0, X26, -4)       \
	DIR(V6, X7, 4, X25, 0, X25, 4, X25, 8, X26, 4)           \
	DIR(V7, X7, 8, X25, 4, X25, 8, X25, 12, X26, 8)          \
	DIR(V8, X25, -4, X26, -8, X26, -4, X26, 0, X28, -4)      \
	DIR(V9, X25, 0, X26, -4, X26, 0, X26, 4, X28, 0)         \
	DIR(V10, X25, 4, X26, 0, X26, 4, X26, 8, X28, 4)         \
	DIR(V11, X26, 0, X28, -4, X28, 0, X28, 4, X29, 0)

#define TAP(R, O, ACC)   \
	LD(V17, R, O)    \
	VFMACCVV V17, ACC, V21

#define OUTPLANE(P, D)        \
	ROWS(P)               \
	LD(V21, X25, 0)       \
	TAP(X28, 0, V11)      \
	TAP(X26, 4, V10)      \
	TAP(X26, 0, V9)       \
	TAP(X26, -4, V8)      \
	TAP(X25, 8, V7)       \
	TAP(X25, 4, V6)       \
	TAP(X25, -4, V5)      \
	TAP(X25, -8, V4)      \
	TAP(X7, 4, V3)        \
	TAP(X7, 0, V2)        \
	TAP(X7, -4, V1)       \
	TAP(X6, 0, V0)        \
	VFDIVVV V20, V21, V21 \
	VSE32V  V21, (D)

#define WEIGHT(ACC)            \
	VFMVVF   F0, V17       \
	VFMACCVV ACC, V19, V17 \
	VFMAXVV  V23, V17, ACC

#define ADVANCE()      \
	SLLI $2, X8, X30 \
	ADD  X30, X10  \
	ADD  X30, X11  \
	ADD  X30, X12  \
	ADD  X30, X13  \
	ADD  X30, X14  \
	ADD  X30, X15

// func epf0RVV(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
//	k *[6][8]float32, y, x0, x1 int)
//
// One row of the first EPF pass over [x0, x1). Every direction of the diamond
// compares the same five centre taps against a shifted copy, so the taps are
// loaded once per plane and the twelve accumulators stay in registers. The
// inner loop is capped at eight elements so one sigma covers the whole vector.
TEXT ·epf0RVV(SB), NOSPLIT, $0-72
	MOV  stride+16(FP), X16
	SLLI $2, X16

	MOV  y+48(FP), X28
	ADD  $-3, X28
	MUL  X16, X28, X28
	MOV  x0+56(FP), X29
	SLLI $2, X29
	ADD  X29, X28

	MOV src+8(FP), X30
	MOV (X30), X13
	ADD X28, X13
	MOV 8(X30), X14
	ADD X28, X14
	MOV 16(X30), X15
	ADD X28, X15

	SLLI $1, X16, X17
	ADD  X16, X17, X18
	ADD  X18, X28

	MOV dst+0(FP), X30
	MOV (X30), X10
	ADD X28, X10
	MOV 8(X30), X11
	ADD X28, X11
	MOV 16(X30), X12
	ADD X28, X12

	MOV  y+48(FP), X28
	SRAI $3, X28
	MOV  sigmaStride+32(FP), X29
	MUL  X29, X28, X28
	MOV  x0+56(FP), X29
	SRAI $3, X29
	ADD  X29, X28
	SLLI $2, X28
	MOV  sigma+24(FP), X20
	ADD  X28, X20

	MOV  x1+64(FP), X21
	MOV  x0+56(FP), X28
	SUB  X28, X21
	SRLI $3, X21
	BEQZ X21, done

	SLLI $2, X16, X19

	MOV k+40(FP), X22
	ADD $160, X22, X23

	MOV   $0x3f800000, X28
	FMVWX X28, F0

loop:
	MOVWU (X20), X28
	MOV   $0xc079ef80, X29
	BLTU  X29, X28, copy

	MOV $8, X9
	MOV X23, X24

inner:
	VSETVLI X9, E32, M1, TA, MA, X8

	VMVVI $0, V0
	VMVVI $0, V1
	VMVVI $0, V2
	VMVVI $0, V3
	VMVVI $0, V4
	VMVVI $0, V5
	VMVVI $0, V6
	VMVVI $0, V7
	VMVVI $0, V8
	VMVVI $0, V9
	VMVVI $0, V10
	VMVVI $0, V11
	VMVVI $0, V23

	SADPLANE(X13, 0)
	SADPLANE(X14, 32)
	SADPLANE(X15, 64)

	VLE32V  (X24), V22
	MOVF    (X20), F3
	VFMULVF F3, V22, V19

	WEIGHT(V0)
	WEIGHT(V1)
	WEIGHT(V2)
	WEIGHT(V3)
	WEIGHT(V4)
	WEIGHT(V5)
	WEIGHT(V6)
	WEIGHT(V7)
	WEIGHT(V8)
	WEIGHT(V9)
	WEIGHT(V10)
	WEIGHT(V11)

	VFMVVF  F0, V20
	VFADDVV V0, V20, V20
	VFADDVV V1, V20, V20
	VFADDVV V2, V20, V20
	VFADDVV V3, V20, V20
	VFADDVV V4, V20, V20
	VFADDVV V5, V20, V20
	VFADDVV V6, V20, V20
	VFADDVV V7, V20, V20
	VFADDVV V8, V20, V20
	VFADDVV V9, V20, V20
	VFADDVV V10, V20, V20
	VFADDVV V11, V20, V20

	OUTPLANE(X13, X10)
	OUTPLANE(X14, X11)
	OUTPLANE(X15, X12)

	ADVANCE()
	SLLI $2, X8, X30
	ADD  X30, X24
	SUB  X8, X9
	BNEZ X9, inner

	JMP next

copy:
	MOV $8, X9

copyinner:
	VSETVLI X9, E32, M1, TA, MA, X8

	ADD    X18, X13, X30
	VLE32V (X30), V20
	VSE32V V20, (X10)
	ADD    X18, X14, X30
	VLE32V (X30), V20
	VSE32V V20, (X11)
	ADD    X18, X15, X30
	VLE32V (X30), V20
	VSE32V V20, (X12)

	ADVANCE()
	SUB  X8, X9
	BNEZ X9, copyinner

next:
	ADD  $4, X20
	ADD  $-1, X21
	BNEZ X21, loop

done:
	RET
