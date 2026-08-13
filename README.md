## jxl
[![Status](https://github.com/gen2brain/jxl/actions/workflows/test.yml/badge.svg)](https://github.com/gen2brain/jxl/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/gen2brain/jxl.svg)](https://pkg.go.dev/github.com/gen2brain/jxl)

[JPEG XL](https://en.wikipedia.org/wiki/JPEG_XL) image decoder in pure Go.

A port of [jxl-rs](https://github.com/libjxl/jxl-rs), checked against
[libjxl](https://github.com/libjxl/libjxl). No CGo, no dependencies.

Both Modular and VarDCT decode, and the whole of the `main_level10` conformance suite passes:
lossless and lossy, palettes, Squeeze, RCT, all 27 DCT transform types, chroma subsampling,
XYB and YCbCr, Gaborish and EPF, patches, splines, noise, upsampling, progressive LF frames,
animation and layer blending, and embedded ICC profiles.

Not implemented: colour management through embedded ICC profiles, JPEG reconstruction, and
Brotli-compressed metadata boxes. See `docs/STATUS.md`.

### Decoding

```go
img, err := jxl.Decode(r)
```

`jxl.Decode` returns `*image.NRGBA`, or `*image.NRGBA64` above 8 bits, and registers itself
with `image.RegisterFormat`. `jxl.DecodeAll` returns every frame of an animation with its
delay, and `jxl.DecodeFloat` returns the decoder's native float samples.

### License

The decoder is a port of jxl-rs and carries the JPEG XL Project Authors' BSD-3-Clause notice,
in [LICENSE](LICENSE), together with the patent grant in [PATENTS](PATENTS).

This project is an implementation of a decoder. It gives you no special rights on any patents
that may read on JPEG XL.
