//go:build !windows

package logger

func captureCrashes(string) error { return nil }
