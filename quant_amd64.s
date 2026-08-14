//go:build amd64 && !noasm

#include "textflag.h"

// ADJUST is adjustQuantBias branchlessly: zero for q == 0, bias * float32(q)
// for |q| == 1 since float32(q) is then exactly plus or minus one, and
// v - biases[3]/v otherwise. Y8 holds biases[3], Y11 the channel bias.
#define ADJUST(Q, TMP, OUT)   \
	VCVTDQ2PS Q, TMP      \
	VDIVPS    TMP, Y8, OUT \
	VSUBPS    OUT, TMP, OUT \
	VPCMPEQD  Y15, Q, Y5  \
	VPABSD    Q, Y6       \
	VPCMPEQD  Y14, Y6, Y6 \
	VMULPS    TMP, Y11, TMP \
	VPOR      Y6, Y5, Y5  \
	VANDNPS   OUT, Y5, OUT \
	VANDPS    Y6, TMP, TMP \
	VORPS     TMP, OUT, OUT

// CAND evaluates candidate Q, leaving adjust(Q) in A and |adjust(Q) - Y9| in E.
#define CAND(Q, A, E)   \
	ADJUST(Q, Y4, A) \
	VSUBPS Y9, A, E \
	VANDPS 64(R14), E, E

// func quantizeAVX2(a *quantArgs, k *[4][8]float32)
//
// The forward quantiser over a.n coefficients, n a multiple of eight, and the
// mirror of dequantAVX2. Candidates are tried near-1, near, near+1 with a
// strict less-than so a tie keeps the earlier one, which is the order the
// scalar loop uses.
TEXT ·quantizeAVX2(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ k+8(FP), R14

	MOVQ (AX), R8
	MOVQ 8(AX), R9
	MOVQ 16(AX), R10

	MOVQ 24(AX), R11
	MOVQ 32(AX), R12
	MOVQ 40(AX), R13

	MOVQ 48(AX), BX
	MOVQ 56(AX), CX
	MOVQ 64(AX), DX

	MOVQ 72(AX), SI

	VBROADCASTSS 92(AX), Y8
	VXORPS       Y15, Y15, Y15
	VMOVUPS      96(R14), Y14

loop:
	// Y channel: target = in1 / (my * scaledY)
	VBROADCASTSS 100(AX), Y0
	VMULPS       (CX), Y0, Y7
	VMOVUPS      (R9), Y9
	VDIVPS       Y7, Y9, Y9

	VBROADCASTSS 84(AX), Y11

	VCMPPS $4, Y9, Y9, Y10
	VMAXPS 128(R14), Y9, Y9
	VMINPS 32(R14), Y9, Y9

	VCVTTPS2DQ Y9, Y12
	VCVTDQ2PS  Y12, Y13
	VSUBPS     Y13, Y9, Y13
	VCMPPS     $5, 0(R14), Y13, Y2
	VCMPPS     $2, 160(R14), Y13, Y3
	VPAND      Y14, Y2, Y2
	VPAND      Y14, Y3, Y3
	VPSUBD     Y3, Y2, Y2
	VPADDD     Y2, Y12, Y12

	VPSUBD Y14, Y12, Y3
	CAND(Y3, Y1, Y2)
	VMOVDQU Y3, Y0

	CAND(Y12, Y13, Y7)
	VCMPPS    $1, Y2, Y7, Y6
	VBLENDVPS Y6, Y7, Y2, Y2
	VBLENDVPS Y6, Y13, Y1, Y1
	VPBLENDVB Y6, Y12, Y0, Y0

	VPADDD Y14, Y12, Y3
	CAND(Y3, Y13, Y7)
	VCMPPS    $1, Y2, Y7, Y6
	VBLENDVPS Y6, Y13, Y1, Y1
	VPBLENDVB Y6, Y3, Y0, Y0

	VPANDN  Y0, Y10, Y0
	VMOVDQU Y0, (R12)

	VANDNPS Y1, Y10, Y1

	// dy = adjust(qy) * my * scaledY, grouped as the scalar loop groups it
	VMULPS       (CX), Y1, Y7
	VBROADCASTSS 100(AX), Y0
	VMULPS       Y0, Y7, Y7

	// X channel: target = (in0 - xcc*dy) / (mx * scaledX)
	VBROADCASTSS 108(AX), Y0
	VMULPS       Y7, Y0, Y0
	VMOVUPS      (R8), Y9
	VSUBPS       Y0, Y9, Y9
	VBROADCASTSS 96(AX), Y0
	VMULPS       (BX), Y0, Y0
	VDIVPS       Y0, Y9, Y9

	VBROADCASTSS 80(AX), Y11

	VCMPPS $4, Y9, Y9, Y10
	VMAXPS 128(R14), Y9, Y9
	VMINPS 32(R14), Y9, Y9

	VCVTTPS2DQ Y9, Y12
	VCVTDQ2PS  Y12, Y13
	VSUBPS     Y13, Y9, Y13
	VCMPPS     $5, 0(R14), Y13, Y2
	VCMPPS     $2, 160(R14), Y13, Y3
	VPAND      Y14, Y2, Y2
	VPAND      Y14, Y3, Y3
	VPSUBD     Y3, Y2, Y2
	VPADDD     Y2, Y12, Y12

	VPSUBD Y14, Y12, Y3
	CAND(Y3, Y1, Y2)
	VMOVDQU Y3, Y0

	CAND(Y12, Y13, Y1)
	VCMPPS    $1, Y2, Y1, Y6
	VBLENDVPS Y6, Y1, Y2, Y2
	VPBLENDVB Y6, Y12, Y0, Y0

	VPADDD Y14, Y12, Y3
	CAND(Y3, Y13, Y1)
	VCMPPS    $1, Y2, Y1, Y6
	VPBLENDVB Y6, Y3, Y0, Y0

	VPANDN  Y0, Y10, Y0
	VMOVDQU Y0, (R11)

	// B channel: target = (in2 - bcc*dy) / (mb * scaledB)
	VBROADCASTSS 112(AX), Y0
	VMULPS       Y7, Y0, Y0
	VMOVUPS      (R10), Y9
	VSUBPS       Y0, Y9, Y9
	VBROADCASTSS 104(AX), Y0
	VMULPS       (DX), Y0, Y0
	VDIVPS       Y0, Y9, Y9

	VBROADCASTSS 88(AX), Y11

	VCMPPS $4, Y9, Y9, Y10
	VMAXPS 128(R14), Y9, Y9
	VMINPS 32(R14), Y9, Y9

	VCVTTPS2DQ Y9, Y12
	VCVTDQ2PS  Y12, Y13
	VSUBPS     Y13, Y9, Y13
	VCMPPS     $5, 0(R14), Y13, Y2
	VCMPPS     $2, 160(R14), Y13, Y3
	VPAND      Y14, Y2, Y2
	VPAND      Y14, Y3, Y3
	VPSUBD     Y3, Y2, Y2
	VPADDD     Y2, Y12, Y12

	VPSUBD Y14, Y12, Y3
	CAND(Y3, Y1, Y2)
	VMOVDQU Y3, Y0

	CAND(Y12, Y13, Y1)
	VCMPPS    $1, Y2, Y1, Y6
	VBLENDVPS Y6, Y1, Y2, Y2
	VPBLENDVB Y6, Y12, Y0, Y0

	VPADDD Y14, Y12, Y3
	CAND(Y3, Y13, Y1)
	VCMPPS    $1, Y2, Y1, Y6
	VPBLENDVB Y6, Y3, Y0, Y0

	VPANDN  Y0, Y10, Y0
	VMOVDQU Y0, (R13)

	ADDQ $32, R8
	ADDQ $32, R9
	ADDQ $32, R10
	ADDQ $32, R11
	ADDQ $32, R12
	ADDQ $32, R13
	ADDQ $32, BX
	ADDQ $32, CX
	ADDQ $32, DX

	SUBQ $8, SI
	JNZ  loop

	VZEROUPPER
	RET
