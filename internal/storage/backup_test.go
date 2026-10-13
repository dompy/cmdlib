package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateExactBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.json")
	raw := []byte("[\n]\n")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s := JSON{Path: path}
	backup, err := s.Backup(context.Background(), "delete")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(backup)
	if err != nil || string(got) != string(raw) {
		t.Fatal("backup did not retain original bytes", err)
	}
	info, _ := os.Stat(backup)
	if info.Mode().Perm() != 0600 {
		t.Fatal("backup is not private")
	}
	original, _ := os.ReadFile(path)
	if string(original) != string(raw) {
		t.Fatal("backup changed the library")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Backup(ctx, "cancelled"); err == nil {
		t.Fatal("cancelled backup succeeded")
	}
}
