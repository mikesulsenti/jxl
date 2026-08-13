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

#define CUBE512(V, SB)         \
	VMULPS V, V, Z3        \
	VMULPS 192(AX), V, Z4  \
	VMULPS Z4, Z3, Z3      \
	VADDPS SB(AX), Z3, V

#define MIX512(A, B, C)       \
	VMULPS A(AX), Z6, Z0  \
	VMULPS B(AX), Z7, Z3  \
	VADDPS Z3, Z0, Z0     \
	VMULPS C(AX), Z8, Z3  \
	VADDPS Z3, Z0, Z0

#define HORNER512(PI, QI)     \
	VMULPS Z2, Z3, Z3     \
	VADDPS PI(AX), Z3, Z3 \
	VMULPS Z2, Z4, Z4     \
	VADDPS QI(AX), Z4, Z4

#define SRGB512(D)                   \
	VANDNPS Z0, Z15, Z1          \
	VANDPS  Z15, Z0, Z0          \
	VSQRTPS Z0, Z2               \
	VMOVUPS 1280(AX), Z3         \
	VMOVUPS 1600(AX), Z4         \
	HORNER512(1216, 1536)        \
	HORNER512(1152, 1472)        \
	HORNER512(1088, 1408)        \
	HORNER512(1024, 1344)        \
	VDIVPS  Z4, Z3, Z3           \
	VMULPS  1664(AX), Z0, Z5     \
	VCMPPS  $1, 1728(AX), Z0, K1 \
	VMOVAPS Z5, K1, Z3           \
	VORPS   Z1, Z3, Z3           \
	VMOVUPS Z3, (D)

// func xybSRGB512(x, y, b *float32, n int, k *[29][16]float32)
//
// xybSRGBAVX2 over sixteen samples at a time. The constants are the same
// splatted table at twice the stride, so every offset is doubled, and the
// branch of linearToSRGB becomes a mask register and one merge-masked move
// rather than the AND, ANDN and OR pair.
TEXT ·xybSRGB512(SB), NOSPLIT, $0-40
	MOVQ x+0(FP), DI
	MOVQ y+8(FP), SI
	MOVQ b+16(FP), DX
	MOVQ n+24(FP), CX
	MOVQ k+32(FP), AX

	SHRQ $4, CX
	JZ   done512

	VMOVUPS 1792(AX), Z15

loop512:
	VMOVUPS (DI), Z0
	VMOVUPS (SI), Z1
	VMOVUPS (DX), Z2

	VADDPS Z0, Z1, Z6
	VSUBPS 0(AX), Z6, Z6
	VSUBPS Z0, Z1, Z7
	VSUBPS 64(AX), Z7, Z7
	VSUBPS 128(AX), Z2, Z8

	CUBE512(Z6, 256)
	CUBE512(Z7, 320)
	CUBE512(Z8, 384)

	MIX512(448, 512, 576)
	SRGB512(DI)
	MIX512(640, 704, 768)
	SRGB512(SI)
	MIX512(832, 896, 960)
	SRGB512(DX)

	ADDQ $64, DI
	ADDQ $64, SI
	ADDQ $64, DX
	SUBQ $1, CX
	JNZ  loop512

	VZEROUPPER

done512:
	RET
