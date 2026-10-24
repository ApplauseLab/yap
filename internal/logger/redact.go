package logger

import "regexp"

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-proj-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`Bearer\s+[A-Za-z0-9._-]{20,}`),
}

// Redact removes sensitive values before messages are written to disk.
func Redact(message string) string {
	for _, pattern := range sensitivePatterns {
		message = pattern.ReplaceAllString(message, "[REDACTED]")
	}
	return message
}
