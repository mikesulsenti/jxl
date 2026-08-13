//go:build arm64 && !noasm

#include "textflag.h"

#define FADD4S(Vd, Vn, Vm) WORD $(0x4e20d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FSUB4S(Vd, Vn, Vm) WORD $(0x4ea0d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMUL4S(Vd, Vn, Vm) WORD $(0x6e20dc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FDIV4S(Vd, Vn, Vm) WORD $(0x6e20fc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMAX4S(Vd, Vn, Vm) WORD $(0x4e20f400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMLA4S(Vd, Vn, Vm) WORD $(0x4e20cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))

#define ROWS(P)      \
	MOVD P, R13  \
	ADD  R6, P, R14 \
	ADD  R7, P, R15 \
	ADD  R8, P, R16 \
	ADD  R9, P, R17

#define ABS(V) VAND V22.B16, V.B16, V.B16

#define SADPLANE(P, SOFF)          \
	ROWS(P)                    \
	VLD1 (R13), [V4.S4]        \
	SUB  $4, R14, R19          \
	VLD1 (R19), [V5.S4]        \
	VLD1 (R14), [V6.S4]        \
	ADD  $4, R14, R19          \
	VLD1 (R19), [V7.S4]        \
	SUB  $8, R15, R19          \
	VLD1 (R19), [V8.S4]        \
	SUB  $4, R15, R19          \
	VLD1 (R19), [V9.S4]        \
	VLD1 (R15), [V10.S4]       \
	ADD  $4, R15, R19          \
	VLD1 (R19), [V11.S4]       \
	ADD  $8, R15, R19          \
	VLD1 (R19), [V12.S4]       \
	SUB  $4, R16, R19          \
	VLD1 (R19), [V13.S4]       \
	VLD1 (R16), [V14.S4]       \
	ADD  $4, R16, R19          \
	VLD1 (R19), [V15.S4]       \
	VLD1 (R17), [V16.S4]       \
	MOVWU SOFF(R12), R19       \
	VDUP R19, V17.S4           \
	                           \
	FSUB4S(27, 4, 6)           \
	ABS(V27)                   \
	FSUB4S(28, 5, 9)           \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 10, 6)          \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 7, 11)          \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 10, 14)         \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FMLA4S(0, 27, 17)         \
	                           \
	FSUB4S(27, 5, 6)           \
	ABS(V27)                   \
	FSUB4S(28, 8, 9)           \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 10, 9)          \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 10, 11)         \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 13, 14)         \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FMLA4S(1, 27, 17)         \
	                           \
	FSUB4S(27, 7, 6)           \
	ABS(V27)                   \
	FSUB4S(28, 10, 9)          \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 10, 11)         \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 12, 11)         \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 15, 14)         \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FMLA4S(2, 27, 17)         \
	                           \
	FSUB4S(27, 10, 6)          \
	ABS(V27)                   \
	FSUB4S(28, 13, 9)          \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 10, 14)         \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 15, 11)         \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FSUB4S(28, 16, 14)         \
	ABS(V28)                   \
	FADD4S(27, 27, 28)         \
	FMLA4S(3, 27, 17)

#define OUTPLANE(P, D)       \
	ROWS(P)              \
	VLD1 (R15), [V27.S4] \
	VLD1 (R16), [V28.S4] \
	FMLA4S(27, 28, 3)    \
	ADD  $4, R15, R19    \
	VLD1 (R19), [V28.S4] \
	FMLA4S(27, 28, 2)    \
	SUB  $4, R15, R19    \
	VLD1 (R19), [V28.S4] \
	FMLA4S(27, 28, 1)    \
	VLD1 (R14), [V28.S4] \
	FMLA4S(27, 28, 0)    \
	FDIV4S(27, 27, 26)   \
	VST1 [V27.S4], (D)

#define HALF(MUL)            \
	VEOR V0.B16, V0.B16, V0.B16 \
	VEOR V1.B16, V1.B16, V1.B16 \
	VEOR V2.B16, V2.B16, V2.B16 \
	VEOR V3.B16, V3.B16, V3.B16 \
	                     \
	SADPLANE(R3, 0)      \
	SADPLANE(R4, 4)      \
	SADPLANE(R5, 8)      \
	                     \
	MOVWU (R10), R19     \
	VDUP R19, V25.S4     \
	FMUL4S(25, 25, MUL)  \
	                     \
	VORR V20.B16, V20.B16, V27.B16 \
	FMLA4S(27, 0, 25)     \
	FMAX4S(0, 27, 21)     \
	VORR V20.B16, V20.B16, V27.B16 \
	FMLA4S(27, 1, 25)     \
	FMAX4S(1, 27, 21)     \
	VORR V20.B16, V20.B16, V27.B16 \
	FMLA4S(27, 2, 25)     \
	FMAX4S(2, 27, 21)     \
	VORR V20.B16, V20.B16, V27.B16 \
	FMLA4S(27, 3, 25)     \
	FMAX4S(3, 27, 21)     \
	                     \
	VORR V20.B16, V20.B16, V26.B16 \
	FADD4S(26, 26, 0)    \
	FADD4S(26, 26, 1)    \
	FADD4S(26, 26, 2)    \
	FADD4S(26, 26, 3)    \
	                     \
	OUTPLANE(R3, R0)     \
	OUTPLANE(R4, R1)     \
	OUTPLANE(R5, R2)

#define COPYHALF()           \
	ADD  R7, R3, R19     \
	VLD1 (R19), [V27.S4] \
	VST1 [V27.S4], (R0)  \
	ADD  R7, R4, R19     \
	VLD1 (R19), [V27.S4] \
	VST1 [V27.S4], (R1)  \
	ADD  R7, R5, R19     \
	VLD1 (R19), [V27.S4] \
	VST1 [V27.S4], (R2)

#define ADVANCE()  \
	ADD $16, R0 \
	ADD $16, R1 \
	ADD $16, R2 \
	ADD $16, R3 \
	ADD $16, R4 \
	ADD $16, R5

// func epf1NEON(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
//	mul, scales *float32, y, x0, x1 int)
//
// One row of the second EPF pass over [x0, x1), four lanes at a time. The two
// halves of each group of eight share one sigma but not one border multiplier,
// so the group is unrolled rather than the row.
TEXT ·epf1NEON(SB), NOSPLIT, $0-80
	MOVD stride+16(FP), R6
	LSL  $2, R6

	MOVD y+56(FP), R19
	SUB  $2, R19
	MUL  R6, R19, R19
	MOVD x0+64(FP), R20
	LSL  $2, R20
	ADD  R20, R19

	MOVD src+8(FP), R21
	MOVD (R21), R3
	ADD  R19, R3
	MOVD 8(R21), R4
	ADD  R19, R4
	MOVD 16(R21), R5
	ADD  R19, R5

	LSL  $1, R6, R7
	ADD  R7, R19

	MOVD dst+0(FP), R21
	MOVD (R21), R0
	ADD  R19, R0
	MOVD 8(R21), R1
	ADD  R19, R1
	MOVD 16(R21), R2
	ADD  R19, R2

	MOVD  y+56(FP), R19
	ASR   $3, R19
	MOVD  sigmaStride+32(FP), R20
	MUL   R20, R19, R19
	MOVD  x0+64(FP), R20
	ASR   $3, R20
	ADD   R20, R19
	LSL   $2, R19
	MOVD  sigma+24(FP), R10
	ADD   R19, R10

	MOVD x1+72(FP), R11
	MOVD x0+64(FP), R19
	SUB  R19, R11
	LSR  $3, R11
	CBZ  R11, done

	ADD  R6, R7, R8
	LSL  $2, R6, R9

	MOVD scales+48(FP), R12

	MOVD $0x3f800000, R19
	VDUP R19, V20.S4
	VEOR V21.B16, V21.B16, V21.B16
	MOVD $0x7fffffff, R19
	VDUP R19, V22.S4

	MOVD mul+40(FP), R19
	VLD1 (R19), [V23.S4, V24.S4]

	MOVD $0xc079ef80, R19
	FMOVS R19, F30

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
