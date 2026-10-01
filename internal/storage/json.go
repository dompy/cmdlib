package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
