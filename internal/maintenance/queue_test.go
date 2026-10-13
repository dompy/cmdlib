package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQueuedAIConsumesAttemptBeforeAnalysisAndNeverRetries(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "uncertain-failure"}[fails], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "commands.json")
			if err := os.WriteFile(path, []byte(`[{"id":"one","name":"Example","command":"printf 'NEVER-SEND-THIS'","risk":"READ","host":"local"}]`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := writePrivate(path+".assistant-request.json", AIRequest{RequestedAt: time.Now(), Status: "pending"}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			fake := func(ctx context.Context, prompt string) (string, error) {
				calls++
				b, _ := os.ReadFile(path + ".assistant-request.json")
				var request AIRequest
				_ = json.Unmarshal(b, &request)
				if request.Status != "attempted" || request.AttemptedAt == nil {
					t.Fatal("attempt not recorded before analysis")
				}
				if strings.Contains(prompt, "NEVER-SEND-THIS") {
					t.Fatal("queued AI leaked raw command")
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 90*time.Second {
					t.Fatal("analysis not time bounded")
				}
				if fails {
					return "", errors.New("uncertain test failure")
				}
				return "One short test suggestion.", nil
			}
			for i := 0; i < 2; i++ {
				if err := runQueued(context.Background(), path, Report{Valid: true}, &bytes.Buffer{}, fake); err != nil {
					t.Fatal(err)
				}
			}
			if calls != 1 {
				t.Fatal("analysis was automatically retried")
			}
			if !fails {
				fi, err := os.Stat(path + ".assistant.txt")
				if err != nil || fi.Mode().Perm() != 0600 {
					t.Fatal("advice result was not private")
				}
			}
		})
	}
}
