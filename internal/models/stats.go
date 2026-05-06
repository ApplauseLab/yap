package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Stats holds usage statistics
type Stats struct {
	// All-time stats
	TotalRecordings    int     `json:"totalRecordings"`
	TotalWords         int     `json:"totalWords"`
	TotalRecordingTime float64 `json:"totalRecordingTime"` // seconds
	TotalTimeSaved     float64 `json:"totalTimeSaved"`     // seconds (estimated)

	// Weekly stats (reset each week)
	WeekStartDate       string  `json:"weekStartDate"` // ISO date string
	WeeklyRecordings    int     `json:"weeklyRecordings"`
	WeeklyWords         int     `json:"weeklyWords"`
	WeeklyRecordingTime float64 `json:"weeklyRecordingTime"` // seconds
	WeeklyTimeSaved     float64 `json:"weeklyTimeSaved"`     // seconds

	// For WPM calculation
	RecentWPMs []float64 `json:"recentWPMs"` // Last 20 WPM values for averaging
}

// StatsManager handles loading and saving statistics
type StatsManager struct {
	configDir string
	statsFile string
	stats     *Stats
	mu        sync.RWMutex
}

// NewStatsManager creates a new stats manager
func NewStatsManager(configDir string) (*StatsManager, error) {
	sm := &StatsManager{
		configDir: configDir,
		statsFile: filepath.Join(configDir, "stats.json"),
		stats:     &Stats{},
	}

	if err := sm.load(); err != nil {
		// Initialize with empty stats if file doesn't exist
		sm.stats = &Stats{
			WeekStartDate: getWeekStart(time.Now()),
			RecentWPMs:    make([]float64, 0),
		}
	}

	// Check if we need to reset weekly stats
	sm.checkWeeklyReset()

	return sm, nil
}

// Get returns the current stats
func (sm *StatsManager) Get() Stats {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return *sm.stats
}

// RecordTranscription updates stats after a successful transcription
func (sm *StatsManager) RecordTranscription(text string, recordingDuration float64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if we need to reset weekly stats
	sm.checkWeeklyResetLocked()

	// Count words
	words := countWords(text)

	// Calculate WPM for this recording
	var wpm float64
	if recordingDuration > 0 {
		wpm = float64(words) / (recordingDuration / 60.0)
	}

	// Estimate time saved (assuming average typing speed of 40 WPM)
	const avgTypingWPM = 40.0
	typingTime := float64(words) / avgTypingWPM * 60.0 // seconds
	timeSaved := typingTime - recordingDuration
	if timeSaved < 0 {
		timeSaved = 0
	}

	// Update all-time stats
	sm.stats.TotalRecordings++
	sm.stats.TotalWords += words
	sm.stats.TotalRecordingTime += recordingDuration
	sm.stats.TotalTimeSaved += timeSaved

	// Update weekly stats
	sm.stats.WeeklyRecordings++
	sm.stats.WeeklyWords += words
	sm.stats.WeeklyRecordingTime += recordingDuration
	sm.stats.WeeklyTimeSaved += timeSaved

	// Update WPM history (keep last 20)
	if wpm > 0 && wpm < 500 { // Sanity check
		sm.stats.RecentWPMs = append(sm.stats.RecentWPMs, wpm)
		if len(sm.stats.RecentWPMs) > 20 {
			sm.stats.RecentWPMs = sm.stats.RecentWPMs[1:]
		}
	}

	// Save to disk
	sm.saveLocked()
}

// GetAverageWPM returns the average words per minute
func (sm *StatsManager) GetAverageWPM() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.stats.RecentWPMs) == 0 {
		return 0
	}

	var sum float64
	for _, wpm := range sm.stats.RecentWPMs {
		sum += wpm
	}
	return sum / float64(len(sm.stats.RecentWPMs))
}

// checkWeeklyReset checks if we need to reset weekly stats (must hold lock)
func (sm *StatsManager) checkWeeklyResetLocked() {
	currentWeekStart := getWeekStart(time.Now())
	if sm.stats.WeekStartDate != currentWeekStart {
		// New week, reset weekly stats
		sm.stats.WeekStartDate = currentWeekStart
		sm.stats.WeeklyRecordings = 0
		sm.stats.WeeklyWords = 0
		sm.stats.WeeklyRecordingTime = 0
		sm.stats.WeeklyTimeSaved = 0
	}
}

// checkWeeklyReset checks if we need to reset weekly stats
func (sm *StatsManager) checkWeeklyReset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.checkWeeklyResetLocked()
}

// load reads stats from disk
func (sm *StatsManager) load() error {
	data, err := os.ReadFile(sm.statsFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, sm.stats)
}

// saveLocked saves stats to disk (must hold lock)
func (sm *StatsManager) saveLocked() error {
	data, err := json.MarshalIndent(sm.stats, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sm.statsFile, data, 0644)
}

// Save saves stats to disk
func (sm *StatsManager) Save() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.saveLocked()
}

// Helper functions

func getWeekStart(t time.Time) string {
	// Get the start of the week (Monday)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday
	}
	weekStart := t.AddDate(0, 0, -(weekday - 1))
	return weekStart.Format("2006-01-02")
}

func countWords(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	words := strings.Fields(text)
	return len(words)
}
