//go:build darwin || linux

package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIdleOnlyWhenAllSessionsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.json")
	l1, err := AcquireUsageLock(path)
	if err != nil {
		t.Fatal(err)
	}
	l2, err := AcquireUsageLock(path)
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	inspect := func(context.Context) Report {
		called++
		return Report{Valid: true, Version: "test"}
	}
	var out bytes.Buffer
	if err := runIdle(context.Background(), path, &out, inspect); err != nil || called != 0 {
		t.Fatal("background diagnosis ran while two sessions were open")
	}
	if _, err := os.Stat(path + ".maintenance.json"); !os.IsNotExist(err) {
		t.Fatal("background wrote a report while in use")
	}
	_ = l1.Close()
	if err := runIdle(context.Background(), path, &out, inspect); err != nil || called != 0 {
		t.Fatal("background diagnosis ran with one session open")
	}
	_ = l2.Close()
	if err := runIdle(context.Background(), path, &out, inspect); err != nil || called != 1 {
		t.Fatalf("idle diagnosis should run once after all sessions close: %v", err)
	}
	fi, err := os.Stat(path + ".maintenance.json")
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("report was not saved privately")
	}
	b, _ := os.ReadFile(path + ".maintenance.json")
	var report Report
	if err := json.Unmarshal(b, &report); err != nil || report.Version != "test" {
		t.Fatal("saved report is invalid")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("background created/changed the command library")
	}
}

func TestForegroundOpeningCancelsBackgroundPromptly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.json")
	started, finished := make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- runIdle(context.Background(), path, &bytes.Buffer{}, func(ctx context.Context) Report {
			close(started)
			<-ctx.Done()
			return Report{Version: "cancelled"}
		})
	}()
	<-started
	begin := time.Now()
	lock, err := AcquireUsageLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if time.Since(begin) > time.Second {
		t.Fatal("foreground did not promptly preempt background")
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".maintenance.json"); !os.IsNotExist(err) {
		t.Fatal("cancelled background wrote a final report")
	}
}
