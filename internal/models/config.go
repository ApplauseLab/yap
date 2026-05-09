package models

import (
	"encoding/json"
	"os"
	"path/filepath"
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

	// Hotkey configuration (legacy - kept for compatibility)
	HotkeyModifiers []string `json:"hotkeyModifiers"` // e.g., ["cmd", "shift"]
	HotkeyKey       string   `json:"hotkeyKey"`       // e.g., "space"

	// Recording hotkey type: "rightOption", "leftOption", "fn", "doubleRightOption"
	RecordingHotkey string `json:"recordingHotkey"`

	// Sound enabled for recording start/stop
	SoundEnabled *bool `json:"soundEnabled,omitempty"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	soundEnabled := true
	return &Config{
		Provider:         "local",
		Model:            "base.en",
		AutoPaste:        true,
		ShowNotification: true,
		HotkeyModifiers:  []string{"cmd", "shift"},
		HotkeyKey:        "space",
		RecordingHotkey:  "rightOption",
		SoundEnabled:     &soundEnabled,
	}
}

// ConfigManager handles loading and saving configuration
type ConfigManager struct {
	configDir  string
	configFile string
	config     *Config
}

// NewConfigManager creates a new config manager
func NewConfigManager() (*ConfigManager, error) {
	// Get user config directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}

	appConfigDir := filepath.Join(configDir, "yap")
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
	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cm.configFile, data, 0644)
}

// Get returns the current configuration
func (cm *ConfigManager) Get() *Config {
	return cm.config
}

// Update updates the configuration
func (cm *ConfigManager) Update(config *Config) error {
	cm.config = config
	return cm.Save()
}

// SetProvider updates the provider setting
func (cm *ConfigManager) SetProvider(provider string) error {
	cm.config.Provider = provider
	return cm.Save()
}

// SetModel updates the model setting
func (cm *ConfigManager) SetModel(model string) error {
	cm.config.Model = model
	return cm.Save()
}

// SetOpenAIAPIKey updates the API key
func (cm *ConfigManager) SetOpenAIAPIKey(key string) error {
	cm.config.OpenAIAPIKey = key
	return cm.Save()
}

// SetAutoPaste updates the auto-paste setting
func (cm *ConfigManager) SetAutoPaste(enabled bool) error {
	cm.config.AutoPaste = enabled
	return cm.Save()
}

// SetAudioInputDevice updates the audio input device setting
func (cm *ConfigManager) SetAudioInputDevice(deviceName string) error {
	cm.config.AudioInputDevice = deviceName
	return cm.Save()
}

// SetRecordingHotkey updates the recording hotkey setting
func (cm *ConfigManager) SetRecordingHotkey(hotkey string) error {
	cm.config.RecordingHotkey = hotkey
	return cm.Save()
}

// SetSoundEnabled updates the sound enabled setting
func (cm *ConfigManager) SetSoundEnabled(enabled bool) error {
	cm.config.SoundEnabled = &enabled
	return cm.Save()
}
