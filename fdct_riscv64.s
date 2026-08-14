//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

// FDCT8 is dct1d(8) on a vector of columns, taking x0..x7 in V0..V7 and leaving
// the interleaved result in V8, V0, V9, V1, V10, V2, V11, V3 for indices 0..7.
// It clobbers V4..V7 and V12. The two sqrt2 steps use VFMACCVF because the Go
// compiler contracts that multiply and add into an FMADD on riscv64.
#define FDCT8                 \
	VFADDVV V7, V0, V8    \
	VFADDVV V6, V1, V9    \
	VFADDVV V5, V2, V10   \
	VFADDVV V4, V3, V11   \
	VFSUBVV V7, V0, V0    \
	VFSUBVV V6, V1, V1    \
	VFSUBVV V5, V2, V2    \
	VFSUBVV V4, V3, V3    \
	                      \
	VFADDVV V11, V8, V4   \
	VFADDVV V10, V9, V5   \
	VFSUBVV V11, V8, V6   \
	VFSUBVV V10, V9, V7   \
	VFADDVV V5, V4, V8    \
	VFSUBVV V5, V4, V10   \
	VFMULVF F1, V6, V6    \
	VFMULVF F2, V7, V7    \
	VFADDVV V7, V6, V4    \
	VFSUBVV V7, V6, V11   \
	VMVVV   V11, V9       \
	VFMACCVF V4, F0, V9   \
	                      \
	VFMULVF F3, V0, V0    \
	VFMULVF F4, V1, V1    \
	VFMULVF F5, V2, V2    \
	VFMULVF F6, V3, V3    \
	VFADDVV V3, V0, V4    \
	VFADDVV V2, V1, V5    \
	VFSUBVV V3, V0, V6    \
	VFSUBVV V2, V1, V7    \
	VFADDVV V5, V4, V0    \
	VFSUBVV V5, V4, V2    \
	VFMULVF F1, V6, V6    \
	VFMULVF F2, V7, V7    \
	VFADDVV V7, V6, V4    \
	VFSUBVV V7, V6, V3    \
	VMVVV   V3, V1        \
	VFMACCVF V4, F0, V1   \
	                      \
	VMVVV   V1, V12       \
	VFMACCVF V0, F0, V12  \
	VFADDVV V2, V1, V1    \
	VFADDVV V3, V2, V2    \
	VMVVV   V12, V0

#define CONSTS8         \
	MOVF 0(X13), F0 \
	MOVF 32(X13), F1 \
	MOVF 64(X13), F2 \
	MOVF 96(X13), F3 \
	MOVF 128(X13), F4 \
	MOVF 160(X13), F5 \
	MOVF 192(X13), F6

#define LOAD8            \
	VLE32V (X10), V0 \
	ADD    X11, X10  \
	VLE32V (X10), V1 \
	ADD    X11, X10  \
	VLE32V (X10), V2 \
	ADD    X11, X10  \
	VLE32V (X10), V3 \
	ADD    X11, X10  \
	VLE32V (X10), V4 \
	ADD    X11, X10  \
	VLE32V (X10), V5 \
	ADD    X11, X10  \
	VLE32V (X10), V6 \
	ADD    X11, X10  \
	VLE32V (X10), V7

// func fdct8ColsRVV(src *float32, srcStride int, dst *float32, dstStride int, k *[34][8]float32, scale *float32, n int)
TEXT ·fdct8ColsRVV(SB), NOSPLIT, $0-56
	MOV src+0(FP), X10
	MOV srcStride+8(FP), X11
	MOV dst+16(FP), X12
	MOV dstStride+24(FP), X20
	MOV k+32(FP), X13
	MOV scale+40(FP), X14
	MOV n+48(FP), X15

	SLLI $2, X11
	SLLI $2, X20

	CONSTS8
	MOVF (X14), F7

	MOV X10, X24
	MOV X12, X25

col8:
	VSETVLI X15, E32, M1, TA, MA, X16

	MOV X24, X10
	LOAD8
	FDCT8

	MOV X25, X17

	VFMULVF F7, V8, V8
	VSE32V  V8, (X17)
	ADD     X20, X17
	VFMULVF F7, V0, V0
	VSE32V  V0, (X17)
	ADD     X20, X17
	VFMULVF F7, V9, V9
	VSE32V  V9, (X17)
	ADD     X20, X17
	VFMULVF F7, V1, V1
	VSE32V  V1, (X17)
	ADD     X20, X17
	VFMULVF F7, V10, V10
	VSE32V  V10, (X17)
	ADD     X20, X17
	VFMULVF F7, V2, V2
	VSE32V  V2, (X17)
	ADD     X20, X17
	VFMULVF F7, V11, V11
	VSE32V  V11, (X17)
	ADD     X20, X17
	VFMULVF F7, V3, V3
	VSE32V  V3, (X17)

	SLLI $2, X16, X22
	ADD  X22, X24
	ADD  X22, X25
	SUB  X16, X15
	BNEZ X15, col8

	RET

// STAGE16 reads rows I and 15-I, leaving the sum in RV and writing the
// difference, pre-multiplied by dctMultipliers[4], to the frame at FOFF.
#define STAGE16(MOFF, FOFF, RV)  \
	VLE32V  (X10), RV        \
	VLE32V  (X18), V13       \
	VFSUBVV V13, RV, V14     \
	MOVF    MOFF(X13), F8    \
	VFMULVF F8, V14, V14     \
	MOV     X19, X21         \
	ADD     $FOFF, X21       \
	VSE32V  V14, (X21)       \
	VFADDVV V13, RV, RV      \
	ADD     X11, X10         \
	SUB     X11, X18

#define FLOAD(FOFF, V)   \
	MOV    X19, X21  \
	ADD    $FOFF, X21 \
	VLE32V (X21), V

#define STORE2(V)          \
	VFMULVF F7, V, V   \
	VSE32V  V, (X17)   \
	ADD     X20, X17   \
	ADD     X20, X17

// func fdct16ColsRVV(src *float32, srcStride int, dst *float32, dstStride int, k *[34][8]float32, scale *float32, n int, tmp *float32)
TEXT ·fdct16ColsRVV(SB), NOSPLIT, $0-64
	MOV src+0(FP), X10
	MOV srcStride+8(FP), X11
	MOV dst+16(FP), X12
	MOV dstStride+24(FP), X20
	MOV k+32(FP), X13
	MOV scale+40(FP), X14
	MOV n+48(FP), X15
	MOV tmp+56(FP), X19

	SLLI $2, X11
	SLLI $2, X20

	CONSTS8

	MOV X10, X24
	MOV X12, X25

col16:
	VSETVLI X15, E32, M1, TA, MA, X16

	MOV  X11, X22
	MOV  $15, X23
	MUL  X23, X22
	MOV  X24, X10
	MOV  X24, X18
	ADD  X22, X18

	STAGE16(224, 0, V0)
	STAGE16(256, 128, V1)
	STAGE16(288, 256, V2)
	STAGE16(320, 384, V3)
	STAGE16(352, 512, V4)
	STAGE16(384, 640, V5)
	STAGE16(416, 768, V6)
	STAGE16(448, 896, V7)

	FDCT8

	MOVF (X14), F7
	MOV  X25, X17

	STORE2(V8)
	STORE2(V0)
	STORE2(V9)
	STORE2(V1)
	STORE2(V10)
	STORE2(V2)
	STORE2(V11)
	STORE2(V3)

	FLOAD(0, V0)
	FLOAD(128, V1)
	FLOAD(256, V2)
	FLOAD(384, V3)
	FLOAD(512, V4)
	FLOAD(640, V5)
	FLOAD(768, V6)
	FLOAD(896, V7)

	FDCT8

	VMVVV    V0, V12
	VFMACCVF V8, F0, V12
	VMVVV    V12, V8
	VFADDVV  V9, V0, V0
	VFADDVV  V1, V9, V9
	VFADDVV  V10, V1, V1
	VFADDVV  V2, V10, V10
	VFADDVV  V11, V2, V2
	VFADDVV  V3, V11, V11

	MOVF (X14), F7
	MOV  X25, X17
	ADD  X20, X17

	STORE2(V8)
	STORE2(V0)
	STORE2(V9)
	STORE2(V1)
	STORE2(V10)
	STORE2(V2)
	STORE2(V11)
	STORE2(V3)

	SLLI $2, X16, X22
	ADD  X22, X24
	ADD  X22, X25
	SUB  X16, X15
	BNEZ X15, col16

	RET

// STAGE32 reads rows I and 31-I, writing the sum to the even half of the
// staging buffer and the difference, pre-multiplied by dctMultipliers[5], to
// the odd half.
#define STAGE32(MOFF, EOFF, OOFF) \
	VLE32V  (X10), V0         \
	VLE32V  (X18), V1         \
	VFSUBVV V1, V0, V2        \
	MOVF    MOFF(X13), F8     \
	VFMULVF F8, V2, V2        \
	MOV     X19, X21          \
	ADD     $OOFF, X21        \
	VSE32V  V2, (X21)         \
	VFADDVV V1, V0, V0        \
	MOV     X19, X21          \
	ADD     $EOFF, X21        \
	VSE32V  V0, (X21)         \
	ADD     X11, X10          \
	SUB     X11, X18

// func fdct32SplitRVV(src *float32, srcStride int, half *float32, k *[34][8]float32, n int)
TEXT ·fdct32SplitRVV(SB), NOSPLIT, $0-40
	MOV src+0(FP), X10
	MOV srcStride+8(FP), X11
	MOV half+16(FP), X19
	MOV k+24(FP), X13
	MOV n+32(FP), X15

	SLLI $2, X11

	VSETVLI X15, E32, M1, TA, MA, X16

	MOV  X11, X22
	MOV  $31, X23
	MUL  X23, X22
	MOV  X10, X18
	ADD  X22, X18

	STAGE32(480, 0, 2048)
	STAGE32(512, 128, 2176)
	STAGE32(544, 256, 2304)
	STAGE32(576, 384, 2432)
	STAGE32(608, 512, 2560)
	STAGE32(640, 640, 2688)
	STAGE32(672, 768, 2816)
	STAGE32(704, 896, 2944)
	STAGE32(736, 1024, 3072)
	STAGE32(768, 1152, 3200)
	STAGE32(800, 1280, 3328)
	STAGE32(832, 1408, 3456)
	STAGE32(864, 1536, 3584)
	STAGE32(896, 1664, 3712)
	STAGE32(928, 1792, 3840)
	STAGE32(960, 1920, 3968)

	RET

// func fdct32MergeRVV(half *float32, dst *float32, dstStride int, k *[34][8]float32, scale *float32, n int)
TEXT ·fdct32MergeRVV(SB), NOSPLIT, $0-48
	MOV half+0(FP), X19
	MOV dst+8(FP), X12
	MOV dstStride+16(FP), X20
	MOV k+24(FP), X13
	MOV scale+32(FP), X14
	MOV n+40(FP), X15

	SLLI $2, X20

	MOVF 0(X13), F0
	MOVF (X14), F7

	VSETVLI X15, E32, M1, TA, MA, X16

	MOV    X19, X21
	ADD    $2048, X21
	VLE32V (X21), V0
	MOV    X19, X22
	ADD    $2176, X22
	VLE32V (X22), V1
	VMVVV  V1, V2
	VFMACCVF V0, F0, V2
	VSE32V V2, (X21)

	MOV X19, X23
	ADD $2176, X23
	MOV $0, X24

merge:
	MOV    X23, X21
	ADD    X24, X21
	VLE32V (X21), V0
	MOV    X21, X22
	ADD    $128, X22
	VLE32V (X22), V1
	VFADDVV V1, V0, V0
	VSE32V V0, (X21)

	ADD  $128, X24
	MOV  $1792, X25
	BLT  X24, X25, merge

	MOV X12, X17
	MOV $0, X24

store:
	MOV     X19, X21
	ADD     X24, X21
	VLE32V  (X21), V0
	VFMULVF F7, V0, V0
	VSE32V  V0, (X17)
	ADD     X20, X17

	MOV     X19, X21
	ADD     $2048, X21
	ADD     X24, X21
	VLE32V  (X21), V0
	VFMULVF F7, V0, V0
	VSE32V  V0, (X17)
	ADD     X20, X17

	ADD  $128, X24
	MOV  $2048, X25
	BLT  X24, X25, store

	RET
