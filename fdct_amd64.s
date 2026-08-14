//go:build amd64 && !noasm

#include "textflag.h"

// func fdct8Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[8][8]float32)
//
// Eight columns of the eight point forward DCT at once, one column per lane, so
// the butterflies are elementwise and no transpose is needed. This is dct1d(8)
// unrolled: a sum and difference stage, dct1d(4) on each half, the odd half
// pre-multiplied by dctMultipliers[3], then the trailing accumulation. The
// groupings follow dct1d exactly, including the multiply and add of the sqrt2
// step being separate, so the result is bit-identical to the scalar.
TEXT ·fdct8Cols8AVX2(SB), NOSPLIT, $0-40
	MOVQ src+0(FP), SI
	MOVQ srcStride+8(FP), AX
	MOVQ dst+16(FP), DI
	MOVQ dstStride+24(FP), BX
	MOVQ k+32(FP), CX

	SHLQ $2, AX
	SHLQ $2, BX

	VMOVUPS (SI), Y0
	ADDQ    AX, SI
	VMOVUPS (SI), Y1
	ADDQ    AX, SI
	VMOVUPS (SI), Y2
	ADDQ    AX, SI
	VMOVUPS (SI), Y3
	ADDQ    AX, SI
	VMOVUPS (SI), Y4
	ADDQ    AX, SI
	VMOVUPS (SI), Y5
	ADDQ    AX, SI
	VMOVUPS (SI), Y6
	ADDQ    AX, SI
	VMOVUPS (SI), Y7

	// tmp[i] = x[i] + x[7-i], tmp[4+i] = x[i] - x[7-i]
	VADDPS Y7, Y0, Y8
	VADDPS Y6, Y1, Y9
	VADDPS Y5, Y2, Y10
	VADDPS Y4, Y3, Y11
	VSUBPS Y7, Y0, Y0
	VSUBPS Y6, Y1, Y1
	VSUBPS Y5, Y2, Y2
	VSUBPS Y4, Y3, Y3

	// dct1d(4) on the even half
	VADDPS Y11, Y8, Y4
	VADDPS Y10, Y9, Y5
	VSUBPS Y11, Y8, Y6
	VSUBPS Y10, Y9, Y7

	VADDPS Y5, Y4, Y8
	VSUBPS Y5, Y4, Y10

	VMULPS 32(CX), Y6, Y6
	VMULPS 64(CX), Y7, Y7
	VADDPS Y7, Y6, Y4
	VSUBPS Y7, Y6, Y11

	VMULPS 0(CX), Y4, Y9
	VADDPS Y11, Y9, Y9

	// dct1d(4) on the odd half, after dctMultipliers[3]
	VMULPS 96(CX), Y0, Y0
	VMULPS 128(CX), Y1, Y1
	VMULPS 160(CX), Y2, Y2
	VMULPS 192(CX), Y3, Y3

	VADDPS Y3, Y0, Y4
	VADDPS Y2, Y1, Y5
	VSUBPS Y3, Y0, Y6
	VSUBPS Y2, Y1, Y7

	VADDPS Y5, Y4, Y0
	VSUBPS Y5, Y4, Y2

	VMULPS 32(CX), Y6, Y6
	VMULPS 64(CX), Y7, Y7
	VADDPS Y7, Y6, Y4
	VSUBPS Y7, Y6, Y3

	VMULPS 0(CX), Y4, Y1
	VADDPS Y3, Y1, Y1

	// tmp[4] = tmp[4]*sqrt2 + tmp[5], then tmp[5] += tmp[6], tmp[6] += tmp[7]
	VMULPS 0(CX), Y0, Y0
	VADDPS Y1, Y0, Y0
	VADDPS Y2, Y1, Y1
	VADDPS Y3, Y2, Y2

	// interleave the halves and scale by 1/n
	VMULPS  224(CX), Y8, Y8
	VMOVUPS Y8, (DI)
	ADDQ    BX, DI
	VMULPS  224(CX), Y0, Y0
	VMOVUPS Y0, (DI)
	ADDQ    BX, DI
	VMULPS  224(CX), Y9, Y9
	VMOVUPS Y9, (DI)
	ADDQ    BX, DI
	VMULPS  224(CX), Y1, Y1
	VMOVUPS Y1, (DI)
	ADDQ    BX, DI
	VMULPS  224(CX), Y10, Y10
	VMOVUPS Y10, (DI)
	ADDQ    BX, DI
	VMULPS  224(CX), Y2, Y2
	VMOVUPS Y2, (DI)
	ADDQ    BX, DI
	VMULPS  224(CX), Y11, Y11
	VMOVUPS Y11, (DI)
	ADDQ    BX, DI
	VMULPS  224(CX), Y3, Y3
	VMOVUPS Y3, (DI)

	VZEROUPPER
	RET
