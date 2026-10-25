package main

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"os"
	"runtime"

	"yap/internal/crashlog"
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
	session := startCrashDiagnostics()
	defer crashlog.LogPanic("main")

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
	var instanceLock *options.SingleInstanceLock
	if runtime.GOOS == "windows" {
		configDir, _ := os.UserConfigDir()
		instanceLock = &options.SingleInstanceLock{
			UniqueId: fmt.Sprintf("yap-%x", sha256.Sum256([]byte(configDir))),
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				filelogger.Info("Second launch received; showing existing Yap window")
				if app.WaitForStartup() == nil {
					app.ShowWindow()
				}
			},
		}
	}

	// A nil *FileLogger stored in the interface would silently drop Wails logs.
	var wailsLogger wailslogger.Logger
	if logger != nil {
		wailsLogger = logger
	}

	err := wails.Run(&options.App{
		SingleInstanceLock: instanceLock,
		Title:              "Yap",
		Width:              950,
		Height:             620,
		MinWidth:           850,
		MinHeight:          550,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:   &options.RGBA{R: 22, G: 19, B: 31, A: 255},
		OnStartup:          app.startup,
		OnDomReady:         app.domReady,
		OnBeforeClose:      app.beforeClose,
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

	filelogger.Info("Wails runtime exited")

	// Cleanup systray when Wails exits
	tray.Quit()

	if err != nil {
		filelogger.Error("Wails runtime error: " + err.Error())
		println("Error:", err.Error())
	}
	session.End()
}

// startCrashDiagnostics keeps crash output and reports earlier sessions that
// ended without a clean shutdown (crash, force quit or power loss).
func startCrashDiagnostics() *crashlog.Session {
	logsDir := filelogger.GetLogsDir()
	if path, err := crashlog.RedirectStderr(logsDir, appVersion); err != nil {
		filelogger.Warning("Failed to redirect stderr to the logs directory: " + err.Error())
	} else if path != "" {
		filelogger.Info("Crash output is written to " + path)
	}

	session, unclean, err := crashlog.StartSession(logsDir, appVersion)
	if err != nil {
		filelogger.Warning("Failed to record session start: " + err.Error())
	}
	for _, previous := range unclean {
		filelogger.Warning(fmt.Sprintf("Previous Yap session pid=%d (%s) exited without a clean shutdown; crash output, if any, is in %s<date>.log", previous.PID, previous.Details, crashlog.StderrLogPrefix))
	}

	crashlog.LogAppLifecycle()
	return session
}
