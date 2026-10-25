//go:build !windows

package system

func revealFile(path string) error { return revealWithCommand(path) }
