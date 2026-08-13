//go:build arm64 && !noasm

#include "textflag.h"

#define FADD4S(Vd, Vn, Vm) WORD $(0x4e20d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FSUB4S(Vd, Vn, Vm) WORD $(0x4ea0d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMUL4S(Vd, Vn, Vm) WORD $(0x6e20dc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FDIV4S(Vd, Vn, Vm) WORD $(0x6e20fc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMAX4S(Vd, Vn, Vm) WORD $(0x4e20f400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define MOV16B(Vd, Vn)     WORD $(0x4ea01c00 | ((Vn) << 16) | ((Vn) << 5) | (Vd))
#define FMLA4S(Vd, Vn, Vm) WORD $(0x4e20cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))

#define ABS(V) VAND V20.B16, V.B16, V.B16

#define NEIGH(O)             \
	ADD  O, R2, R14      \
	VLD1 (R14), [V8.S4]  \
	ADD  O, R3, R14      \
	VLD1 (R14), [V9.S4]  \
	ADD  O, R4, R14      \
	VLD1 (R14), [V10.S4] \
	                     \
	FSUB4S(11, 8, 0)     \
	ABS(V11)             \
	FMUL4S(11, 11, 16)   \
	FSUB4S(12, 9, 1)     \
	ABS(V12)             \
	FMLA4S(11, 12, 17)   \
	FSUB4S(12, 10, 2)    \
	ABS(V12)             \
	FMLA4S(11, 12, 18)   \
	MOV16B(12, 19)       \
	FMLA4S(12, 11, 7)    \
	FMAX4S(11, 12, 13)   \
	FADD4S(6, 6, 11)     \
	FMLA4S(3, 11, 8)     \
	FMLA4S(4, 11, 9)     \
	FMLA4S(5, 11, 10)

#define HALF(MUL)                    \
	MOVWU (R7), R14              \
	VDUP  R14, V7.S4             \
	FMUL4S(7, 7, MUL)            \
	                             \
	ADD  R1, R2, R14             \
	VLD1 (R14), [V0.S4]          \
	ADD  R1, R3, R14             \
	VLD1 (R14), [V1.S4]          \
	ADD  R1, R4, R14             \
	VLD1 (R14), [V2.S4]          \
	                             \
	MOV16B(3, 0)                 \
	MOV16B(4, 1)                 \
	MOV16B(5, 2)                 \
	MOV16B(6, 19)                \
	                             \
	NEIGH(ZR)                    \
	SUB  $4, R1, R15             \
	NEIGH(R15)                   \
	ADD  $4, R1, R15             \
	NEIGH(R15)                   \
	NEIGH(R6)                    \
	                             \
	FDIV4S(3, 3, 6)              \
	VST1 [V3.S4], (R8)           \
	FDIV4S(4, 4, 6)              \
	VST1 [V4.S4], (R9)           \
	FDIV4S(5, 5, 6)              \
	VST1 [V5.S4], (R10)

#define COPYHALF()          \
	ADD  R1, R2, R14    \
	VLD1 (R14), [V0.S4] \
	VST1 [V0.S4], (R8)  \
	ADD  R1, R3, R14    \
	VLD1 (R14), [V0.S4] \
	VST1 [V0.S4], (R9)  \
	ADD  R1, R4, R14    \
	VLD1 (R14), [V0.S4] \
	VST1 [V0.S4], (R10)

#define ADVANCE()  \
	ADD $16, R2 \
	ADD $16, R3 \
	ADD $16, R4 \
	ADD $16, R8 \
	ADD $16, R9 \
	ADD $16, R10

// func epf2NEON(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
//	k *[6][8]float32, y, x0, x1 int)
//
// One row of the third EPF pass over [x0, x1), four lanes at a time, so the
// group of eight is unrolled into halves that share a sigma but not a border
// multiplier.
TEXT ·epf2NEON(SB), NOSPLIT, $0-72
	MOVD stride+16(FP), R1
	LSL  $2, R1

	MOVD y+48(FP), R14
	SUB  $1, R14
	MUL  R1, R14, R14
	MOVD x0+56(FP), R15
	LSL  $2, R15
	ADD  R15, R14

	MOVD src+8(FP), R11
	MOVD (R11), R2
	ADD  R14, R2
	MOVD 8(R11), R3
	ADD  R14, R3
	MOVD 16(R11), R4
	ADD  R14, R4

	ADD R1, R14

	MOVD dst+0(FP), R11
	MOVD (R11), R8
	ADD  R14, R8
	MOVD 8(R11), R9
	ADD  R14, R9
	MOVD 16(R11), R10
	ADD  R14, R10

	MOVD y+48(FP), R14
	ASR  $3, R14
	MOVD sigmaStride+32(FP), R15
	MUL  R15, R14, R14
	MOVD x0+56(FP), R15
	ASR  $3, R15
	ADD  R15, R14
	LSL  $2, R14
	MOVD sigma+24(FP), R7
	ADD  R14, R7

	MOVD x1+64(FP), R12
	MOVD x0+56(FP), R14
	SUB  R14, R12
	LSR  $3, R12
	CBZ  R12, done

	LSL $1, R1, R6

	MOVD k+40(FP), R5
	VLD1 (R5), [V16.S4]
	ADD  $32, R5, R14
	VLD1 (R14), [V17.S4]
	ADD  $64, R5, R14
	VLD1 (R14), [V18.S4]
	ADD  $96, R5, R14
	VLD1 (R14), [V19.S4]
	ADD  $128, R5, R14
	VLD1 (R14), [V20.S4]
	ADD  $160, R5, R14
	VLD1 (R14), [V21.S4, V22.S4]

	VEOR V13.B16, V13.B16, V13.B16

	MOVD  $0xc079ef80, R14
	FMOVS R14, F23

loop:
	FMOVS (R7), F24
	FCMPS F23, F24
	BLO   copy

	HALF(21)
	ADVANCE()
	HALF(22)
	ADVANCE()

	JMP next

copy:
	COPYHALF()
	ADVANCE()
	COPYHALF()
	ADVANCE()

next:
	ADD  $4, R7
	SUB  $1, R12
	CBNZ R12, loop

done:
	RET
