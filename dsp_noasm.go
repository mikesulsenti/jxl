//go:build noasm || (!amd64 && !arm64 && !(riscv64 && riscv64.rva23u64))

package jxl

func dspInit() {}

func idct8x8([]float32) bool { return false }

func idctCols8(int, []float32, int, []float32, int, []float32) bool { return false }

func transpose8([]float32, int, []float32, int) {}
