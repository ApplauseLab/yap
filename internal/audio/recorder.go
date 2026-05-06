package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
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
	stream        *portaudio.Stream
	buffer        []float32
	mu            sync.Mutex
	isRecording   bool
	startTime     time.Time
	deviceName    string         // Empty string means use system default
	levelCallback func(float32)  // Callback for audio level updates
	levelChan     chan float32   // Channel for level updates
	stopLevelChan chan struct{}  // Signal to stop level goroutine
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

// SetLevelCallback sets a callback that receives audio level (0.0-1.0) during recording
func (r *Recorder) SetLevelCallback(cb func(float32)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.levelCallback = cb
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
	
	// Initialize level channels
	r.levelChan = make(chan float32, 1) // Buffered, drop old values
	r.stopLevelChan = make(chan struct{})

	// Start the stream
	if err := stream.Start(); err != nil {
		r.isRecording = false
		return fmt.Errorf("failed to start stream: %w", err)
	}

	// Start a goroutine to process level updates (prevents goroutine accumulation)
	go r.levelLoop()
	
	// Start a goroutine to read audio data
	go r.readLoop(inputBuffer)

	return nil
}

// levelLoop processes audio level updates at a controlled rate
func (r *Recorder) levelLoop() {
	ticker := time.NewTicker(33 * time.Millisecond) // ~30fps
	defer ticker.Stop()
	
	var lastLevel float32
	
	for {
		select {
		case <-r.stopLevelChan:
			return
		case level := <-r.levelChan:
			lastLevel = level
		case <-ticker.C:
			r.mu.Lock()
			cb := r.levelCallback
			r.mu.Unlock()
			if cb != nil && lastLevel > 0 {
				cb(lastLevel)
			}
		}
	}
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
		levelChan := r.levelChan
		r.mu.Unlock()

		// Read from stream - this is the critical path, don't block it
		err := stream.Read()
		if err != nil {
			// Stream might be closed, exit loop
			return
		}

		// Append to buffer first (priority)
		r.mu.Lock()
		if r.isRecording {
			r.buffer = append(r.buffer, inputBuffer...)
		}
		r.mu.Unlock()

		// Calculate audio level (RMS) - fast operation
		var sum float32
		for _, sample := range inputBuffer {
			sum += sample * sample
		}
		rms := float32(0)
		if len(inputBuffer) > 0 {
			rms = float32(math.Sqrt(float64(sum / float32(len(inputBuffer)))))
		}
		// Normalize to 0-1 range (typical speech is around 0.01-0.1 RMS)
		level := rms * 10 // Scale up for visibility
		if level > 1.0 {
			level = 1.0
		}

		// Send level to channel (non-blocking, drops if full)
		// Re-check under lock since Stop() may have set it to nil
		r.mu.Lock()
		levelChan = r.levelChan
		r.mu.Unlock()
		
		if levelChan != nil {
			select {
			case levelChan <- level:
			default:
				// Channel full, skip this update
			}
		}
	}
}

// Stop ends recording and returns the recorded audio as float32 samples
func (r *Recorder) Stop() ([]float32, error) {
	r.mu.Lock()
	
	if !r.isRecording {
		r.mu.Unlock()
		return nil, fmt.Errorf("not recording")
	}

	r.isRecording = false
	
	// Stop level loop first (this will exit the levelLoop goroutine)
	stopChan := r.stopLevelChan
	r.stopLevelChan = nil
	
	// Clear level channel reference (don't close - readLoop might still send)
	r.levelChan = nil
	r.levelCallback = nil

	// Stop and close stream
	stream := r.stream
	r.stream = nil
	
	// Return a copy of the buffer
	result := make([]float32, len(r.buffer))
	copy(result, r.buffer)
	
	// Clear buffer immediately
	r.buffer = nil
	
	r.mu.Unlock()

	// Close stop channel outside lock to signal levelLoop to exit
	if stopChan != nil {
		close(stopChan)
	}
	
	// Stop and close PortAudio stream outside lock
	if stream != nil {
		stream.Stop()
		stream.Close()
	}
	
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
