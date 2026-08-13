//go:build amd64 && !noasm

#include "textflag.h"

// func gaborishAVX2(dst, top, mid, bot *float32, n int, k *[3][8]float32)
//
// The Gaborish three by three over n samples of one row, with n a multiple of
// eight and every pointer at least one sample inside the row so the clamp at
// the ends is the caller's problem.
TEXT ·gaborishAVX2(SB), NOSPLIT, $0-48
	MOVQ dst+0(FP), DI
	MOVQ top+8(FP), SI
	MOVQ mid+16(FP), DX
	MOVQ bot+24(FP), BX
	MOVQ n+32(FP), CX
	MOVQ k+40(FP), AX

	SHRQ $3, CX
	JZ   done

	VMOVUPS (AX), Y13
	VMOVUPS 32(AX), Y14
	VMOVUPS 64(AX), Y15

loop:
	VMOVUPS (SI), Y0
	VMOVUPS -4(SI), Y1
	VMOVUPS 4(SI), Y2
	VMOVUPS (DX), Y3
	VMOVUPS -4(DX), Y4
	VMOVUPS 4(DX), Y5
	VMOVUPS (BX), Y6
	VMOVUPS -4(BX), Y7
	VMOVUPS 4(BX), Y8

	VADDPS Y4, Y0, Y9
	VADDPS Y6, Y9, Y9
	VADDPS Y5, Y9, Y9

	VADDPS Y2, Y1, Y10
	VADDPS Y7, Y10, Y10
	VADDPS Y8, Y10, Y10

	VMULPS Y13, Y3, Y11
	VMULPS Y14, Y9, Y12
	VADDPS Y12, Y11, Y11
	VMULPS Y15, Y10, Y12
	VADDPS Y12, Y11, Y11

	VMOVUPS Y11, (DI)

	ADDQ $32, DI
	ADDQ $32, SI
	ADDQ $32, DX
	ADDQ $32, BX
	SUBQ $1, CX
	JNZ  loop

	VZEROUPPER

done:
	RET
