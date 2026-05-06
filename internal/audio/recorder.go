package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

const (
	SampleRate = 16000 // Whisper requires 16kHz
	Channels   = 1     // Mono
	FrameSize  = 1024  // Samples per frame
)

// AudioInputDevice represents an audio input device
type AudioInputDevice struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

// Recorder handles audio capture from microphone
type Recorder struct {
	stream      *portaudio.Stream
	buffer      []float32
	mu          sync.Mutex
	isRecording bool
	startTime   time.Time
	deviceName  string // Empty string means use system default
}

// NewRecorder creates a new audio recorder
func NewRecorder() *Recorder {
	return &Recorder{
		buffer: make([]float32, 0),
	}
}

// Initialize sets up PortAudio
func Initialize() error {
	return portaudio.Initialize()
}

// Terminate cleans up PortAudio
func Terminate() error {
	return portaudio.Terminate()
}

// GetAudioInputDevices returns a list of available audio input devices
func GetAudioInputDevices() ([]AudioInputDevice, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, fmt.Errorf("failed to get devices: %w", err)
	}

	defaultDevice, err := portaudio.DefaultInputDevice()
	if err != nil {
		return nil, fmt.Errorf("failed to get default device: %w", err)
	}

	var inputDevices []AudioInputDevice
	for _, d := range devices {
		// Only include input devices (devices with input channels)
		if d.MaxInputChannels > 0 {
			inputDevices = append(inputDevices, AudioInputDevice{
				Name:      d.Name,
				IsDefault: d.Name == defaultDevice.Name,
			})
		}
	}

	return inputDevices, nil
}

// GetDefaultInputDevice returns the default input device
func GetDefaultInputDevice() (*AudioInputDevice, error) {
	device, err := portaudio.DefaultInputDevice()
	if err != nil {
		return nil, fmt.Errorf("failed to get default device: %w", err)
	}
	return &AudioInputDevice{
		Name:      device.Name,
		IsDefault: true,
	}, nil
}

// SetDevice sets the device to use for recording (empty string = system default)
func (r *Recorder) SetDevice(deviceName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deviceName = deviceName
}

// GetDevice returns the current device name (empty = system default)
func (r *Recorder) GetDevice() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.deviceName
}

// findDeviceByName finds a PortAudio device by name
func findDeviceByName(name string) (*portaudio.DeviceInfo, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		if d.Name == name && d.MaxInputChannels > 0 {
			return d, nil
		}
	}
	return nil, fmt.Errorf("device not found: %s", name)
}

// Start begins recording audio from the configured microphone
func (r *Recorder) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isRecording {
		return fmt.Errorf("already recording")
	}

	// Clear previous buffer
	r.buffer = make([]float32, 0)

	// Create input buffer for each callback
	inputBuffer := make([]float32, FrameSize)

	var stream *portaudio.Stream
	var err error

	// Use specific device if set, otherwise use default
	if r.deviceName != "" {
		device, findErr := findDeviceByName(r.deviceName)
		if findErr != nil {
			// Device not found, fall back to default
			fmt.Printf("Warning: Device '%s' not found, using default\n", r.deviceName)
			stream, err = portaudio.OpenDefaultStream(
				Channels, 0, SampleRate, FrameSize, inputBuffer,
			)
		} else {
			// Open stream with specific device
			streamParams := portaudio.StreamParameters{
				Input: portaudio.StreamDeviceParameters{
					Device:   device,
					Channels: Channels,
					Latency:  device.DefaultLowInputLatency,
				},
				SampleRate:      SampleRate,
				FramesPerBuffer: FrameSize,
			}
			stream, err = portaudio.OpenStream(streamParams, inputBuffer)
		}
	} else {
		// Use default input stream
		stream, err = portaudio.OpenDefaultStream(
			Channels, 0, SampleRate, FrameSize, inputBuffer,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	r.stream = stream
	r.isRecording = true
	r.startTime = time.Now()

	// Start the stream
	if err := stream.Start(); err != nil {
		r.isRecording = false
		return fmt.Errorf("failed to start stream: %w", err)
	}

	// Start a goroutine to read audio data
	go r.readLoop(inputBuffer)

	return nil
}

// readLoop continuously reads audio data from the stream
func (r *Recorder) readLoop(inputBuffer []float32) {
	for {
		r.mu.Lock()
		if !r.isRecording {
			r.mu.Unlock()
			return
		}
		stream := r.stream
		r.mu.Unlock()

		// Read from stream
		err := stream.Read()
		if err != nil {
			// Stream might be closed, exit loop
			return
		}

		// Append to buffer
		r.mu.Lock()
		if r.isRecording {
			r.buffer = append(r.buffer, inputBuffer...)
		}
		r.mu.Unlock()
	}
}

// Stop ends recording and returns the recorded audio as float32 samples
func (r *Recorder) Stop() ([]float32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRecording {
		return nil, fmt.Errorf("not recording")
	}

	r.isRecording = false

	// Stop and close stream
	if r.stream != nil {
		r.stream.Stop()
		r.stream.Close()
		r.stream = nil
	}

	// Return a copy of the buffer
	result := make([]float32, len(r.buffer))
	copy(result, r.buffer)

	return result, nil
}

// IsRecording returns whether recording is in progress
func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isRecording
}

// Duration returns the current recording duration
func (r *Recorder) Duration() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.isRecording {
		return 0
	}
	return time.Since(r.startTime)
}

// ToWAV converts float32 samples to WAV format bytes
func ToWAV(samples []float32) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Convert float32 to int16
	int16Samples := make([]int16, len(samples))
	for i, s := range samples {
		// Clamp to [-1, 1]
		if s > 1.0 {
			s = 1.0
		}
		if s < -1.0 {
			s = -1.0
		}
		int16Samples[i] = int16(s * 32767)
	}

	// WAV header
	dataSize := uint32(len(int16Samples) * 2) // 2 bytes per sample
	fileSize := dataSize + 36                  // Header is 44 bytes, minus 8 for RIFF header

	// RIFF header
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, fileSize)
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))    // Chunk size
	binary.Write(buf, binary.LittleEndian, uint16(1))     // Audio format (PCM)
	binary.Write(buf, binary.LittleEndian, uint16(Channels))
	binary.Write(buf, binary.LittleEndian, uint32(SampleRate))
	binary.Write(buf, binary.LittleEndian, uint32(SampleRate*Channels*2)) // Byte rate
	binary.Write(buf, binary.LittleEndian, uint16(Channels*2))            // Block align
	binary.Write(buf, binary.LittleEndian, uint16(16))                    // Bits per sample

	// data chunk
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, dataSize)

	// Write samples
	for _, s := range int16Samples {
		binary.Write(buf, binary.LittleEndian, s)
	}

	return buf.Bytes(), nil
}
