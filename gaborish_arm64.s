//go:build arm64 && !noasm

#include "textflag.h"

#define FADD4S(Vd, Vn, Vm) WORD $(0x4e20d400 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMUL4S(Vd, Vn, Vm) WORD $(0x6e20dc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))
#define FMLA4S(Vd, Vn, Vm) WORD $(0x4e20cc00 | ((Vm) << 16) | ((Vn) << 5) | (Vd))

#define TAPS(P, L, C, R)     \
	SUB  $4, P, R6       \
	VLD1 (R6), [L.S4]    \
	VLD1 (P), [C.S4]     \
	ADD  $4, P, R6       \
	VLD1 (R6), [R.S4]

// func gaborishNEON(dst, top, mid, bot *float32, n int, k *[3][8]float32)
//
// The Gaborish three by three over n samples of one row, four lanes at a
// time. n is a multiple of eight and every pointer is at least one sample
// inside the row.
TEXT ·gaborishNEON(SB), NOSPLIT, $0-48
	MOVD dst+0(FP), R0
	MOVD top+8(FP), R1
	MOVD mid+16(FP), R2
	MOVD bot+24(FP), R3
	MOVD n+32(FP), R4
	MOVD k+40(FP), R5

	LSR  $2, R4
	CBZ  R4, done

	VLD1 (R5), [V0.S4]
	ADD  $32, R5, R6
	VLD1 (R6), [V1.S4]
	ADD  $64, R5, R6
	VLD1 (R6), [V2.S4]

loop:
	TAPS(R1, V4, V3, V5)
	TAPS(R2, V7, V6, V8)
	TAPS(R3, V10, V9, V11)

	FADD4S(12, 3, 7)
	FADD4S(12, 12, 9)
	FADD4S(12, 12, 8)

	FADD4S(13, 4, 5)
	FADD4S(13, 13, 10)
	FADD4S(13, 13, 11)

	FMUL4S(14, 6, 0)
	FMLA4S(14, 12, 1)
	FMLA4S(14, 13, 2)

	VST1 [V14.S4], (R0)

	ADD  $16, R0
	ADD  $16, R1
	ADD  $16, R2
	ADD  $16, R3
	SUB  $1, R4
	CBNZ R4, loop

done:
	RET
