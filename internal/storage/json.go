package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dompy/cmdlib/internal/library"
)

// Store is the boundary for a future HTTP-backed library.
type Store interface {
	Load(context.Context) ([]library.Command, error)
	Save(context.Context, []library.Command) error
}

type JSON struct{ Path string }

func DefaultPath() (string, error) {
	if p := os.Getenv("CMDLIB_FILE"); p != "" {
		return p, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "cmdlib", "commands.json"), nil
}

func (s JSON) Load(ctx context.Context) ([]library.Command, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		cs := library.Seeds()
		if err := s.write(ctx, cs, false); os.IsExist(err) {
			return s.Load(ctx)
		} else {
			return cs, err
		}
	}
	if err != nil {
		return nil, fmt.Errorf("read library %s: %w", s.Path, err)
	}
	var cs []library.Command
	if err = json.Unmarshal(b, &cs); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s (file left untouched): %w", s.Path, err)
	}
	if err = validate(cs); err != nil {
		return nil, fmt.Errorf("invalid library %s: %w", s.Path, err)
	}
	return cs, nil
}

func validate(cs []library.Command) error {
	seen := map[string]bool{}
	for _, c := range cs {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("%q: %w", c.Name, err)
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate id %q", c.ID)
		}
		seen[c.ID] = true
	}
	return nil
}

func (s JSON) Save(ctx context.Context, cs []library.Command) error {
	return s.write(ctx, cs, true)
}

// Backup preserves the exact private library before an organization change.
// It never exports the data or rewrites the source library.
func (s JSON) Backup(ctx context.Context, reason string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	b, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	reason = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, strings.ToLower(reason))
	if len(reason) > 40 {
		reason = reason[:40]
	}
	dir := filepath.Join(filepath.Dir(s.Path), "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, filepath.Base(s.Path)+"."+time.Now().UTC().Format("20060102T150405.000000000Z")+"."+reason+".bak")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := f.Write(b)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(path)
		if writeErr != nil {
			return "", writeErr
		}
		return "", closeErr
	}
	return path, nil
}

func (s JSON) write(ctx context.Context, cs []library.Command, replace bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validate(cs); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	dir := filepath.Dir(s.Path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create library directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".cmdlib-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if !replace {
		// Link the complete seed file only if the destination is still absent.
		// A simultaneous first launch must never replace another library.
		return os.Link(f.Name(), s.Path)
	}
	if err = os.Rename(f.Name(), s.Path); err != nil {
		return fmt.Errorf("replace library: %w", err)
	}
	return nil
}
