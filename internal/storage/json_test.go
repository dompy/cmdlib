package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	s := JSON{filepath.Join(t.TempDir(), "nested", "commands.json")}
	ctx := context.Background()
	cs, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 5 {
		t.Fatal(len(cs))
	}
	cs[0].Name = "Edited"
	cs[0].UseCount = 2
	now := cs[0].CreatedAt
	cs[0].LastUsedAt = &now
	if err = s.Save(ctx, cs); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "Edited" || got[0].UseCount != 2 || !got[0].LastUsedAt.Equal(now) {
		t.Fatal("lost fields")
	}
	info, _ := os.Stat(s.Path)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}
func TestInvalidUntouched(t *testing.T) {
	s := JSON{filepath.Join(t.TempDir(), "commands.json")}
	raw := []byte("{broken")
	if err := os.WriteFile(s.Path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	got, _ := os.ReadFile(s.Path)
	if string(got) != string(raw) {
		t.Fatal("file changed")
	}
}
func TestValidationEmpty(t *testing.T) {
	s := JSON{filepath.Join(t.TempDir(), "commands.json")}
	ctx := context.Background()
	cs, _ := s.Load(ctx)
	cs[0].Risk = "UNKNOWN"
	if err := s.Save(ctx, cs); err == nil {
		t.Fatal("bad risk accepted")
	}
	cs[0].Risk = "READ"
	cs[1].ID = cs[0].ID
	if err := s.Save(ctx, cs); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := s.Save(ctx, cs[:0]); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(ctx)
	if err != nil || len(got) != 0 {
		t.Fatal("empty reseeded")
	}
}

func TestExistingLibraryUntouched(t *testing.T) {
	for _, raw := range []string{"[]\n", "null\n", `[{
 "id":"example-existing", "name":"Existing entry", "command":"printf 'fixture'",
 "description":"Synthetic upgrade fixture", "explanation":"Prints a word",
 "tags":["fixture"], "host":"example-host", "risk":"READ", "prerequisite":"",
 "created_at":"2025-01-01T00:00:00Z", "updated_at":"2025-02-01T00:00:00Z",
 "last_used_at":"2025-03-01T00:00:00Z", "use_count":7
}]`} {
		s := JSON{filepath.Join(t.TempDir(), "commands.json")}
		if err := os.WriteFile(s.Path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(s.Path)
		if err != nil || !bytes.Equal(got, []byte(raw)) {
			t.Fatal("existing data changed")
		}
	}
}
func TestSeedNeverReplacesExisting(t *testing.T) {
	s := JSON{filepath.Join(t.TempDir(), "commands.json")}
	if err := os.WriteFile(s.Path, []byte("[]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.write(context.Background(), nil, false); !os.IsExist(err) {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(s.Path)
	if string(got) != "[]\n" {
		t.Fatal("seed replaced data")
	}
}
func TestConcurrentFirstLoad(t *testing.T) {
	s := JSON{filepath.Join(t.TempDir(), "commands.json")}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs, err := s.Load(context.Background())
			if err != nil || len(cs) != 5 {
				t.Errorf("load: %d %v", len(cs), err)
			}
		}()
	}
	wg.Wait()
}
func TestPathOverride(t *testing.T) {
	p := filepath.Join(t.TempDir(), "isolated.json")
	t.Setenv("CMDLIB_FILE", p)
	got, err := DefaultPath()
	if err != nil || got != p {
		t.Fatal("override ignored")
	}
}
