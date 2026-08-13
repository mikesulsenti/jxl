//go:build arm64 && !noasm

#include "textflag.h"

#define FADD4S(Vd, Vn, Vm) WORD $(0x4e20d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FSUB4S(Vd, Vn, Vm) WORD $(0x4ea0d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMUL4S(Vd, Vn, Vm) WORD $(0x6e20dc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FDIV4S(Vd, Vn, Vm) WORD $(0x6e20fc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMAX4S(Vd, Vn, Vm) WORD $(0x4e20f400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMLA4S(Vd, Vn, Vm) WORD $(0x4e20cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define LDQ(Vt, Rn, Off)   WORD $(0x3cc00000 | (((Off) & 0x1ff) << 12) | ((Rn) << 5) | (Vt))

#define ABS(V) VAND V20.B16, V.B16, V.B16

#define ROWS(P)          \
	MOVD P, R13      \
	ADD  R6, P, R14  \
	ADD  R7, P, R15  \
	ADD  R8, P, R16  \
	ADD  R9, P, R17  \
	ADD  R21, P, R19 \
	ADD  R22, P, R20

#define DIR(ACC, B0, O0, B1, O1, B2, O2, B3, O3, B4, O4) \
	LDQ(18, B0, O0)                                  \
	FSUB4S(18, 12, 18)                               \
	ABS(V18)                                         \
	LDQ(19, B1, O1)                                  \
	FSUB4S(19, 13, 19)                               \
	ABS(V19)                                         \
	FADD4S(18, 18, 19)                               \
	LDQ(19, B2, O2)                                  \
	FSUB4S(19, 14, 19)                               \
	ABS(V19)                                         \
	FADD4S(18, 18, 19)                               \
	LDQ(19, B3, O3)                                  \
	FSUB4S(19, 15, 19)                               \
	ABS(V19)                                         \
	FADD4S(18, 18, 19)                               \
	LDQ(19, B4, O4)                                  \
	FSUB4S(19, 16, 19)                               \
	ABS(V19)                                         \
	FADD4S(18, 18, 19)                               \
	FMLA4S(ACC, 18, 17)

#define SADPLANE(P, SOFF)                             \
	ROWS(P)                                       \
	LDQ(12, 15, 0)                                \
	LDQ(13, 16, -4)                               \
	LDQ(14, 16, 0)                                \
	LDQ(15, 16, 4)                                \
	LDQ(16, 17, 0)                                \
	MOVWU SOFF(R12), R23                          \
	VDUP  R23, V17.S4                             \
	                                              \
	DIR(0, 13, 0, 14, -4, 14, 0, 14, 4, 15, 0)    \
	DIR(1, 14, -4, 15, -8, 15, -4, 15, 0, 16, -4) \
	DIR(2, 14, 0, 15, -4, 15, 0, 15, 4, 16, 0)    \
	DIR(3, 14, 4, 15, 0, 15, 4, 15, 8, 16, 4)     \
	DIR(4, 15, -8, 16, -12, 16, -8, 16, -4, 17, -8) \
	DIR(5, 15, -4, 16, -8, 16, -4, 16, 0, 17, -4) \
	DIR(6, 15, 4, 16, 0, 16, 4, 16, 8, 17, 4)     \
	DIR(7, 15, 8, 16, 4, 16, 8, 16, 12, 17, 8)    \
	DIR(8, 16, -4, 17, -8, 17, -4, 17, 0, 19, -4) \
	DIR(9, 16, 0, 17, -4, 17, 0, 17, 4, 19, 0)    \
	DIR(10, 16, 4, 17, 0, 17, 4, 17, 8, 19, 4)    \
	DIR(11, 17, 0, 19, -4, 19, 0, 19, 4, 20, 0)

#define TAP(B, O, ACC) \
	LDQ(28, B, O)  \
	FMLA4S(27, 28, ACC)

#define OUTPLANE(P, D)     \
	ROWS(P)            \
	LDQ(27, 16, 0)     \
	TAP(19, 0, 11)     \
	TAP(17, 4, 10)     \
	TAP(17, 0, 9)      \
	TAP(17, -4, 8)     \
	TAP(16, 8, 7)      \
	TAP(16, 4, 6)      \
	TAP(16, -4, 5)     \
	TAP(16, -8, 4)     \
	TAP(15, 4, 3)      \
	TAP(15, 0, 2)      \
	TAP(15, -4, 1)     \
	TAP(14, 0, 0)      \
	FDIV4S(27, 27, 26) \
	VST1 [V27.S4], (D)

#define WEIGHT(ACC)                    \
	VORR V21.B16, V21.B16, V18.B16 \
	FMLA4S(18, ACC, 25)            \
	FMAX4S(ACC, 18, 22)

#define HALF(MUL)                   \
	VEOR V0.B16, V0.B16, V0.B16 \
	VEOR V1.B16, V1.B16, V1.B16 \
	VEOR V2.B16, V2.B16, V2.B16 \
	VEOR V3.B16, V3.B16, V3.B16 \
	VEOR V4.B16, V4.B16, V4.B16 \
	VEOR V5.B16, V5.B16, V5.B16 \
	VEOR V6.B16, V6.B16, V6.B16 \
	VEOR V7.B16, V7.B16, V7.B16 \
	VEOR V8.B16, V8.B16, V8.B16 \
	VEOR V9.B16, V9.B16, V9.B16 \
	VEOR V10.B16, V10.B16, V10.B16 \
	VEOR V11.B16, V11.B16, V11.B16 \
	                            \
	SADPLANE(R3, 0)             \
	SADPLANE(R4, 32)            \
	SADPLANE(R5, 64)            \
	                            \
	MOVWU (R10), R23            \
	VDUP  R23, V25.S4           \
	FMUL4S(25, 25, MUL)         \
	                            \
	WEIGHT(0)                   \
	WEIGHT(1)                   \
	WEIGHT(2)                   \
	WEIGHT(3)                   \
	WEIGHT(4)                   \
	WEIGHT(5)                   \
	WEIGHT(6)                   \
	WEIGHT(7)                   \
	WEIGHT(8)                   \
	WEIGHT(9)                   \
	WEIGHT(10)                  \
	WEIGHT(11)                  \
	                            \
	VORR V21.B16, V21.B16, V26.B16 \
	FADD4S(26, 26, 0)           \
	FADD4S(26, 26, 1)           \
	FADD4S(26, 26, 2)           \
	FADD4S(26, 26, 3)           \
	FADD4S(26, 26, 4)           \
	FADD4S(26, 26, 5)           \
	FADD4S(26, 26, 6)           \
	FADD4S(26, 26, 7)           \
	FADD4S(26, 26, 8)           \
	FADD4S(26, 26, 9)           \
	FADD4S(26, 26, 10)          \
	FADD4S(26, 26, 11)          \
	                            \
	OUTPLANE(R3, R0)            \
	OUTPLANE(R4, R1)            \
	OUTPLANE(R5, R2)

#define COPYHALF()           \
	ADD  R8, R3, R23     \
	VLD1 (R23), [V27.S4] \
	VST1 [V27.S4], (R0)  \
	ADD  R8, R4, R23     \
	VLD1 (R23), [V27.S4] \
	VST1 [V27.S4], (R1)  \
	ADD  R8, R5, R23     \
	VLD1 (R23), [V27.S4] \
	VST1 [V27.S4], (R2)

#define ADVANCE()  \
	ADD $16, R0 \
	ADD $16, R1 \
	ADD $16, R2 \
	ADD $16, R3 \
	ADD $16, R4 \
	ADD $16, R5

// func epf0NEON(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
//	k *[6][8]float32, y, x0, x1 int)
//
// One row of the first EPF pass over [x0, x1), four lanes at a time. Every
// direction of the diamond compares the same five centre taps against a shifted
// copy, so the taps stay in registers and the shifted ones are unscaled loads.
TEXT ·epf0NEON(SB), NOSPLIT, $0-72
	MOVD stride+16(FP), R6
	LSL  $2, R6

	MOVD y+48(FP), R19
	SUB  $3, R19
	MUL  R6, R19, R19
	MOVD x0+56(FP), R20
	LSL  $2, R20
	ADD  R20, R19

	MOVD src+8(FP), R21
	MOVD (R21), R3
	ADD  R19, R3
	MOVD 8(R21), R4
	ADD  R19, R4
	MOVD 16(R21), R5
	ADD  R19, R5

	MOVD $3, R20
	MUL  R6, R20, R20
	ADD  R20, R19

	MOVD dst+0(FP), R21
	MOVD (R21), R0
	ADD  R19, R0
	MOVD 8(R21), R1
	ADD  R19, R1
	MOVD 16(R21), R2
	ADD  R19, R2

	MOVD y+48(FP), R19
	ASR  $3, R19
	MOVD sigmaStride+32(FP), R20
	MUL  R20, R19, R19
	MOVD x0+56(FP), R20
	ASR  $3, R20
	ADD  R20, R19
	LSL  $2, R19
	MOVD sigma+24(FP), R10
	ADD  R19, R10

	MOVD x1+64(FP), R11
	MOVD x0+56(FP), R19
	SUB  R19, R11
	LSR  $3, R11
	CBZ  R11, done

	LSL $1, R6, R7
	ADD R6, R7, R8
	LSL $2, R6, R9
	ADD R6, R9, R21
	ADD R7, R9, R22

	MOVD k+40(FP), R12

	ADD  $96, R12, R23
	VLD1 (R23), [V21.S4]
	ADD  $128, R12, R23
	VLD1 (R23), [V20.S4]
	ADD  $160, R12, R23
	VLD1 (R23), [V23.S4, V24.S4]

	VEOR V22.B16, V22.B16, V22.B16

	MOVD  $0xc079ef80, R23
	FMOVS R23, F30

loop:
	FMOVS (R10), F29
	FCMPS F30, F29
	BLO   copy

	HALF(23)
	ADVANCE()
	HALF(24)
	ADVANCE()

	JMP next

copy:
	COPYHALF()
	ADVANCE()
	COPYHALF()
	ADVANCE()

next:
	ADD  $4, R10
	SUB  $1, R11
	CBNZ R11, loop

done:
	RET
