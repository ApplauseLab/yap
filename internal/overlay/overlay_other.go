//go:build !darwin

package overlay

// Overlay is a stub for non-macOS platforms
type Overlay struct{}

// New creates a new overlay manager
func New() *Overlay {
	return &Overlay{}
}

// Show displays the recording overlay (no-op on non-macOS)
func (o *Overlay) Show() {}

// Hide hides the recording overlay (no-op on non-macOS)
func (o *Overlay) Hide() {}

// SetStatus updates the status text (no-op on non-macOS)
func (o *Overlay) SetStatus(status string) {}

// SetAudioLevel updates the audio level for waveform visualization (no-op on non-macOS)
func (o *Overlay) SetAudioLevel(level float32) {}

// SetStopCallback sets the callback for when stop is clicked (no-op on non-macOS)
func (o *Overlay) SetStopCallback(cb func()) {}

// SetCancelCallback sets the callback for when cancel is clicked (no-op on non-macOS)
func (o *Overlay) SetCancelCallback(cb func()) {}

// Destroy cleans up the overlay (no-op on non-macOS)
func (o *Overlay) Destroy() {}
