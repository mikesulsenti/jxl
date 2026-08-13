//go:build amd64 && !noasm

#include "textflag.h"

#define BIAS(C, BOFF)             \
	VMOVDQU   (C), Y0         \
	VCVTDQ2PS Y0, Y1          \
	VDIVPS    Y1, Y8, Y2      \
	VSUBPS    Y2, Y1, Y2      \
	VPCMPEQD  Y15, Y0, Y3     \
	VPABSD    Y0, Y4          \
	VPCMPEQD  Y14, Y4, Y4     \
	VBROADCASTSS BOFF(AX), Y5 \
	VMULPS    Y1, Y5, Y5      \
	VPOR      Y4, Y3, Y3      \
	VANDNPS   Y2, Y3, Y2      \
	VANDPS    Y4, Y5, Y5      \
	VORPS     Y5, Y2, Y2

#define SIDE(C, BOFF, M, S, CC, O) \
	BIAS(C, BOFF)              \
	VMULPS (M), Y2, Y7         \
	VMULPS S, Y7, Y7           \
	VMULPS CC, Y6, Y2          \
	VADDPS Y7, Y2, Y2          \
	VMOVUPS Y2, (O)

// func dequantAVX2(a *dequantArgs)
//
// The dequantiser inner loop over a.n coefficients, n a multiple of eight.
// adjustQuantBias is two compares rather than four branches: for |q| == 1 the
// answer is bias * float32(q), since float32(q) is exactly plus or minus one,
// which is why the negated biases never have to be held anywhere.
TEXT ·dequantAVX2(SB), NOSPLIT, $0-8
	MOVQ a+0(FP), AX

	MOVQ (AX), R8
	MOVQ 8(AX), R9
	MOVQ 16(AX), R10
	MOVQ 24(AX), R11
	MOVQ 32(AX), R12
	MOVQ 40(AX), R13
	MOVQ 48(AX), DI
	MOVQ 56(AX), SI
	MOVQ 64(AX), BX
	MOVQ 72(AX), CX

	SHRQ $3, CX
	JZ   done

	VBROADCASTSS 92(AX), Y8
	VBROADCASTSS 96(AX), Y9
	VBROADCASTSS 100(AX), Y10
	VBROADCASTSS 104(AX), Y11
	VBROADCASTSS 108(AX), Y12
	VBROADCASTSS 112(AX), Y13

	MOVL         $1, DX
	MOVQ         DX, X14
	VPBROADCASTD X14, Y14

	VPXOR Y15, Y15, Y15

loop:
	BIAS(R12, 84)
	VMULPS  (SI), Y2, Y6
	VMULPS  Y10, Y6, Y6
	VMOVUPS Y6, (R9)

	SIDE(R11, 80, DI, Y9, Y12, R8)
	SIDE(R13, 88, BX, Y11, Y13, R10)

	ADDQ $32, R8
	ADDQ $32, R9
	ADDQ $32, R10
	ADDQ $32, R11
	ADDQ $32, R12
	ADDQ $32, R13
	ADDQ $32, DI
	ADDQ $32, SI
	ADDQ $32, BX
	SUBQ $1, CX
	JNZ  loop

	VZEROUPPER

done:
	RET

#define BIAS512(C, B)                  \
	VMOVDQU32 (C), Z0              \
	VCVTDQ2PS Z0, Z1               \
	VDIVPS    Z1, Z8, Z2           \
	VSUBPS    Z2, Z1, Z2           \
	VPTESTNMD Z0, Z0, K1           \
	VPABSD    Z0, Z4               \
	VPCMPEQD  Z14, Z4, K2          \
	VMULPS    B, Z1, K2, Z2        \
	VMOVAPS   Z15, K1, Z2

#define SIDE512(C, B, M, S, CC, O) \
	BIAS512(C, B)              \
	VMULPS  (M), Z2, Z7        \
	VMULPS  S, Z7, Z7          \
	VMULPS  CC, Z6, Z2         \
	VADDPS  Z7, Z2, Z2         \
	VMOVUPS Z2, (O)

// func dequant512(a *dequantArgs)
//
// dequantAVX2 over sixteen coefficients at a time. The four-way select of
// adjustQuantBias becomes two mask registers, so the AND, ANDN and OR of the
// AVX2 version turn into a merge-masked multiply and a merge-masked move.
TEXT ·dequant512(SB), NOSPLIT, $0-8
	MOVQ a+0(FP), AX

	MOVQ (AX), R8
	MOVQ 8(AX), R9
	MOVQ 16(AX), R10
	MOVQ 24(AX), R11
	MOVQ 32(AX), R12
	MOVQ 40(AX), R13
	MOVQ 48(AX), DI
	MOVQ 56(AX), SI
	MOVQ 64(AX), BX
	MOVQ 72(AX), CX

	SHRQ $4, CX
	JZ   done512

	VBROADCASTSS 92(AX), Z8
	VBROADCASTSS 96(AX), Z9
	VBROADCASTSS 100(AX), Z10
	VBROADCASTSS 104(AX), Z11
	VBROADCASTSS 108(AX), Z12
	VBROADCASTSS 112(AX), Z13
	VBROADCASTSS 80(AX), Z16
	VBROADCASTSS 84(AX), Z17
	VBROADCASTSS 88(AX), Z18

	MOVL         $1, DX
	MOVQ         DX, X14
	VPBROADCASTD X14, Z14

	VPXORD Z15, Z15, Z15

loop512:
	BIAS512(R12, Z17)
	VMULPS  (SI), Z2, Z6
	VMULPS  Z10, Z6, Z6
	VMOVUPS Z6, (R9)

	SIDE512(R11, Z16, DI, Z9, Z12, R8)
	SIDE512(R13, Z18, BX, Z11, Z13, R10)

	ADDQ $64, R8
	ADDQ $64, R9
	ADDQ $64, R10
	ADDQ $64, R11
	ADDQ $64, R12
	ADDQ $64, R13
	ADDQ $64, DI
	ADDQ $64, SI
	ADDQ $64, BX
	SUBQ $1, CX
	JNZ  loop512

	VZEROUPPER

done512:
	RET
