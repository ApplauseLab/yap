//go:build windows

package tray

import _ "embed"

//go:embed yap.ico
var windowsIcon []byte

func init() {
	iconData = windowsIcon
	iconRecording = windowsIcon
}
