package qr

import (
	"errors"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	DefaultSize = 256
	MinSize     = 128
	MaxSize     = 1024
)

var ErrInvalidSize = errors.New("QR size must be between 128 and 1024 pixels")

// Generate returns a PNG QR code for the supplied content.
func Generate(content string, size int) ([]byte, error) {
	if content == "" {
		return nil, errors.New("QR content cannot be empty")
	}
	if size == 0 {
		size = DefaultSize
	}
	if size < MinSize || size > MaxSize {
		return nil, ErrInvalidSize
	}
	return qrcode.Encode(content, qrcode.Medium, size)
}
