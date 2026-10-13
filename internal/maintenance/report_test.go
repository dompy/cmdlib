package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dompy/cmdlib/internal/library"
)

func absentTool(string) (string, error) { return "", errors.New("not installed") }

func TestInspectReadOnlyAndDuplicateMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.json")
	cs := []library.Command{
		{ID: "first", Name: "Example", Category: "Tools", Command: "printf 'DO-NOT-SEND-RAW-COMMAND'", Explanation: "DO-NOT-SEND-EXPLANATION", Prerequisite: "DO-NOT-SEND-PREREQUISITE", Host: "local", Risk: "READ"},
		{ID: "second", Name: "Example", Category: "Tools", Command: "  printf 'DO-NOT-SEND-RAW-COMMAND'  ", Host: "local", Risk: "READ"},
		{ID: "remote", Name: "Different", Command: "printf 'DO-NOT-SEND-RAW-COMMAND'", Host: "remote", Risk: "READ"},
	}
	before, _ := json.Marshal(cs)
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	r, loaded := Inspect(context.Background(), path, "custom-test", absentTool, nil)
	if !r.Valid || r.Count != 3 || len(r.Duplicates) != 2 || len(r.Duplicates[0].Entries) != 2 {
		t.Fatalf("unexpected duplicate report: valid=%v count=%d duplicateGroups=%d", r.Valid, r.Count, len(r.Duplicates))
	}
	prompt, err := AdvisorPrompt(r, loaded)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"DO-NOT-SEND-RAW-COMMAND", "DO-NOT-SEND-EXPLANATION", "DO-NOT-SEND-PREREQUISITE", "last_used_at", "use_count"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("metadata leaked prohibited field %s", forbidden)
		}
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("read-only diagnosis changed the library")
	}
}

func TestMissingLibraryNotSeededAndMalformedReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	r, _ := Inspect(context.Background(), path, "test", absentTool, nil)
	if r.Valid {
		t.Fatal("missing library marked valid")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("diagnosis created a library")
	}
	if err := os.WriteFile(path, []byte(`[{"id":"one","name":"Safe","command":"true","risk":"BAD"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	r, _ = Inspect(context.Background(), path, "test", absentTool, nil)
	if r.Valid || len(r.Issues) == 0 {
		t.Fatal("invalid risk not reported")
	}
}

func TestAdvisorRequiresBoundedCLIAndNeverCallsModel(t *testing.T) {
	called := []string{}
	runner := func(_ context.Context, executable string, args ...string) (string, error) {
		called = append(called, strings.Join(args, " "))
		switch strings.Join(args, " ") {
		case "login status":
			return "Logged in using ChatGPT", nil
		case "--help":
			return "--sandbox --disable --ask-for-approval", nil
		case "features list":
			return "shell_tool stable true\nunified_exec stable true\nhooks stable true\nplugins stable true\napps stable true\n", nil
		default:
			t.Fatal("unexpected model invocation")
			return "", nil
		}
	}
	cmd, err := AdvisorCommand(context.Background(), "/fake/codex", t.TempDir(), "Only metadata", runner)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cmd.Args, " ")
	for _, required := range []string{"--sandbox read-only", "--ask-for-approval never", "--disable shell_tool", "--disable plugins", "mcp_servers={}", "Only metadata"} {
		if !strings.Contains(args, required) {
			t.Fatalf("missing advisor constraint %s", required)
		}
	}
	if len(called) != 3 {
		t.Fatalf("expected auth method + two CLI capability reads, got %d", len(called))
	}
	_, err = AdvisorCommand(context.Background(), "/fake/codex", t.TempDir(), "metadata", func(context.Context, string, ...string) (string, error) { return "old CLI", nil })
	if err == nil {
		t.Fatal("unsupported CLI was allowed")
	}
}

func TestMetadataRedactsTokenPatterns(t *testing.T) {
	e := metadata(library.Command{ID: "safe", Name: "example sk-123456abcdef", Category: "Tools", Host: "local", Risk: "READ"})
	if strings.Contains(e.Name, "sk-123456") || !strings.Contains(e.Name, "[redacted]") {
		t.Fatal("token-like metadata was not redacted")
	}
}
