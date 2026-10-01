package main

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestTea(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "1")
	got := renderTea()
	if ansi.Strip(got) != got {
		t.Fatal("forced color")
	}
	for _, s := range []string{"Go dress cmdlib in Lip Gloss.", "Serve Bubble Tea with Bubbles.", "ready.", "╭", "╰"} {
		if !strings.Contains(got, s) {
			t.Fatalf("missing %q", s)
		}
	}
	lines := strings.Split(got, "\n")
	w := ansi.StringWidth(lines[0])
	for _, line := range lines {
		if ansi.StringWidth(line) != w {
			t.Fatal("unbalanced frame")
		}
	}
}
