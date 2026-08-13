//go:build arm64 && !noasm

#include "textflag.h"

#define FSUB4S(Vd, Vn, Vm) WORD $(0x4ea0d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMUL4S(Vd, Vn, Vm) WORD $(0x6e20dc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FDIV4S(Vd, Vn, Vm) WORD $(0x6e20fc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMLA4S(Vd, Vn, Vm) WORD $(0x4e20cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define BIC16B(Vd, Vn, Vm) WORD $(0x4e601c00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define SCVTF4S(Vd, Vn)    WORD $(0x4e21d800 | ((Vn) << 5) | (Vd))

#define BIAS(C, BN)                  \
	VLD1 (C), [V0.S4]            \
	SCVTF4S(1, 0)                \
	FDIV4S(2, 16, 1)             \
	FSUB4S(2, 1, 2)              \
	FMUL4S(4, 1, BN)             \
	VCMEQ V25.S4, V0.S4, V3.S4   \
	VCMEQ V26.S4, V0.S4, V6.S4   \
	VORR  V6.B16, V3.B16, V3.B16 \
	VCMEQ V27.S4, V0.S4, V6.S4   \
	VORR  V6.B16, V3.B16, V7.B16 \
	BIC16B(2, 2, 7)              \
	VAND  V3.B16, V4.B16, V4.B16 \
	VORR  V4.B16, V2.B16, V2.B16

#define SIDE(C, BN, M, SN, CN, O) \
	BIAS(C, BN)               \
	VLD1 (M), [V6.S4]         \
	FMUL4S(2, 2, 6)           \
	FMUL4S(2, 2, SN)          \
	FMLA4S(2, 5, CN)          \
	VST1 [V2.S4], (O)

#define BCAST(OFF, V)      \
	MOVWU OFF(R0), R11 \
	VDUP  R11, V.S4

// func dequantNEON(a *dequantArgs)
//
// The dequantiser inner loop, four lanes at a time. adjustQuantBias is two
// compares rather than four branches: for |q| == 1 the answer is
// bias * float32(q), since float32(q) is exactly plus or minus one there.
TEXT ·dequantNEON(SB), NOSPLIT, $0-8
	MOVD a+0(FP), R0

	MOVD 0(R0), R1
	MOVD 8(R0), R2
	MOVD 16(R0), R3
	MOVD 24(R0), R4
	MOVD 32(R0), R5
	MOVD 40(R0), R6
	MOVD 48(R0), R7
	MOVD 56(R0), R8
	MOVD 64(R0), R9
	MOVD 72(R0), R10

	CBZ R10, done

	BCAST(92, V16)
	BCAST(80, V17)
	BCAST(84, V18)
	BCAST(88, V19)
	BCAST(96, V20)
	BCAST(100, V21)
	BCAST(104, V22)
	BCAST(108, V23)
	BCAST(112, V24)

	MOVD $1, R11
	VDUP R11, V25.S4
	MOVD $-1, R11
	VDUP R11, V26.S4
	VEOR V27.B16, V27.B16, V27.B16

	LSR $2, R10

loop:
	BIAS(R5, 18)
	VLD1 (R8), [V6.S4]
	FMUL4S(5, 2, 6)
	FMUL4S(5, 5, 21)
	VST1 [V5.S4], (R2)

	SIDE(R4, 17, R7, 20, 23, R1)
	SIDE(R6, 19, R9, 22, 24, R3)

	ADD  $16, R1
	ADD  $16, R2
	ADD  $16, R3
	ADD  $16, R4
	ADD  $16, R5
	ADD  $16, R6
	ADD  $16, R7
	ADD  $16, R8
	ADD  $16, R9
	SUB  $1, R10
	CBNZ R10, loop

done:
	RET
