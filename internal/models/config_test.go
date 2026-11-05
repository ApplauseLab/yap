package models

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"yap/internal/paths"
)

func TestModelFavoritesPersist(t *testing.T) {
	t.Setenv(paths.ConfigDirEnv, t.TempDir())
	cm, err := NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	if cm.IsFavoriteModel("small") {
		t.Fatal("no favorites by default")
	}
	for _, step := range []struct {
		model    string
		favorite bool
	}{{"small", true}, {"tiny.en", true}, {"small", true}, {"tiny.en", false}} {
		if err := cm.SetModelFavorite(step.model, step.favorite); err != nil {
			t.Fatal(err)
		}
	}

	reloaded, err := NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get().FavoriteModels; !reflect.DeepEqual(got, []string{"small"}) {
		t.Fatalf("FavoriteModels = %v, want [small]", got)
	}
	if !reloaded.IsFavoriteModel("small") || reloaded.IsFavoriteModel("tiny.en") {
		t.Fatal("favorites did not round-trip")
	}
}

func TestConcurrentConfigWritesKeepEveryChange(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(paths.ConfigDirEnv, dir)
	cm, err := NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"tiny", "tiny.en", "base", "base.en", "small", "small.en", "medium", "medium.en", "large-v3"}
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(2)
		go func(name string) {
			defer wg.Done()
			if err := cm.SetModelFavorite(name, true); err != nil {
				t.Error(err)
			}
		}(name)
		go func(name string) {
			defer wg.Done()
			_ = cm.IsFavoriteModel(name)
			_ = cm.Get().FavoriteModels
			if err := cm.SetAutoPaste(name != "tiny"); err != nil {
				t.Error(err)
			}
		}(name)
	}
	wg.Wait()

	reloaded, err := NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	got := append([]string(nil), reloaded.Get().FavoriteModels...)
	sort.Strings(got)
	want := append([]string(nil), names...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("favorites after concurrent saves = %v, want %v", got, want)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("temporary config files left behind: %v", matches)
	}
}

func TestGetReturnsSnapshot(t *testing.T) {
	t.Setenv(paths.ConfigDirEnv, t.TempDir())
	cm, err := NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	if err := cm.SetModelFavorite("small", true); err != nil {
		t.Fatal(err)
	}
	snapshot := cm.Get()
	snapshot.Model = "changed"
	snapshot.FavoriteModels[0] = "changed"
	if cm.Get().Model == "changed" || !cm.IsFavoriteModel("small") {
		t.Fatal("mutating a snapshot must not change the stored config")
	}
}

func TestTranscriptionOutputSettingsPersistAndDefaultOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(paths.ConfigDirEnv, dir)
	// Existing configurations have neither of the new fields.
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"provider":"local","model":"base.en","autoPaste":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	cm, err := NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	if cm.Get().SaveTranscriptions || cm.Get().TranscriptionOutputDir != "" {
		t.Fatal("automatic text files must default to off")
	}
	output := t.TempDir()
	var wg sync.WaitGroup
	for _, update := range []func() error{
		func() error { return cm.SetTranscriptionOutputDir(output) },
		func() error { return cm.SetSaveTranscriptions(true) },
		func() error { return cm.SetAutoPaste(false) },
	} {
		wg.Add(1)
		go func(update func() error) {
			defer wg.Done()
			if err := update(); err != nil {
				t.Error(err)
			}
		}(update)
	}
	wg.Wait()
	reloaded, err := NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	config := reloaded.Get()
	if !config.SaveTranscriptions || config.TranscriptionOutputDir != output || config.AutoPaste || config.Model != "base.en" {
		t.Fatalf("concurrent changes were not persisted: %+v", config)
	}
}

func TestFailedSettingsWriteRollsBackEffectiveConfig(t *testing.T) {
	t.Setenv(paths.ConfigDirEnv, t.TempDir())
	cm, err := NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	before := cm.Get()
	// Force the final config rename to fail, leaving the real config intact.
	cm.configFile = t.TempDir()
	for _, update := range []func() error{
		func() error { return cm.SetSaveTranscriptions(true) },
		func() error { return cm.SetTranscriptionOutputDir("/unpersisted") },
		func() error { return cm.SetModelFavorite("small", true) },
	} {
		if err := update(); err == nil {
			t.Fatal("expected persistence failure")
		}
		if !reflect.DeepEqual(cm.Get(), before) {
			t.Fatal("a failed write changed the effective configuration")
		}
	}
	reloaded, err := NewConfigManager()
	if err != nil || !reflect.DeepEqual(reloaded.Get(), before) {
		t.Fatalf("persisted configuration changed: %v", err)
	}
}
