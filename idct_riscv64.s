//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

#define VPASS()                  \
	VFADDVV   V5, V1, V9     \
	VFSUBVV   V5, V1, V10    \
	VFADDVV   V3, V7, V11    \
	VMVVV     V3, V12        \
	VFMADDVF  V11, F0, V12   \
	VMVVV     V3, V13        \
	VFMSUBVF  V11, F0, V13   \
	VMVVV     V12, V14       \
	VFMADDVF  V9, F1, V14    \
	VMVVV     V12, V15       \
	VFNMSUBVF V9, F1, V15    \
	VMVVV     V13, V16       \
	VFMADDVF  V10, F2, V16   \
	VMVVV     V13, V17       \
	VFNMSUBVF V10, F2, V17   \
	                         \
	VFADDVV   V6, V8, V8     \
	VFADDVV   V4, V6, V6     \
	VFADDVV   V2, V4, V4     \
	VMVVV     V2, V18        \
	VFMADDVF  V6, F0, V18    \
	VMVVV     V2, V19        \
	VFMSUBVF  V6, F0, V19    \
	VFADDVV   V4, V8, V20    \
	VMVVV     V4, V21        \
	VFMADDVF  V20, F0, V21   \
	VMVVV     V4, V22        \
	VFMSUBVF  V20, F0, V22   \
	VMVVV     V21, V23       \
	VFMADDVF  V18, F1, V23   \
	VMVVV     V21, V24       \
	VFNMSUBVF V18, F1, V24   \
	VMVVV     V22, V25       \
	VFMADDVF  V19, F2, V25   \
	VMVVV     V22, V26       \
	VFNMSUBVF V19, F2, V26   \
	                         \
	VMVVV     V23, V1        \
	VFMADDVF  V14, F3, V1    \
	VMVVV     V23, V8        \
	VFNMSUBVF V14, F3, V8    \
	VMVVV     V25, V2        \
	VFMADDVF  V16, F4, V2    \
	VMVVV     V25, V7        \
	VFNMSUBVF V16, F4, V7    \
	VMVVV     V26, V3        \
	VFMADDVF  V17, F5, V3    \
	VMVVV     V26, V6        \
	VFNMSUBVF V17, F5, V6    \
	VMVVV     V24, V4        \
	VFMADDVF  V15, F6, V4    \
	VMVVV     V24, V5        \
	VFNMSUBVF V15, F6, V5

#define LOADROWS(P, S)   \
	MOV    P, X17    \
	VLE32V (X17), V1 \
	ADD    S, X17    \
	VLE32V (X17), V2 \
	ADD    S, X17    \
	VLE32V (X17), V3 \
	ADD    S, X17    \
	VLE32V (X17), V4 \
	ADD    S, X17    \
	VLE32V (X17), V5 \
	ADD    S, X17    \
	VLE32V (X17), V6 \
	ADD    S, X17    \
	VLE32V (X17), V7 \
	ADD    S, X17    \
	VLE32V (X17), V8

#define STOREROWS(P, S)  \
	MOV    P, X17    \
	VSE32V V1, (X17) \
	ADD    S, X17    \
	VSE32V V2, (X17) \
	ADD    S, X17    \
	VSE32V V3, (X17) \
	ADD    S, X17    \
	VSE32V V4, (X17) \
	ADD    S, X17    \
	VSE32V V5, (X17) \
	ADD    S, X17    \
	VSE32V V6, (X17) \
	ADD    S, X17    \
	VSE32V V7, (X17) \
	ADD    S, X17    \
	VSE32V V8, (X17)

#define COMB16(VI, FM)          \
	VLE32V    (X17), V9     \
	VMVVV     VI, V10       \
	VFMADDVF  V9, FM, V10   \
	VSE32V    V10, (X21)    \
	VMVVV     VI, V11       \
	VFNMSUBVF V9, FM, V11   \
	VSE32V    V11, (X22)    \
	ADD       X18, X17      \
	ADD       X13, X21      \
	SUB       X13, X22

#define CONSTS()          \
	MOVF 0(X14), F0   \
	MOVF 32(X14), F1  \
	MOVF 64(X14), F2  \
	MOVF 96(X14), F3  \
	MOVF 128(X14), F4 \
	MOVF 160(X14), F5 \
	MOVF 192(X14), F6

// func idct8Cols8RVV(src *float32, srcStride int, dst *float32, dstStride int, k *[15][8]float32, tmp *float32)
//
// An eight point inverse DCT down eight adjacent columns. Where the amd64 pass
// is plain multiplies and adds, this one fuses everywhere Go's riscv64 backend
// does, which is every product in idct4 and idct8 including the two that the
// source spells as separate variables.
TEXT ·idct8Cols8RVV(SB), NOSPLIT, $0-48
	MOV src+0(FP), X10
	MOV srcStride+8(FP), X11
	MOV dst+16(FP), X12
	MOV dstStride+24(FP), X13
	MOV k+32(FP), X14

	SLLI $2, X11
	SLLI $2, X13

	CONSTS()

	MOV $8, X15

loop:
	VSETVLI X15, E32, M1, TA, MA, X16

	LOADROWS(X10, X11)
	VPASS()
	STOREROWS(X12, X13)

	SLLI $2, X16, X18
	ADD  X18, X10
	ADD  X18, X12
	SUB  X16, X15
	BNEZ X15, loop

	RET

// func idct16Cols8RVV(src *float32, srcStride int, dst *float32, dstStride int, k *[15][8]float32, tmp *float32)
//
// A sixteen point inverse DCT down eight adjacent columns: the eight point
// pass over the even rows, then over the odd rows after their running sum, and
// the combine. The even half goes to the caller's scratch because the register
// file does not hold sixteen results and the working set at once.
TEXT ·idct16Cols8RVV(SB), NOSPLIT, $0-48
	MOV src+0(FP), X10
	MOV srcStride+8(FP), X11
	MOV dst+16(FP), X12
	MOV dstStride+24(FP), X13
	MOV k+32(FP), X14
	MOV tmp+40(FP), X19

	SLLI $2, X11
	SLLI $2, X13

	CONSTS()

	MOV  X11, X20
	SLLI $1, X20

	MOV $8, X15

loop16:
	VSETVLI X15, E32, M1, TA, MA, X16

	SLLI $2, X16, X18

	LOADROWS(X10, X20)
	VPASS()

	MOV    X19, X17
	VSE32V V1, (X17)
	ADD    X18, X17
	VSE32V V2, (X17)
	ADD    X18, X17
	VSE32V V3, (X17)
	ADD    X18, X17
	VSE32V V4, (X17)
	ADD    X18, X17
	VSE32V V5, (X17)
	ADD    X18, X17
	VSE32V V6, (X17)
	ADD    X18, X17
	VSE32V V7, (X17)
	ADD    X18, X17
	VSE32V V8, (X17)

	ADD X11, X10, X21
	LOADROWS(X21, X20)

	VFADDVV V7, V8, V8
	VFADDVV V6, V7, V7
	VFADDVV V5, V6, V6
	VFADDVV V4, V5, V5
	VFADDVV V3, V4, V4
	VFADDVV V2, V3, V3
	VFADDVV V1, V2, V2
	VFMULVF F0, V1, V1

	VPASS()

	MOV X13, X22
	MOV $15, X23
	MUL X23, X22, X22
	ADD X12, X22
	MOV X12, X21
	MOV X19, X17

	MOVF 224(X14), F7
	MOVF 256(X14), F8
	MOVF 288(X14), F9
	MOVF 320(X14), F10
	MOVF 352(X14), F11
	MOVF 384(X14), F12
	MOVF 416(X14), F13
	MOVF 448(X14), F14

	COMB16(V1, F7)
	COMB16(V2, F8)
	COMB16(V3, F9)
	COMB16(V4, F10)
	COMB16(V5, F11)
	COMB16(V6, F12)
	COMB16(V7, F13)
	COMB16(V8, F14)

	ADD  X18, X10
	ADD  X18, X12
	SUB  X16, X15
	BNEZ X15, loop16

	RET

// func idct32PreRVV(src *float32, srcStride int, tmp *float32, k *[31][8]float32)
//
// The gather and running sum that open a thirty-two point transform.
TEXT ·idct32PreRVV(SB), NOSPLIT, $0-32
	MOV src+0(FP), X10
	MOV srcStride+8(FP), X11
	MOV tmp+16(FP), X12
	MOV k+24(FP), X13

	SLLI $2, X11
	SLLI $1, X11, X14

	MOVF 0(X13), F0

	MOV $8, X15

outer32:
	VSETVLI X15, E32, M1, TA, MA, X16

	SLLI $2, X16, X17

	MOV X10, X18
	MOV X12, X19
	MOV $16, X20

even32:
	VLE32V (X18), V1
	VSE32V V1, (X19)
	ADD    X14, X18
	ADD    $32, X19
	ADD    $-1, X20
	BNEZ   X20, even32

	ADD X11, X10, X18
	ADD $512, X12, X19
	MOV $16, X20

odd32:
	VLE32V (X18), V1
	VSE32V V1, (X19)
	ADD    X14, X18
	ADD    $32, X19
	ADD    $-1, X20
	BNEZ   X20, odd32

	ADD $992, X12, X19
	MOV $15, X20

chain32:
	ADD     $-32, X19, X21
	VLE32V  (X21), V2
	VLE32V  (X19), V1
	VFADDVV V2, V1, V1
	VSE32V  V1, (X19)
	MOV     X21, X19
	ADD     $-1, X20
	BNEZ    X20, chain32

	ADD     $512, X12, X19
	VLE32V  (X19), V1
	VFMULVF F0, V1, V1
	VSE32V  V1, (X19)

	ADD  X17, X10
	ADD  X17, X12
	SUB  X16, X15
	BNEZ X15, outer32

	RET

// func idct32PostRVV(tmp *float32, dst *float32, dstStride int, k *[31][8]float32)
//
// The combine that closes it, pairing row i with row thirty-one minus i.
TEXT ·idct32PostRVV(SB), NOSPLIT, $0-32
	MOV tmp+0(FP), X10
	MOV dst+8(FP), X11
	MOV dstStride+16(FP), X12
	MOV k+24(FP), X13

	SLLI $2, X12

	MOV $8, X15

outerp32:
	VSETVLI X15, E32, M1, TA, MA, X16

	SLLI $2, X16, X17

	MOV X12, X18
	MOV $31, X19
	MUL X19, X18, X18
	ADD X11, X18
	MOV X11, X20
	ADD $480, X13, X21
	MOV X10, X22
	ADD $512, X10, X23
	MOV $16, X24

post32:
	VLE32V    (X22), V1
	VLE32V    (X23), V2
	MOVF      (X21), F1
	VMVVV     V2, V3
	VFMADDVF  V1, F1, V3
	VSE32V    V3, (X20)
	VMVVV     V2, V4
	VFNMSUBVF V1, F1, V4
	VSE32V    V4, (X18)
	ADD       $32, X22
	ADD       $32, X23
	ADD       $32, X21
	ADD       X12, X20
	SUB       X12, X18
	ADD       $-1, X24
	BNEZ      X24, post32

	ADD  X17, X10
	ADD  X17, X11
	SUB  X16, X15
	BNEZ X15, outerp32

	RET

// func transpose8RVV(src *float32, srcStride int, dst *float32, dstStride int)
//
// One eight by eight tile. RVV has no lane shuffle worth using here and does
// not need one: a strided store writes a source row straight down a
// destination column.
TEXT ·transpose8RVV(SB), NOSPLIT, $0-32
	MOV src+0(FP), X10
	MOV dst+16(FP), X11
	MOV srcStride+8(FP), X12
	MOV dstStride+24(FP), X13

	SLLI $2, X12
	SLLI $2, X13

	MOV $8, X15

outert:
	VSETVLI X15, E32, M1, TA, MA, X16

	MOV X10, X18
	MOV X11, X19
	MOV $8, X20

rowt:
	VLE32V  (X18), V1
	VSSE32V V1, X13, (X19)
	ADD     X12, X18
	ADD     $4, X19
	ADD     $-1, X20
	BNEZ    X20, rowt

	SLLI $2, X16, X17
	ADD  X17, X10
	MUL  X16, X13, X21
	ADD  X21, X11
	SUB  X16, X15
	BNEZ X15, outert

	RET
