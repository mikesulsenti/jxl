//go:build amd64 && !noasm

#include "textflag.h"

// The constant table mirrors idctConsts and appends the three scales:
//   0     dctSqrt2
//   32    dctMultipliers[2], two entries
//   96    dctMultipliers[3], four entries
//   224   dctMultipliers[4], eight entries
//   480   dctMultipliers[5], sixteen entries
//   992   1/8, 1/16, 1/32
#define KSQRT2 0
#define KM4    32
#define KM8    96
#define KM16   224
#define KM32   480
#define KINV8  992
#define KINV16 1024
#define KINV32 1056

// FDCT8 is dct1d(8) on eight columns, taking x0..x7 in Y0..Y7 and leaving the
// interleaved result in Y8, Y0, Y9, Y1, Y10, Y2, Y11, Y3 for indices 0..7. It
// clobbers Y4..Y7. The groupings follow dct1d exactly, including the sqrt2 step
// being a separate multiply and add, so the result is bit-identical.
#define FDCT8                  \
	VADDPS Y7, Y0, Y8      \
	VADDPS Y6, Y1, Y9      \
	VADDPS Y5, Y2, Y10     \
	VADDPS Y4, Y3, Y11     \
	VSUBPS Y7, Y0, Y0      \
	VSUBPS Y6, Y1, Y1      \
	VSUBPS Y5, Y2, Y2      \
	VSUBPS Y4, Y3, Y3      \
	                       \
	VADDPS Y11, Y8, Y4     \
	VADDPS Y10, Y9, Y5     \
	VSUBPS Y11, Y8, Y6     \
	VSUBPS Y10, Y9, Y7     \
	VADDPS Y5, Y4, Y8      \
	VSUBPS Y5, Y4, Y10     \
	VMULPS KM4(CX), Y6, Y6 \
	VMULPS KM4+32(CX), Y7, Y7 \
	VADDPS Y7, Y6, Y4      \
	VSUBPS Y7, Y6, Y11     \
	VMULPS KSQRT2(CX), Y4, Y9 \
	VADDPS Y11, Y9, Y9     \
	                       \
	VMULPS KM8(CX), Y0, Y0 \
	VMULPS KM8+32(CX), Y1, Y1 \
	VMULPS KM8+64(CX), Y2, Y2 \
	VMULPS KM8+96(CX), Y3, Y3 \
	VADDPS Y3, Y0, Y4      \
	VADDPS Y2, Y1, Y5      \
	VSUBPS Y3, Y0, Y6      \
	VSUBPS Y2, Y1, Y7      \
	VADDPS Y5, Y4, Y0      \
	VSUBPS Y5, Y4, Y2      \
	VMULPS KM4(CX), Y6, Y6 \
	VMULPS KM4+32(CX), Y7, Y7 \
	VADDPS Y7, Y6, Y4      \
	VSUBPS Y7, Y6, Y3      \
	VMULPS KSQRT2(CX), Y4, Y1 \
	VADDPS Y3, Y1, Y1      \
	                       \
	VMULPS KSQRT2(CX), Y0, Y0 \
	VADDPS Y1, Y0, Y0      \
	VADDPS Y2, Y1, Y1      \
	VADDPS Y3, Y2, Y2

// func fdct8Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[34][8]float32)
TEXT ·fdct8Cols8AVX2(SB), NOSPLIT, $0-40
	MOVQ src+0(FP), SI
	MOVQ srcStride+8(FP), AX
	MOVQ dst+16(FP), DI
	MOVQ dstStride+24(FP), BX
	MOVQ k+32(FP), CX

	SHLQ $2, AX
	SHLQ $2, BX

	VMOVUPS (SI), Y0
	ADDQ    AX, SI
	VMOVUPS (SI), Y1
	ADDQ    AX, SI
	VMOVUPS (SI), Y2
	ADDQ    AX, SI
	VMOVUPS (SI), Y3
	ADDQ    AX, SI
	VMOVUPS (SI), Y4
	ADDQ    AX, SI
	VMOVUPS (SI), Y5
	ADDQ    AX, SI
	VMOVUPS (SI), Y6
	ADDQ    AX, SI
	VMOVUPS (SI), Y7

	FDCT8

	VMULPS  KINV8(CX), Y8, Y8
	VMOVUPS Y8, (DI)
	ADDQ    BX, DI
	VMULPS  KINV8(CX), Y0, Y0
	VMOVUPS Y0, (DI)
	ADDQ    BX, DI
	VMULPS  KINV8(CX), Y9, Y9
	VMOVUPS Y9, (DI)
	ADDQ    BX, DI
	VMULPS  KINV8(CX), Y1, Y1
	VMOVUPS Y1, (DI)
	ADDQ    BX, DI
	VMULPS  KINV8(CX), Y10, Y10
	VMOVUPS Y10, (DI)
	ADDQ    BX, DI
	VMULPS  KINV8(CX), Y2, Y2
	VMOVUPS Y2, (DI)
	ADDQ    BX, DI
	VMULPS  KINV8(CX), Y11, Y11
	VMOVUPS Y11, (DI)
	ADDQ    BX, DI
	VMULPS  KINV8(CX), Y3, Y3
	VMOVUPS Y3, (DI)

	VZEROUPPER
	RET

// STAGE16 reads rows I and 15-I, leaving the sum in R and writing the
// difference, pre-multiplied by dctMultipliers[4], to the local frame.
#define STAGE16(MOFF, FOFF, R)  \
	VMOVUPS (SI), R         \
	VMOVUPS (DX), Y13       \
	VSUBPS  Y13, R, Y14     \
	VMULPS  MOFF(CX), Y14, Y14 \
	VMOVUPS Y14, FOFF(SP)   \
	VADDPS  Y13, R, R       \
	ADDQ    AX, SI          \
	SUBQ    AX, DX

// STORE2 scales by Y12 and writes to the row R9 points at, then steps two rows.
#define STORE2(V)         \
	VMULPS  Y12, V, V \
	VMOVUPS V, (R9)   \
	ADDQ    BX, R9    \
	ADDQ    BX, R9

// func fdct16Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[34][8]float32, scale *[8]float32)
//
// dct1d(16) on eight columns. The sum half stays in registers through FDCT8 and
// goes straight to the even output rows; the difference half is pre-multiplied
// by dctMultipliers[4] and parked in the frame, then run through the same core,
// accumulated, and written to the odd rows.
TEXT ·fdct16Cols8AVX2(SB), NOSPLIT, $256-48
	MOVQ src+0(FP), SI
	MOVQ srcStride+8(FP), AX
	MOVQ dst+16(FP), DI
	MOVQ dstStride+24(FP), BX
	MOVQ k+32(FP), CX
	MOVQ scale+40(FP), R8

	SHLQ $2, AX
	SHLQ $2, BX

	MOVQ  AX, R9
	IMULQ $15, R9
	MOVQ  SI, DX
	ADDQ  R9, DX

	STAGE16(KM16, 0, Y0)
	STAGE16(KM16+32, 32, Y1)
	STAGE16(KM16+64, 64, Y2)
	STAGE16(KM16+96, 96, Y3)
	STAGE16(KM16+128, 128, Y4)
	STAGE16(KM16+160, 160, Y5)
	STAGE16(KM16+192, 192, Y6)
	STAGE16(KM16+224, 224, Y7)

	FDCT8

	VMOVUPS (R8), Y12
	MOVQ    DI, R9

	STORE2(Y8)
	STORE2(Y0)
	STORE2(Y9)
	STORE2(Y1)
	STORE2(Y10)
	STORE2(Y2)
	STORE2(Y11)
	STORE2(Y3)

	VMOVUPS 0(SP), Y0
	VMOVUPS 32(SP), Y1
	VMOVUPS 64(SP), Y2
	VMOVUPS 96(SP), Y3
	VMOVUPS 128(SP), Y4
	VMOVUPS 160(SP), Y5
	VMOVUPS 192(SP), Y6
	VMOVUPS 224(SP), Y7

	FDCT8

	VMULPS KSQRT2(CX), Y8, Y8
	VADDPS Y0, Y8, Y8
	VADDPS Y9, Y0, Y0
	VADDPS Y1, Y9, Y9
	VADDPS Y10, Y1, Y1
	VADDPS Y2, Y10, Y10
	VADDPS Y11, Y2, Y2
	VADDPS Y3, Y11, Y11

	VMOVUPS (R8), Y12
	MOVQ    DI, R9
	ADDQ    BX, R9

	STORE2(Y8)
	STORE2(Y0)
	STORE2(Y9)
	STORE2(Y1)
	STORE2(Y10)
	STORE2(Y2)
	STORE2(Y11)
	STORE2(Y3)

	VZEROUPPER
	RET

// STAGE32 reads rows I and 31-I, writing the sum to the even frame slot and the
// difference, pre-multiplied by dctMultipliers[5], to the odd frame slot.
#define STAGE32(MOFF, EOFF, OOFF) \
	VMOVUPS (SI), Y0          \
	VMOVUPS (DX), Y1          \
	VSUBPS  Y1, Y0, Y2        \
	VMULPS  MOFF(CX), Y2, Y2  \
	VMOVUPS Y2, OOFF(SP)      \
	VADDPS  Y1, Y0, Y0        \
	VMOVUPS Y0, EOFF(SP)      \
	ADDQ    AX, SI            \
	SUBQ    AX, DX

// func fdct32Split(src *float32, srcStride int, half *float32, k *[34][8]float32)
//
// The dct1d(32) sum and difference stage. The sums land in the first sixteen
// rows of half and the pre-multiplied differences in the next sixteen, so each
// can be run through fdct16Cols8AVX2 unscaled and then recombined.
TEXT ·fdct32Split(SB), NOSPLIT, $0-32
	MOVQ src+0(FP), SI
	MOVQ srcStride+8(FP), AX
	MOVQ half+16(FP), BX
	MOVQ k+24(FP), CX

	SHLQ $2, AX

	MOVQ  AX, R9
	IMULQ $31, R9
	MOVQ  SI, DX
	ADDQ  R9, DX

	MOVQ SP, R10
	MOVQ BX, SP

	STAGE32(KM32, 0, 512)
	STAGE32(KM32+32, 32, 544)
	STAGE32(KM32+64, 64, 576)
	STAGE32(KM32+96, 96, 608)
	STAGE32(KM32+128, 128, 640)
	STAGE32(KM32+160, 160, 672)
	STAGE32(KM32+192, 192, 704)
	STAGE32(KM32+224, 224, 736)
	STAGE32(KM32+256, 256, 768)
	STAGE32(KM32+288, 288, 800)
	STAGE32(KM32+320, 320, 832)
	STAGE32(KM32+352, 352, 864)
	STAGE32(KM32+384, 384, 896)
	STAGE32(KM32+416, 416, 928)
	STAGE32(KM32+448, 448, 960)
	STAGE32(KM32+480, 480, 992)

	MOVQ R10, SP

	VZEROUPPER
	RET

// func fdct32Merge(half *float32, dst *float32, dstStride int, k *[34][8]float32, scale *[8]float32)
//
// The trailing accumulation of dct1d(32) over the odd half, then the interleave
// of the two halves into dst with the 1/32 scale.
TEXT ·fdct32Merge(SB), NOSPLIT, $0-40
	MOVQ half+0(FP), SI
	MOVQ dst+8(FP), DI
	MOVQ dstStride+16(FP), BX
	MOVQ k+24(FP), CX
	MOVQ scale+32(FP), R8

	SHLQ $2, BX

	LEAQ 512(SI), DX

	VMOVUPS (DX), Y0
	VMOVUPS 32(DX), Y1
	VMULPS  KSQRT2(CX), Y0, Y0
	VADDPS  Y1, Y0, Y0
	VMOVUPS Y0, (DX)

	MOVQ $0, R9

merge:
	VMOVUPS 32(DX)(R9*1), Y0
	VMOVUPS 64(DX)(R9*1), Y1
	VADDPS  Y1, Y0, Y0
	VMOVUPS Y0, 32(DX)(R9*1)

	ADDQ $32, R9
	CMPQ R9, $448
	JLT  merge

	VMOVUPS (R8), Y12
	MOVQ    $0, R9

store:
	VMOVUPS (SI)(R9*1), Y0
	VMULPS  Y12, Y0, Y0
	VMOVUPS Y0, (DI)
	ADDQ    BX, DI

	VMOVUPS (DX)(R9*1), Y0
	VMULPS  Y12, Y0, Y0
	VMOVUPS Y0, (DI)
	ADDQ    BX, DI

	ADDQ $32, R9
	CMPQ R9, $512
	JLT  store

	VZEROUPPER
	RET
