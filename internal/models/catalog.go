package models

import (
	"fmt"
	"os"
	"strings"
)

const (
	// BaseURL is the base URL for downloading models
	BaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"

	// ModelBaseURLEnv overrides BaseURL, e.g. to point development builds at a
	// local mirror for exercising download failures.
	ModelBaseURLEnv = "YAP_MODEL_BASE_URL"

	// ProviderLocal identifies models run on-device by whisper.cpp.
	ProviderLocal = "local"
)

// ModelSpec is the canonical description of a supported transcription model.
// Speed and Accuracy are curated relative ratings from 1 (lowest) to 5.
// DisplayName is shared by a model's English-only and multilingual variants;
// EnglishOnly distinguishes them.
type ModelSpec struct {
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	Filename       string `json:"filename"`
	SizeBytes      int64  `json:"sizeBytes"`
	Size           string `json:"size"`
	EnglishOnly    bool   `json:"englishOnly"`
	Provider       string `json:"provider"`
	Speed          int    `json:"speed"`
	Accuracy       int    `json:"accuracy"`
	UseCase        string `json:"useCase"`
	Recommendation string `json:"recommendation"`
}

var catalog = []ModelSpec{
	{
		Name: "tiny.en", DisplayName: "Whisper Tiny", Filename: "ggml-tiny.en.bin",
		SizeBytes: 77704715, Size: "78 MB", EnglishOnly: true, Provider: ProviderLocal,
		Speed: 5, Accuracy: 1,
		UseCase:        "Quick notes on older or low-power machines where speed matters most.",
		Recommendation: "Best for speed",
	},
	{
		Name: "tiny", DisplayName: "Whisper Tiny", Filename: "ggml-tiny.bin",
		SizeBytes: 77691713, Size: "78 MB", EnglishOnly: false, Provider: ProviderLocal,
		Speed: 5, Accuracy: 1,
		UseCase: "Fast rough drafts in any language on low-power machines.",
	},
	{
		Name: "base.en", DisplayName: "Whisper Base", Filename: "ggml-base.en.bin",
		SizeBytes: 147964211, Size: "148 MB", EnglishOnly: true, Provider: ProviderLocal,
		Speed: 4, Accuracy: 2,
		UseCase:        "Everyday English dictation with a good balance of speed and accuracy.",
		Recommendation: "Recommended",
	},
	{
		Name: "base", DisplayName: "Whisper Base", Filename: "ggml-base.bin",
		SizeBytes: 147951465, Size: "148 MB", EnglishOnly: false, Provider: ProviderLocal,
		Speed: 4, Accuracy: 2,
		UseCase: "Everyday dictation in many languages with modest hardware needs.",
	},
	{
		Name: "small.en", DisplayName: "Whisper Small", Filename: "ggml-small.en.bin",
		SizeBytes: 487614201, Size: "488 MB", EnglishOnly: true, Provider: ProviderLocal,
		Speed: 3, Accuracy: 3,
		UseCase:        "Longer English dictation where accuracy matters more than speed.",
		Recommendation: "Better accuracy",
	},
	{
		Name: "small", DisplayName: "Whisper Small", Filename: "ggml-small.bin",
		SizeBytes: 487601967, Size: "488 MB", EnglishOnly: false, Provider: ProviderLocal,
		Speed: 3, Accuracy: 3,
		UseCase:        "The best everyday balance for languages other than English.",
		Recommendation: "Recommended multilingual",
	},
	{
		Name: "medium.en", DisplayName: "Whisper Medium", Filename: "ggml-medium.en.bin",
		SizeBytes: 1533774781, Size: "1.5 GB", EnglishOnly: true, Provider: ProviderLocal,
		Speed: 2, Accuracy: 4,
		UseCase: "High-accuracy English on fast machines with plenty of memory.",
	},
	{
		Name: "medium", DisplayName: "Whisper Medium", Filename: "ggml-medium.bin",
		SizeBytes: 1533763059, Size: "1.5 GB", EnglishOnly: false, Provider: ProviderLocal,
		Speed: 2, Accuracy: 4,
		UseCase: "High-accuracy multilingual transcription on fast machines.",
	},
	{
		Name: "large-v3", DisplayName: "Whisper Large v3", Filename: "ggml-large-v3.bin",
		SizeBytes: 3095033483, Size: "3.1 GB", EnglishOnly: false, Provider: ProviderLocal,
		Speed: 1, Accuracy: 5,
		UseCase:        "Maximum accuracy in any language; slow and memory hungry.",
		Recommendation: "Most accurate",
	},
}

// Catalog returns every supported model, ordered from fastest to most accurate.
func Catalog() []ModelSpec {
	out := make([]ModelSpec, len(catalog))
	copy(out, catalog)
	return out
}

// LookupModel returns the catalog entry for name.
func LookupModel(name string) (ModelSpec, bool) {
	for _, spec := range catalog {
		if spec.Name == name {
			return spec, true
		}
	}
	return ModelSpec{}, false
}

// RequireModel returns the catalog entry for name or an unknown-model error.
func RequireModel(name string) (ModelSpec, error) {
	spec, ok := LookupModel(name)
	if !ok {
		return ModelSpec{}, fmt.Errorf("%w: %q", ErrUnknownModel, name)
	}
	return spec, nil
}

func defaultBaseURL() string {
	if override := strings.TrimRight(os.Getenv(ModelBaseURLEnv), "/"); override != "" {
		return override
	}
	return BaseURL
}
