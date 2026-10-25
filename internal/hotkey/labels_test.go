package hotkey

import (
	"encoding/json"
	"os"
	"testing"
)

// testdata/short_labels.json is shared with frontend/src/hotkeyLabels.test.ts
// so the native and in-app overlays show the same labels.
func TestShortLabelFor(t *testing.T) {
	data, err := os.ReadFile("testdata/short_labels.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []map[string]string
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no label cases")
	}
	for _, c := range cases {
		for _, goos := range []string{"darwin", "windows", "linux"} {
			if got := ShortLabelFor(c["key"], goos); got != c[goos] {
				t.Errorf("ShortLabelFor(%q, %q) = %q, want %q", c["key"], goos, got, c[goos])
			}
		}
	}
}
