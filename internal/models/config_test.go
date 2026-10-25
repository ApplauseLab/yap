package models

import (
	"reflect"
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
