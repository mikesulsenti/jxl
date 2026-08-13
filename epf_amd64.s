//go:build amd64 && !noasm

#include "textflag.h"

#define SADPLANE(P, SOFF)            \
	VMOVUPS      (P)(BX*1), Y4    \
	VMOVUPS      -4(P)(R13*1), Y5 \
	VMOVUPS      (P)(R13*1), Y6   \
	VMOVUPS      4(P)(R13*1), Y7  \
	VMOVUPS      (P)(R14*1), Y8   \
	VBROADCASTSS SOFF(R12), Y9    \
	                              \
	VSUBPS -0(P), Y4, Y10         \
	VANDPS Y15, Y10, Y10          \
	VSUBPS -4(P)(BX*1), Y5, Y11   \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS Y4, Y6, Y11            \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS 4(P)(BX*1), Y7, Y11    \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS Y8, Y6, Y11            \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VMULPS Y9, Y10, Y10           \
	VADDPS Y10, Y0, Y0            \
	                              \
	VSUBPS -4(P)(BX*1), Y4, Y10   \
	VANDPS Y15, Y10, Y10          \
	VSUBPS -8(P)(R13*1), Y5, Y11  \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS Y5, Y6, Y11            \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS Y7, Y6, Y11            \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS -4(P)(R14*1), Y8, Y11  \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VMULPS Y9, Y10, Y10           \
	VADDPS Y10, Y1, Y1            \
	                              \
	VSUBPS 4(P)(BX*1), Y4, Y10    \
	VANDPS Y15, Y10, Y10          \
	VSUBPS Y5, Y6, Y11            \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS Y7, Y6, Y11            \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS 8(P)(R13*1), Y7, Y11   \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS 4(P)(R14*1), Y8, Y11   \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VMULPS Y9, Y10, Y10           \
	VADDPS Y10, Y2, Y2            \
	                              \
	VSUBPS Y4, Y6, Y10            \
	VANDPS Y15, Y10, Y10          \
	VSUBPS -4(P)(R14*1), Y5, Y11  \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS Y8, Y6, Y11            \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS 4(P)(R14*1), Y7, Y11   \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VSUBPS (P)(R15*1), Y8, Y11    \
	VANDPS Y15, Y11, Y11          \
	VADDPS Y11, Y10, Y10          \
	VMULPS Y9, Y10, Y10           \
	VADDPS Y10, Y3, Y3

#define OUTPLANE(P, D)               \
	VMOVUPS (P)(R13*1), Y10      \
	VMULPS  (P)(R14*1), Y3, Y11  \
	VADDPS  Y11, Y10, Y10        \
	VMULPS  4(P)(R13*1), Y2, Y11 \
	VADDPS  Y11, Y10, Y10        \
	VMULPS  -4(P)(R13*1), Y1, Y11 \
	VADDPS  Y11, Y10, Y10        \
	VMULPS  (P)(BX*1), Y0, Y11   \
	VADDPS  Y11, Y10, Y10        \
	VDIVPS  Y14, Y10, Y10        \
	VMOVUPS Y10, (D)

// func epf1AVX2(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
//	mul, scales *float32, y, x0, x1 int)
//
// One row of the second EPF pass over [x0, x1), which the caller keeps inside
// the image by two pixels on every side so the mirror tables are not needed.
// x0 and x1 are multiples of eight, and sigma is constant across each group of
// eight because a block is eight wide.
TEXT ·epf1AVX2(SB), NOSPLIT, $0-80
	MOVQ stride+16(FP), BX
	SHLQ $2, BX

	MOVQ  y+56(FP), AX
	SUBQ  $2, AX
	IMULQ BX, AX
	MOVQ  x0+64(FP), DX
	SHLQ  $2, DX
	ADDQ  DX, AX

	MOVQ src+8(FP), R9
	MOVQ (R9), DI
	ADDQ AX, DI
	MOVQ 8(R9), SI
	ADDQ AX, SI
	MOVQ 16(R9), R8
	ADDQ AX, R8

	LEAQ (AX)(BX*2), DX

	MOVQ dst+0(FP), R12
	MOVQ (R12), R9
	ADDQ DX, R9
	MOVQ 8(R12), R10
	ADDQ DX, R10
	MOVQ 16(R12), R11
	ADDQ DX, R11

	MOVQ  y+56(FP), AX
	SARQ  $3, AX
	IMULQ sigmaStride+32(FP), AX
	MOVQ  x0+64(FP), DX
	SARQ  $3, DX
	ADDQ  DX, AX
	SHLQ  $2, AX
	MOVQ  sigma+24(FP), DX
	ADDQ  AX, DX

	MOVQ x1+72(FP), CX
	SUBQ x0+64(FP), CX
	SHRQ $3, CX
	JZ   done

	MOVQ BX, R13
	SHLQ $1, R13
	LEAQ (BX)(R13*1), R14
	MOVQ R13, R15
	SHLQ $1, R15

	MOVQ scales+48(FP), R12

	VPCMPEQD Y15, Y15, Y15
	VPSRLD   $1, Y15, Y15

	MOVL         $0x3f800000, AX
	MOVQ         AX, X12
	VPBROADCASTD X12, Y12

	MOVQ    mul+40(FP), AX
	VMOVUPS (AX), Y13

loop:
	MOVSS   (DX), X10
	MOVL    $0xc079ef80, AX
	MOVQ    AX, X11
	UCOMISS X11, X10
	JCS     copy

	VXORPS Y0, Y0, Y0
	VXORPS Y1, Y1, Y1
	VXORPS Y2, Y2, Y2
	VXORPS Y3, Y3, Y3

	SADPLANE(DI, 0)
	SADPLANE(SI, 4)
	SADPLANE(R8, 8)

	VBROADCASTSS (DX), Y9
	VMULPS       Y13, Y9, Y9
	VXORPS       Y11, Y11, Y11

	VMULPS Y9, Y0, Y0
	VADDPS Y12, Y0, Y0
	VMAXPS Y11, Y0, Y0
	VMULPS Y9, Y1, Y1
	VADDPS Y12, Y1, Y1
	VMAXPS Y11, Y1, Y1
	VMULPS Y9, Y2, Y2
	VADDPS Y12, Y2, Y2
	VMAXPS Y11, Y2, Y2
	VMULPS Y9, Y3, Y3
	VADDPS Y12, Y3, Y3
	VMAXPS Y11, Y3, Y3

	VMOVAPS Y12, Y14
	VADDPS  Y0, Y14, Y14
	VADDPS  Y1, Y14, Y14
	VADDPS  Y2, Y14, Y14
	VADDPS  Y3, Y14, Y14

	OUTPLANE(DI, R9)
	OUTPLANE(SI, R10)
	OUTPLANE(R8, R11)

	JMP next

copy:
	VMOVUPS (DI)(R13*1), Y10
	VMOVUPS Y10, (R9)
	VMOVUPS (SI)(R13*1), Y10
	VMOVUPS Y10, (R10)
	VMOVUPS (R8)(R13*1), Y10
	VMOVUPS Y10, (R11)

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
