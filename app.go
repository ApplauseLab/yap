package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"yap/internal/audio"
	"yap/internal/crashlog"
	"yap/internal/hotkey"
	"yap/internal/logger"
	"yap/internal/models"
	"yap/internal/overlay"
	"yap/internal/sounds"
	"yap/internal/system"
	"yap/internal/transcribe"

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

// ModelInfo describes a catalog model and its local availability for the frontend.
type ModelInfo struct {
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	Size           string `json:"size"`
	SizeBytes      int64  `json:"sizeBytes"`
	Downloaded     bool   `json:"downloaded"`
	EnglishOnly    bool   `json:"englishOnly"`
	Provider       string `json:"provider"`
	Speed          int    `json:"speed"`
	Accuracy       int    `json:"accuracy"`
	UseCase        string `json:"useCase"`
	Recommendation string `json:"recommendation"`
	Favorite       bool   `json:"favorite"`
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
	recordingMu   sync.Mutex // Serialize hotkey/UI start, stop and cancel transitions.
	startupDone   chan struct{}
	startupErr    error // Published by closing startupDone.
	ctx           context.Context
	recorder      *audio.Recorder
	localEngine   *transcribe.LocalEngine
	openaiEngine  *transcribe.OpenAIEngine
	configManager *models.ConfigManager
	modelManager  *models.Manager
	downloader    *models.Downloader
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
		startupDone:   make(chan struct{}),
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
	// Wails can serve frontend binding calls before OnStartup returns.
	// Publish all managers together; the UI waits before mounting App.
	a.startupErr = fmt.Errorf("application startup did not complete")
	defer close(a.startupDone)
	a.ctx = ctx
	runtime.LogInfo(a.ctx, "OnStartup: initializing")

	// Initialize PortAudio
	if err := audio.Initialize(); err != nil {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to initialize audio: %v", err))
	}

	// Initialize native sounds
	if err := sounds.Init(); err != nil {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to initialize sounds: %v", err))
	} else {
		runtime.LogInfo(a.ctx, "Native sounds initialized successfully")
	}

	// Initialize config manager
	configManager, err := models.NewConfigManager()
	if err != nil {
		a.startupErr = fmt.Errorf("initialize configuration: %w", err)
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to initialize config: %v", err))
		return
	}
	a.configManager = configManager

	// Initialize stats manager
	statsManager, err := models.NewStatsManager(configManager.GetConfigDir())
	if err != nil {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to initialize stats manager: %v", err))
	} else {
		a.statsManager = statsManager
	}

	// Audio device preference is applied in StartRecording when recorder is created
	if deviceName := configManager.Get().AudioInputDevice; deviceName != "" {
		runtime.LogInfo(a.ctx, fmt.Sprintf("Will use audio input device: %s", deviceName))
	}

	// Initialize model manager
	modelManager, err := models.NewManager(configManager.GetModelsDir())
	if err != nil {
		a.startupErr = fmt.Errorf("initialize models: %w", err)
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to initialize model manager: %v", err))
		return
	}
	a.modelManager = modelManager
	a.downloader = models.NewDownloader(modelManager, func(state models.DownloadState) {
		runtime.EventsEmit(a.ctx, "modelDownloadState", state)
	}, a.onModelDownloadFinished)

	// Initialize transcription engines
	a.localEngine = transcribe.NewLocalEngine(configManager.GetModelsDir())
	a.localEngine.ReapOrphanedWorker()
	a.localEngine.SetModel(transcribe.Model(configManager.Get().Model))
	a.handleTerminationSignals()

	a.openaiEngine = transcribe.NewOpenAIEngine(configManager.Get().OpenAIAPIKey)
	if err := a.validateReadyToRecord(); err != nil {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Transcription not ready: %v", err))
	} else if configManager.Get().Provider == "local" {
		a.prewarmLocalEngine()
	}

	// Check accessibility permissions without prompting.
	// Onboarding handles the explicit permission request UX.
	if !hotkey.HasAccessibilityPermissions() {
		runtime.LogWarning(a.ctx, "Accessibility permissions not granted; escape cancel and auto-paste may not work")
	} else {
		runtime.LogInfo(a.ctx, "Accessibility permissions granted")
	}

	// Apply configured hotkey type before registering
	hotkeyType := configManager.Get().RecordingHotkey
	if hotkeyType == "" {
		hotkeyType = models.DefaultRecordingHotkey()
	}
	a.hotkeyManager.SetHotkeyType(hotkeyType)

	// Apply configured cancel key
	cancelKey := configManager.Get().CancelHotkey
	if cancelKey == "" {
		cancelKey = "escape"
	}
	a.hotkeyManager.SetCancelKey(cancelKey)

	// Register global hotkey
	if err := a.hotkeyManager.Register(func() {
		a.ToggleRecording()
	}); err != nil {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to register hotkey: %v", err))
	} else {
		a.hotkeyEnabled = true
		runtime.LogInfo(a.ctx, fmt.Sprintf("Global hotkey registered: %s", hotkey.GetHotkeyDisplayName(hotkeyType)))
	}

	// Load history from disk
	a.loadHistory()
	runtime.LogInfo(a.ctx, "OnStartup: complete")
	a.startupErr = nil
}

// WaitForStartup is a binding used before mounting the frontend. Closing the
// channel publishes initialization writes to all waiting callers.
func (a *App) WaitForStartup() error {
	if a.startupDone == nil {
		return fmt.Errorf("application startup has not been scheduled")
	}
	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()
	select {
	case <-a.startupDone:
		return a.startupErr
	case <-timer.C:
		return fmt.Errorf("application startup timed out; check Yap logs")
	}
}

// handleTerminationSignals stops the Whisper worker when Yap is terminated by
// a signal, which bypasses OnShutdown and would otherwise orphan the worker.
func (a *App) handleTerminationSignals() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-signals
		logger.Info(fmt.Sprintf("Received %s; stopping local transcription worker", sig))
		if a.localEngine != nil {
			_ = a.localEngine.Close()
		}
		go func() {
			time.Sleep(5 * time.Second)
			os.Exit(0)
		}()
		runtime.Quit(a.ctx)
	}()
}

// domReady is called when the frontend has loaded its DOM.
func (a *App) domReady(ctx context.Context) {
	runtime.LogInfo(ctx, "OnDomReady: frontend loaded")
}

// shutdown is called when the app closes
func (a *App) shutdown(ctx context.Context) {
	runtime.LogInfo(ctx, "OnShutdown: starting cleanup")
	if a.downloader != nil {
		a.downloader.Shutdown(5 * time.Second)
	}
	if a.hotkeyManager != nil {
		a.hotkeyManager.Unregister()
	}
	if a.overlay != nil {
		a.overlay.Destroy()
	}
	if a.localEngine != nil {
		if err := a.localEngine.Close(); err != nil {
			runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to stop local transcription worker: %v", err))
		}
	}
	audio.Terminate()
	sounds.Cleanup()
	runtime.LogInfo(ctx, "OnShutdown: cleanup complete")
}

// GetState returns the current app state
func (a *App) GetState() AppState {
	if err := a.WaitForStartup(); err != nil {
		return AppState{Error: err.Error()}
	}
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
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to parse history: %v", err))
		return
	}

	a.mu.Lock()
	a.history = history
	a.mu.Unlock()
	runtime.LogInfo(a.ctx, fmt.Sprintf("Loaded %d history items", len(history)))
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
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to serialize history: %v", err))
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to save history: %v", err))
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
			runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to delete audio file: %v", err))
		}
	}

	a.saveHistory()
	runtime.EventsEmit(a.ctx, "historyChanged", a.history)
	return nil
}

// ShowInFolder opens the system file manager with the audio file selected
func (a *App) ShowInFolder(id string) error {
	a.mu.Lock()
	var selected *HistoryItem

	for _, item := range a.history {
		if item.ID == id {
			copy := item
			selected = &copy
			break
		}
	}
	a.mu.Unlock()
	if selected == nil {
		return fmt.Errorf("history item not found")
	}
	if !selected.HasAudio || selected.AudioPath == "" {
		return fmt.Errorf("no audio available for this item")
	}
	if err := system.RevealFile(selected.AudioPath); err != nil {
		logger.Error(fmt.Sprintf("ShowInFolder failed: %v", err))
		return err
	}
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
	a.recordingMu.Lock()
	defer a.recordingMu.Unlock()
	runtime.LogInfo(a.ctx, "StartRecording called")
	runtime.LogDebug(a.ctx, "StartRecording: entering function")
	if err := a.validateReadyToRecord(); err != nil {
		a.mu.Lock()
		a.state = StateError
		a.lastError = err.Error()
		a.mu.Unlock()
		runtime.LogWarning(a.ctx, fmt.Sprintf("StartRecording blocked: %v", err))
		a.emitState()
		return err
	}

	// Save the current frontmost app before we do anything (for auto-paste later)
	system.SaveFrontmostApp()

	a.mu.Lock()
	if a.state != StateReady && a.state != StateError {
		a.mu.Unlock()
		runtime.LogWarning(a.ctx, fmt.Sprintf("Cannot start recording in state: %s", a.state))
		return fmt.Errorf("cannot start recording in state: %s", a.state)
	}

	// Check if sound is enabled
	soundEnabled := a.configManager != nil && (a.configManager.Get().SoundEnabled == nil || *a.configManager.Get().SoundEnabled)
	runtime.LogDebug(a.ctx, fmt.Sprintf("StartRecording: soundEnabled=%v", soundEnabled))

	// Set state to recording first
	a.state = StateRecording
	a.lastError = ""
	a.recordStartTime = time.Now()
	onTrayUpdate := a.onTrayUpdate
	a.mu.Unlock()

	// Play start sound (non-blocking) - afplay goes to speakers, not mic input
	if soundEnabled {
		runtime.LogDebug(a.ctx, "StartRecording: playing start sound")
		sounds.PlayStart()
	}

	runtime.LogInfo(a.ctx, "State changed to recording, emitting state")

	// Update tray icon
	if onTrayUpdate != nil {
		onTrayUpdate(true)
	}

	a.emitState()

	// Create fresh recorder for each recording session. GetState reads it
	// under a.mu while the frontend polls during recording.
	a.mu.Lock()
	a.recorder = audio.NewRecorder()
	a.mu.Unlock()

	// Set audio device from config if available
	if a.configManager != nil {
		config := a.configManager.Get()
		if config.AudioInputDevice != "" {
			a.recorder.SetDevice(config.AudioInputDevice)
		}
	}

	a.recorder.SetLevelCallback(a.overlay.SetAudioLevel)
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

	// Enable cancel key to cancel recording (native/global)
	runtime.LogDebug(a.ctx, "Enabling cancel key")
	a.hotkeyManager.EnableCancelKey(func() {
		runtime.LogDebug(a.ctx, "Cancel key callback triggered")
		a.CancelRecording()
	})

	return nil
}

// StopRecording ends audio capture and starts transcription
func (a *App) StopRecording() error {
	a.recordingMu.Lock()
	defer a.recordingMu.Unlock()
	a.mu.Lock()
	if a.state != StateRecording {
		a.mu.Unlock()
		return fmt.Errorf("not recording")
	}
	recordDuration := time.Since(a.recordStartTime).Seconds()
	a.state = StateTranscribing
	a.mu.Unlock()

	// Disable cancel key and audio level callback
	a.hotkeyManager.DisableCancelKey()
	a.recorder.SetLevelCallback(nil)

	// Update overlay to show transcribing status
	a.overlay.SetStatus("Transcribing...")

	a.emitState()

	samples, err := a.recorder.Stop()
	runtime.LogInfo(a.ctx, fmt.Sprintf("Audio capture stopped: samples=%d error=%v", len(samples), err))
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
	a.recordingMu.Lock()
	defer a.recordingMu.Unlock()
	a.mu.Lock()
	if a.state != StateRecording {
		a.mu.Unlock()
		return fmt.Errorf("not recording")
	}
	a.state = StateReady
	onTrayUpdate := a.onTrayUpdate
	a.mu.Unlock()

	// Disable cancel key and audio level callback
	a.hotkeyManager.DisableCancelKey()
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
	defer crashlog.LogPanic("transcribe")
	config := a.configManager.Get()

	var text string
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	transcriptionStarted := time.Now()

	if config.Provider == "openai" {
		text, err = a.openaiEngine.Transcribe(ctx, samples)
	} else {
		text, err = a.localEngine.Transcribe(ctx, samples)
	}
	if err != nil {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Transcription failed: provider=%s model=%s duration=%.2fs samples=%d error=%v", config.Provider, config.Model, duration, len(samples), err))
	} else {
		backend := config.Provider
		if config.Provider == "local" {
			backend = a.localEngine.Backend()
		}
		runtime.LogInfo(a.ctx, fmt.Sprintf("Transcription complete: provider=%s model=%s backend=%s audioDuration=%.2fs elapsed=%s", config.Provider, config.Model, backend, duration, time.Since(transcriptionStarted).Round(time.Millisecond)))
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
			runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to save audio: %v", saveErr))
			audioPath = ""
		} else {
			hasAudio = true
			runtime.LogInfo(a.ctx, fmt.Sprintf("Saved audio to: %s", audioPath))
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
			Timestamp: time.Now().Format("2 Jan 2006, 3:04 pm"),
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
		crashlog.Go("save history", a.saveHistory)

		// Record stats
		if a.statsManager != nil {
			a.statsManager.RecordTranscription(text, duration)
		}

		// Copy to clipboard and optionally paste
		runtime.LogDebug(a.ctx, fmt.Sprintf("AutoPaste=%v, text length=%d", config.AutoPaste, len(text)))
		if config.AutoPaste {
			go func(textToPaste string) {
				defer crashlog.Recover("auto-paste")
				// Wait for the overlay to hide and the previous app to regain focus
				runtime.LogDebug(a.ctx, "Waiting 500ms before paste")
				time.Sleep(500 * time.Millisecond)
				runtime.LogDebug(a.ctx, "Calling CopyAndPaste now")
				if err := system.CopyAndPaste(textToPaste); err != nil {
					runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to paste: %v", err))
				} else {
					runtime.LogDebug(a.ctx, "CopyAndPaste succeeded")
				}
			}(text)
		} else {
			runtime.LogDebug(a.ctx, "AutoPaste disabled, only copying to clipboard")
			go func(textToCopy string) {
				defer crashlog.Recover("copy to clipboard")
				if err := system.CopyToClipboard(textToCopy); err != nil {
					runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to copy to clipboard: %v", err))
				}
			}(text)
		}
	}
	a.mu.Unlock()

	// Update tray icon
	if onTrayUpdate != nil {
		onTrayUpdate(false)
	}

	// Hide overlay
	a.overlay.Hide()

	// Play stop sound (after recording/transcription complete)
	soundEnabled := a.configManager != nil && (a.configManager.Get().SoundEnabled == nil || *a.configManager.Get().SoundEnabled)
	if soundEnabled {
		sounds.PlayStop()
	}

	a.emitState()
	runtime.EventsEmit(a.ctx, "historyChanged", a.GetHistory())
}

func (a *App) validateReadyToRecord() error {
	if a.configManager == nil {
		return fmt.Errorf("configuration is not ready")
	}

	config := a.configManager.Get()
	if config.Provider == "openai" {
		if a.openaiEngine == nil || !a.openaiEngine.IsAvailable() {
			return fmt.Errorf("OpenAI API key is not configured")
		}
		return nil
	}

	if a.modelManager == nil {
		return fmt.Errorf("model manager is not ready")
	}
	if !a.modelManager.IsModelDownloaded(config.Model) {
		return fmt.Errorf("model %q is not downloaded", config.Model)
	}
	if a.localEngine == nil || !a.localEngine.IsAvailable() {
		return fmt.Errorf("local transcription is not available; install whisper-cli and download the selected model")
	}

	return nil
}

// emitState sends the current state to the frontend
func (a *App) emitState() {
	if a.ctx != nil {
		state := a.GetState()
		runtime.LogInfo(a.ctx, fmt.Sprintf("Emitting stateChanged event: state=%s", state.State))
		runtime.EventsEmit(a.ctx, "stateChanged", state)
	}
}

// GetModels returns every catalog model with its download status
func (a *App) GetModels() []ModelInfo {
	if a.WaitForStartup() != nil {
		return nil
	}
	catalog := models.Catalog()
	result := make([]ModelInfo, len(catalog))
	for i, spec := range catalog {
		result[i] = ModelInfo{
			Name:           spec.Name,
			DisplayName:    spec.DisplayName,
			Size:           spec.Size,
			SizeBytes:      spec.SizeBytes,
			Downloaded:     a.modelManager != nil && a.modelManager.IsModelDownloaded(spec.Name),
			EnglishOnly:    spec.EnglishOnly,
			Provider:       spec.Provider,
			Speed:          spec.Speed,
			Accuracy:       spec.Accuracy,
			UseCase:        spec.UseCase,
			Recommendation: spec.Recommendation,
			Favorite:       a.configManager != nil && a.configManager.IsFavoriteModel(spec.Name),
		}
	}
	return result
}

// SetModel activates a downloaded catalog model. It never downloads.
func (a *App) SetModel(model string) error {
	if _, err := models.RequireModel(model); err != nil {
		return err
	}
	if a.configManager.Get().Provider == "local" && !a.modelManager.IsModelDownloaded(model) {
		return fmt.Errorf("model %q is not downloaded", model)
	}
	if err := a.configManager.SetModel(model); err != nil {
		return err
	}
	a.localEngine.SetModel(transcribe.Model(model))
	if a.configManager.Get().Provider == "local" {
		a.prewarmLocalEngine()
	}
	a.emitState()
	return nil
}

// SetProvider changes the transcription provider
func (a *App) SetProvider(provider string) error {
	if err := a.configManager.SetProvider(provider); err != nil {
		return err
	}
	if a.localEngine == nil {
		a.emitState()
		return nil
	}
	if provider == "local" {
		a.localEngine.Open()
		if a.modelManager != nil && a.modelManager.IsModelDownloaded(a.configManager.Get().Model) {
			a.prewarmLocalEngine()
		}
	} else {
		_ = a.localEngine.Close()
	}
	a.emitState()
	return nil
}

func (a *App) prewarmLocalEngine() {
	if a.localEngine == nil {
		return
	}
	go func() {
		defer crashlog.LogPanic("whisper prewarm")
		started := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := a.localEngine.Prewarm(ctx); err != nil {
			runtime.LogWarning(a.ctx, fmt.Sprintf("Persistent Whisper prewarm failed: %v", err))
			return
		}
		runtime.LogInfo(a.ctx, fmt.Sprintf("Persistent Whisper prewarm complete: backend=%s elapsed=%s", a.localEngine.Backend(), time.Since(started).Round(time.Millisecond)))
	}()
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
	if a.WaitForStartup() != nil {
		return nil
	}
	return a.configManager.Get()
}

// SetAutoPaste enables/disables auto-paste
func (a *App) SetAutoPaste(enabled bool) error {
	return a.configManager.SetAutoPaste(enabled)
}

// SetSoundEnabled enables/disables recording start/stop sound
func (a *App) SetSoundEnabled(enabled bool) error {
	return a.configManager.SetSoundEnabled(enabled)
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

// SetRecordingHotkey sets the recording hotkey
// Accepts any key name, e.g., "rightoption", "f9", "space"
func (a *App) SetRecordingHotkey(keyName string) error {
	keyName = strings.ToLower(strings.TrimSpace(keyName))
	if keyName == "" {
		return fmt.Errorf("hotkey cannot be empty")
	}

	// Update the hotkey manager
	if a.hotkeyManager != nil {
		a.hotkeyManager.SetHotkeyType(keyName)
	}

	// Save to config
	return a.configManager.SetRecordingHotkey(keyName)
}

// GetRecordingHotkey returns the current recording hotkey string
func (a *App) GetRecordingHotkey() string {
	if a.configManager != nil {
		hotkeyType := a.configManager.Get().RecordingHotkey
		if hotkeyType == "" {
			return models.DefaultRecordingHotkey()
		}
		return hotkeyType
	}
	return models.DefaultRecordingHotkey()
}

// SetCancelHotkey sets the cancel hotkey
// Accepts any key name, e.g., "escape", "f10"
func (a *App) SetCancelHotkey(keyName string) error {
	keyName = strings.ToLower(strings.TrimSpace(keyName))
	if keyName == "" {
		return fmt.Errorf("cancel hotkey cannot be empty")
	}

	// Update the hotkey manager
	if a.hotkeyManager != nil {
		a.hotkeyManager.SetCancelKey(keyName)
	}

	// Save to config
	return a.configManager.SetCancelHotkey(keyName)
}

// GetCancelHotkey returns the current cancel hotkey string
func (a *App) GetCancelHotkey() string {
	if a.configManager != nil {
		cancelKey := a.configManager.Get().CancelHotkey
		if cancelKey == "" {
			return "escape"
		}
		return cancelKey
	}
	return "escape"
}

// GetPlatform returns the current operating system
func (a *App) GetPlatform() string {
	return goruntime.GOOS
}

// GetLogsDir returns the directory where debug logs are written.
func (a *App) GetLogsDir() string {
	return logger.GetLogsDir()
}

// GetRecordingHotkeyDisplayName returns the display name for the current hotkey
func (a *App) GetRecordingHotkeyDisplayName() string {
	return hotkey.GetHotkeyDisplayName(a.GetRecordingHotkey())
}

// GetCancelHotkeyDisplayName returns the display name for the cancel hotkey
func (a *App) GetCancelHotkeyDisplayName() string {
	return hotkey.GetHotkeyDisplayName(a.GetCancelHotkey())
}

// CheckAccessibilityPermission checks if accessibility permissions are granted (no prompt)
func (a *App) CheckAccessibilityPermission() bool {
	return hotkey.HasAccessibilityPermissions()
}

// RequestAccessibilityPermission requests accessibility permissions and re-registers hotkey if granted
func (a *App) RequestAccessibilityPermission() bool {
	granted := hotkey.RequestAccessibilityPermissions()
	if granted {
		// Re-register hotkey now that we have permissions
		if err := a.ReregisterHotkey(); err != nil {
			runtime.LogWarning(a.ctx, fmt.Sprintf("Failed to re-register hotkey after permission grant: %v", err))
		}
	}
	return granted
}

// ReregisterHotkey unregisters and re-registers the global hotkey
// This is useful after permissions are granted or settings change
func (a *App) ReregisterHotkey() error {
	if a.hotkeyManager == nil {
		return fmt.Errorf("hotkey manager not initialized")
	}

	// Unregister current hotkey
	a.hotkeyManager.Unregister()

	// Re-apply hotkey settings
	hotkeyType := a.configManager.Get().RecordingHotkey
	if hotkeyType == "" {
		hotkeyType = models.DefaultRecordingHotkey()
	}
	a.hotkeyManager.SetHotkeyType(hotkeyType)

	cancelKey := a.configManager.Get().CancelHotkey
	if cancelKey == "" {
		cancelKey = "escape"
	}
	a.hotkeyManager.SetCancelKey(cancelKey)

	// Register the hotkey
	if err := a.hotkeyManager.Register(func() {
		a.ToggleRecording()
	}); err != nil {
		a.hotkeyEnabled = false
		return fmt.Errorf("failed to register hotkey: %w", err)
	}

	a.hotkeyEnabled = true
	runtime.LogInfo(a.ctx, fmt.Sprintf("Hotkey re-registered: %s", hotkey.GetHotkeyDisplayName(hotkeyType)))
	return nil
}

// DownloadModel starts downloading a model in the background. It validates the
// model and the single-download constraint synchronously, and never changes the
// active model.
func (a *App) DownloadModel(model string) error {
	if a.downloader == nil {
		return fmt.Errorf("model manager is not ready")
	}
	if err := a.downloader.Start(model); err != nil {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Model download rejected: model=%s error=%v", model, err))
		return err
	}
	runtime.LogInfo(a.ctx, fmt.Sprintf("Model download started: model=%s", model))
	return nil
}

// SetModelFavorite stars or unstars a catalog model in the library.
func (a *App) SetModelFavorite(model string, favorite bool) error {
	if _, err := models.RequireModel(model); err != nil {
		return err
	}
	if a.WaitForStartup() != nil || a.configManager == nil {
		return fmt.Errorf("configuration is not ready")
	}
	return a.configManager.SetModelFavorite(model, favorite)
}

// GetModelDownloadState returns the current or most recent download state.
func (a *App) GetModelDownloadState() models.DownloadState {
	if a.WaitForStartup() != nil || a.downloader == nil {
		return models.DownloadState{Status: models.DownloadIdle}
	}
	return a.downloader.State()
}

// CancelModelDownload cancels the active download if it matches model.
func (a *App) CancelModelDownload(model string) error {
	if a.downloader == nil {
		return fmt.Errorf("model manager is not ready")
	}
	return a.downloader.Cancel(model)
}

// DeleteModel removes a downloaded model that is neither active nor downloading.
func (a *App) DeleteModel(model string) error {
	if a.downloader == nil || a.configManager == nil {
		return fmt.Errorf("model manager is not ready")
	}
	if err := a.downloader.Delete(model, a.configManager.Get().Model); err != nil {
		return err
	}
	runtime.LogInfo(a.ctx, fmt.Sprintf("Model deleted: model=%s", model))
	a.emitState()
	return nil
}

func (a *App) onModelDownloadFinished(state models.DownloadState) {
	if state.Status == models.DownloadFailed {
		runtime.LogWarning(a.ctx, fmt.Sprintf("Model download failed: model=%s error=%s", state.Model, state.Error))
	} else {
		runtime.LogInfo(a.ctx, fmt.Sprintf("Model download finished: model=%s status=%s", state.Model, state.Status))
	}
	if state.Status == models.DownloadCompleted && a.configManager != nil {
		config := a.configManager.Get()
		if config.Provider == "local" && config.Model == state.Model {
			a.prewarmLocalEngine()
		}
	}
	a.emitState()
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

// IsOnboardingCompleted checks if the user has completed onboarding
func (a *App) IsOnboardingCompleted() bool {
	if a.WaitForStartup() != nil {
		return false
	}
	return a.configManager.IsOnboardingCompleted()
}

// SetOnboardingCompleted marks onboarding as completed
func (a *App) SetOnboardingCompleted(completed bool) error {
	return a.configManager.SetOnboardingCompleted(completed)
}

// CheckMicrophonePermission checks if the app has microphone permission
func (a *App) CheckMicrophonePermission() string {
	return system.CheckMicrophonePermission()
}

// RequestMicrophonePermission requests microphone permission from the user
func (a *App) RequestMicrophonePermission() string {
	return system.RequestMicrophonePermission()
}
