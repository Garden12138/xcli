package modelbackend

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"sort"

	"github.com/Garden12138/xcli/internal/model"
)

func (b *OpenAI) Generate(ctx context.Context, req model.Object) (model.Object, error) {
	return b.jsonRequest(ctx, "/images/generations", req)
}

func (b *OpenAI) Edit(ctx context.Context, req model.ImageEditRequest) (model.Object, error) {
	// Inputs are bounded in memory before constructing the multipart request; no temp files.
	var buffer bytes.Buffer
	w := multipart.NewWriter(&buffer)
	keys := make([]string, 0, len(req.Fields))
	for key := range req.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := w.WriteField(key, req.Fields[key]); err != nil {
			return nil, err
		}
	}
	file := func(field string, input model.ImageInput) error {
		header := textproto.MIMEHeader{}
		// Use fixed safe names. Client upload filenames never become paths or header values.
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="upload"`, field))
		header.Set("Content-Type", input.MIME)
		part, err := w.CreatePart(header)
		if err != nil {
			return err
		}
		_, err = part.Write(input.Data)
		return err
	}
	field := "image"
	if len(req.Images) > 1 {
		field = "image[]"
	}
	for _, input := range req.Images {
		if err := file(field, input); err != nil {
			return nil, err
		}
	}
	if req.Mask != nil {
		if err := file("mask", *req.Mask); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	res, err := b.request(ctx, "/images/edits", w.FormDataContentType(), &buffer)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return decodeResponse(res.Body)
}
