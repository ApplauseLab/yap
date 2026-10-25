package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Opt-in because this opens Explorer; CI uses a disposable desktop.
func TestNativeRevealFile(t *testing.T) {
	if os.Getenv("YAP_TEST_NATIVE_REVEAL") != "1" {
		t.Skip("set YAP_TEST_NATIVE_REVEAL=1 to verify Explorer selection")
	}
	dir := filepath.Join(t.TempDir(), "Yap recordings тест")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "recording with spaces.wav")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RevealFile(path); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `
$shell = New-Object -ComObject Shell.Application
$deadline = (Get-Date).AddSeconds(20)
while ((Get-Date) -lt $deadline) {
  foreach ($window in $shell.Windows()) {
    try {
      foreach ($item in $window.Document.SelectedItems()) {
        if ($item.Path -eq $env:YAP_REVEAL_TEST_PATH) {
          Write-Output "Explorer selected the requested recording"
          $window.Quit()
          exit 0
        }
      }
    } catch {}
  }
  Start-Sleep -Milliseconds 200
}
throw "Explorer did not select the requested recording"
`)
	cmd.Env = append(os.Environ(), "YAP_REVEAL_TEST_PATH="+path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	t.Log(string(output))
}
