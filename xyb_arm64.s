//go:build arm64 && !noasm

#include "textflag.h"

#define FADD4S(Vd, Vn, Vm)  WORD $(0x4e20d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FSUB4S(Vd, Vn, Vm)  WORD $(0x4ea0d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMUL4S(Vd, Vn, Vm)  WORD $(0x6e20dc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FDIV4S(Vd, Vn, Vm)  WORD $(0x6e20fc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FCMGT4S(Vd, Vn, Vm) WORD $(0x6e20e400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define BIC16B(Vd, Vn, Vm)  WORD $(0x4e601c00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FSQRT4S(Vd, Vn)     WORD $(0x6ea1f800 | ((Vn) << 5) | (Vd))
#define FMLA4S(Vd, Vn, Vm)  WORD $(0x4e20cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define MOV16B(Vd, Vn)      WORD $(0x4ea01c00 | ((Vn) << 16) | ((Vn) << 5) | (Vd))

#define KLOAD(OFF, V)      \
	ADD  $OFF, R4, R5  \
	VLD1 (R5), [V.S4]

#define CUBE(L, SB)        \
	FMUL4S(4, L, L)    \
	FMUL4S(5, L, 3)    \
	MOV16B(L, SB)      \
	FMLA4S(L, 4, 5)

#define MIX(A, B, C)       \
	KLOAD(B, V4)       \
	FMUL4S(6, 4, 1)    \
	KLOAD(A, V4)       \
	FMLA4S(6, 4, 0)    \
	KLOAD(C, V4)       \
	FMLA4S(6, 4, 2)

#define HORNER(P, Q, DP, SP, DQ, SQ) \
	MOV16B(DP, P)      \
	FMLA4S(DP, SP, 9)  \
	MOV16B(DQ, Q)      \
	FMLA4S(DQ, SQ, 9)

#define SRGB(D)                       \
	BIC16B(7, 6, 25)              \
	VAND V25.B16, V6.B16, V8.B16  \
	FSQRT4S(9, 8)                 \
	VORR V17.B16, V17.B16, V10.B16 \
	VORR V22.B16, V22.B16, V11.B16 \
	HORNER(16, 21, 4, 10, 5, 11)  \
	HORNER(15, 20, 10, 4, 11, 5)  \
	HORNER(14, 19, 4, 10, 5, 11)  \
	HORNER(13, 18, 10, 4, 11, 5)  \
	FDIV4S(10, 10, 11)            \
	FMUL4S(12, 8, 23)             \
	FCMGT4S(4, 24, 8)             \
	VAND V4.B16, V12.B16, V12.B16 \
	BIC16B(10, 10, 4)             \
	VORR V12.B16, V10.B16, V10.B16 \
	VORR V7.B16, V10.B16, V10.B16 \
	VST1 [V10.S4], (D)

// func xybSRGBNEON(x, y, b *float32, n int, k *[29][8]float32)
//
// The opsin inverse followed by the sRGB transfer function, in place, over n
// samples with n a multiple of eight. The rational polynomial's ten
// coefficients stay in registers because every one of them is read three times
// per group; the opsin matrix is read once each and comes from k.
TEXT ·xybSRGBNEON(SB), NOSPLIT, $0-40
	MOVD x+0(FP), R0
	MOVD y+8(FP), R1
	MOVD b+16(FP), R2
	MOVD n+24(FP), R3
	MOVD k+32(FP), R4

	LSR  $2, R3
	CBZ  R3, done

	KLOAD(512, V13)
	KLOAD(544, V14)
	KLOAD(576, V15)
	KLOAD(608, V16)
	KLOAD(640, V17)
	KLOAD(672, V18)
	KLOAD(704, V19)
	KLOAD(736, V20)
	KLOAD(768, V21)
	KLOAD(800, V22)
	KLOAD(832, V23)
	KLOAD(864, V24)
	KLOAD(896, V25)
	KLOAD(0, V26)
	KLOAD(32, V27)
	KLOAD(64, V28)
	KLOAD(128, V29)
	KLOAD(160, V30)
	KLOAD(192, V31)
	KLOAD(96, V3)

loop:
	VLD1 (R0), [V0.S4]
	VLD1 (R1), [V1.S4]
	VLD1 (R2), [V2.S4]

	FADD4S(4, 1, 0)
	FSUB4S(5, 1, 0)
	FSUB4S(0, 4, 26)
	FSUB4S(1, 5, 27)
	FSUB4S(2, 2, 28)

	CUBE(0, 29)
	CUBE(1, 30)
	CUBE(2, 31)

	MIX(224, 256, 288)
	SRGB(R0)
	MIX(320, 352, 384)
	SRGB(R1)
	MIX(416, 448, 480)
	SRGB(R2)

	ADD  $16, R0
	ADD  $16, R1
	ADD  $16, R2
	SUB  $1, R3
	CBNZ R3, loop

done:
	RET
