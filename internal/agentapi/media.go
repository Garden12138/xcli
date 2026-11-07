package agentapi

import (
	"mime"
	"strings"
	"time"
)

type Input struct {
	APIVersion int       `json:"api_version"`
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	MediaType  string    `json:"media_type"`
	ByteLength int       `json:"byte_length"`
	SHA256     string    `json:"sha256"`
	Validation string    `json:"validation"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type Provenance struct {
	Status         string  `json:"status"`
	Method         *string `json:"method"`
	ObservedTool   *string `json:"observed_tool"`
	EvidenceSHA256 *string `json:"evidence_sha256"`
	ReviewerID     *string `json:"reviewer_id"`
}
type ReviewEvidence struct {
	ReviewerID     string    `json:"reviewer_id"`
	EvidenceSHA256 string    `json:"evidence_sha256"`
	CheckedAt      time.Time `json:"checked_at"`
}
type Artifact struct {
	SemanticReview  *ReviewEvidence `json:"semantic_review"`
	APIVersion      int             `json:"api_version"`
	ID              string          `json:"id"`
	TaskID          string          `json:"task_id"`
	SessionID       string          `json:"session_id"`
	MediaType       string          `json:"media_type"`
	ByteLength      int             `json:"byte_length"`
	Width           int             `json:"width"`
	Height          int             `json:"height"`
	SHA256          string          `json:"sha256"`
	CreatedAt       time.Time       `json:"created_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	DeliveryState   string          `json:"delivery_state"`
	Provenance      Provenance      `json:"provenance"`
	Validation      Validation      `json:"validation"`
	ReferenceSHA256 *string         `json:"reference_sha256"`
}
type Attestation struct {
	Source         string `json:"source"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}
type Review struct {
	Decision              string       `json:"decision"`
	ArtifactID            string       `json:"artifact_id"`
	ExpectedSHA256        string       `json:"expected_sha256"`
	SemanticCheck         string       `json:"semantic_check,omitempty"`
	ProvenanceAttestation *Attestation `json:"provenance_attestation,omitempty"`
}

func InputType(value string) (string, int64, error) {
	typ, params, e := mime.ParseMediaType(value)
	if e != nil {
		return "", 0, Failure(422, "unsupported_parameter")
	}
	if typ == "text/plain" && len(params) == 1 && strings.ToLower(params["charset"]) == "utf-8" {
		return "text/plain; charset=utf-8", 1 << 20, nil
	}
	if len(params) == 0 && (typ == "image/png" || typ == "image/jpeg" || typ == "image/webp") {
		return typ, 20 << 20, nil
	}
	return "", 0, Failure(422, "unsupported_parameter")
}
