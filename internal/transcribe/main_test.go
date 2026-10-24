package transcribe

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// The worker watchdog re-executes the current binary, which is the test
	// binary here.
	RunWorkerWatchdogIfRequested()
	os.Exit(m.Run())
}
