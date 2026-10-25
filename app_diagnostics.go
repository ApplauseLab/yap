package main

import (
	"context"

	"yap/internal/logger"
)

// beforeClose logs why the app is quitting. Wails calls it for Cmd+Q, the app
// menu, closing the main window and runtime.Quit; returning false lets the
// quit continue.
func (a *App) beforeClose(ctx context.Context) bool {
	logger.Info("Quit requested; shutting down")
	return false
}
