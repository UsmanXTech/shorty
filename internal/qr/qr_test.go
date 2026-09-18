package qr

import (
	"bytes"
	"testing"
)

func TestGeneratePNG(t *testing.T) {
	data, err := Generate("https://short.example/abc", 256)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("expected PNG output")
	}
}

func TestGenerateDefaultsAndRejectsInvalidSizes(t *testing.T) {
	if _, err := Generate("https://short.example/abc", 0); err != nil {
		t.Fatalf("default size failed: %v", err)
	}
	for _, size := range []int{1, 127, 1025} {
		if _, err := Generate("https://short.example/abc", size); err != ErrInvalidSize {
			t.Fatalf("size %d: expected ErrInvalidSize, got %v", size, err)
		}
	}
	if _, err := Generate("", 256); err == nil {
		t.Fatal("expected empty content to fail")
	}
}
