package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"applause-whisper/internal/audio"
	"applause-whisper/internal/hotkey"
	"applause-whisper/internal/models"
	"applause-whisper/internal/overlay"
	"applause-whisper/internal/system"
	"applause-whisper/internal/transcribe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// RecordingState represents the current state of the app
type RecordingState string

const (
	StateReady        RecordingState = "ready"
	StateRecording    RecordingState = "recording"
	StateTranscribing RecordingState = "transcribing"
	StateError        RecordingState = "error"
)

// AppState is sent to the frontend
type AppState struct {
	State           RecordingState `json:"state"`
	RecordingTime   float64        `json:"recordingTime"`
	LastTranscript  string         `json:"lastTranscript"`
	Error           string         `json:"error"`
	CurrentModel    string         `json:"currentModel"`
	CurrentProvider string         `json:"currentProvider"`
	ModelReady      bool           `json:"modelReady"`
	HotkeyEnabled   bool           `json:"hotkeyEnabled"`
}

// HistoryItem represents a transcription history entry
type HistoryItem struct {
	ID        string  `json:"id"`
	Text      string  `json:"text"`
	Timestamp string  `json:"timestamp"`
	Duration  float64 `json:"duration"`
	AudioPath string  `json:"audioPath,omitempty"`
	HasAudio  bool    `json:"hasAudio"`
}

// ModelInfo for frontend
type ModelInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Size        string `json:"size"`
	Downloaded  bool   `json:"downloaded"`
	EnglishOnly bool   `json:"englishOnly"`
}

// UsageStats for frontend
type UsageStats struct {
	AverageWPM         float64 `json:"averageWPM"`
	WordsThisWeek      int     `json:"wordsThisWeek"`
	RecordingsThisWeek int     `json:"recordingsThisWeek"`
	TimeSavedThisWeek  float64 `json:"timeSavedThisWeek"` // in minutes
	TotalRecordings    int     `json:"totalRecordings"`
	TotalWords         int     `json:"totalWords"`
}

// App struct
type App struct {
	ctx           context.Context
	recorder      *audio.Recorder
	localEngine   *transcribe.LocalEngine
	openaiEngine  *transcribe.OpenAIEngine
	configManager *models.ConfigManager
	modelManager  *models.Manager
	statsManager  *models.StatsManager
	hotkeyManager *hotkey.Manager
	overlay       *overlay.Overlay

	mu              sync.Mutex
	state           RecordingState
	lastTranscript  string
	lastError       string
	recordStartTime time.Time
	hotkeyEnabled   bool
	history         []HistoryItem
	
	// Tray callback to update icon
	onTrayUpdate func(recording bool)
}

// NewApp creates a new App application struct
func NewApp() *App {
	app := &App{
		recorder:      nil, // Created fresh for each recording
		hotkeyManager: hotkey.NewManager(),
		overlay:       overlay.New(),
		state:         StateReady,
		history:       make([]HistoryItem, 0),
	}
	
	// Set up overlay stop callback
	app.overlay.SetStopCallback(func() {
		app.ToggleRecording()
	})
	
	// Set up overlay cancel callback
	app.overlay.SetCancelCallback(func() {
		app.CancelRecording()
	})
	
	return app
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize PortAudio
	if err := audio.Initialize(); err != nil {
		fmt.Printf("Warning: Failed to initialize audio: %v\n", err)
	}

	// Initialize config manager
	configManager, err := models.NewConfigManager()
	if err != nil {
		fmt.Printf("Warning: Failed to initialize config: %v\n", err)
		return
	}
	a.configManager = configManager

	// Initialize stats manager
	statsManager, err := models.NewStatsManager(configManager.GetConfigDir())
	if err != nil {
		fmt.Printf("Warning: Failed to initialize stats manager: %v\n", err)
	} else {
		a.statsManager = statsManager
	}

	// Audio device preference is applied in StartRecording when recorder is created
	if deviceName := configManager.Get().AudioInputDevice; deviceName != "" {
		fmt.Printf("Will use audio input device: %s\n", deviceName)
	}

	// Initialize model manager
	modelManager, err := models.NewManager(configManager.GetModelsDir())
	if err != nil {
		fmt.Printf("Warning: Failed to initialize model manager: %v\n", err)
		return
	}
	a.modelManager = modelManager

	// Initialize transcription engines
	a.localEngine = transcribe.NewLocalEngine(configManager.GetModelsDir())
	a.localEngine.SetModel(transcribe.Model(configManager.Get().Model))

	a.openaiEngine = transcribe.NewOpenAIEngine(configManager.Get().OpenAIAPIKey)

	// Register global hotkey
	if err := a.hotkeyManager.Register(func() {
		a.ToggleRecording()
	}); err != nil {
		fmt.Printf("Warning: Failed to register hotkey: %v\n", err)
	} else {
		a.hotkeyEnabled = true
		fmt.Println("Global hotkey registered: Cmd+Shift+Space")
	}

	// Load history from disk
	a.loadHistory()
}

// shutdown is called when the app closes
func (a *App) shutdown(ctx context.Context) {
	if a.hotkeyManager != nil {
		a.hotkeyManager.Unregister()
	}
	if a.overlay != nil {
		a.overlay.Destroy()
	}
	audio.Terminate()
}

// GetState returns the current app state
func (a *App) GetState() AppState {
	a.mu.Lock()
	defer a.mu.Unlock()

	var recordingTime float64
	if a.state == StateRecording && a.recorder != nil {
		recordingTime = a.recorder.Duration().Seconds()
	}

	config := a.configManager.Get()
	modelReady := false
	if config.Provider == "local" {
		modelReady = a.modelManager.IsModelDownloaded(config.Model)
	} else {
		modelReady = a.openaiEngine.IsAvailable()
	}

	return AppState{
		State:           a.state,
		RecordingTime:   recordingTime,
		LastTranscript:  a.lastTranscript,
		Error:           a.lastError,
		CurrentModel:    config.Model,
		CurrentProvider: config.Provider,
		ModelReady:      modelReady,
		HotkeyEnabled:   a.hotkeyEnabled,
	}
}

// GetHistory returns the transcription history
func (a *App) GetHistory() []HistoryItem {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.history
}

// ClearHistory clears all history
func (a *App) ClearHistory() {
	a.mu.Lock()
	a.history = make([]HistoryItem, 0)
	a.mu.Unlock()
	a.saveHistory()
	runtime.EventsEmit(a.ctx, "historyChanged", a.history)
}

// getHistoryPath returns the path to the history file
func (a *App) getHistoryPath() string {
	if a.configManager == nil {
		return ""
	}
	configDir := filepath.Dir(a.configManager.GetModelsDir())
	return filepath.Join(configDir, "history.json")
}

// loadHistory loads history from disk
func (a *App) loadHistory() {
	path := a.getHistoryPath()
	if path == "" {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist yet, that's OK
		return
	}

	var history []HistoryItem
	if err := json.Unmarshal(data, &history); err != nil {
		fmt.Printf("Warning: Failed to parse history: %v\n", err)
		return
	}

	a.mu.Lock()
	a.history = history
	a.mu.Unlock()
	fmt.Printf("Loaded %d history items\n", len(history))
}

// saveHistory saves history to disk
func (a *App) saveHistory() {
	path := a.getHistoryPath()
	if path == "" {
		return
	}

	a.mu.Lock()
	data, err := json.MarshalIndent(a.history, "", "  ")
	a.mu.Unlock()

	if err != nil {
		fmt.Printf("Warning: Failed to serialize history: %v\n", err)
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("Warning: Failed to save history: %v\n", err)
	}
}

// CopyHistoryItem copies a history item to clipboard
func (a *App) CopyHistoryItem(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	for _, item := range a.history {
		if item.ID == id {
			return system.CopyToClipboard(item.Text)
		}
	}
	return fmt.Errorf("history item not found")
}

// DeleteHistoryItem deletes a history item by ID
func (a *App) DeleteHistoryItem(id string) error {
	a.mu.Lock()
	
	// Find and remove the item
	var audioPath string
	found := false
	for i, item := range a.history {
		if item.ID == id {
			audioPath = item.AudioPath
			a.history = append(a.history[:i], a.history[i+1:]...)
			found = true
			break
		}
	}
	a.mu.Unlock()
	
	if !found {
		return fmt.Errorf("history item not found")
	}
	
	// Delete the audio file if it exists
	if audioPath != "" {
		if err := os.Remove(audioPath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: Failed to delete audio file: %v\n", err)
		}
	}
	
	a.saveHistory()
	runtime.EventsEmit(a.ctx, "historyChanged", a.history)
	return nil
}

// GetAudioData returns the audio data for a history item as base64
func (a *App) GetAudioData(id string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	for _, item := range a.history {
		if item.ID == id {
			if !item.HasAudio || item.AudioPath == "" {
				return "", fmt.Errorf("no audio available for this item")
			}
			
			data, err := audio.LoadWAV(item.AudioPath)
			if err != nil {
				return "", fmt.Errorf("failed to load audio: %v", err)
			}
			
			// Return as base64
			return base64.StdEncoding.EncodeToString(data), nil
		}
	}
	return "", fmt.Errorf("history item not found")
}

// ToggleRecording starts or stops recording
func (a *App) ToggleRecording() error {
	a.mu.Lock()
	currentState := a.state
	a.mu.Unlock()

	if currentState == StateRecording {
		return a.StopRecording()
	}
	return a.StartRecording()
}

// StartRecording begins audio capture
func (a *App) StartRecording() error {
	runtime.LogInfo(a.ctx, "StartRecording called")
	a.mu.Lock()
	if a.state != StateReady {
		a.mu.Unlock()
		runtime.LogWarning(a.ctx, fmt.Sprintf("Cannot start recording in state: %s", a.state))
		return fmt.Errorf("cannot start recording in state: %s", a.state)
	}
	a.state = StateRecording
	a.lastError = ""
	a.recordStartTime = time.Now()
	onTrayUpdate := a.onTrayUpdate
	a.mu.Unlock()

	runtime.LogInfo(a.ctx, "State changed to recording, emitting state")

	// Update tray icon
	if onTrayUpdate != nil {
		onTrayUpdate(true)
	}

	a.emitState()

	// Create fresh recorder for each recording session
	a.recorder = audio.NewRecorder()
	
	// Set audio device from config if available
	if a.configManager != nil {
		config := a.configManager.Get()
		if config.AudioInputDevice != "" {
			a.recorder.SetDevice(config.AudioInputDevice)
		}
	}

	if err := a.recorder.Start(); err != nil {
		a.mu.Lock()
		a.state = StateError
		a.lastError = err.Error()
		a.mu.Unlock()
		if onTrayUpdate != nil {
			onTrayUpdate(false)
		}
		a.emitState()
		return err
	}

	// Show native overlay
	a.overlay.Show()

	// Enable escape key to cancel recording (native/global)
	a.hotkeyManager.EnableEscapeCancel(func() {
		a.CancelRecording()
	})

	return nil
}

// StopRecording ends audio capture and starts transcription
func (a *App) StopRecording() error {
	a.mu.Lock()
	if a.state != StateRecording {
		a.mu.Unlock()
		return fmt.Errorf("not recording")
	}
	recordDuration := time.Since(a.recordStartTime).Seconds()
	a.state = StateTranscribing
	a.mu.Unlock()

	// Disable escape cancel and audio level callback
	a.hotkeyManager.DisableEscapeCancel()
	a.recorder.SetLevelCallback(nil)

	// Update overlay to show transcribing status
	a.overlay.SetStatus("Transcribing...")

	a.emitState()

	samples, err := a.recorder.Stop()
	if err != nil {
		a.mu.Lock()
		a.state = StateError
		a.lastError = err.Error()
		a.mu.Unlock()
		a.overlay.Hide()
		a.emitState()
		return err
	}

	go a.transcribe(samples, recordDuration)

	return nil
}

// CancelRecording stops recording without transcribing
func (a *App) CancelRecording() error {
	a.mu.Lock()
	if a.state != StateRecording {
		a.mu.Unlock()
		return fmt.Errorf("not recording")
	}
	a.state = StateReady
	onTrayUpdate := a.onTrayUpdate
	a.mu.Unlock()

	// Disable escape cancel and audio level callback
	a.hotkeyManager.DisableEscapeCancel()
	a.recorder.SetLevelCallback(nil)

	// Stop the recorder (discard samples)
	a.recorder.Stop()

	// Hide overlay
	a.overlay.Hide()

	// Update tray icon
	if onTrayUpdate != nil {
		onTrayUpdate(false)
	}

	a.emitState()


	return nil
}

// getAudioDir returns the directory for storing audio recordings
func (a *App) getAudioDir() string {
	if a.configManager == nil {
		return ""
	}
	audioDir := filepath.Join(filepath.Dir(a.configManager.GetModelsDir()), "recordings")
	os.MkdirAll(audioDir, 0755)
	return audioDir
}

// transcribe processes the audio samples
func (a *App) transcribe(samples []float32, duration float64) {
	config := a.configManager.Get()

	var text string
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if config.Provider == "openai" {
		text, err = a.openaiEngine.Transcribe(ctx, samples)
	} else {
		text, err = a.localEngine.Transcribe(ctx, samples)
	}

	// Generate unique ID for this recording
	recordingID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Save audio to file
	var audioPath string
	var hasAudio bool
	audioDir := a.getAudioDir()
	if audioDir != "" {
		audioPath = filepath.Join(audioDir, recordingID+".wav")
		if saveErr := audio.SaveWAV(audioPath, samples); saveErr != nil {
			fmt.Printf("Warning: Failed to save audio: %v\n", saveErr)
			audioPath = ""
		} else {
			hasAudio = true
			fmt.Printf("Saved audio to: %s\n", audioPath)
		}
	}

	a.mu.Lock()
	onTrayUpdate := a.onTrayUpdate
	if err != nil {
		a.state = StateError
		a.lastError = err.Error()
	} else {
		a.state = StateReady
		a.lastTranscript = text

		// Add to history
		historyItem := HistoryItem{
			ID:        recordingID,
			Text:      text,
			Timestamp: time.Now().Format("2 Jan 3:04 pm"),
			Duration:  duration,
			AudioPath: audioPath,
			HasAudio:  hasAudio,
		}
		a.history = append([]HistoryItem{historyItem}, a.history...)
		
		// Keep only last 50 items
		if len(a.history) > 50 {
			// Delete audio files for items being removed
			for _, item := range a.history[50:] {
				if item.AudioPath != "" {
					os.Remove(item.AudioPath)
				}
			}
			a.history = a.history[:50]
		}

		// Save history to disk
		go a.saveHistory()

		// Record stats
		if a.statsManager != nil {
			a.statsManager.RecordTranscription(text, duration)
		}

		// Copy to clipboard and optionally paste
		if config.AutoPaste {
			go func(textToPaste string) {
				// Wait for the overlay to hide and the previous app to regain focus
				time.Sleep(500 * time.Millisecond)
				if err := system.CopyAndPaste(textToPaste); err != nil {
					fmt.Printf("Failed to paste: %v\n", err)
				}
			}(text)
		} else {
			go system.CopyToClipboard(text)
		}
	}
	a.mu.Unlock()

	// Update tray icon
	if onTrayUpdate != nil {
		onTrayUpdate(false)
	}

	// Hide overlay
	a.overlay.Hide()

	a.emitState()
	runtime.EventsEmit(a.ctx, "historyChanged", a.GetHistory())
}

// emitState sends the current state to the frontend
func (a *App) emitState() {
	if a.ctx != nil {
		state := a.GetState()
		runtime.LogInfo(a.ctx, fmt.Sprintf("Emitting stateChanged event: state=%s", state.State))
		runtime.EventsEmit(a.ctx, "stateChanged", state)
	}
}

// GetModels returns available models with download status
func (a *App) GetModels() []ModelInfo {
	available := transcribe.AvailableModels()
	result := make([]ModelInfo, len(available))

	for i, m := range available {
		result[i] = ModelInfo{
			Name:        string(m.Name),
			DisplayName: m.DisplayName,
			Size:        m.Size,
			Downloaded:  a.modelManager.IsModelDownloaded(string(m.Name)),
			EnglishOnly: m.EnglishOnly,
		}
	}

	return result
}

// SetModel changes the current model
func (a *App) SetModel(model string) error {
	if err := a.configManager.SetModel(model); err != nil {
		return err
	}
	a.localEngine.SetModel(transcribe.Model(model))
	a.emitState()
	return nil
}

// SetProvider changes the transcription provider
func (a *App) SetProvider(provider string) error {
	if err := a.configManager.SetProvider(provider); err != nil {
		return err
	}
	a.emitState()
	return nil
}

// SetOpenAIKey sets the OpenAI API key
func (a *App) SetOpenAIKey(key string) error {
	if err := a.configManager.SetOpenAIAPIKey(key); err != nil {
		return err
	}
	a.openaiEngine.SetAPIKey(key)
	a.emitState()
	return nil
}

// GetConfig returns the current configuration
func (a *App) GetConfig() *models.Config {
	return a.configManager.Get()
}

// SetAutoPaste enables/disables auto-paste
func (a *App) SetAutoPaste(enabled bool) error {
	return a.configManager.SetAutoPaste(enabled)
}

// AudioInputDevice represents an audio input device for the frontend
type AudioInputDevice struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

// GetAudioInputDevices returns the list of available audio input devices
func (a *App) GetAudioInputDevices() ([]AudioInputDevice, error) {
	devices, err := audio.GetAudioInputDevices()
	if err != nil {
		return nil, err
	}

	// Convert to app-level struct
	result := make([]AudioInputDevice, len(devices))
	for i, d := range devices {
		result[i] = AudioInputDevice{
			Name:      d.Name,
			IsDefault: d.IsDefault,
		}
	}
	return result, nil
}

// SetAudioInputDevice sets the audio input device
func (a *App) SetAudioInputDevice(deviceName string) error {
	// Save to config - will be applied when recorder is created in StartRecording
	return a.configManager.SetAudioInputDevice(deviceName)
}

// GetCurrentAudioInputDevice returns the currently selected audio input device name
func (a *App) GetCurrentAudioInputDevice() string {
	if a.configManager != nil {
		return a.configManager.Get().AudioInputDevice
	}
	return ""
}

// GetStats returns usage statistics
func (a *App) GetStats() UsageStats {
	if a.statsManager == nil {
		return UsageStats{}
	}
	
	stats := a.statsManager.Get()
	return UsageStats{
		AverageWPM:         a.statsManager.GetAverageWPM(),
		WordsThisWeek:      stats.WeeklyWords,
		RecordingsThisWeek: stats.WeeklyRecordings,
		TimeSavedThisWeek:  stats.WeeklyTimeSaved / 60.0, // Convert to minutes
		TotalRecordings:    stats.TotalRecordings,
		TotalWords:         stats.TotalWords,
	}
}

// DownloadModel downloads a model
func (a *App) DownloadModel(model string) error {
	go func() {
		err := a.modelManager.DownloadModel(model, func(downloaded, total int64) {
			progress := float64(downloaded) / float64(total) * 100
			runtime.EventsEmit(a.ctx, "downloadProgress", map[string]interface{}{
				"model":      model,
				"downloaded": downloaded,
				"total":      total,
				"progress":   progress,
			})
		})

		if err != nil {
			runtime.EventsEmit(a.ctx, "downloadError", map[string]interface{}{
				"model": model,
				"error": err.Error(),
			})
		} else {
			runtime.EventsEmit(a.ctx, "downloadComplete", map[string]interface{}{
				"model": model,
			})
			a.emitState()
		}
	}()

	return nil
}

// IsModelDownloaded checks if a model is downloaded
func (a *App) IsModelDownloaded(model string) bool {
	return a.modelManager.IsModelDownloaded(model)
}

// Quit closes the application
func (a *App) Quit() {
	runtime.Quit(a.ctx)
}

// Minimize minimizes the window
func (a *App) Minimize() {
	runtime.WindowMinimise(a.ctx)
}

// Hide hides the window
func (a *App) Hide() {
	runtime.WindowHide(a.ctx)
}

// ShowWindow shows and focuses the window
func (a *App) ShowWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
		runtime.WindowSetAlwaysOnTop(a.ctx, true)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
	}
}

// QuitApp quits the application
func (a *App) QuitApp() {
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// SetTray sets the callback for updating tray icon
func (a *App) SetTray(onUpdate func(recording bool)) {
	a.onTrayUpdate = onUpdate
}
