//go:build amd64 && !noasm

#include "textflag.h"

#define VPASS()                 \
	VADDPS Y4, Y0, Y8       \
	VSUBPS Y4, Y0, Y9       \
	VADDPS Y2, Y6, Y10      \
	VMULPS 0(AX), Y2, Y11   \
	VADDPS Y10, Y11, Y12    \
	VSUBPS Y10, Y11, Y13    \
	VMULPS 32(AX), Y12, Y12 \
	VMULPS 64(AX), Y13, Y13 \
	VADDPS Y12, Y8, Y0      \
	VADDPS Y13, Y9, Y2      \
	VSUBPS Y13, Y9, Y4      \
	VSUBPS Y12, Y8, Y6      \
	                        \
	VADDPS Y5, Y7, Y7       \
	VADDPS Y3, Y5, Y5       \
	VADDPS Y1, Y3, Y3       \
	VMULPS 0(AX), Y1, Y1    \
	                        \
	VADDPS Y5, Y1, Y8       \
	VSUBPS Y5, Y1, Y9       \
	VADDPS Y3, Y7, Y10      \
	VMULPS 0(AX), Y3, Y11   \
	VADDPS Y10, Y11, Y12    \
	VSUBPS Y10, Y11, Y13    \
	VMULPS 32(AX), Y12, Y12 \
	VMULPS 64(AX), Y13, Y13 \
	VADDPS Y12, Y8, Y1      \
	VADDPS Y13, Y9, Y3      \
	VSUBPS Y13, Y9, Y5      \
	VSUBPS Y12, Y8, Y7      \
	                        \
	VMULPS 96(AX), Y1, Y1   \
	VMULPS 128(AX), Y3, Y3  \
	VMULPS 160(AX), Y5, Y5  \
	VMULPS 192(AX), Y7, Y7  \
	                        \
	VADDPS Y1, Y0, Y8       \
	VSUBPS Y1, Y0, Y15      \
	VADDPS Y3, Y2, Y9       \
	VSUBPS Y3, Y2, Y14      \
	VADDPS Y5, Y4, Y10      \
	VSUBPS Y5, Y4, Y13      \
	VADDPS Y7, Y6, Y11      \
	VSUBPS Y7, Y6, Y12      \
	                        \
	VMOVAPS Y8, Y0          \
	VMOVAPS Y9, Y1          \
	VMOVAPS Y10, Y2         \
	VMOVAPS Y11, Y3         \
	VMOVAPS Y12, Y4         \
	VMOVAPS Y13, Y5         \
	VMOVAPS Y14, Y6         \
	VMOVAPS Y15, Y7

#define TRANSPOSE()                 \
	VUNPCKLPS Y1, Y0, Y8        \
	VUNPCKHPS Y1, Y0, Y9        \
	VUNPCKLPS Y3, Y2, Y10       \
	VUNPCKHPS Y3, Y2, Y11       \
	VUNPCKLPS Y5, Y4, Y12       \
	VUNPCKHPS Y5, Y4, Y13       \
	VUNPCKLPS Y7, Y6, Y14       \
	VUNPCKHPS Y7, Y6, Y15       \
	                            \
	VSHUFPS $0x44, Y10, Y8, Y0  \
	VSHUFPS $0xee, Y10, Y8, Y1  \
	VSHUFPS $0x44, Y11, Y9, Y2  \
	VSHUFPS $0xee, Y11, Y9, Y3  \
	VSHUFPS $0x44, Y14, Y12, Y4 \
	VSHUFPS $0xee, Y14, Y12, Y5 \
	VSHUFPS $0x44, Y15, Y13, Y6 \
	VSHUFPS $0xee, Y15, Y13, Y7 \
	                            \
	VPERM2F128 $0x20, Y4, Y0, Y8  \
	VPERM2F128 $0x20, Y5, Y1, Y9  \
	VPERM2F128 $0x20, Y6, Y2, Y10 \
	VPERM2F128 $0x20, Y7, Y3, Y11 \
	VPERM2F128 $0x31, Y4, Y0, Y12 \
	VPERM2F128 $0x31, Y5, Y1, Y13 \
	VPERM2F128 $0x31, Y6, Y2, Y14 \
	VPERM2F128 $0x31, Y7, Y3, Y15 \
	                            \
	VMOVAPS Y8, Y0              \
	VMOVAPS Y9, Y1              \
	VMOVAPS Y10, Y2             \
	VMOVAPS Y11, Y3             \
	VMOVAPS Y12, Y4             \
	VMOVAPS Y13, Y5             \
	VMOVAPS Y14, Y6             \
	VMOVAPS Y15, Y7

// func idct8x8AVX2(data *float32, k *[15][8]float32)
//
// The whole of idct2d for an eight by eight block: a vertical pass over the
// eight rows held in registers, an in-register transpose, and a second
// vertical pass. That is what the scalar path spells as sixteen strided
// idct8 calls, two transposeBlock passes and a copy.
TEXT ·idct8x8AVX2(SB), NOSPLIT, $0-16
	MOVQ data+0(FP), DI
	MOVQ k+8(FP), AX

	VMOVUPS (DI), Y0
	VMOVUPS 32(DI), Y1
	VMOVUPS 64(DI), Y2
	VMOVUPS 96(DI), Y3
	VMOVUPS 128(DI), Y4
	VMOVUPS 160(DI), Y5
	VMOVUPS 192(DI), Y6
	VMOVUPS 224(DI), Y7

	VPASS()
	TRANSPOSE()
	VPASS()

	VMOVUPS Y0, (DI)
	VMOVUPS Y1, 32(DI)
	VMOVUPS Y2, 64(DI)
	VMOVUPS Y3, 96(DI)
	VMOVUPS Y4, 128(DI)
	VMOVUPS Y5, 160(DI)
	VMOVUPS Y6, 192(DI)
	VMOVUPS Y7, 224(DI)

	VZEROUPPER
	RET

#define LOADROWS(P, S)     \
	VMOVUPS (P), Y0    \
	ADDQ    S, P       \
	VMOVUPS (P), Y1    \
	ADDQ    S, P       \
	VMOVUPS (P), Y2    \
	ADDQ    S, P       \
	VMOVUPS (P), Y3    \
	ADDQ    S, P       \
	VMOVUPS (P), Y4    \
	ADDQ    S, P       \
	VMOVUPS (P), Y5    \
	ADDQ    S, P       \
	VMOVUPS (P), Y6    \
	ADDQ    S, P       \
	VMOVUPS (P), Y7

#define STOREROWS(P, S)    \
	VMOVUPS Y0, (P)    \
	ADDQ    S, P       \
	VMOVUPS Y1, (P)    \
	ADDQ    S, P       \
	VMOVUPS Y2, (P)    \
	ADDQ    S, P       \
	VMOVUPS Y3, (P)    \
	ADDQ    S, P       \
	VMOVUPS Y4, (P)    \
	ADDQ    S, P       \
	VMOVUPS Y5, (P)    \
	ADDQ    S, P       \
	VMOVUPS Y6, (P)    \
	ADDQ    S, P       \
	VMOVUPS Y7, (P)

// func idct8Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[15][8]float32)
//
// An eight point inverse DCT down eight adjacent columns, which turns the
// strided single-column loads of idct1d into contiguous vector ones.
TEXT ·idct8Cols8AVX2(SB), NOSPLIT, $0-40
	MOVQ src+0(FP), DI
	MOVQ srcStride+8(FP), BX
	MOVQ dst+16(FP), SI
	MOVQ dstStride+24(FP), DX
	MOVQ k+32(FP), AX

	SHLQ $2, BX
	SHLQ $2, DX

	LOADROWS(DI, BX)
	VPASS()
	STOREROWS(SI, DX)

	VZEROUPPER
	RET

#define COMB(YI, EOFF, MOFF)     \
	VMOVUPS e-EOFF(SP), Y8   \
	VMULPS  MOFF(AX), YI, Y9 \
	VADDPS  Y8, Y9, Y10      \
	VMOVUPS Y10, (R10)       \
	VSUBPS  Y9, Y8, Y11      \
	VMOVUPS Y11, (R11)       \
	ADDQ    DX, R10          \
	SUBQ    DX, R11

// func idct16Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[15][8]float32)
//
// A sixteen point inverse DCT down eight adjacent columns. It is the eight
// point pass twice — once over the even rows, once over the odd ones after
// their running sum — and the combine that idct1d spells as its tail. The even
// half is spilled because sixteen columns of eight lanes is exactly the
// register file.
TEXT ·idct16Cols8AVX2(SB), NOSPLIT, $256-40
	MOVQ src+0(FP), DI
	MOVQ srcStride+8(FP), BX
	MOVQ dst+16(FP), SI
	MOVQ dstStride+24(FP), DX
	MOVQ k+32(FP), AX

	SHLQ $2, BX
	SHLQ $2, DX

	MOVQ BX, R9
	SHLQ $1, R9

	MOVQ DI, R8
	LOADROWS(R8, R9)
	VPASS()

	VMOVUPS Y0, e-256(SP)
	VMOVUPS Y1, e-224(SP)
	VMOVUPS Y2, e-192(SP)
	VMOVUPS Y3, e-160(SP)
	VMOVUPS Y4, e-128(SP)
	VMOVUPS Y5, e-96(SP)
	VMOVUPS Y6, e-64(SP)
	VMOVUPS Y7, e-32(SP)

	LEAQ (DI)(BX*1), R8
	LOADROWS(R8, R9)

	VADDPS Y6, Y7, Y7
	VADDPS Y5, Y6, Y6
	VADDPS Y4, Y5, Y5
	VADDPS Y3, Y4, Y4
	VADDPS Y2, Y3, Y3
	VADDPS Y1, Y2, Y2
	VADDPS Y0, Y1, Y1
	VMULPS 0(AX), Y0, Y0

	VPASS()

	MOVQ  DX, R11
	IMULQ $15, R11
	ADDQ  SI, R11
	MOVQ  SI, R10

	COMB(Y0, 256, 224)
	COMB(Y1, 224, 256)
	COMB(Y2, 192, 288)
	COMB(Y3, 160, 320)
	COMB(Y4, 128, 352)
	COMB(Y5, 96, 384)
	COMB(Y6, 64, 416)
	COMB(Y7, 32, 448)

	VZEROUPPER
	RET

// func idct32PreAVX2(src *float32, srcStride int, tmp *float32, k *[31][8]float32)
//
// The gather and running sum that open a thirty-two point transform: even rows
// into the first half of tmp, odd rows into the second, then tmp[31] down to
// tmp[17] accumulate downwards and tmp[16] takes the square root of two.
TEXT ·idct32PreAVX2(SB), NOSPLIT, $0-32
	MOVQ src+0(FP), DI
	MOVQ srcStride+8(FP), BX
	MOVQ tmp+16(FP), SI
	MOVQ k+24(FP), AX

	SHLQ $2, BX
	MOVQ BX, CX
	SHLQ $1, CX

	MOVQ DI, DX
	MOVQ SI, R8
	MOVQ $16, R9

even:
	VMOVUPS (DX), Y0
	VMOVUPS Y0, (R8)
	ADDQ    CX, DX
	ADDQ    $32, R8
	SUBQ    $1, R9
	JNZ     even

	LEAQ (DI)(BX*1), DX
	LEAQ 512(SI), R8
	MOVQ $16, R9

odd:
	VMOVUPS (DX), Y0
	VMOVUPS Y0, (R8)
	ADDQ    CX, DX
	ADDQ    $32, R8
	SUBQ    $1, R9
	JNZ     odd

	LEAQ 992(SI), R10
	MOVQ $15, R9

chain:
	VMOVUPS -32(R10), Y1
	VMOVUPS (R10), Y0
	VADDPS  Y1, Y0, Y0
	VMOVUPS Y0, (R10)
	SUBQ    $32, R10
	SUBQ    $1, R9
	JNZ     chain

	VMOVUPS 512(SI), Y0
	VMULPS  (AX), Y0, Y0
	VMOVUPS Y0, 512(SI)

	VZEROUPPER
	RET

// func idct32PostAVX2(tmp *float32, dst *float32, dstStride int, k *[31][8]float32)
//
// The combine that closes it, pairing row i with row thirty-one minus i.
TEXT ·idct32PostAVX2(SB), NOSPLIT, $0-32
	MOVQ tmp+0(FP), SI
	MOVQ dst+8(FP), DI
	MOVQ dstStride+16(FP), BX
	MOVQ k+24(FP), AX

	SHLQ  $2, BX
	MOVQ  BX, DX
	IMULQ $31, DX
	LEAQ  (DI)(DX*1), R8

	LEAQ 480(AX), R10
	MOVQ SI, R9
	LEAQ 512(SI), R11
	MOVQ $16, CX

post:
	VMOVUPS (R9), Y0
	VMOVUPS (R11), Y1
	VMULPS  (R10), Y1, Y1
	VADDPS  Y0, Y1, Y2
	VMOVUPS Y2, (DI)
	VSUBPS  Y1, Y0, Y3
	VMOVUPS Y3, (R8)
	ADDQ    $32, R9
	ADDQ    $32, R11
	ADDQ    $32, R10
	ADDQ    BX, DI
	SUBQ    BX, R8
	SUBQ    $1, CX
	JNZ     post

	VZEROUPPER
	RET

// func transpose8AVX2(src *float32, srcStride int, dst *float32, dstStride int)
//
// One eight by eight tile, using the same in-register sequence as idct8x8AVX2.
TEXT ·transpose8AVX2(SB), NOSPLIT, $0-32
	MOVQ src+0(FP), DI
	MOVQ srcStride+8(FP), BX
	MOVQ dst+16(FP), SI
	MOVQ dstStride+24(FP), DX

	SHLQ $2, BX
	SHLQ $2, DX

	LOADROWS(DI, BX)
	TRANSPOSE()
	STOREROWS(SI, DX)

	VZEROUPPER
	RET
