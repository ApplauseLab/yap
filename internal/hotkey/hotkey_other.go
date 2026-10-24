//go:build !darwin && !windows && !linux

package hotkey

import (
	"fmt"
	"strings"
	"sync"

	"yap/internal/logger"
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

	// Hotkeys not supported on this platform
	logger.Warning("Hotkeys are not supported on this platform")
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

// EnableCancelKey starts monitoring for the cancel key (no-op)
func (m *Manager) EnableCancelKey(cb func()) {
	// Not supported on this platform
}

// DisableCancelKey stops monitoring for the cancel key (no-op)
func (m *Manager) DisableCancelKey() {
	// Not supported on this platform
}

// SetHotkeyType sets the hotkey (no-op)
func (m *Manager) SetHotkeyType(hotkeyType string) {
	// Not supported on this platform
}

// SetCancelKey sets the cancel key (no-op)
func (m *Manager) SetCancelKey(keyName string) {
	// Not supported on this platform
}

// GetHotkeyDisplayName returns the display name for a hotkey
func GetHotkeyDisplayName(hotkeyName string) string {
	return KeyNameToDisplayName(hotkeyName)
}

// RequestAccessibilityPermissions is a no-op on unsupported platforms
func RequestAccessibilityPermissions() bool {
	return true
}

// HasAccessibilityPermissions always returns true on unsupported platforms
func HasAccessibilityPermissions() bool {
	return true
}

// KeyNameToDisplayName converts a key name to a display-friendly name
func KeyNameToDisplayName(name string) string {
	switch strings.ToLower(name) {
	case "rightalt", "rightoption":
		return "Right Alt"
	case "leftalt", "leftoption":
		return "Left Alt"
	case "escape", "esc":
		return "Escape"
	case "space":
		return "Space"
	default:
		return strings.ToUpper(name)
	}
}
