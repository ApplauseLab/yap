package transcribe

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/sashabaranov/go-openai"
)

// OpenAIEngine uses OpenAI's Whisper API for transcription
type OpenAIEngine struct {
	client *openai.Client
	model  Model
	apiKey string
}

// NewOpenAIEngine creates a new OpenAI transcription engine
func NewOpenAIEngine(apiKey string) *OpenAIEngine {
	var client *openai.Client
	if apiKey != "" {
		client = openai.NewClient(apiKey)
	}
	return &OpenAIEngine{
		client: client,
		model:  ModelBase, // Default, but OpenAI only has one model
		apiKey: apiKey,
	}
}

// SetAPIKey updates the API key
func (e *OpenAIEngine) SetAPIKey(apiKey string) {
	e.apiKey = apiKey
	if apiKey != "" {
		e.client = openai.NewClient(apiKey)
	} else {
		e.client = nil
	}
}

// Transcribe converts audio samples to text
func (e *OpenAIEngine) Transcribe(ctx context.Context, samples []float32) (string, error) {
	// Convert samples to WAV
	wavData, err := samplesToWAV(samples)
	if err != nil {
		return "", fmt.Errorf("failed to convert samples to WAV: %w", err)
	}
	return e.TranscribeWAV(ctx, wavData)
}

// TranscribeWAV transcribes WAV audio data
func (e *OpenAIEngine) TranscribeWAV(ctx context.Context, wavData []byte) (string, error) {
	if e.client == nil {
		return "", fmt.Errorf("OpenAI API key not configured")
	}

	// Create a temporary file for the audio
	tmpFile, err := os.CreateTemp("", "whisper-*.wav")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(wavData); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	// Call OpenAI API
	req := openai.AudioRequest{
		Model:    openai.Whisper1,
		FilePath: tmpFile.Name(),
	}

	resp, err := e.client.CreateTranscription(ctx, req)
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}

	return resp.Text, nil
}

// SetModel sets the model (OpenAI only has whisper-1)
func (e *OpenAIEngine) SetModel(model Model) error {
	e.model = model
	return nil
}

// GetModel returns the current model
func (e *OpenAIEngine) GetModel() Model {
	return e.model
}

// IsAvailable checks if the engine is ready
func (e *OpenAIEngine) IsAvailable() bool {
	return e.client != nil && e.apiKey != ""
}

// Name returns the provider name
func (e *OpenAIEngine) Name() Provider {
	return ProviderOpenAI
}

// samplesToWAV converts float32 samples to WAV format
func samplesToWAV(samples []float32) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Convert float32 to int16
	int16Samples := make([]int16, len(samples))
	for i, s := range samples {
		if s > 1.0 {
			s = 1.0
		}
		if s < -1.0 {
			s = -1.0
		}
		int16Samples[i] = int16(s * 32767)
	}

	// WAV constants
	const sampleRate = 16000
	const channels = 1
	const bitsPerSample = 16

	dataSize := uint32(len(int16Samples) * 2)
	fileSize := dataSize + 36

	// RIFF header
	buf.WriteString("RIFF")
	writeUint32LE(buf, fileSize)
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	writeUint32LE(buf, 16)               // Chunk size
	writeUint16LE(buf, 1)                // Audio format (PCM)
	writeUint16LE(buf, channels)         // Channels
	writeUint32LE(buf, sampleRate)       // Sample rate
	writeUint32LE(buf, sampleRate*channels*2) // Byte rate
	writeUint16LE(buf, channels*2)       // Block align
	writeUint16LE(buf, bitsPerSample)    // Bits per sample

	// data chunk
	buf.WriteString("data")
	writeUint32LE(buf, dataSize)

	// Write samples
	for _, s := range int16Samples {
		writeInt16LE(buf, s)
	}

	return buf.Bytes(), nil
}

func writeUint32LE(buf *bytes.Buffer, v uint32) {
	buf.WriteByte(byte(v))
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v >> 16))
	buf.WriteByte(byte(v >> 24))
}

func writeUint16LE(buf *bytes.Buffer, v uint16) {
	buf.WriteByte(byte(v))
	buf.WriteByte(byte(v >> 8))
}

func writeInt16LE(buf *bytes.Buffer, v int16) {
	buf.WriteByte(byte(v))
	buf.WriteByte(byte(v >> 8))
}
