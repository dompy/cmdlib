package navi

import (
	"fmt"
	"io"
	"strings"

	"github.com/dompy/cmdlib/internal/library"
)

// Export skips syntax that Navi could reinterpret as variables or directives.
// It never changes a command in order to make it exportable.
func Export(w io.Writer, cs []library.Command) (int, error) {
	skipped := 0
	clean := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	for _, c := range cs {
		raw := c.Command
		if strings.ContainsAny(raw, "\n\r<>") || strings.HasPrefix(strings.TrimSpace(raw), "#") || strings.HasPrefix(strings.TrimSpace(raw), "%") || strings.HasPrefix(strings.TrimSpace(raw), "$") {
			skipped++
			continue
		}
		tags := clean(strings.Join(c.Tags, ", "))
		if tags == "" {
			tags = "cmdlib"
		}
		if _, err := fmt.Fprintf(w, "%% %s\n# %s [%s; %s]\n%s\n\n", tags, clean(c.Name), c.Risk, clean(c.Host), raw); err != nil {
			return skipped, err
		}
	}
	return skipped, nil
}
