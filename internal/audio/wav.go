package audio

import (
	"encoding/binary"
	"os"
)

// SaveWAV saves float32 audio samples to a WAV file
// Assumes 16kHz sample rate, mono, 16-bit PCM
func SaveWAV(path string, samples []float32) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	sampleRate := uint32(16000)
	numChannels := uint16(1)
	bitsPerSample := uint16(16)
	byteRate := sampleRate * uint32(numChannels) * uint32(bitsPerSample) / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := uint32(len(samples) * 2) // 2 bytes per sample (16-bit)

	// Write RIFF header
	file.Write([]byte("RIFF"))
	binary.Write(file, binary.LittleEndian, uint32(36+dataSize))
	file.Write([]byte("WAVE"))

	// Write fmt chunk
	file.Write([]byte("fmt "))
	binary.Write(file, binary.LittleEndian, uint32(16))         // chunk size
	binary.Write(file, binary.LittleEndian, uint16(1))          // audio format (PCM)
	binary.Write(file, binary.LittleEndian, numChannels)        // num channels
	binary.Write(file, binary.LittleEndian, sampleRate)         // sample rate
	binary.Write(file, binary.LittleEndian, byteRate)           // byte rate
	binary.Write(file, binary.LittleEndian, blockAlign)         // block align
	binary.Write(file, binary.LittleEndian, bitsPerSample)      // bits per sample

	// Write data chunk
	file.Write([]byte("data"))
	binary.Write(file, binary.LittleEndian, dataSize)

	// Convert float32 samples to int16 and write
	for _, sample := range samples {
		// Clamp to [-1, 1]
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}
		// Convert to int16
		intSample := int16(sample * 32767)
		binary.Write(file, binary.LittleEndian, intSample)
	}

	return nil
}

// LoadWAV loads a WAV file and returns the raw bytes
func LoadWAV(path string) ([]byte, error) {
	return os.ReadFile(path)
}
