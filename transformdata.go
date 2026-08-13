package jxl

type opsinInverseMatrix struct {
	inverse [9]float32
	biases  [3]float32
	quant   [4]float32
}

func defaultOpsinInverseMatrix() opsinInverseMatrix {
	return opsinInverseMatrix{
		inverse: [9]float32{
			11.031566901960783, -9.866943921568629, -0.16462299647058826,
			-3.254147380392157, 4.418770392156863, -0.16462299647058826,
			-3.6588512862745097, 2.7129230470588235, 1.9459282392156863,
		},
		biases: [3]float32{
			-0.0037930732552754493, -0.0037930732552754493, -0.0037930732552754493,
		},
		quant: [4]float32{
			1.0 - 0.05465007330715401, 1.0 - 0.07005449891748593,
			1.0 - 0.049935103337343655, 0.145,
		},
	}
}

func (o *opsinInverseMatrix) read(r *bitReader) {
	*o = defaultOpsinInverseMatrix()

	if r.bit() {
		return
	}

	for i := range o.inverse {
		o.inverse[i] = r.f16()
	}
	for i := range o.biases {
		o.biases[i] = r.f16()
	}
	for i := range o.quant {
		o.quant[i] = r.f16()
	}
}

type customTransformData struct {
	opsin    opsinInverseMatrix
	weights2 [15]float32
	weights4 [55]float32
	weights8 [210]float32
}

func defaultCustomTransformData() customTransformData {
	return customTransformData{
		opsin:    defaultOpsinInverseMatrix(),
		weights2: defaultKern2,
		weights4: defaultKern4,
		weights8: defaultKern8,
	}
}

func (c *customTransformData) read(r *bitReader, xybEncoded bool) {
	*c = defaultCustomTransformData()

	if r.bit() {
		return
	}

	if xybEncoded {
		c.opsin.read(r)
	}

	mask := uint32(r.bits(3))

	if mask&1 != 0 {
		for i := range c.weights2 {
			c.weights2[i] = r.f16()
		}
	}
	if mask&2 != 0 {
		for i := range c.weights4 {
			c.weights4[i] = r.f16()
		}
	}
	if mask&4 != 0 {
		for i := range c.weights8 {
			c.weights8[i] = r.f16()
		}
	}
}
