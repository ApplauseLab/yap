package transcribe

import (
	"context"
	"strings"

	"yap/internal/models"
)

// Provider identifies which transcription backend to use
type Provider string

const (
	ProviderLocal  Provider = "local"  // whisper.cpp
	ProviderOpenAI Provider = "openai" // OpenAI Whisper API
)

// Model identifies which Whisper model to use. Metadata for each model lives
// in the canonical catalog in internal/models.
type Model string

const (
	ModelTiny     Model = "tiny"
	ModelTinyEn   Model = "tiny.en"
	ModelBase     Model = "base"
	ModelBaseEn   Model = "base.en"
	ModelSmall    Model = "small"
	ModelSmallEn  Model = "small.en"
	ModelMedium   Model = "medium"
	ModelMediumEn Model = "medium.en"
	ModelLargeV3  Model = "large-v3"
)

// WhisperLanguage returns the language passed to whisper.cpp for m: "en" for
// English-only models and "auto" (detect per recording) otherwise. whisper.cpp
// itself defaults to "en", which makes multilingual models translate other
// languages into English instead of transcribing them.
func (m Model) WhisperLanguage() string {
	if spec, ok := models.LookupModel(string(m)); ok {
		if spec.EnglishOnly {
			return "en"
		}
		return "auto"
	}
	if strings.HasSuffix(string(m), ".en") {
		return "en"
	}
	return "auto"
}

// KnownModels lists the typed identifiers of every catalog model.
func KnownModels() []Model {
	specs := models.Catalog()
	out := make([]Model, len(specs))
	for i, spec := range specs {
		out[i] = Model(spec.Name)
	}
	return out
}

// Engine defines the interface for transcription backends
type Engine interface {
	// Transcribe converts audio samples to text
	// samples should be 16kHz mono float32
	Transcribe(ctx context.Context, samples []float32) (string, error)

	// TranscribeWAV transcribes a WAV file
	TranscribeWAV(ctx context.Context, wavData []byte) (string, error)

	// SetModel sets the model to use
	SetModel(model Model) error

	// GetModel returns the current model
	GetModel() Model

	// IsAvailable checks if the engine is ready to use
	IsAvailable() bool

	// Name returns the provider name
	Name() Provider
}

// Result contains transcription output
type Result struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence,omitempty"`
	Duration   float64 `json:"duration,omitempty"` // seconds
}
