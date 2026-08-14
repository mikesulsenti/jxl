//go:build arm64 && !noasm

#include "textflag.h"

#define FADD4S(Vd, Vn, Vm) WORD $(0x4e20d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FSUB4S(Vd, Vn, Vm) WORD $(0x4ea0d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMUL4S(Vd, Vn, Vm) WORD $(0x6e20dc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMLA4S(Vd, Vn, Vm) WORD $(0x4e20cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define MOV16B(Vd, Vn)     WORD $(0x4ea01c00 | ((Vn) << 16) | ((Vn) << 5) | (Vd))

#define KLOAD(OFF, V)     \
	ADD  $OFF, R4, R11 \
	VLD1 (R11), [V.S4]

// FDCT8 is dct1d(8) on four columns, taking x0..x7 in V0..V7 and leaving the
// interleaved result in V8, V0, V9, V1, V10, V2, V11, V3 for indices 0..7. It
// clobbers V4..V7 and V12. The two sqrt2 steps use FMLA because the Go compiler
// contracts that multiply and add into an FMADD on arm64.
#define FDCT8              \
	FADD4S(8, 0, 7)    \
	FADD4S(9, 1, 6)    \
	FADD4S(10, 2, 5)   \
	FADD4S(11, 3, 4)   \
	FSUB4S(0, 0, 7)    \
	FSUB4S(1, 1, 6)    \
	FSUB4S(2, 2, 5)    \
	FSUB4S(3, 3, 4)    \
	                   \
	FADD4S(4, 8, 11)   \
	FADD4S(5, 9, 10)   \
	FSUB4S(6, 8, 11)   \
	FSUB4S(7, 9, 10)   \
	FADD4S(8, 4, 5)    \
	FSUB4S(10, 4, 5)   \
	FMUL4S(6, 6, 17)   \
	FMUL4S(7, 7, 18)   \
	FADD4S(4, 6, 7)    \
	FSUB4S(11, 6, 7)   \
	MOV16B(9, 11)      \
	FMLA4S(9, 4, 16)   \
	                   \
	FMUL4S(0, 0, 19)   \
	FMUL4S(1, 1, 20)   \
	FMUL4S(2, 2, 21)   \
	FMUL4S(3, 3, 22)   \
	FADD4S(4, 0, 3)    \
	FADD4S(5, 1, 2)    \
	FSUB4S(6, 0, 3)    \
	FSUB4S(7, 1, 2)    \
	FADD4S(0, 4, 5)    \
	FSUB4S(2, 4, 5)    \
	FMUL4S(6, 6, 17)   \
	FMUL4S(7, 7, 18)   \
	FADD4S(4, 6, 7)    \
	FSUB4S(3, 6, 7)    \
	MOV16B(1, 3)       \
	FMLA4S(1, 4, 16)   \
	                   \
	MOV16B(12, 1)      \
	FMLA4S(12, 0, 16)  \
	FADD4S(1, 1, 2)    \
	FADD4S(2, 2, 3)    \
	MOV16B(0, 12)

#define CONSTS8       \
	KLOAD(0, V16) \
	KLOAD(32, V17) \
	KLOAD(64, V18) \
	KLOAD(96, V19) \
	KLOAD(128, V20) \
	KLOAD(160, V21) \
	KLOAD(192, V22)

#define LOAD8         \
	VLD1 (R0), [V0.S4] \
	ADD  R1, R0        \
	VLD1 (R0), [V1.S4] \
	ADD  R1, R0        \
	VLD1 (R0), [V2.S4] \
	ADD  R1, R0        \
	VLD1 (R0), [V3.S4] \
	ADD  R1, R0        \
	VLD1 (R0), [V4.S4] \
	ADD  R1, R0        \
	VLD1 (R0), [V5.S4] \
	ADD  R1, R0        \
	VLD1 (R0), [V6.S4] \
	ADD  R1, R0        \
	VLD1 (R0), [V7.S4]

// func fdct8Cols4NEON(src *float32, srcStride int, dst *float32, dstStride int, k *[34][8]float32, scale *[8]float32)
//
// dct1d(8) on four columns. NEON is four lanes wide, so the eight column step
// the caller takes is two of these.
TEXT ·fdct8Cols4NEON(SB), NOSPLIT, $0-48
	MOVD src+0(FP), R0
	MOVD srcStride+8(FP), R1
	MOVD dst+16(FP), R2
	MOVD dstStride+24(FP), R3
	MOVD k+32(FP), R4
	MOVD scale+40(FP), R5

	LSL $2, R1
	LSL $2, R3

	CONSTS8
	VLD1 (R5), [V23.S4]

	LOAD8
	FDCT8

	FMUL4S(8, 8, 23)
	VST1 [V8.S4], (R2)
	ADD  R3, R2
	FMUL4S(0, 0, 23)
	VST1 [V0.S4], (R2)
	ADD  R3, R2
	FMUL4S(9, 9, 23)
	VST1 [V9.S4], (R2)
	ADD  R3, R2
	FMUL4S(1, 1, 23)
	VST1 [V1.S4], (R2)
	ADD  R3, R2
	FMUL4S(10, 10, 23)
	VST1 [V10.S4], (R2)
	ADD  R3, R2
	FMUL4S(2, 2, 23)
	VST1 [V2.S4], (R2)
	ADD  R3, R2
	FMUL4S(11, 11, 23)
	VST1 [V11.S4], (R2)
	ADD  R3, R2
	FMUL4S(3, 3, 23)
	VST1 [V3.S4], (R2)

	RET

// STAGE16 reads rows I and 15-I, leaving the sum in R and writing the
// difference, pre-multiplied by dctMultipliers[4], to the local frame.
#define STAGE16(MOFF, FOFF, RN, RV) \
	VLD1 (R0), [RV.S4]          \
	VLD1 (R6), [V13.S4]         \
	FSUB4S(14, RN, 13)          \
	ADD   $MOFF, R4, R11        \
	VLD1  (R11), [V15.S4]       \
	FMUL4S(14, 14, 15)          \
	ADD   $FOFF, RSP, R11       \
	VST1  [V14.S4], (R11)       \
	FADD4S(RN, RN, 13)          \
	ADD   R1, R0                \
	SUB   R1, R6

#define FLOAD(FOFF, V)      \
	ADD  $FOFF, RSP, R11 \
	VLD1 (R11), [V.S4]

// STORE2 scales by V23 and writes to the row R7 points at, then steps two rows.
#define STORE2(VN, VV)      \
	FMUL4S(VN, VN, 23)  \
	VST1 [VV.S4], (R7)  \
	ADD  R3, R7         \
	ADD  R3, R7

// func fdct16Cols4NEON(src *float32, srcStride int, dst *float32, dstStride int, k *[34][8]float32, scale *[8]float32)
TEXT ·fdct16Cols4NEON(SB), NOSPLIT, $128-48
	MOVD src+0(FP), R0
	MOVD srcStride+8(FP), R1
	MOVD dst+16(FP), R2
	MOVD dstStride+24(FP), R3
	MOVD k+32(FP), R4
	MOVD scale+40(FP), R5

	LSL $2, R1
	LSL $2, R3

	CONSTS8

	MOVD $15, R11
	MUL  R1, R11
	ADD  R0, R11, R6

	STAGE16(224, 0, 0, V0)
	STAGE16(256, 16, 1, V1)
	STAGE16(288, 32, 2, V2)
	STAGE16(320, 48, 3, V3)
	STAGE16(352, 64, 4, V4)
	STAGE16(384, 80, 5, V5)
	STAGE16(416, 96, 6, V6)
	STAGE16(448, 112, 7, V7)

	FDCT8

	VLD1 (R5), [V23.S4]
	MOVD R2, R7

	STORE2(8, V8)
	STORE2(0, V0)
	STORE2(9, V9)
	STORE2(1, V1)
	STORE2(10, V10)
	STORE2(2, V2)
	STORE2(11, V11)
	STORE2(3, V3)

	FLOAD(0, V0)
	FLOAD(16, V1)
	FLOAD(32, V2)
	FLOAD(48, V3)
	FLOAD(64, V4)
	FLOAD(80, V5)
	FLOAD(96, V6)
	FLOAD(112, V7)

	FDCT8

	MOV16B(12, 0)
	FMLA4S(12, 8, 16)
	MOV16B(8, 12)
	FADD4S(0, 0, 9)
	FADD4S(9, 9, 1)
	FADD4S(1, 1, 10)
	FADD4S(10, 10, 2)
	FADD4S(2, 2, 11)
	FADD4S(11, 11, 3)

	VLD1 (R5), [V23.S4]
	MOVD R2, R7
	ADD  R3, R7

	STORE2(8, V8)
	STORE2(0, V0)
	STORE2(9, V9)
	STORE2(1, V1)
	STORE2(10, V10)
	STORE2(2, V2)
	STORE2(11, V11)
	STORE2(3, V3)

	RET

// STAGE32 reads rows I and 31-I, writing the sum to the even half and the
// difference, pre-multiplied by dctMultipliers[5], to the odd half.
#define STAGE32(MOFF, EOFF, OOFF) \
	VLD1 (R0), [V0.S4]        \
	VLD1 (R6), [V1.S4]        \
	FSUB4S(2, 0, 1)           \
	ADD   $MOFF, R4, R11      \
	VLD1  (R11), [V3.S4]      \
	FMUL4S(2, 2, 3)           \
	ADD   $OOFF, R2, R11      \
	VST1  [V2.S4], (R11)      \
	FADD4S(0, 0, 1)           \
	ADD   $EOFF, R2, R11      \
	VST1  [V0.S4], (R11)      \
	ADD   R1, R0              \
	SUB   R1, R6

// func fdct32SplitNEON(src *float32, srcStride int, half *float32, k *[34][8]float32)
TEXT ·fdct32SplitNEON(SB), NOSPLIT, $0-32
	MOVD src+0(FP), R0
	MOVD srcStride+8(FP), R1
	MOVD half+16(FP), R2
	MOVD k+24(FP), R4

	LSL $2, R1

	MOVD $31, R11
	MUL  R1, R11
	ADD  R0, R11, R6

	STAGE32(480, 0, 256)
	STAGE32(512, 16, 272)
	STAGE32(544, 32, 288)
	STAGE32(576, 48, 304)
	STAGE32(608, 64, 320)
	STAGE32(640, 80, 336)
	STAGE32(672, 96, 352)
	STAGE32(704, 112, 368)
	STAGE32(736, 128, 384)
	STAGE32(768, 144, 400)
	STAGE32(800, 160, 416)
	STAGE32(832, 176, 432)
	STAGE32(864, 192, 448)
	STAGE32(896, 208, 464)
	STAGE32(928, 224, 480)
	STAGE32(960, 240, 496)

	RET

// func fdct32MergeNEON(half *float32, dst *float32, dstStride int, k *[34][8]float32, scale *[8]float32)
TEXT ·fdct32MergeNEON(SB), NOSPLIT, $0-40
	MOVD half+0(FP), R0
	MOVD dst+8(FP), R1
	MOVD dstStride+16(FP), R2
	MOVD k+24(FP), R4
	MOVD scale+32(FP), R5

	LSL $2, R2

	ADD $0, R4, R11
	VLD1 (R11), [V16.S4]

	ADD  $256, R0, R6
	VLD1 (R6), [V0.S4]
	ADD  $272, R0, R11
	VLD1 (R11), [V1.S4]
	MOV16B(2, 1)
	FMLA4S(2, 0, 16)
	VST1 [V2.S4], (R6)

	MOVD $0, R7

merge:
	ADD  $272, R0, R11
	ADD  R7, R11
	VLD1 (R11), [V0.S4]
	ADD  $16, R11, R8
	VLD1 (R8), [V1.S4]
	FADD4S(0, 0, 1)
	VST1 [V0.S4], (R11)

	ADD  $16, R7
	CMP  $224, R7
	BLT  merge

	VLD1 (R5), [V23.S4]
	MOVD $0, R7

store:
	ADD  R7, R0, R11
	VLD1 (R11), [V0.S4]
	FMUL4S(0, 0, 23)
	VST1 [V0.S4], (R1)
	ADD  R2, R1

	ADD  $256, R0, R11
	ADD  R7, R11
	VLD1 (R11), [V0.S4]
	FMUL4S(0, 0, 23)
	VST1 [V0.S4], (R1)
	ADD  R2, R1

	ADD  $16, R7
	CMP  $256, R7
	BLT  store

	RET
