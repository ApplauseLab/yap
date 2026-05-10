package main

import (
	"embed"

	"yap/internal/tray"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

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

	// Set the tray reference in app
	app.SetTray(tray.SetRecording)

	err := wails.Run(&options.App{
		Title:     "Yap",
		Width:     950,
		Height:    620,
		MinWidth:  850,
		MinHeight: 550,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 22, G: 19, B: 31, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Frameless:        false,
		StartHidden:      false,
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
			Message: "Speech-to-Text Desktop App\nby applauselab.ai\nv0.2.0",
		},
		},
	})

	// Cleanup systray when Wails exits
	tray.Quit()

	if err != nil {
		println("Error:", err.Error())
	}
}
