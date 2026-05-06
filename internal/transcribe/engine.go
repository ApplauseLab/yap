package transcribe

import "context"

// Provider identifies which transcription backend to use
type Provider string

const (
	ProviderLocal  Provider = "local"  // whisper.cpp
	ProviderOpenAI Provider = "openai" // OpenAI Whisper API
)

// Model identifies which Whisper model to use
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

// ModelInfo contains metadata about a model
type ModelInfo struct {
	Name        Model  `json:"name"`
	DisplayName string `json:"displayName"`
	Size        string `json:"size"`
	EnglishOnly bool   `json:"englishOnly"`
}

// AvailableModels returns all available models
func AvailableModels() []ModelInfo {
	return []ModelInfo{
		{ModelTiny, "Tiny", "~75MB", false},
		{ModelTinyEn, "Tiny (English)", "~75MB", true},
		{ModelBase, "Base", "~150MB", false},
		{ModelBaseEn, "Base (English)", "~150MB", true},
		{ModelSmall, "Small", "~500MB", false},
		{ModelSmallEn, "Small (English)", "~500MB", true},
		{ModelMedium, "Medium", "~1.5GB", false},
		{ModelMediumEn, "Medium (English)", "~1.5GB", true},
		{ModelLargeV3, "Large V3", "~3GB", false},
	}
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
