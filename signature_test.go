package jxl

import (
	"errors"
	"testing"
)

func TestSignature(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		container bool
		skip      int
		err       error
	}{
		{"codestream", sigCodestream + "rest", false, 0, nil},
		{"container", sigContainer + "rest", true, 12, nil},
		{"empty", "", false, 0, errSignature},
		{"marker only first byte", "\xff", false, 0, errSignature},
		{"jpeg", "\xff\xd8\xff\xe0", false, 0, errSignature},
		{"truncated container", sigContainer[:11], false, 0, errSignature},
		{"box of the right size, wrong type", "\x00\x00\x00\x0cJXL!\x0d\x0a\x87\x0a", false, 0, errSignature},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container, skip, err := signature([]byte(tt.in))

			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if container != tt.container {
				t.Errorf("container = %v, want %v", container, tt.container)
			}
			if skip != tt.skip {
				t.Errorf("skip = %d, want %d", skip, tt.skip)
			}
		})
	}
}
