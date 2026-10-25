package transcribe

import (
	"sort"
	"testing"
)

func TestModelConstantsMatchCatalog(t *testing.T) {
	constants := []Model{
		ModelTiny, ModelTinyEn, ModelBase, ModelBaseEn, ModelSmall,
		ModelSmallEn, ModelMedium, ModelMediumEn, ModelLargeV3,
	}
	known := KnownModels()
	got := make([]string, len(known))
	for i, m := range known {
		got[i] = string(m)
	}
	want := make([]string, len(constants))
	for i, m := range constants {
		want[i] = string(m)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("catalog models %v, constants %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("catalog models %v, constants %v", got, want)
		}
	}
}
