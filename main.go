package main

import (
	"embed"

	filelogger "yap/internal/logger"
	"yap/internal/transcribe"
	"yap/internal/tray"

	"github.com/wailsapp/wails/v2"
	wailslogger "github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

var appVersion = "dev"

func main() {
	// Yap re-executes itself as the Whisper worker watchdog; that mode must
	// not initialise logging, the tray, or Wails.
	transcribe.RunWorkerWatchdogIfRequested()

	if err := filelogger.Init(); err != nil {
		println("Warning: Failed to initialize file logger:", err.Error())
	}
	logger := filelogger.GetDefault()
	defer func() {
		if logger != nil {
			logger.Close()
		}
	}()
	filelogger.Info("Yap app version: " + appVersion)

	app := NewApp()
	filelogger.Info("App instance created")

	// Start systray (non-blocking with external loop)
	tray.Start(tray.Callbacks{
		OnToggleRecording: func() {
			app.ToggleRecording()
		},
		OnShowWindow: func() {
			app.ShowWindow()
		},
		OnSettings: func() {
			app.ShowWindow()
		},
		OnQuit: func() {
			app.QuitApp()
		},
	})
	filelogger.Info("System tray started")

	// Set the tray reference in app
	app.SetTray(tray.SetRecording)
	filelogger.Info("Starting Wails runtime")

	// A nil *FileLogger stored in the interface would silently drop Wails logs.
	var wailsLogger wailslogger.Logger
	if logger != nil {
		wailsLogger = logger
	}

	err := wails.Run(&options.App{
		Title:     "Yap",
		Width:     950,
		Height:    620,
		MinWidth:  850,
		MinHeight: 550,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:   &options.RGBA{R: 22, G: 19, B: 31, A: 255},
		OnStartup:          app.startup,
		OnDomReady:         app.domReady,
		OnShutdown:         app.shutdown,
		Logger:             wailsLogger,
		LogLevel:           wailslogger.DEBUG,
		LogLevelProduction: wailslogger.DEBUG,
		Frameless:          false,
		StartHidden:        false,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				HideTitleBar:               false,
				FullSizeContent:            true,
				UseToolbar:                 false,
			},
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "Yap",
				Message: "Speech-to-Text Desktop App\nby applauselab.ai\nv" + appVersion,
			},
		},
	})

	// Cleanup systray when Wails exits
	tray.Quit()

	if err != nil {
		println("Error:", err.Error())
	}
}
