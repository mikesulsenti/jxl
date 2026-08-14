//go:build riscv64 && riscv64.rva23u64 && !noasm

#include "textflag.h"

DATA quantConstsRVV+0(SB)/4, $0x3F000000
DATA quantConstsRVV+4(SB)/4, $0xBF000000
DATA quantConstsRVV+8(SB)/4, $0x4E800000
DATA quantConstsRVV+12(SB)/4, $0xCE800000
GLOBL quantConstsRVV(SB), RODATA|NOPTR, $16

// ADJUST is adjustQuantBias for the integers in VQ with the channel bias FB,
// leaving the result in V4. Three merges rather than four branches: the general
// case is computed for every lane, which divides by zero where q is, and the
// result is merged away again along with the two cases that |q| == 1 covers,
// where bias * float32(q) gives both signs at once.
#define ADJUST(VQ, FB)           \
	VFCVTFXV  VQ, V2         \
	VFRDIVVF  F3, V2, V3     \
	VFSUBVV   V3, V2, V4     \
	VFMULVF   FB, V2, V5     \
	VMSEQVI   $1, VQ, V0     \
	VMERGEVVM V5, V4, V0, V4 \
	VMSEQVI   $-1, VQ, V0    \
	VMERGEVVM V5, V4, V0, V4 \
	VMSEQVI   $0, VQ, V0     \
	VMERGEVIM $0, V4, V0, V4

// CAND evaluates the candidate in VQ into adjust V4 and error V6 against V10.
#define CAND(VQ, FB)        \
	ADJUST(VQ, FB)      \
	VFSUBVV V10, V4, V6 \
	VFSGNJXVV V6, V6, V6

// QUANT quantises the target in V10 with channel bias FB, leaving the chosen
// integers in V14 and the adjusted float of that candidate in V15.
#define QUANT(FB)                  \
	VMFNEVV   V10, V10, V0     \
	VMVVV     V0, V20          \
	VFMAXVF   F12, V10, V10    \
	VFMINVF   F11, V10, V10    \
	VFCVTRTZXFV V10, V11       \
	VFCVTFXV  V11, V12         \
	VFSUBVV   V12, V10, V12    \
	VMFGEVF   F9, V12, V0      \
	VADDVI    $1, V11, V13     \
	VMERGEVVM V13, V11, V0, V11 \
	VMFLEVF   F10, V12, V0     \
	VADDVI    $-1, V11, V13    \
	VMERGEVVM V13, V11, V0, V11 \
	                           \
	VADDVI    $-1, V11, V17    \
	CAND(V17, FB)              \
	VMVVV     V17, V14         \
	VMVVV     V4, V15          \
	VMVVV     V6, V16          \
	                           \
	VMVVV     V11, V17         \
	CAND(V17, FB)              \
	VMFLTVV   V16, V6, V0      \
	VMERGEVVM V17, V14, V0, V14 \
	VMERGEVVM V4, V15, V0, V15 \
	VMERGEVVM V6, V16, V0, V16 \
	                           \
	VADDVI    $1, V11, V17     \
	CAND(V17, FB)              \
	VMFLTVV   V16, V6, V0      \
	VMERGEVVM V17, V14, V0, V14 \
	VMERGEVVM V4, V15, V0, V15 \
	                           \
	VMVVV     V20, V0          \
	VMERGEVIM $0, V14, V0, V14 \
	VMERGEVIM $0, V15, V0, V15

// func quantizeRVV(a *quantArgs)
//
// The forward quantiser, and the mirror of dequantRVV. Candidates are tried
// near-1, near, near+1 with a strict less-than so a tie keeps the earlier one,
// which is the order the scalar loop uses.
TEXT ·quantizeRVV(SB), NOSPLIT, $0-8
	MOV a+0(FP), X21

	MOV 0(X21), X10
	MOV 8(X21), X11
	MOV 16(X21), X12
	MOV 24(X21), X13
	MOV 32(X21), X14
	MOV 40(X21), X15
	MOV 48(X21), X16
	MOV 56(X21), X17
	MOV 64(X21), X18
	MOV 72(X21), X19

	BEQZ X19, done

	MOVF 80(X21), F0
	MOVF 84(X21), F1
	MOVF 88(X21), F2
	MOVF 92(X21), F3
	MOVF 96(X21), F4
	MOVF 100(X21), F5
	MOVF 104(X21), F6
	MOVF 108(X21), F7
	MOVF 112(X21), F8

	MOV  $quantConstsRVV(SB), X23
	MOVF 0(X23), F9
	MOVF 4(X23), F10
	MOVF 8(X23), F11
	MOVF 12(X23), F12

loop:
	VSETVLI X19, E32, M1, TA, MA, X20

	// Y channel: target = in1 / (my * scaledY)
	VLE32V  (X17), V7
	VFMULVF F5, V7, V7
	VLE32V  (X11), V10
	VFDIVVV V7, V10, V10

	QUANT(F1)

	VSE32V V14, (X14)

	// dy = adjust(qy) * my * scaledY, grouped as the scalar groups it
	VLE32V  (X17), V7
	VFMULVV V7, V15, V21
	VFMULVF F5, V21, V21

	// X channel: target = (in0 - xcc*dy) / (mx * scaledX). The subtraction is
	// fused because the Go compiler contracts it into an FNMSUB on riscv64.
	VLE32V   (X10), V10
	VFNMSACVF V21, F7, V10
	VLE32V   (X16), V7
	VFMULVF  F4, V7, V7
	VFDIVVV  V7, V10, V10

	QUANT(F0)

	VSE32V V14, (X13)

	// B channel: target = (in2 - bcc*dy) / (mb * scaledB), likewise fused
	VLE32V   (X12), V10
	VFNMSACVF V21, F8, V10
	VLE32V   (X18), V7
	VFMULVF  F6, V7, V7
	VFDIVVV  V7, V10, V10

	QUANT(F2)

	VSE32V V14, (X15)

	SLLI $2, X20, X22
	ADD  X22, X10
	ADD  X22, X11
	ADD  X22, X12
	ADD  X22, X13
	ADD  X22, X14
	ADD  X22, X15
	ADD  X22, X16
	ADD  X22, X17
	ADD  X22, X18
	SUB  X20, X19
	BNEZ X19, loop

done:
	RET
