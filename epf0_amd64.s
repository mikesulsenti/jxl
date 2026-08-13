//go:build amd64 && !noasm

#include "textflag.h"

#define DIR(ACC, P, S0, S1, S2, S3, S4) \
	VMOVUPS (P)(R12*1), Y13         \
	VSUBPS  S0, Y13, Y13            \
	VANDPS  Y15, Y13, Y13           \
	VMOVUPS -4(P)(R13*1), Y12       \
	VSUBPS  S1, Y12, Y12            \
	VANDPS  Y15, Y12, Y12           \
	VADDPS  Y12, Y13, Y13           \
	VMOVUPS (P)(R13*1), Y12         \
	VSUBPS  S2, Y12, Y12            \
	VANDPS  Y15, Y12, Y12           \
	VADDPS  Y12, Y13, Y13           \
	VMOVUPS 4(P)(R13*1), Y12        \
	VSUBPS  S3, Y12, Y12            \
	VANDPS  Y15, Y12, Y12           \
	VADDPS  Y12, Y13, Y13           \
	VMOVUPS (P)(R12*2), Y12         \
	VSUBPS  S4, Y12, Y12            \
	VANDPS  Y15, Y12, Y12           \
	VADDPS  Y12, Y13, Y13           \
	VMULPS  Y14, Y13, Y13           \
	VADDPS  Y13, ACC, ACC

#define SADPLANE(P, SOFF)                                                                     \
	VBROADCASTSS SOFF(AX), Y14                                                            \
	DIR(Y0, P, (P), -4(P)(BX*1), (P)(BX*1), 4(P)(BX*1), (P)(R12*1))                       \
	DIR(Y1, P, -4(P)(BX*1), -8(P)(R12*1), -4(P)(R12*1), (P)(R12*1), -4(P)(R13*1))         \
	DIR(Y2, P, (P)(BX*1), -4(P)(R12*1), (P)(R12*1), 4(P)(R12*1), (P)(R13*1))              \
	DIR(Y3, P, 4(P)(BX*1), (P)(R12*1), 4(P)(R12*1), 8(P)(R12*1), 4(P)(R13*1))             \
	DIR(Y4, P, -8(P)(R12*1), -12(P)(R13*1), -8(P)(R13*1), -4(P)(R13*1), -8(P)(R12*2))     \
	DIR(Y5, P, -4(P)(R12*1), -8(P)(R13*1), -4(P)(R13*1), (P)(R13*1), -4(P)(R12*2))        \
	DIR(Y6, P, 4(P)(R12*1), (P)(R13*1), 4(P)(R13*1), 8(P)(R13*1), 4(P)(R12*2))            \
	DIR(Y7, P, 8(P)(R12*1), 4(P)(R13*1), 8(P)(R13*1), 12(P)(R13*1), 8(P)(R12*2))          \
	DIR(Y8, P, -4(P)(R13*1), -8(P)(R12*2), -4(P)(R12*2), (P)(R12*2), -4(P)(R14*1))        \
	DIR(Y9, P, (P)(R13*1), -4(P)(R12*2), (P)(R12*2), 4(P)(R12*2), (P)(R14*1))             \
	DIR(Y10, P, 4(P)(R13*1), (P)(R12*2), 4(P)(R12*2), 8(P)(R12*2), 4(P)(R14*1))           \
	DIR(Y11, P, (P)(R12*2), -4(P)(R14*1), (P)(R14*1), 4(P)(R14*1), (P)(R13*2))

#define WEIGHT(ACC)          \
	VMULPS Y14, ACC, ACC \
	VADDPS 96(AX), ACC, ACC \
	VMAXPS Y15, ACC, ACC \
	VADDPS ACC, Y13, Y13

#define TAP(M, ACC)          \
	VMULPS M, ACC, Y15   \
	VADDPS Y15, Y12, Y12

#define OUTPLANE(P, D)                \
	VMOVUPS (P)(R13*1), Y12       \
	TAP((P)(R14*1), Y11)          \
	TAP(4(P)(R12*2), Y10)         \
	TAP((P)(R12*2), Y9)           \
	TAP(-4(P)(R12*2), Y8)         \
	TAP(8(P)(R13*1), Y7)          \
	TAP(4(P)(R13*1), Y6)          \
	TAP(-4(P)(R13*1), Y5)         \
	TAP(-8(P)(R13*1), Y4)         \
	TAP(4(P)(R12*1), Y3)          \
	TAP((P)(R12*1), Y2)           \
	TAP(-4(P)(R12*1), Y1)         \
	TAP((P)(BX*1), Y0)            \
	VDIVPS  Y13, Y12, Y12         \
	VMOVUPS Y12, (D)

// func epf0AVX2(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
//	k *[6][8]float32, y, x0, x1 int)
//
// One row of the first EPF pass over [x0, x1), which the caller keeps three
// pixels inside the image on every side. The twelve directions of the diamond
// each compare the same five centre taps against their shifted copies, so the
// twelve accumulators fill Y0 through Y11 and the centre taps are re-read from
// L1 rather than held.
TEXT ·epf0AVX2(SB), NOSPLIT, $0-72
	MOVQ stride+16(FP), BX
	SHLQ $2, BX

	MOVQ  y+48(FP), AX
	SUBQ  $3, AX
	IMULQ BX, AX
	MOVQ  x0+56(FP), DX
	SHLQ  $2, DX
	ADDQ  DX, AX

	MOVQ src+8(FP), R9
	MOVQ (R9), DI
	ADDQ AX, DI
	MOVQ 8(R9), SI
	ADDQ AX, SI
	MOVQ 16(R9), R8
	ADDQ AX, R8

	LEAQ (BX)(BX*2), DX
	ADDQ DX, AX

	MOVQ dst+0(FP), R12
	MOVQ (R12), R9
	ADDQ AX, R9
	MOVQ 8(R12), R10
	ADDQ AX, R10
	MOVQ 16(R12), R11
	ADDQ AX, R11

	MOVQ  y+48(FP), AX
	SARQ  $3, AX
	IMULQ sigmaStride+32(FP), AX
	MOVQ  x0+56(FP), DX
	SARQ  $3, DX
	ADDQ  DX, AX
	SHLQ  $2, AX
	MOVQ  sigma+24(FP), DX
	ADDQ  AX, DX

	MOVQ x1+64(FP), CX
	SUBQ x0+56(FP), CX
	SHRQ $3, CX
	JZ   done

	MOVQ BX, R12
	SHLQ $1, R12
	LEAQ (BX)(R12*1), R13
	LEAQ (BX)(R12*2), R14

	MOVQ k+40(FP), AX

loop:
	MOVSS   (DX), X11
	MOVL    $0xc079ef80, R15
	MOVQ    R15, X12
	UCOMISS X12, X11
	JCS     copy

	VXORPS Y0, Y0, Y0
	VXORPS Y1, Y1, Y1
	VXORPS Y2, Y2, Y2
	VXORPS Y3, Y3, Y3
	VXORPS Y4, Y4, Y4
	VXORPS Y5, Y5, Y5
	VXORPS Y6, Y6, Y6
	VXORPS Y7, Y7, Y7
	VXORPS Y8, Y8, Y8
	VXORPS Y9, Y9, Y9
	VXORPS Y10, Y10, Y10
	VXORPS Y11, Y11, Y11

	VMOVUPS 128(AX), Y15

	SADPLANE(DI, 0)
	SADPLANE(SI, 32)
	SADPLANE(R8, 64)

	VBROADCASTSS (DX), Y14
	VMULPS       160(AX), Y14, Y14

	VXORPS  Y15, Y15, Y15
	VMOVUPS 96(AX), Y13

	WEIGHT(Y0)
	WEIGHT(Y1)
	WEIGHT(Y2)
	WEIGHT(Y3)
	WEIGHT(Y4)
	WEIGHT(Y5)
	WEIGHT(Y6)
	WEIGHT(Y7)
	WEIGHT(Y8)
	WEIGHT(Y9)
	WEIGHT(Y10)
	WEIGHT(Y11)

	OUTPLANE(DI, R9)
	OUTPLANE(SI, R10)
	OUTPLANE(R8, R11)

	JMP next

copy:
	VMOVUPS (DI)(R13*1), Y0
	VMOVUPS Y0, (R9)
	VMOVUPS (SI)(R13*1), Y0
	VMOVUPS Y0, (R10)
	VMOVUPS (R8)(R13*1), Y0
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
