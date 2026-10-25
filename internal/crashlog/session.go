package crashlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const sessionMarkerPrefix = "yap-session-"

// UncleanSession describes an earlier Yap process that exited without
// running its clean shutdown path.
type UncleanSession struct {
	PID     int
	Details string
}

// Session marks this process as running until End is called.
type Session struct {
	path string
}

// StartSession reports sessions whose process is gone but whose marker was
// never removed, then writes a marker for the current process. Markers of
// processes that are still running (another Yap instance) are left alone.
func StartSession(logsDir, version string) (*Session, []UncleanSession, error) {
	unclean := collectUncleanSessions(logsDir)

	path := filepath.Join(logsDir, fmt.Sprintf("%s%d", sessionMarkerPrefix, os.Getpid()))
	details := fmt.Sprintf("version=%s started=%s", version, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(details), 0644); err != nil {
		return nil, unclean, err
	}
	return &Session{path: path}, unclean, nil
}

// End marks the session as cleanly shut down.
func (s *Session) End() {
	if s != nil {
		_ = os.Remove(s.path)
	}
}

func collectUncleanSessions(logsDir string) []UncleanSession {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil
	}

	var unclean []UncleanSession
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, sessionMarkerPrefix) {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimPrefix(name, sessionMarkerPrefix))
		if err != nil || pid <= 0 || pid == os.Getpid() || processRunning(pid) {
			continue
		}
		path := filepath.Join(logsDir, name)
		data, _ := os.ReadFile(path)
		unclean = append(unclean, UncleanSession{PID: pid, Details: strings.TrimSpace(string(data))})
		_ = os.Remove(path)
	}
	return unclean
}
