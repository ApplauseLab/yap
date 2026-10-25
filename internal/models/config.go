package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"yap/internal/paths"
)

// Config holds application settings
type Config struct {
	// Provider: "local" or "openai"
	Provider string `json:"provider"`

	// Model name (e.g., "base.en", "small", etc.)
	Model string `json:"model"`

	// OpenAI API key (encrypted or stored in keychain ideally)
	OpenAIAPIKey string `json:"openaiApiKey,omitempty"`

	// Audio input device name (empty string = system default)
	AudioInputDevice string `json:"audioInputDevice,omitempty"`

	// Auto-paste after transcription
	AutoPaste bool `json:"autoPaste"`

	// Show notification after transcription
	ShowNotification bool `json:"showNotification"`

	// Recording hotkey - any single key name, e.g., "rightoption", "f9", "space"
	// macOS default: "rightoption"
	// Windows/Linux default: "rightalt"
	RecordingHotkey string `json:"recordingHotkey"`

	// Cancel hotkey - key to cancel recording, default: "escape"
	CancelHotkey string `json:"cancelHotkey"`

	// Sound enabled for recording start/stop
	SoundEnabled *bool `json:"soundEnabled,omitempty"`

	// OnboardingCompleted indicates whether the user has completed the onboarding flow
	OnboardingCompleted bool `json:"onboardingCompleted"`

	// FavoriteModels lists catalog model names the user starred in the library
	FavoriteModels []string `json:"favoriteModels,omitempty"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	soundEnabled := true
	return &Config{
		Provider:         "local",
		Model:            "base.en",
		AutoPaste:        true,
		ShowNotification: true,
		RecordingHotkey:  DefaultRecordingHotkey(),
		CancelHotkey:     "escape",
		SoundEnabled:     &soundEnabled,
	}
}

// DefaultRecordingHotkey returns the default recording hotkey for the current platform
func DefaultRecordingHotkey() string {
	switch runtime.GOOS {
	case "darwin":
		return "rightoption"
	case "windows", "linux":
		return "rightalt"
	default:
		return "rightalt"
	}
}

// ConfigManager handles loading and saving configuration
type ConfigManager struct {
	configDir  string
	configFile string

	mu     sync.RWMutex
	config *Config
}

// NewConfigManager creates a new config manager
func NewConfigManager() (*ConfigManager, error) {
	appConfigDir, err := paths.AppDataDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		return nil, err
	}

	cm := &ConfigManager{
		configDir:  appConfigDir,
		configFile: filepath.Join(appConfigDir, "config.json"),
	}

	// Load or create config
	if err := cm.Load(); err != nil {
		// Use default config if load fails
		cm.config = DefaultConfig()
	}

	return cm, nil
}

// GetConfigDir returns the configuration directory
func (cm *ConfigManager) GetConfigDir() string {
	return cm.configDir
}

// GetModelsDir returns the models directory
func (cm *ConfigManager) GetModelsDir() string {
	return filepath.Join(cm.configDir, "models")
}

// Load reads configuration from file
func (cm *ConfigManager) Load() error {
	data, err := os.ReadFile(cm.configFile)
	if err != nil {
		if os.IsNotExist(err) {
			cm.config = DefaultConfig()
			return cm.Save()
		}
		return err
	}

	cm.config = &Config{}
	if err := json.Unmarshal(data, cm.config); err != nil {
		return err
	}

	return nil
}

// Save writes configuration to file
func (cm *ConfigManager) Save() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.saveLocked()
}

// saveLocked writes the config atomically: a crash or a concurrent reader
// never sees a partially written config.json. Callers hold cm.mu.
func (cm *ConfigManager) saveLocked() error {
	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(cm.configDir, "config-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, cm.configFile); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// update applies change and saves while holding the lock, so concurrent
// setters cannot lose each other's writes.
func (cm *ConfigManager) update(change func(*Config)) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	change(cm.config)
	return cm.saveLocked()
}

// Get returns a snapshot of the current configuration
func (cm *ConfigManager) Get() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	snapshot := *cm.config
	snapshot.FavoriteModels = append([]string(nil), cm.config.FavoriteModels...)
	return &snapshot
}

// Update replaces the configuration
func (cm *ConfigManager) Update(config *Config) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.config = config
	return cm.saveLocked()
}

// SetProvider updates the provider setting
func (cm *ConfigManager) SetProvider(provider string) error {
	return cm.update(func(c *Config) { c.Provider = provider })
}

// SetModel updates the model setting
func (cm *ConfigManager) SetModel(model string) error {
	return cm.update(func(c *Config) { c.Model = model })
}

// SetOpenAIAPIKey updates the API key
func (cm *ConfigManager) SetOpenAIAPIKey(key string) error {
	return cm.update(func(c *Config) { c.OpenAIAPIKey = key })
}

// SetAutoPaste updates the auto-paste setting
func (cm *ConfigManager) SetAutoPaste(enabled bool) error {
	return cm.update(func(c *Config) { c.AutoPaste = enabled })
}

// SetAudioInputDevice updates the audio input device setting
func (cm *ConfigManager) SetAudioInputDevice(deviceName string) error {
	return cm.update(func(c *Config) { c.AudioInputDevice = deviceName })
}

// SetRecordingHotkey updates the recording hotkey setting
func (cm *ConfigManager) SetRecordingHotkey(hotkey string) error {
	return cm.update(func(c *Config) { c.RecordingHotkey = hotkey })
}

// SetCancelHotkey updates the cancel hotkey setting
func (cm *ConfigManager) SetCancelHotkey(hotkey string) error {
	return cm.update(func(c *Config) { c.CancelHotkey = hotkey })
}

// SetSoundEnabled updates the sound enabled setting
func (cm *ConfigManager) SetSoundEnabled(enabled bool) error {
	return cm.update(func(c *Config) { c.SoundEnabled = &enabled })
}

// IsOnboardingCompleted returns whether onboarding has been completed
func (cm *ConfigManager) IsOnboardingCompleted() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.OnboardingCompleted
}

// SetOnboardingCompleted marks onboarding as completed
func (cm *ConfigManager) SetOnboardingCompleted(completed bool) error {
	return cm.update(func(c *Config) { c.OnboardingCompleted = completed })
}

// IsFavoriteModel reports whether the user starred model
func (cm *ConfigManager) IsFavoriteModel(model string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	for _, name := range cm.config.FavoriteModels {
		if name == model {
			return true
		}
	}
	return false
}

// SetModelFavorite stars or unstars model
func (cm *ConfigManager) SetModelFavorite(model string, favorite bool) error {
	return cm.update(func(c *Config) {
		kept := make([]string, 0, len(c.FavoriteModels)+1)
		for _, name := range c.FavoriteModels {
			if name != model {
				kept = append(kept, name)
			}
		}
		if favorite {
			kept = append(kept, model)
		}
		c.FavoriteModels = kept
	})
}
