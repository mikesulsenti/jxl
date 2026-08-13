//go:build amd64 && !noasm

#include "textflag.h"

#define CUBE(V, SB)          \
	VMULPS V, V, Y3      \
	VMULPS 96(AX), V, Y4 \
	VMULPS Y4, Y3, Y3    \
	VADDPS SB(AX), Y3, V

#define MIX(A, B, C)          \
	VMULPS A(AX), Y6, Y0  \
	VMULPS B(AX), Y7, Y3  \
	VADDPS Y3, Y0, Y0     \
	VMULPS C(AX), Y8, Y3  \
	VADDPS Y3, Y0, Y0

#define HORNER(PI, QI)         \
	VMULPS Y2, Y3, Y3      \
	VADDPS PI(AX), Y3, Y3  \
	VMULPS Y2, Y4, Y4      \
	VADDPS QI(AX), Y4, Y4

#define SRGB(D)                  \
	VANDNPS Y0, Y15, Y1      \
	VANDPS  Y15, Y0, Y0      \
	VSQRTPS Y0, Y2           \
	VMOVUPS 640(AX), Y3      \
	VMOVUPS 800(AX), Y4      \
	HORNER(608, 768)         \
	HORNER(576, 736)         \
	HORNER(544, 704)         \
	HORNER(512, 672)         \
	VDIVPS  Y4, Y3, Y3       \
	VMULPS  832(AX), Y0, Y5  \
	VCMPPS  $1, 864(AX), Y0, Y0 \
	VANDPS  Y0, Y5, Y5       \
	VANDNPS Y3, Y0, Y3       \
	VORPS   Y5, Y3, Y3       \
	VORPS   Y1, Y3, Y3       \
	VMOVUPS Y3, (D)

// func xybSRGBAVX2(x, y, b *float32, n int, k *[29][8]float32)
//
// The opsin inverse followed by the sRGB transfer function, in place, over n
// samples with n a multiple of eight. Every constant is splatted into k, so
// each one is a memory operand rather than a broadcast and no register is
// spent holding it.
TEXT ·xybSRGBAVX2(SB), NOSPLIT, $0-40
	MOVQ x+0(FP), DI
	MOVQ y+8(FP), SI
	MOVQ b+16(FP), DX
	MOVQ n+24(FP), CX
	MOVQ k+32(FP), AX

	SHRQ $3, CX
	JZ   done

	VMOVUPS 896(AX), Y15

loop:
	VMOVUPS (DI), Y0
	VMOVUPS (SI), Y1
	VMOVUPS (DX), Y2

	VADDPS Y0, Y1, Y6
	VSUBPS 0(AX), Y6, Y6
	VSUBPS Y0, Y1, Y7
	VSUBPS 32(AX), Y7, Y7
	VSUBPS 64(AX), Y2, Y8

	CUBE(Y6, 128)
	CUBE(Y7, 160)
	CUBE(Y8, 192)

	MIX(224, 256, 288)
	SRGB(DI)
	MIX(320, 352, 384)
	SRGB(SI)
	MIX(416, 448, 480)
	SRGB(DX)

	ADDQ $32, DI
	ADDQ $32, SI
	ADDQ $32, DX
	SUBQ $1, CX
	JNZ  loop

	VZEROUPPER

done:
	RET
