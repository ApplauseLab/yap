package main

import (
	"errors"
	"testing"
	"time"

	"yap/internal/models"
)

func TestEarlyFrontendReadsDoNotPanic(t *testing.T) {
	// Replays the calls in the Windows log before startup publishes managers.
	a := &App{}
	for name, call := range map[string]func(){
		"GetConfig":             func() { a.GetConfig() },
		"GetState":              func() { a.GetState() },
		"GetModels":             func() { a.GetModels() },
		"IsOnboardingCompleted": func() { a.IsOnboardingCompleted() },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if err := recover(); err != nil {
					t.Fatalf("early frontend call panicked: %v", err)
				}
			}()
			call()
		})
	}
}

func TestFrontendReadWaitsForStartupPublication(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := &App{startupDone: make(chan struct{})}
	result := make(chan *models.Config, 1)
	go func() { result <- a.GetConfig() }()
	select {
	case <-result:
		t.Fatal("GetConfig returned before startup completed")
	case <-time.After(50 * time.Millisecond):
	}
	manager, err := models.NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	a.configManager = manager
	close(a.startupDone)
	select {
	case config := <-result:
		if config == nil || config.Model == "" {
			t.Fatal("configuration was not published")
		}
	case <-time.After(time.Second):
		t.Fatal("GetConfig did not unblock")
	}
}

func TestStartupFailureReachesFrontend(t *testing.T) {
	want := errors.New("configuration directory is not writable")
	a := &App{startupDone: make(chan struct{}), startupErr: want}
	close(a.startupDone)
	if err := a.WaitForStartup(); !errors.Is(err, want) {
		t.Fatalf("startup error = %v", err)
	}
	if state := a.GetState(); state.Error != want.Error() {
		t.Fatalf("state error = %q", state.Error)
	}
	if a.GetConfig() != nil || a.GetModels() != nil || a.IsOnboardingCompleted() {
		t.Fatal("failed startup exposed ready data")
	}
}
