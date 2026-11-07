//go:build darwin && cgo

package overlay

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Compile the actual Cocoa implementation into a standalone harness so the
// regression exercises the rendered image, including C indexing and updates.
func TestNativeWaveformSingleImpulse(t *testing.T) {
	source, err := os.ReadFile("overlay_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	_, native, ok := strings.Cut(string(source), "/*")
	if !ok {
		t.Fatal("missing native implementation")
	}
	native, _, _ = strings.Cut(native, "*/")
	var lines []string
	for _, line := range strings.Split(native, "\n") {
		if !strings.HasPrefix(line, "#cgo") {
			lines = append(lines, line)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "overlay_native.h"), []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "waveform-test")
	cmd := exec.Command("clang", "-x", "objective-c", "-fblocks", "-fsanitize=address,undefined", "-g", "-I", dir,
		"testdata/waveform_darwin.m", "-framework", "Cocoa", "-framework", "QuartzCore", "-o", binary)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile native harness: %v\n%s", err, output)
	}
	cmd = exec.Command(binary)
	cmd.Env = append(os.Environ(), "ASAN_OPTIONS=detect_leaks=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native waveform regression: %v\n%s", err, output)
	} else {
		t.Logf("%s", output)
	}
}
