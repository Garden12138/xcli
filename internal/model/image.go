package model

import (
	"encoding/base64"
	"github.com/Garden12138/xcli/internal/media"
	"strings"
)

const MaxImageBytes = media.MaxImageBytes
const MaxImagePixels = media.MaxImagePixels

type ImageInfo = media.ImageInfo

func ValidateImage(data []byte, claimed string) (ImageInfo, error) {
	info, e := media.ValidateImage(data, claimed)
	if e == nil {
		return info, nil
	}
	m := e.(*media.Error)
	message := "image must be a complete supported PNG, JPEG or WebP"
	if m.Code == "image_too_large" {
		message = "image exceeds byte limit"
	}
	if m.Code == "image_dimensions_exceeded" {
		message = "image exceeds pixel limit"
	}
	return ImageInfo{}, Failure(m.Status, m.Code, message)
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
