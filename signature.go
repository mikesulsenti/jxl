package jxl

import "errors"

const (
	sigCodestream = "\xff\x0a"

	sigContainer = "\x00\x00\x00\x0cJXL \x0d\x0a\x87\x0a"
)

var errSignature = errors.New("jxl: not a JPEG XL file")

func signature(b []byte) (container bool, skip int, err error) {
	switch {
	case len(b) >= len(sigContainer) && string(b[:len(sigContainer)]) == sigContainer:
		return true, len(sigContainer), nil
	case len(b) >= len(sigCodestream) && string(b[:len(sigCodestream)]) == sigCodestream:
		return false, 0, nil
	}

	return false, 0, errSignature
}
