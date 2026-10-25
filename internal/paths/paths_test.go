package paths

import (
	"path/filepath"
	"testing"
)

func TestAppDataDirHonoursOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ConfigDirEnv, dir)
	got, err := AppDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("AppDataDir() = %q, want %q", got, dir)
	}
}

func TestAppDataDirDefaultsToUserConfig(t *testing.T) {
	t.Setenv(ConfigDirEnv, "")
	got, err := AppDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "yap" {
		t.Fatalf("AppDataDir() = %q, want a yap directory", got)
	}
}
