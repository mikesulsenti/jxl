## jxl
[![Status](https://github.com/gen2brain/jxl/actions/workflows/test.yml/badge.svg)](https://github.com/gen2brain/jxl/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/gen2brain/jxl.svg)](https://pkg.go.dev/github.com/gen2brain/jxl)

[JPEG XL](https://en.wikipedia.org/wiki/JPEG_XL) image decoder and encoder in pure Go.

A port of [jxl-rs](https://github.com/libjxl/jxl-rs), checked against [libjxl](https://github.com/libjxl/libjxl). No CGo, no dependencies.

SIMD support for amd64 (AVX2, AVX-512), arm64 (NEON) and riscv64 (RVV, with `GORISCV64=rva23u64`).
Build with `-tags noasm` for pure Go everywhere.

### Decoding

```go
img, err := jxl.Decode(r)
```

`jxl.Decode` returns `*image.NRGBA`, or `*image.NRGBA64` above 8 bits, and registers itself with
`image.RegisterFormat`. `jxl.DecodeAll` returns every frame with its delay.

### Encoding

```go
err := jxl.Encode(w, img, jxl.EncodeOptions{Quality: 95})
```

Lossy, within 1.02x to 1.15x of `cjxl` at matched ssimulacra2. Input is treated as sRGB and alpha is dropped.

### Supported

Lossless and lossy, 8 to 32-bit, grayscale and color, alpha, animation, chroma subsampling, patches,
splines, noise, progressive files, embedded ICC profiles, Exif and XMP.

Not implemented: color management through embedded ICC profiles and JPEG reconstruction.

### License

The decoder is a port of jxl-rs and carries the JPEG XL Project Authors' BSD-3-Clause notice, in
[LICENSE](LICENSE), together with the patent grant in [PATENTS](PATENTS).

This project is an implementation of a codec. It gives you no special rights on any patents that may read on JPEG XL.
