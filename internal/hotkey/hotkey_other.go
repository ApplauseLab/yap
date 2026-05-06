//go:build !darwin

package hotkey

import (
	"fmt"
	"sync"
)

// Callback is called when the hotkey is pressed
type Callback func()

// Manager handles global hotkey registration
type Manager struct {
	mu       sync.Mutex
	running  bool
	callback Callback
}

// NewManager creates a new hotkey manager
func NewManager() *Manager {
	return &Manager{}
}

// Register registers the global hotkey
func (m *Manager) Register(callback Callback) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("hotkey already registered")
	}

	m.callback = callback
	m.running = true

	// TODO: Implement for other platforms
	return nil
}

// Unregister removes the hotkey registration
func (m *Manager) Unregister() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.running = false
	return nil
}

// IsRegistered returns whether a hotkey is currently registered
func (m *Manager) IsRegistered() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// EnableEscapeCancel starts monitoring for Escape key (no-op on non-darwin)
func (m *Manager) EnableEscapeCancel(cb func()) {
	// TODO: Implement for other platforms
}

// DisableEscapeCancel stops monitoring for Escape key (no-op on non-darwin)
func (m *Manager) DisableEscapeCancel() {
	// TODO: Implement for other platforms
}

// SetHotkeyType changes the hotkey type (no-op on non-darwin)
func (m *Manager) SetHotkeyType(hotkeyType string) {
	// TODO: Implement for other platforms
}

// GetHotkeyDisplayName returns the display name for a hotkey type
func GetHotkeyDisplayName(hotkeyType string) string {
	switch hotkeyType {
	case "leftOption":
		return "Left Option (⌥)"
	case "fn":
		return "Fn"
	case "doubleRightOption":
		return "Double-tap Right Option"
	default:
		return "Right Option (⌥)"
	}
}
