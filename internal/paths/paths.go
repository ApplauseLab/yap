// Package paths resolves Yap's on-disk locations.
package paths

import (
	"os"
	"path/filepath"
)

// ConfigDirEnv overrides the Yap data directory. It lets development builds
// run beside an installed Yap without sharing configuration, models, or logs.
const ConfigDirEnv = "YAP_CONFIG_DIR"

// AppDataDir returns the directory holding Yap's config, models, and logs.
func AppDataDir() (string, error) {
	if dir := os.Getenv(ConfigDirEnv); dir != "" {
		return filepath.Abs(dir)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "yap"), nil
}
