//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

#define ABS(V) VFSGNJXVV V, V, V

#define SADPLANE(P, SOFF)     \
	MOV P, X5             \
	ADD X16, P, X6        \
	ADD X17, P, X7        \
	ADD X18, P, X25       \
	ADD X19, P, X26       \
	                      \
	VLE32V (X5), V1       \
	ADD    $-4, X6, X28   \
	VLE32V (X28), V2      \
	VLE32V (X6), V3       \
	ADD    $4, X6, X28    \
	VLE32V (X28), V4      \
	ADD    $-8, X7, X28   \
	VLE32V (X28), V5      \
	ADD    $-4, X7, X28   \
	VLE32V (X28), V6      \
	VLE32V (X7), V7       \
	ADD    $4, X7, X28    \
	VLE32V (X28), V8      \
	ADD    $8, X7, X28    \
	VLE32V (X28), V9      \
	ADD    $-4, X25, X28  \
	VLE32V (X28), V10     \
	VLE32V (X25), V11     \
	ADD    $4, X25, X28   \
	VLE32V (X28), V12     \
	VLE32V (X26), V13     \
	                      \
	MOVF SOFF(X22), F1    \
	                      \
	VFSUBVV V3, V1, V20   \
	ABS(V20)              \
	VFSUBVV V6, V2, V21   \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V3, V7, V21   \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V8, V4, V21   \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V11, V7, V21  \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFMACCVF V20, F1, V16 \
	                      \
	VFSUBVV V3, V2, V20   \
	ABS(V20)              \
	VFSUBVV V6, V5, V21   \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V6, V7, V21   \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V8, V7, V21   \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V11, V10, V21 \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFMACCVF V20, F1, V17 \
	                      \
	VFSUBVV V3, V4, V20   \
	ABS(V20)              \
	VFSUBVV V6, V7, V21   \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V8, V7, V21   \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V8, V9, V21   \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V11, V12, V21 \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFMACCVF V20, F1, V18 \
	                      \
	VFSUBVV V3, V7, V20   \
	ABS(V20)              \
	VFSUBVV V6, V10, V21  \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V11, V7, V21  \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V8, V12, V21  \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFSUBVV V11, V13, V21 \
	ABS(V21)              \
	VFADDVV V21, V20, V20 \
	VFMACCVF V20, F1, V19

#define OUTPLANE(P, D)        \
	MOV P, X5             \
	ADD X16, P, X6        \
	ADD X17, P, X7        \
	ADD X18, P, X25       \
	                      \
	VLE32V  (X7), V20     \
	VLE32V  (X25), V21    \
	VFMACCVV V19, V21, V20 \
	ADD     $4, X7, X28   \
	VLE32V  (X28), V21    \
	VFMACCVV V18, V21, V20 \
	ADD     $-4, X7, X28  \
	VLE32V  (X28), V21    \
	VFMACCVV V17, V21, V20 \
	VLE32V  (X6), V21     \
	VFMACCVV V16, V21, V20 \
	VFDIVVV V14, V20, V20 \
	VSE32V  V20, (D)

#define ADVANCE()      \
	SLLI $2, X8, X28 \
	ADD  X28, X10  \
	ADD  X28, X11  \
	ADD  X28, X12  \
	ADD  X28, X13  \
	ADD  X28, X14  \
	ADD  X28, X15

// func epf1RVV(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
//	mul, scales *float32, y, x0, x1 int)
//
// One row of the second EPF pass over [x0, x1). The inner loop is capped at
// eight elements so that one sigma and one slice of the border multipliers
// cover the whole vector however long VLEN makes it.
TEXT ·epf1RVV(SB), NOSPLIT, $0-80
	MOV  stride+16(FP), X16
	SLLI $2, X16

	MOV  y+56(FP), X28
	ADD  $-2, X28
	MUL  X16, X28, X28
	MOV  x0+64(FP), X29
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
	ADD  X17, X28

	MOV dst+0(FP), X30
	MOV (X30), X10
	ADD X28, X10
	MOV 8(X30), X11
	ADD X28, X11
	MOV 16(X30), X12
	ADD X28, X12

	MOV  y+56(FP), X28
	SRAI $3, X28
	MOV  sigmaStride+32(FP), X29
	MUL  X29, X28, X28
	MOV  x0+64(FP), X29
	SRAI $3, X29
	ADD  X29, X28
	SLLI $2, X28
	MOV  sigma+24(FP), X20
	ADD  X28, X20

	MOV  x1+72(FP), X21
	MOV  x0+64(FP), X28
	SUB  X28, X21
	SRLI $3, X21
	BEQZ X21, done

	ADD  X16, X17, X18
	SLLI $2, X16, X19

	MOV scales+48(FP), X22
	MOV mul+40(FP), X23

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

	VMVVI $0, V15
	VMVVI $0, V16
	VMVVI $0, V17
	VMVVI $0, V18
	VMVVI $0, V19

	SADPLANE(X13, 0)
	SADPLANE(X14, 4)
	SADPLANE(X15, 8)

	VLE32V  (X24), V22
	MOVF    (X20), F3
	VFMULVF F3, V22, V23

	VFMVVF  F0, V20
	VFMACCVV V16, V23, V20
	VFMAXVV V15, V20, V16
	VFMVVF  F0, V20
	VFMACCVV V17, V23, V20
	VFMAXVV V15, V20, V17
	VFMVVF  F0, V20
	VFMACCVV V18, V23, V20
	VFMAXVV V15, V20, V18
	VFMVVF  F0, V20
	VFMACCVV V19, V23, V20
	VFMAXVV V15, V20, V19

	VFMVVF  F0, V14
	VFADDVV V16, V14, V14
	VFADDVV V17, V14, V14
	VFADDVV V18, V14, V14
	VFADDVV V19, V14, V14

	OUTPLANE(X13, X10)
	OUTPLANE(X14, X11)
	OUTPLANE(X15, X12)

	ADVANCE()
	SLLI $2, X8, X28
	ADD  X28, X24
	SUB  X8, X9
	BNEZ X9, inner

	JMP next

copy:
	MOV $8, X9

copyinner:
	VSETVLI X9, E32, M1, TA, MA, X8

	ADD    X17, X13, X28
	VLE32V (X28), V20
	VSE32V V20, (X10)
	ADD    X17, X14, X28
	VLE32V (X28), V20
	VSE32V V20, (X11)
	ADD    X17, X15, X28
	VLE32V (X28), V20
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
