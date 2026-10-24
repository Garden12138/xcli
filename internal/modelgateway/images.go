package modelgateway

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/Garden12138/xcli/internal/model"
)

const maxImageRequestBytes = 32 << 20

func imageParameters(req model.Object) (int, string, error) {
	n := 1
	if raw, ok := req["n"]; ok {
		if json.Unmarshal(raw, &n) != nil || n < 1 || n > 4 {
			return 0, "", model.Failure(400, "invalid_request", "image n must be between 1 and 4")
		}
	}
	if strings.TrimSpace(req.String("prompt")) == "" {
		return 0, "", model.Failure(400, "invalid_request", "image prompt is required")
	}
	if raw, ok := req["stream"]; ok && string(raw) != "false" {
		return 0, "", model.Failure(422, "unsupported_capability", "image streaming is not supported")
	}
	if format := req.String("response_format"); format != "" && format != "b64_json" {
		return 0, "", model.Failure(422, "unsupported_capability", "only verified b64_json image output is supported")
	}
	size := req.String("size")
	if size != "" && size != "auto" {
		w, h, err := parseSize(size)
		if err != nil || int64(w)*int64(h) > model.MaxImagePixels {
			return 0, "", model.Failure(400, "invalid_request", "invalid or excessive image size")
		}
	}
	return n, size, nil
}

func parseSize(size string) (int, int, error) {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("size")
	}
	w, e1 := strconv.Atoi(parts[0])
	h, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || w < 1 || h < 1 || w > model.MaxImagePixels || h > model.MaxImagePixels {
		return 0, 0, fmt.Errorf("size")
	}
	return w, h, nil
}

func (s *Server) generate(w http.ResponseWriter, r *http.Request, c Client) {
	req, err := readObject(w, r, maxJSONBody)
	if err != nil {
		writeError(w, err)
		return
	}
	n, size, err := imageParameters(req)
	if err != nil {
		writeError(w, err)
		return
	}
	// Reject reference-shaped generation fields rather than silently dropping an intended edit.
	for _, field := range []string{"image", "images", "mask", "image_url", "reference_images"} {
		if req[field] != nil {
			writeError(w, model.Failure(422, "unsupported_capability", "reference images require the image edits endpoint"))
			return
		}
	}
	d, err := s.resolve(c, req.String("model"), model.ImageGenerate)
	if err != nil {
		writeError(w, err)
		return
	}
	if err = s.consent(r, c, d); err != nil {
		writeError(w, err)
		return
	}
	b := s.options.Images[d.BackendID]
	if b == nil {
		writeError(w, model.Failure(503, "backend_unavailable", "configured image backend unavailable"))
		return
	}
	result, err := b.Generate(r.Context(), req.WithModel(d.UpstreamModel))
	if r.Context().Err() != nil {
		err = r.Context().Err()
	}
	if err != nil {
		writeError(w, requestError(err))
		return
	}
	if err = validateImages(result, n, size); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func validateImages(result model.Object, n int, size string) error {
	var data []model.Object
	bad := func() error {
		return model.Failure(502, "invalid_upstream_image", "upstream did not return the requested number of valid inline images")
	}
	if json.Unmarshal(result["data"], &data) != nil || len(data) != n || result["created"] == nil {
		return bad()
	}
	total := 0
	for _, entry := range data {
		if entry["url"] != nil {
			return model.Failure(502, "upstream_image_url_disabled", "upstream image URL downloads are disabled; configure native base64 output")
		}
		encoded := entry.String("b64_json")
		if len(encoded) > base64.StdEncoding.EncodedLen(model.MaxImageBytes) {
			return bad()
		}
		total += len(encoded)
		if total > maxImageRequestBytes {
			return bad()
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil {
			return bad()
		}
		info, err := model.ValidateImage(decoded, "")
		if err != nil {
			return bad()
		}
		if size != "" && size != "auto" {
			w, h, _ := parseSize(size)
			if w != info.Width || h != info.Height {
				return model.Failure(502, "image_size_mismatch", "upstream image dimensions do not match requested size")
			}
		}
	}
	return nil
}

func (s *Server) edit(w http.ResponseWriter, r *http.Request, c Client) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "multipart/form-data" {
		writeError(w, model.Failure(415, "invalid_content_type", "multipart/form-data is required"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImageRequestBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, model.Failure(400, "invalid_request", "invalid multipart body"))
		return
	}
	req := model.ImageEditRequest{Fields: map[string]string{}}
	total := 0
	parts := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, model.Failure(413, "invalid_or_excessive_upload", "multipart body invalid or exceeds request limit"))
			return
		}
		parts++
		if parts > 32 {
			part.Close()
			writeError(w, model.Failure(413, "too_many_parts", "multipart part limit exceeded"))
			return
		}
		name := part.FormName()
		isFile := name == "image" || name == "image[]" || name == "mask"
		limit := int64(64 << 10)
		if isFile {
			limit = model.MaxImageBytes
		}
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		part.Close()
		total += len(data)
		if err != nil || int64(len(data)) > limit || total > maxImageRequestBytes {
			writeError(w, model.Failure(413, "upload_too_large", "multipart field or file exceeds byte limit"))
			return
		}
		if isFile {
			info, err := model.ValidateImage(data, part.Header.Get("Content-Type"))
			if err != nil {
				writeError(w, err)
				return
			}
			input := model.ImageInput{Name: "upload", MIME: info.MIME, Data: data}
			if name == "mask" {
				if req.Mask != nil {
					writeError(w, model.Failure(400, "invalid_request", "duplicate mask"))
					return
				}
				req.Mask = &input
			} else {
				req.Images = append(req.Images, input)
				if len(req.Images) > 4 {
					writeError(w, model.Failure(413, "too_many_images", "at most four reference images are supported"))
					return
				}
			}
		} else {
			_, duplicate := req.Fields[name]
			if part.FileName() != "" || duplicate {
				writeError(w, model.Failure(400, "invalid_request", "unexpected file or duplicate form field"))
				return
			}
			switch name {
			case "model", "prompt", "n", "size", "response_format", "quality", "background", "output_format", "input_fidelity", "output_compression", "user":
				req.Fields[name] = string(data)
			default:
				writeError(w, model.Failure(422, "unsupported_parameter", "unsupported edit field"))
				return
			}
		}
	}
	if len(req.Images) == 0 {
		writeError(w, model.Failure(400, "invalid_request", "image upload is required"))
		return
	}
	parameters := model.Object{}
	for key, value := range req.Fields {
		parameters[key], _ = json.Marshal(value)
	}
	if v, ok := req.Fields["n"]; ok {
		parameters["n"] = json.RawMessage(v)
	}
	n, size, err := imageParameters(parameters)
	if err != nil {
		writeError(w, err)
		return
	}
	caps := []model.Capability{model.ImageEdit}
	if len(req.Images) > 1 {
		caps = append(caps, model.ImageReference)
	}
	if req.Mask != nil {
		caps = append(caps, model.ImageMask)
		mask, _ := model.ValidateImage(req.Mask.Data, req.Mask.MIME)
		first, _ := model.ValidateImage(req.Images[0].Data, req.Images[0].MIME)
		if mask.MIME != "image/png" || mask.Width != first.Width || mask.Height != first.Height {
			writeError(w, model.Failure(422, "invalid_mask", "mask must be a PNG with the first image's dimensions"))
			return
		}
	}
	d, err := s.resolve(c, req.Fields["model"], caps...)
	if err != nil {
		writeError(w, err)
		return
	}
	if err = s.consent(r, c, d); err != nil {
		writeError(w, err)
		return
	}
	b := s.options.Images[d.BackendID]
	if b == nil {
		writeError(w, model.Failure(503, "backend_unavailable", "configured image backend unavailable"))
		return
	}
	req.Fields["model"] = d.UpstreamModel
	result, err := b.Edit(r.Context(), req)
	if r.Context().Err() != nil {
		err = r.Context().Err()
	}
	if err != nil {
		writeError(w, requestError(err))
		return
	}
	if err = validateImages(result, n, size); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
