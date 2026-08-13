//go:build amd64 && !noasm

#include "textflag.h"

// cpuidAVX2 reports whether the CPU has AVX2 and FMA and the OS saves the YMM
// state.
TEXT ·cpuidAVX2(SB), NOSPLIT, $0-1
	MOVL $0, AX
	CPUID
	CMPL AX, $7
	JL   no

	MOVL $1, AX
	MOVL $0, CX
	CPUID
	BTL  $27, CX // OSXSAVE
	JNC  no

	MOVL   $0, CX
	XGETBV
	ANDL   $6, AX // XMM and YMM state
	CMPL   AX, $6
	JNE    no

	MOVL $7, AX
	MOVL $0, CX
	CPUID
	BTL  $5, BX // AVX2
	JNC  no

	MOVB $1, ret+0(FP)
	RET

no:
	MOVB $0, ret+0(FP)
	RET

// cpuidAVX512 reports whether the CPU has the AVX-512 subsets the kernels use
// and the OS saves the opmask and ZMM state.
TEXT ·cpuidAVX512(SB), NOSPLIT, $0-1
	MOVL $0, AX
	CPUID
	CMPL AX, $7
	JL   no512

	MOVL $1, AX
	MOVL $0, CX
	CPUID
	ANDL $0x18000000, CX // OSXSAVE and AVX
	CMPL CX, $0x18000000
	JNE  no512

	MOVL   $0, CX
	XGETBV
	MOVL   AX, DX
	ANDL   $6, AX    // XMM and YMM state
	CMPL   AX, $6
	JNE    no512
	ANDL   $0xe0, DX // opmask, ZMM_Hi256 and Hi16_ZMM
	CMPL   DX, $0xe0
	JNE    no512

	MOVL $7, AX
	MOVL $0, CX
	CPUID
	ANDL $0xc0030000, BX // F, DQ, BW, VL
	CMPL BX, $0xc0030000
	JNE  no512

	MOVB $1, ret+0(FP)
	RET

no512:
	MOVB $0, ret+0(FP)
	RET
