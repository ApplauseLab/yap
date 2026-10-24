package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"time"
)

const retentionDays = 7

// FileLogger implements the Wails logger interface and writes logs to disk.
type FileLogger struct {
	mu      sync.Mutex
	file    *os.File
	logsDir string
	writer  io.Writer
}

var defaultLogger *FileLogger

// Init creates the process-wide logger used by Wails and package-level callers.
func Init() error {
	logger, err := New()
	if err != nil {
		return err
	}
	defaultLogger = logger
	return nil
}

// New creates a file logger under the user's Yap config directory.
func New() (*FileLogger, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}

	logsDir := filepath.Join(configDir, "yap", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, err
	}

	logPath := filepath.Join(logsDir, fmt.Sprintf("yap-%s.log", time.Now().Format("2006-01-02")))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	logger := &FileLogger{
		file:    file,
		logsDir: logsDir,
		writer:  io.MultiWriter(file, os.Stdout),
	}
	logger.cleanOldLogs(retentionDays)
	logger.Info(fmt.Sprintf("=== Yap started === os=%s arch=%s logsDir=%s", goruntime.GOOS, goruntime.GOARCH, logsDir))

	return logger, nil
}

// GetDefault returns the process-wide logger if it has been initialized.
func GetDefault() *FileLogger {
	return defaultLogger
}

// GetLogsDir returns the logs directory path even before the logger is initialized.
func GetLogsDir() string {
	if defaultLogger != nil {
		return defaultLogger.logsDir
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "yap", "logs")
}

// Close flushes the shutdown marker and closes the active log file.
func (l *FileLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.Info("=== Yap shutdown ===")
	return l.file.Close()
}

func (l *FileLogger) write(level, message string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	line := fmt.Sprintf("[%s] [%s] %s\n", time.Now().Format("2006-01-02T15:04:05.000"), level, Redact(message))
	_, _ = l.writer.Write([]byte(line))
}

func (l *FileLogger) cleanOldLogs(keepDays int) {
	entries, err := os.ReadDir(l.logsDir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -keepDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(l.logsDir, entry.Name()))
	}
}

func Print(message string) {
	if defaultLogger != nil {
		defaultLogger.Print(message)
	}
}

func Trace(message string) {
	if defaultLogger != nil {
		defaultLogger.Trace(message)
	}
}

func Debug(message string) {
	if defaultLogger != nil {
		defaultLogger.Debug(message)
	}
}

func Info(message string) {
	if defaultLogger != nil {
		defaultLogger.Info(message)
	}
}

func Warning(message string) {
	if defaultLogger != nil {
		defaultLogger.Warning(message)
	}
}

func Error(message string) {
	if defaultLogger != nil {
		defaultLogger.Error(message)
	}
}

func Fatal(message string) {
	if defaultLogger != nil {
		defaultLogger.Fatal(message)
	}
}

func (l *FileLogger) Print(message string)   { l.write("PRINT", message) }
func (l *FileLogger) Trace(message string)   { l.write("TRACE", message) }
func (l *FileLogger) Debug(message string)   { l.write("DEBUG", message) }
func (l *FileLogger) Info(message string)    { l.write("INFO", message) }
func (l *FileLogger) Warning(message string) { l.write("WARN", message) }
func (l *FileLogger) Error(message string)   { l.write("ERROR", message) }
func (l *FileLogger) Fatal(message string)   { l.write("FATAL", message) }
