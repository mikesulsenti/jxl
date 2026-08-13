//go:build amd64 && !noasm

#include "textflag.h"

#define NEIGH(A0, A1, A2)          \
	VMOVUPS A0, Y8             \
	VMOVUPS A1, Y9             \
	VMOVUPS A2, Y10            \
	VSUBPS  Y0, Y8, Y11        \
	VANDPS  128(AX), Y11, Y11  \
	VMULPS  0(AX), Y11, Y11    \
	VSUBPS  Y1, Y9, Y12        \
	VANDPS  128(AX), Y12, Y12  \
	VMULPS  32(AX), Y12, Y12   \
	VADDPS  Y12, Y11, Y11      \
	VSUBPS  Y2, Y10, Y12       \
	VANDPS  128(AX), Y12, Y12  \
	VMULPS  64(AX), Y12, Y12   \
	VADDPS  Y12, Y11, Y11      \
	VMULPS  Y7, Y11, Y11       \
	VADDPS  96(AX), Y11, Y11   \
	VXORPS  Y12, Y12, Y12      \
	VMAXPS  Y12, Y11, Y11      \
	VADDPS  Y11, Y6, Y6        \
	VMULPS  Y11, Y8, Y12       \
	VADDPS  Y12, Y3, Y3        \
	VMULPS  Y11, Y9, Y12       \
	VADDPS  Y12, Y4, Y4        \
	VMULPS  Y11, Y10, Y12      \
	VADDPS  Y12, Y5, Y5

// func epf2AVX2(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
//	k *[6][8]float32, y, x0, x1 int)
//
// One row of the third EPF pass over [x0, x1), which the caller keeps one
// pixel inside the image on every side. k holds the three channel scales, one,
// the absolute-value mask and the eight border multipliers, all splatted, so
// every constant is a memory operand and the sixteen registers go to the four
// neighbours and the three accumulators.
TEXT ·epf2AVX2(SB), NOSPLIT, $0-72
	MOVQ stride+16(FP), BX
	SHLQ $2, BX

	MOVQ  y+48(FP), R12
	SUBQ  $1, R12
	IMULQ BX, R12
	MOVQ  x0+56(FP), DX
	SHLQ  $2, DX
	ADDQ  DX, R12

	MOVQ src+8(FP), R9
	MOVQ (R9), DI
	ADDQ R12, DI
	MOVQ 8(R9), SI
	ADDQ R12, SI
	MOVQ 16(R9), R8
	ADDQ R12, R8

	ADDQ BX, R12

	MOVQ dst+0(FP), R13
	MOVQ (R13), R9
	ADDQ R12, R9
	MOVQ 8(R13), R10
	ADDQ R12, R10
	MOVQ 16(R13), R11
	ADDQ R12, R11

	MOVQ  y+48(FP), R12
	SARQ  $3, R12
	IMULQ sigmaStride+32(FP), R12
	MOVQ  x0+56(FP), DX
	SARQ  $3, DX
	ADDQ  DX, R12
	SHLQ  $2, R12
	MOVQ  sigma+24(FP), DX
	ADDQ  R12, DX

	MOVQ x1+64(FP), CX
	SUBQ x0+56(FP), CX
	SHRQ $3, CX
	JZ   done

	MOVQ BX, R13
	SHLQ $1, R13

	MOVQ k+40(FP), AX

loop:
	MOVSS   (DX), X11
	MOVL    $0xc079ef80, R12
	MOVQ    R12, X12
	UCOMISS X12, X11
	JCS     copy

	VBROADCASTSS (DX), Y7
	VMULPS       160(AX), Y7, Y7

	VMOVUPS (DI)(BX*1), Y0
	VMOVUPS (SI)(BX*1), Y1
	VMOVUPS (R8)(BX*1), Y2

	VMOVAPS Y0, Y3
	VMOVAPS Y1, Y4
	VMOVAPS Y2, Y5
	VMOVUPS 96(AX), Y6

	NEIGH((DI), (SI), (R8))
	NEIGH(-4(DI)(BX*1), -4(SI)(BX*1), -4(R8)(BX*1))
	NEIGH(4(DI)(BX*1), 4(SI)(BX*1), 4(R8)(BX*1))
	NEIGH((DI)(R13*1), (SI)(R13*1), (R8)(R13*1))

	VDIVPS  Y6, Y3, Y3
	VMOVUPS Y3, (R9)
	VDIVPS  Y6, Y4, Y4
	VMOVUPS Y4, (R10)
	VDIVPS  Y6, Y5, Y5
	VMOVUPS Y5, (R11)

	JMP next

copy:
	VMOVUPS (DI)(BX*1), Y0
	VMOVUPS Y0, (R9)
	VMOVUPS (SI)(BX*1), Y0
	VMOVUPS Y0, (R10)
	VMOVUPS (R8)(BX*1), Y0
	VMOVUPS Y0, (R11)

next:
	ADDQ $32, DI
	ADDQ $32, SI
	ADDQ $32, R8
	ADDQ $32, R9
	ADDQ $32, R10
	ADDQ $32, R11
	ADDQ $4, DX
	SUBQ $1, CX
	JNZ  loop

	VZEROUPPER

done:
	RET
