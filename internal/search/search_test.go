package search

import (
	"testing"

	"github.com/dompy/cmdlib/internal/library"
)

func TestFilter(t *testing.T) {
	cs := library.Seeds()
	for _, tc := range []struct {
		q     string
		n     int
		first string
	}{{"", 5, "working-directory"}, {"UTC time", 1, "utc-time"}, {"gt sts", 1, "git-status"}, {"zzzzzz", 0, ""}, {"files READ", 3, "list-files"}} {
		got := Filter(cs, tc.q)
		if len(got) != tc.n {
			t.Errorf("%q: got %d want %d", tc.q, len(got), tc.n)
		}
		if len(got) > 0 && got[0].ID != tc.first {
			t.Errorf("%q first %s", tc.q, got[0].ID)
		}
	}
}
func TestFuzzyUnicode(t *testing.T) {
	cs := []library.Command{{Name: "Café devices"}, {Name: "other"}}
	if got := Filter(cs, "cfé dvc"); len(got) != 1 || got[0].Name != "Café devices" {
		t.Fatal(got)
	}
}
