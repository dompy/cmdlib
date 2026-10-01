package navi

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dompy/cmdlib/internal/library"
)

func TestExport(t *testing.T) {
	cs := library.Seeds()
	cs = append(cs, library.Command{Command: "echo <file>"}, library.Command{Command: "echo one\necho two"})
	var out bytes.Buffer
	n, err := Export(&out, cs)
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if strings.Count(out.String(), "\n# ") != 5 || !strings.Contains(out.String(), cs[0].Command) {
		t.Fatal(out.String())
	}
}
