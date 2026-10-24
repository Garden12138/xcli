package model

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"strings"

	_ "golang.org/x/image/webp"
)

const MaxImageBytes = 20 << 20
const MaxImagePixels = 16 << 20

type ImageInfo struct {
	MIME          string
	Width, Height int
}

// ValidateImage checks dimensions before allocating and verifying the complete decoded raster.
func ValidateImage(data []byte, claimed string) (ImageInfo, error) {
	bad := func() (ImageInfo, error) {
		return ImageInfo{}, Failure(422, "invalid_image", "image must be a complete supported PNG, JPEG or WebP")
	}
	if len(data) == 0 {
		return bad()
	}
	if len(data) > MaxImageBytes {
		return ImageInfo{}, Failure(413, "image_too_large", "image exceeds byte limit")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return bad()
	}
	mimeType := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "webp": "image/webp"}[format]
	if mimeType == "" {
		return bad()
	}
	if config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > MaxImagePixels {
		return ImageInfo{}, Failure(413, "image_dimensions_exceeded", "image exceeds pixel limit")
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

func DecodeDataImage(value string) (ImageInfo, error) {
	if !strings.HasPrefix(value, "data:") {
		return ImageInfo{}, Failure(422, "image_url_not_allowed", "Vision requires an inline data URL; remote and file URLs are disabled")
	}
	parts := strings.SplitN(strings.TrimPrefix(value, "data:"), ",", 2)
	if len(parts) != 2 || !strings.HasSuffix(parts[0], ";base64") {
		return ImageInfo{}, Failure(422, "invalid_image", "image data URL must use base64")
	}
	if len(parts[1]) > base64.StdEncoding.EncodedLen(MaxImageBytes) {
		return ImageInfo{}, Failure(413, "image_too_large", "inline image exceeds byte limit")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return ImageInfo{}, Failure(422, "invalid_image", "invalid image base64")
	}
	return ValidateImage(data, strings.TrimSuffix(parts[0], ";base64"))
}
