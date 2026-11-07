// Package media validates original image bytes without transport-specific errors.
package media

import (
	"bytes"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"mime"
)

type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string                       { return e.Code }
func failure(status int, code, message string) error { return &Error{status, code} }

const MaxImageBytes = 20 << 20
const MaxImagePixels = 16 << 20

type ImageInfo struct {
	MIME          string
	Width, Height int
}

// ValidateImage checks dimensions before allocating and verifying the complete decoded raster.
func ValidateImage(data []byte, claimed string) (ImageInfo, error) {
	bad := func() (ImageInfo, error) {
		return ImageInfo{}, failure(422, "invalid_image", "image must be a complete supported PNG, JPEG or WebP")
	}
	if len(data) == 0 {
		return bad()
	}
	if len(data) > MaxImageBytes {
		return ImageInfo{}, failure(413, "image_too_large", "image exceeds byte limit")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return bad()
	}
	mimeType := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"}[format]
	if mimeType == "" {
		return bad()
	}
	if config.Width < 1 || config.Height < 1 || config.Width > MaxImagePixels/config.Height {
		return ImageInfo{}, failure(413, "image_dimensions_exceeded", "image exceeds pixel limit")
	}
	if claimed != "" && claimed != "application/octet-stream" {
		typ, _, err := mime.ParseMediaType(claimed)
		if err != nil || typ != mimeType {
			return bad()
		}
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return bad()
	}
	return ImageInfo{MIME: mimeType, Width: config.Width, Height: config.Height}, nil
}
