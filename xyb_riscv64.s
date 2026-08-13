//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

#define CUBE(L, SB)          \
	VFMULVV  L, L, V7    \
	VFMULVF  F3, L, V8   \
	VFMVVF   SB, L       \
	VFMACCVV V7, V8, L

#define MIX(A, B, C)          \
	VFMULVF  B, V5, V9    \
	VFMACCVF V4, A, V9    \
	VFMACCVF V6, C, V9

#define HORNER(P, Q)           \
	VFMVVF   P, V15        \
	VFMADDVV V15, V11, V12 \
	VFMVVF   Q, V16        \
	VFMADDVV V16, V11, V13

#define SRGB(D)                     \
	VFSGNJXVV V9, V9, V10       \
	VFSQRTV   V10, V11          \
	VFMVVF    F20, V12          \
	VFMVVF    F25, V13          \
	HORNER(F19, F24)            \
	HORNER(F18, F23)            \
	HORNER(F17, F22)            \
	HORNER(F16, F21)            \
	VFDIVVV   V13, V12, V12     \
	VFMULVF   F26, V10, V14     \
	VMFLTVF   F27, V10, V0      \
	VMERGEVVM V14, V12, V0, V12 \
	VFSGNJVV  V9, V12, V12      \
	VSE32V    V12, (D)

// func xybSRGBRVV(x, y, b *float32, n int, k *[29][8]float32)
//
// The opsin inverse followed by the sRGB transfer function, in place, over n
// samples. Every constant fits in a floating-point register, so the vector
// instructions take them as scalars and k is read once at entry.
TEXT ·xybSRGBRVV(SB), NOSPLIT, $0-40
	MOV x+0(FP), X10
	MOV y+8(FP), X11
	MOV b+16(FP), X12
	MOV n+24(FP), X13
	MOV k+32(FP), X14

	BEQZ X13, done

	MOVF 0(X14), F0
	MOVF 32(X14), F1
	MOVF 64(X14), F2
	MOVF 96(X14), F3
	MOVF 128(X14), F4
	MOVF 160(X14), F5
	MOVF 192(X14), F6
	MOVF 224(X14), F7
	MOVF 256(X14), F8
	MOVF 288(X14), F9
	MOVF 320(X14), F10
	MOVF 352(X14), F11
	MOVF 384(X14), F12
	MOVF 416(X14), F13
	MOVF 448(X14), F14
	MOVF 480(X14), F15
	MOVF 512(X14), F16
	MOVF 544(X14), F17
	MOVF 576(X14), F18
	MOVF 608(X14), F19
	MOVF 640(X14), F20
	MOVF 672(X14), F21
	MOVF 704(X14), F22
	MOVF 736(X14), F23
	MOVF 768(X14), F24
	MOVF 800(X14), F25
	MOVF 832(X14), F26
	MOVF 864(X14), F27

loop:
	VSETVLI X13, E32, M1, TA, MA, X15

	VLE32V (X10), V1
	VLE32V (X11), V2
	VLE32V (X12), V3

	VFADDVV V1, V2, V4
	VFSUBVF F0, V4, V4
	VFSUBVV V1, V2, V5
	VFSUBVF F1, V5, V5
	VFSUBVF F2, V3, V6

	CUBE(V4, F4)
	CUBE(V5, F5)
	CUBE(V6, F6)

	MIX(F7, F8, F9)
	SRGB(X10)
	MIX(F10, F11, F12)
	SRGB(X11)
	MIX(F13, F14, F15)
	SRGB(X12)

	SLLI $2, X15, X16
	ADD  X16, X10
	ADD  X16, X11
	ADD  X16, X12
	SUB  X15, X13
	BNEZ X13, loop

done:
	RET
