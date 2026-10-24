package modelgateway

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"

	"github.com/Garden12138/xcli/internal/model"
)

func embeddingCount(input json.RawMessage) (int, error) {
	bad := func() (int, error) {
		return 0, model.Failure(400, "invalid_request", "embedding input must be nonempty text, text array, token IDs or token arrays")
	}
	var text string
	if json.Unmarshal(input, &text) == nil && text != "" {
		return 1, nil
	}
	var items []json.RawMessage
	if json.Unmarshal(input, &items) != nil || len(items) == 0 || len(items) > 2048 {
		return bad()
	}
	if len(items[0]) == 0 {
		return bad()
	}
	tokens := func(raw json.RawMessage) bool {
		var ids []int
		if json.Unmarshal(raw, &ids) != nil || len(ids) == 0 || len(ids) > 65536 {
			return false
		}
		for _, id := range ids {
			if id < 0 || int64(id) > math.MaxInt32 {
				return false
			}
		}
		return true
	}
	if items[0][0] != '"' && items[0][0] != '[' {
		if tokens(input) {
			return 1, nil
		}
		return bad()
	}
	for _, item := range items {
		if items[0][0] == '"' {
			if json.Unmarshal(item, &text) != nil || text == "" || bytes.Equal(item, []byte("null")) {
				return bad()
			}
		} else if !tokens(item) {
			return bad()
		}
	}
	return len(items), nil
}

func (s *Server) embed(w http.ResponseWriter, r *http.Request, c Client) {
	req, err := readObject(w, r, maxJSONBody)
	if err != nil {
		writeError(w, err)
		return
	}
	count, err := embeddingCount(req["input"])
	if err != nil {
		writeError(w, err)
		return
	}
	d, err := s.resolve(c, req.String("model"), model.Embeddings)
	if err != nil {
		writeError(w, err)
		return
	}
	if err = s.consent(r, c, d); err != nil {
		writeError(w, err)
		return
	}
	if raw, ok := req["dimensions"]; ok {
		var dims int
		if json.Unmarshal(raw, &dims) != nil || dims != d.Metadata.Dimensions {
			writeError(w, model.Failure(422, "embedding_dimensions_mismatch", "requested dimensions must match the configured model alias; use a separate alias for another dimension"))
			return
		}
	}
	encoding := req.String("encoding_format")
	if encoding == "" {
		encoding = "float"
	}
	if encoding != "float" && encoding != "base64" {
		writeError(w, model.Failure(422, "unsupported_capability", "embedding encoding_format must be float or base64"))
		return
	}
	b := s.options.Embeddings[d.BackendID]
	if b == nil {
		writeError(w, model.Failure(503, "backend_unavailable", "no configured embedding service is available"))
		return
	}
	result, err := b.Embed(r.Context(), req.WithModel(d.UpstreamModel))
	if r.Context().Err() != nil {
		err = r.Context().Err()
	}
	if err != nil {
		writeError(w, requestError(err))
		return
	}
	if err = validateEmbeddings(result, count, d.Metadata.Dimensions, encoding); err != nil {
		writeError(w, err)
		return
	}
	result = result.WithModel(d.ID)
	result["xcli"], _ = json.Marshal(map[string]any{"dimensions": d.Metadata.Dimensions, "index_version": d.Metadata.IndexVersion, "index_fingerprint": d.Metadata.IndexFingerprint, "inference_location": d.Metadata.InferenceLocation})
	writeJSON(w, 200, result)
}

func validateEmbeddings(result model.Object, count, dimensions int, encoding string) error {
	bad := func() error {
		return model.Failure(502, "invalid_upstream_embeddings", "embedding count, indices, encoding or dimensions do not match the configured model")
	}
	if result.String("object") != "list" {
		return bad()
	}
	var data []model.Object
	if json.Unmarshal(result["data"], &data) != nil || len(data) != count {
		return bad()
	}
	for i, entry := range data {
		var index int
		if json.Unmarshal(entry["index"], &index) != nil || index != i || entry.String("object") != "embedding" {
			return bad()
		}
		if encoding == "base64" {
			value := entry.String("embedding")
			if len(value) != base64.StdEncoding.EncodedLen(dimensions*4) {
				return bad()
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(value)
			if err != nil || len(decoded) != dimensions*4 {
				return bad()
			}
			for offset := 0; offset < len(decoded); offset += 4 {
				v := float64(math.Float32frombits(binary.LittleEndian.Uint32(decoded[offset:])))
				if math.IsNaN(v) || math.IsInf(v, 0) {
					return bad()
				}
			}
		} else {
			var values []json.RawMessage
			if json.Unmarshal(entry["embedding"], &values) != nil || len(values) != dimensions {
				return bad()
			}
			for _, raw := range values {
				var v float64
				if bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &v) != nil || math.IsNaN(v) || math.IsInf(v, 0) {
					return bad()
				}
			}
		}
	}
	return nil
}
