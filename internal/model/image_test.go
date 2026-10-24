package model

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestImageValidationFormatsAndBoundaries(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	var pngBytes, jpegBytes bytes.Buffer
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegBytes, img, nil); err != nil {
		t.Fatal(err)
	}
	webp, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4TBEAAAAvAAAAAAfQ//73v/+BiOh/AAA=")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		data []byte
		mime string
		w, h int
	}{{pngBytes.Bytes(), "image/png", 2, 3}, {jpegBytes.Bytes(), "image/jpeg", 2, 3}, {webp, "image/webp", 1, 1}} {
		info, err := ValidateImage(fixture.data, fixture.mime)
		if err != nil || info.Width != fixture.w || info.Height != fixture.h {
			t.Fatalf("%s: %+v %v", fixture.mime, info, err)
		}
		if _, err := ValidateImage(fixture.data, "image/gif"); err == nil {
			t.Fatal("MIME mismatch accepted")
		}
	}
	if _, err := ValidateImage(pngBytes.Bytes()[:pngBytes.Len()/2], "image/png"); err == nil {
		t.Fatal("truncated raster accepted")
	}
	if _, err := ValidateImage([]byte("not an image"), ""); err == nil {
		t.Fatal("nonimage accepted")
	}
	large := bytes.Clone(pngBytes.Bytes())
	binary.BigEndian.PutUint32(large[16:20], 20000)
	binary.BigEndian.PutUint32(large[20:24], 20000)
	binary.BigEndian.PutUint32(large[29:33], crc32.ChecksumIEEE(large[12:29]))
	if _, err := ValidateImage(large, ""); err == nil {
		t.Fatal("pixel bomb accepted")
	}
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	if _, err := DecodeDataImage(valid); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"file:///etc/passwd", "http://127.0.0.1/private", "https://example.com/image", "data:image/png;base64,%%%"} {
		if _, err := DecodeDataImage(bad); err == nil {
			t.Fatal("unsafe image accepted", bad)
		}
	}
}
