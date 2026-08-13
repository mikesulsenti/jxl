//go:build arm64 && !noasm

#include "textflag.h"

#define FADD4S(Vd, Vn, Vm) WORD $(0x4e20d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FSUB4S(Vd, Vn, Vm) WORD $(0x4ea0d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMUL4S(Vd, Vn, Vm) WORD $(0x6e20dc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMLA4S(Vd, Vn, Vm) WORD $(0x4e20cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMLS4S(Vd, Vn, Vm) WORD $(0x4ea0cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FNEG4S(Vd, Vn)     WORD $(0x6ea0f800 | ((Vn) << 5) | (Vd))
#define MOV16B(Vd, Vn)     WORD $(0x4ea01c00 | ((Vn) << 16) | ((Vn) << 5) | (Vd))

// VPASS is idct8 over four columns, fusing exactly where Go's arm64 backend
// does. NEON has no negated multiply-subtract, so t2 - t3 is FMLS followed by
// FNEG, which is bit-identical because negation is exact and rounding is
// sign-symmetric.
#define VPASS()          \
	FADD4S(8, 0, 4)  \
	FSUB4S(9, 0, 4)  \
	FADD4S(10, 6, 2) \
	MOV16B(11, 10)   \
	FMLA4S(11, 2, 25) \
	MOV16B(12, 10)   \
	FMLS4S(12, 2, 25) \
	FNEG4S(12, 12)   \
	MOV16B(13, 8)    \
	FMLA4S(13, 11, 26) \
	MOV16B(14, 8)    \
	FMLS4S(14, 11, 26) \
	MOV16B(15, 9)    \
	FMLA4S(15, 12, 27) \
	MOV16B(16, 9)    \
	FMLS4S(16, 12, 27) \
	                 \
	FADD4S(7, 7, 5)  \
	FADD4S(5, 5, 3)  \
	FADD4S(3, 3, 1)  \
	MOV16B(8, 5)     \
	FMLA4S(8, 1, 25) \
	MOV16B(9, 5)     \
	FMLS4S(9, 1, 25) \
	FNEG4S(9, 9)     \
	FADD4S(10, 7, 3) \
	MOV16B(11, 10)   \
	FMLA4S(11, 3, 25) \
	MOV16B(12, 10)   \
	FMLS4S(12, 3, 25) \
	FNEG4S(12, 12)   \
	MOV16B(0, 8)     \
	FMLA4S(0, 11, 26) \
	MOV16B(1, 8)     \
	FMLS4S(1, 11, 26) \
	MOV16B(2, 9)     \
	FMLA4S(2, 12, 27) \
	MOV16B(4, 9)     \
	FMLS4S(4, 12, 27) \
	                 \
	MOV16B(17, 13)   \
	FMLA4S(17, 0, 28) \
	MOV16B(18, 13)   \
	FMLS4S(18, 0, 28) \
	MOV16B(19, 15)   \
	FMLA4S(19, 2, 29) \
	MOV16B(20, 15)   \
	FMLS4S(20, 2, 29) \
	MOV16B(21, 16)   \
	FMLA4S(21, 4, 30) \
	MOV16B(22, 16)   \
	FMLS4S(22, 4, 30) \
	MOV16B(23, 14)   \
	FMLA4S(23, 1, 31) \
	MOV16B(24, 14)   \
	FMLS4S(24, 1, 31) \
	                 \
	MOV16B(0, 17)    \
	MOV16B(1, 19)    \
	MOV16B(2, 21)    \
	MOV16B(3, 23)    \
	MOV16B(4, 24)    \
	MOV16B(5, 22)    \
	MOV16B(6, 20)    \
	MOV16B(7, 18)

#define LOADROWS(P, S)      \
	MOVD P, R6          \
	VLD1 (R6), [V0.S4]  \
	ADD  S, R6          \
	VLD1 (R6), [V1.S4]  \
	ADD  S, R6          \
	VLD1 (R6), [V2.S4]  \
	ADD  S, R6          \
	VLD1 (R6), [V3.S4]  \
	ADD  S, R6          \
	VLD1 (R6), [V4.S4]  \
	ADD  S, R6          \
	VLD1 (R6), [V5.S4]  \
	ADD  S, R6          \
	VLD1 (R6), [V6.S4]  \
	ADD  S, R6          \
	VLD1 (R6), [V7.S4]

#define STOREROWS(P, S)     \
	MOVD P, R6          \
	VST1 [V0.S4], (R6)  \
	ADD  S, R6          \
	VST1 [V1.S4], (R6)  \
	ADD  S, R6          \
	VST1 [V2.S4], (R6)  \
	ADD  S, R6          \
	VST1 [V3.S4], (R6)  \
	ADD  S, R6          \
	VST1 [V4.S4], (R6)  \
	ADD  S, R6          \
	VST1 [V5.S4], (R6)  \
	ADD  S, R6          \
	VST1 [V6.S4], (R6)  \
	ADD  S, R6          \
	VST1 [V7.S4], (R6)

#define KLOAD(OFF, V)     \
	ADD  $OFF, R4, R7 \
	VLD1 (R7), [V.S4]

#define CONSTS()      \
	KLOAD(0, V25) \
	KLOAD(32, V26) \
	KLOAD(64, V27) \
	KLOAD(96, V28) \
	KLOAD(128, V29) \
	KLOAD(160, V30) \
	KLOAD(192, V31)

// func idct8Cols8NEON(src *float32, srcStride int, dst *float32, dstStride int, k *[15][8]float32, tmp *float32)
//
// An eight point inverse DCT down eight adjacent columns, four lanes at a
// time, so the group of eight is two turns of the loop.
TEXT ·idct8Cols8NEON(SB), NOSPLIT, $0-48
	MOVD src+0(FP), R0
	MOVD srcStride+8(FP), R1
	MOVD dst+16(FP), R2
	MOVD dstStride+24(FP), R3
	MOVD k+32(FP), R4

	LSL $2, R1
	LSL $2, R3

	CONSTS()

	MOVD $2, R10

loop:
	LOADROWS(R0, R1)
	VPASS()
	STOREROWS(R2, R3)

	ADD  $16, R0
	ADD  $16, R2
	SUB  $1, R10
	CBNZ R10, loop

	RET

#define COMB16(VI, MOFF)     \
	KLOAD(MOFF, V8)      \
	VLD1 (R6), [V9.S4]   \
	MOV16B(10, 9)        \
	FMLA4S(10, VI, 8)    \
	VST1 [V10.S4], (R8)  \
	MOV16B(11, 9)        \
	FMLS4S(11, VI, 8)    \
	VST1 [V11.S4], (R9)  \
	ADD  $16, R6         \
	ADD  R3, R8          \
	SUB  R3, R9

// func idct16Cols8NEON(src *float32, srcStride int, dst *float32, dstStride int, k *[15][8]float32, tmp *float32)
//
// A sixteen point inverse DCT down eight adjacent columns: the eight point
// pass over the even rows, then over the odd rows after their running sum, and
// the combine. The even half goes to the caller's scratch, and the eight
// multipliers of the combine come from k because the register file is full.
TEXT ·idct16Cols8NEON(SB), NOSPLIT, $0-48
	MOVD src+0(FP), R0
	MOVD srcStride+8(FP), R1
	MOVD dst+16(FP), R2
	MOVD dstStride+24(FP), R3
	MOVD k+32(FP), R4
	MOVD tmp+40(FP), R5

	LSL $2, R1
	LSL $2, R3

	CONSTS()

	LSL $1, R1, R11

	MOVD $2, R10

loop16:
	LOADROWS(R0, R11)
	VPASS()

	MOVD R5, R6
	VST1 [V0.S4], (R6)
	ADD  $16, R6
	VST1 [V1.S4], (R6)
	ADD  $16, R6
	VST1 [V2.S4], (R6)
	ADD  $16, R6
	VST1 [V3.S4], (R6)
	ADD  $16, R6
	VST1 [V4.S4], (R6)
	ADD  $16, R6
	VST1 [V5.S4], (R6)
	ADD  $16, R6
	VST1 [V6.S4], (R6)
	ADD  $16, R6
	VST1 [V7.S4], (R6)

	ADD R1, R0, R12
	LOADROWS(R12, R11)

	FADD4S(7, 7, 6)
	FADD4S(6, 6, 5)
	FADD4S(5, 5, 4)
	FADD4S(4, 4, 3)
	FADD4S(3, 3, 2)
	FADD4S(2, 2, 1)
	FADD4S(1, 1, 0)
	FMUL4S(0, 0, 25)

	VPASS()

	MOVD R3, R9
	MOVD $15, R13
	MUL  R13, R9, R9
	ADD  R2, R9
	MOVD R2, R8
	MOVD R5, R6

	COMB16(0, 224)
	COMB16(1, 256)
	COMB16(2, 288)
	COMB16(3, 320)
	COMB16(4, 352)
	COMB16(5, 384)
	COMB16(6, 416)
	COMB16(7, 448)

	ADD  $16, R0
	ADD  $16, R2
	SUB  $1, R10
	CBNZ R10, loop16

	RET

// func idct32PreNEON(src *float32, srcStride int, tmp *float32, k *[31][8]float32)
//
// The gather and running sum that open a thirty-two point transform.
TEXT ·idct32PreNEON(SB), NOSPLIT, $0-32
	MOVD src+0(FP), R0
	MOVD srcStride+8(FP), R1
	MOVD tmp+16(FP), R2
	MOVD k+24(FP), R3

	LSL $2, R1
	LSL $1, R1, R4

	MOVD R0, R5
	MOVD R2, R6
	MOVD $16, R7

even32:
	VLD1 (R5), [V0.S4, V1.S4]
	VST1 [V0.S4, V1.S4], (R6)
	ADD  R4, R5
	ADD  $32, R6
	SUB  $1, R7
	CBNZ R7, even32

	ADD  R1, R0, R5
	ADD  $512, R2, R6
	MOVD $16, R7

odd32:
	VLD1 (R5), [V0.S4, V1.S4]
	VST1 [V0.S4, V1.S4], (R6)
	ADD  R4, R5
	ADD  $32, R6
	SUB  $1, R7
	CBNZ R7, odd32

	ADD  $992, R2, R6
	MOVD $15, R7

chain32:
	SUB  $32, R6, R8
	VLD1 (R8), [V2.S4, V3.S4]
	VLD1 (R6), [V0.S4, V1.S4]
	FADD4S(0, 0, 2)
	FADD4S(1, 1, 3)
	VST1 [V0.S4, V1.S4], (R6)
	MOVD R8, R6
	SUB  $1, R7
	CBNZ R7, chain32

	ADD  $512, R2, R6
	VLD1 (R3), [V4.S4]
	VLD1 (R6), [V0.S4, V1.S4]
	FMUL4S(0, 0, 4)
	FMUL4S(1, 1, 4)
	VST1 [V0.S4, V1.S4], (R6)

	RET

// func idct32PostNEON(tmp *float32, dst *float32, dstStride int, k *[31][8]float32)
//
// The combine that closes it, pairing row i with row thirty-one minus i.
TEXT ·idct32PostNEON(SB), NOSPLIT, $0-32
	MOVD tmp+0(FP), R0
	MOVD dst+8(FP), R1
	MOVD dstStride+16(FP), R2
	MOVD k+24(FP), R3

	LSL  $2, R2
	MOVD R2, R4
	MOVD $31, R5
	MUL  R5, R4, R4
	ADD  R1, R4

	ADD  $480, R3, R6
	MOVD R0, R7
	ADD  $512, R0, R8
	MOVD $16, R9

post32:
	VLD1 (R7), [V0.S4, V1.S4]
	VLD1 (R8), [V2.S4, V3.S4]
	VLD1 (R6), [V4.S4]
	MOV16B(5, 0)
	FMLA4S(5, 2, 4)
	MOV16B(6, 1)
	FMLA4S(6, 3, 4)
	VST1 [V5.S4, V6.S4], (R1)
	MOV16B(7, 0)
	FMLS4S(7, 2, 4)
	MOV16B(8, 1)
	FMLS4S(8, 3, 4)
	VST1 [V7.S4, V8.S4], (R4)
	ADD  $32, R7
	ADD  $32, R8
	ADD  $32, R6
	ADD  R2, R1
	SUB  R2, R4
	SUB  $1, R9
	CBNZ R9, post32

	RET

#define T4(A, B, C, D, O0, O1, O2, O3) \
	VTRN1 B.S4, A.S4, V16.S4       \
	VTRN2 B.S4, A.S4, V17.S4       \
	VTRN1 D.S4, C.S4, V18.S4       \
	VTRN2 D.S4, C.S4, V19.S4       \
	VZIP1 V18.D2, V16.D2, O0.D2    \
	VZIP1 V19.D2, V17.D2, O1.D2    \
	VZIP2 V18.D2, V16.D2, O2.D2    \
	VZIP2 V19.D2, V17.D2, O3.D2

// func transpose8NEON(src *float32, srcStride int, dst *float32, dstStride int)
//
// One eight by eight tile as four four by four transposes, each a pair of
// TRN over singles and a pair of ZIP over doubles.
TEXT ·transpose8NEON(SB), NOSPLIT, $0-32
	MOVD src+0(FP), R0
	MOVD srcStride+8(FP), R1
	MOVD dst+16(FP), R2
	MOVD dstStride+24(FP), R3

	LSL $2, R1
	LSL $2, R3

	MOVD R0, R6
	VLD1 (R6), [V0.S4, V1.S4]
	ADD  R1, R6
	VLD1 (R6), [V2.S4, V3.S4]
	ADD  R1, R6
	VLD1 (R6), [V4.S4, V5.S4]
	ADD  R1, R6
	VLD1 (R6), [V6.S4, V7.S4]
	ADD  R1, R6
	VLD1 (R6), [V8.S4, V9.S4]
	ADD  R1, R6
	VLD1 (R6), [V10.S4, V11.S4]
	ADD  R1, R6
	VLD1 (R6), [V12.S4, V13.S4]
	ADD  R1, R6
	VLD1 (R6), [V14.S4, V15.S4]

	T4(V0, V2, V4, V6, V0, V2, V4, V6)
	T4(V1, V3, V5, V7, V1, V3, V5, V7)
	T4(V8, V10, V12, V14, V8, V10, V12, V14)
	T4(V9, V11, V13, V15, V9, V11, V13, V15)

	MOVD R2, R6
	VST1 [V0.S4], (R6)
	ADD  R3, R6
	VST1 [V2.S4], (R6)
	ADD  R3, R6
	VST1 [V4.S4], (R6)
	ADD  R3, R6
	VST1 [V6.S4], (R6)
	ADD  R3, R6
	VST1 [V1.S4], (R6)
	ADD  R3, R6
	VST1 [V3.S4], (R6)
	ADD  R3, R6
	VST1 [V5.S4], (R6)
	ADD  R3, R6
	VST1 [V7.S4], (R6)

	ADD  $16, R2, R6
	VST1 [V8.S4], (R6)
	ADD  R3, R6
	VST1 [V10.S4], (R6)
	ADD  R3, R6
	VST1 [V12.S4], (R6)
	ADD  R3, R6
	VST1 [V14.S4], (R6)
	ADD  R3, R6
	VST1 [V9.S4], (R6)
	ADD  R3, R6
	VST1 [V11.S4], (R6)
	ADD  R3, R6
	VST1 [V13.S4], (R6)
	ADD  R3, R6
	VST1 [V15.S4], (R6)

	RET
