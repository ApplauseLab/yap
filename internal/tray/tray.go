package tray

import (
	"fyne.io/systray"
)

// Icon is a simple 22x22 PNG icon for the menu bar
// This is a minimal valid PNG with a purple/indigo color
var iconData = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x16, 0x00, 0x00, 0x00, 0x16,
	0x08, 0x06, 0x00, 0x00, 0x00, 0xc4, 0xb4, 0x6c, 0x3b, 0x00, 0x00, 0x00,
	0x49, 0x49, 0x44, 0x41, 0x54, 0x38, 0x8d, 0x63, 0x64, 0x60, 0x60, 0xf8,
	0xcf, 0xc0, 0xc0, 0xc0, 0xc4, 0xc0, 0xc0, 0xc0, 0x40, 0x21, 0x60, 0x44,
	0x16, 0x60, 0x62, 0xa0, 0x10, 0x30, 0x32, 0x30, 0x30, 0xfc, 0x67, 0x68,
	0x68, 0x68, 0xf8, 0x4f, 0x48, 0x03, 0x23, 0x23, 0x23, 0xc3, 0x7f, 0x86,
	0x86, 0x86, 0x86, 0xff, 0x84, 0x34, 0x30, 0x8c, 0x1a, 0x32, 0x6a, 0xc8,
	0xa8, 0x21, 0xa3, 0x86, 0x8c, 0x1a, 0x32, 0x04, 0x00, 0x93, 0xc6, 0x0c,
	0xab, 0x98, 0x47, 0xac, 0xbd, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
	0x44, 0xae, 0x42, 0x60, 0x82,
}

// Recording icon (same for now, will update later)
var iconRecording = iconData

// Callbacks for menu actions
type Callbacks struct {
	OnToggleRecording func()
	OnShowWindow      func()
	OnSettings        func()
	OnQuit            func()
}

var callbacks Callbacks
var mToggle *systray.MenuItem
var isRecording bool
var started bool

// Start initializes the system tray using external loop mode (non-blocking)
func Start(cb Callbacks) {
	callbacks = cb
	// Use RunWithExternalLoop which doesn't block and works with other main loops
	systray.RunWithExternalLoop(onReady, onExit)
	started = true
}

// Quit stops the systray
func Quit() {
	if started {
		systray.Quit()
	}
}

func onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("")
	systray.SetTooltip("Applause Whisper")

	mToggle = systray.AddMenuItem("Start Recording", "Start/Stop Recording (⌘⇧Space)")
	mShow := systray.AddMenuItem("Show Window", "Show the main window")
	systray.AddSeparator()
	mSettings := systray.AddMenuItem("Settings...", "Open settings")
	systray.AddSeparator()
	mVersion := systray.AddMenuItem("Version 1.0.0", "")
	mVersion.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit Applause Whisper")

	go func() {
		for {
			select {
			case <-mToggle.ClickedCh:
				if callbacks.OnToggleRecording != nil {
					callbacks.OnToggleRecording()
				}
			case <-mShow.ClickedCh:
				if callbacks.OnShowWindow != nil {
					callbacks.OnShowWindow()
				}
			case <-mSettings.ClickedCh:
				if callbacks.OnSettings != nil {
					callbacks.OnSettings()
				}
			case <-mQuit.ClickedCh:
				if callbacks.OnQuit != nil {
					callbacks.OnQuit()
				}
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	// Cleanup
}

// SetRecording updates the tray icon and menu for recording state
func SetRecording(recording bool) {
	if !started || mToggle == nil {
		return
	}
	isRecording = recording
	// Use goroutine to avoid blocking and potential deadlocks
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Ignore panics from systray operations
			}
		}()
		if recording {
			systray.SetIcon(iconRecording)
			mToggle.SetTitle("Stop Recording")
		} else {
			systray.SetIcon(iconData)
			mToggle.SetTitle("Start Recording")
		}
	}()
}
