//go:build arm64 && !noasm

#include "textflag.h"

#define FADD4S(Vd, Vn, Vm)  WORD $(0x4e20d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FSUB4S(Vd, Vn, Vm)  WORD $(0x4ea0d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMUL4S(Vd, Vn, Vm)  WORD $(0x6e20dc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FDIV4S(Vd, Vn, Vm)  WORD $(0x6e20fc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMIN4S(Vd, Vn, Vm)  WORD $(0x4ea0f400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMAX4S(Vd, Vn, Vm)  WORD $(0x4e20f400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FABS4S(Vd, Vn)      WORD $(0x4ea0f800 | ((Vn) << 5) | (Vd))
#define FCMEQ4S(Vd, Vn, Vm) WORD $(0x4e20e400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FCMGE4S(Vd, Vn, Vm) WORD $(0x6e20e400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FCMGT4S(Vd, Vn, Vm) WORD $(0x6ea0e400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FCVTZS4S(Vd, Vn)    WORD $(0x4ea1b800 | ((Vn) << 5) | (Vd))
#define SCVTF4S(Vd, Vn)     WORD $(0x4e21d800 | ((Vn) << 5) | (Vd))
#define ADD4S(Vd, Vn, Vm)   WORD $(0x4ea08400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define SUB4S(Vd, Vn, Vm)   WORD $(0x6ea08400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define BIC16B(Vd, Vn, Vm)  WORD $(0x4e601c00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMLS4S(Vd, Vn, Vm)  WORD $(0x4ea0cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))

#define MOV16B(Vd, Vn)      WORD $(0x4ea01c00 | ((Vn) << 16) | ((Vn) << 5) | (Vd))

// ADJUST is adjustQuantBias for the integers in V6 with the channel bias BC,
// leaving the result in V7. It clobbers V10, V11, V12, V14, V15. For |q| == 1
// the answer is bias * float32(q), since float32(q) is exactly plus or minus one.
#define ADJUST(BC)                      \
	SCVTF4S(10, 6)                  \
	FDIV4S(7, 16, 10)               \
	FSUB4S(7, 10, 7)                \
	FMUL4S(12, 10, BC)              \
	VCMEQ V25.S4, V6.S4, V11.S4     \
	VCMEQ V26.S4, V6.S4, V14.S4     \
	VORR  V14.B16, V11.B16, V11.B16 \
	VCMEQ V27.S4, V6.S4, V14.S4     \
	VORR  V14.B16, V11.B16, V15.B16 \
	BIC16B(7, 7, 15)                \
	VAND  V11.B16, V12.B16, V12.B16 \
	VORR  V12.B16, V7.B16, V7.B16

// CAND evaluates the candidate in V6 into adjust V7 and error V8 against V0.
#define CAND(BC)   \
	ADJUST(BC) \
	FSUB4S(8, 7, 0) \
	FABS4S(8, 8)

// PICK keeps V6/V7/V8 over V3/V4/V5 when the error is strictly smaller, so a
// tie keeps the earlier candidate, which is the order the scalar loop uses.
#define PICK                          \
	FCMGT4S(9, 5, 8)              \
	VAND  V9.B16, V6.B16, V10.B16 \
	BIC16B(3, 3, 9)               \
	VORR  V10.B16, V3.B16, V3.B16 \
	VAND  V9.B16, V7.B16, V10.B16 \
	BIC16B(4, 4, 9)               \
	VORR  V10.B16, V4.B16, V4.B16 \
	VAND  V9.B16, V8.B16, V10.B16 \
	BIC16B(5, 5, 9)               \
	VORR  V10.B16, V5.B16, V5.B16

// QUANT quantises the target in V0 with channel bias BC, leaving the chosen
// integers in V3 and the adjusted float of that same candidate in V4.
#define QUANT(BC)                    \
	FCMEQ4S(1, 0, 0)             \
	FMAX4S(0, 0, 31)             \
	FMIN4S(0, 0, 30)             \
	FCVTZS4S(2, 0)               \
	SCVTF4S(6, 2)                \
	FSUB4S(6, 0, 6)              \
	FCMGE4S(7, 6, 28)            \
	FCMGE4S(8, 29, 6)            \
	VAND V25.B16, V7.B16, V7.B16 \
	VAND V25.B16, V8.B16, V8.B16 \
	SUB4S(7, 7, 8)               \
	ADD4S(2, 2, 7)               \
	                             \
	SUB4S(6, 2, 25)              \
	CAND(BC)                     \
	MOV16B(3, 6)                 \
	MOV16B(4, 7)                 \
	MOV16B(5, 8)                 \
	                             \
	MOV16B(6, 2)                 \
	CAND(BC)                     \
	PICK                         \
	                             \
	ADD4S(6, 2, 25)              \
	CAND(BC)                     \
	PICK                         \
	                             \
	VAND V1.B16, V3.B16, V3.B16  \
	VAND V1.B16, V4.B16, V4.B16

#define BCAST(OFF, V)      \
	MOVWU OFF(R0), R11 \
	VDUP  R11, V.S4

// func quantizeNEON(a *quantArgs)
//
// The forward quantiser, four lanes at a time, and the mirror of dequantNEON.
TEXT ·quantizeNEON(SB), NOSPLIT, $0-8
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

	MOVD $0x3F000000, R11
	VDUP R11, V28.S4
	MOVD $0xBF000000, R11
	VDUP R11, V29.S4
	MOVD $0x4E800000, R11
	VDUP R11, V30.S4
	MOVD $0xCE800000, R11
	VDUP R11, V31.S4

	LSR $2, R10

loop:
	// Y channel: target = in1 / (my * scaledY)
	VLD1 (R8), [V6.S4]
	FMUL4S(6, 6, 21)
	VLD1 (R2), [V0.S4]
	FDIV4S(0, 0, 6)

	QUANT(18)

	VST1 [V3.S4], (R5)

	// dy = adjust(qy) * my * scaledY, grouped as the scalar groups it
	VLD1 (R8), [V6.S4]
	FMUL4S(13, 4, 6)
	FMUL4S(13, 13, 21)

	// X channel: target = (in0 - xcc*dy) / (mx * scaledX), fused because the
	// Go compiler contracts that subtraction into an FMSUB on arm64
	VLD1 (R1), [V0.S4]
	FMLS4S(0, 13, 23)
	VLD1 (R7), [V6.S4]
	FMUL4S(6, 6, 20)
	FDIV4S(0, 0, 6)

	QUANT(17)

	VST1 [V3.S4], (R4)

	// B channel: target = (in2 - bcc*dy) / (mb * scaledB), likewise fused
	VLD1 (R3), [V0.S4]
	FMLS4S(0, 13, 24)
	VLD1 (R9), [V6.S4]
	FMUL4S(6, 6, 22)
	FDIV4S(0, 0, 6)

	QUANT(19)

	VST1 [V3.S4], (R6)

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
