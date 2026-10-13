package tui

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// Preferences are separate from the command array; old libraries stay valid.
func (m *Model) loadViewState() {
	if m.path == "" {
		return
	}
	f, err := os.Open(m.path + ".view-state.json")
	if err != nil {
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return
	}
	var state map[string]bool
	if err = json.NewDecoder(io.LimitReader(f, 65536)).Decode(&state); err != nil {
		return
	}
	for _, c := range m.all {
		if c.Category != "" && state[c.Category] {
			m.expanded[c.Category] = true
		}
	}
}

func (m *Model) saveViewState() {
	if m.path == "" {
		return
	}
	state := map[string]bool{}
	for _, c := range m.all {
		if c.Category != "" && m.expanded[c.Category] {
			state[c.Category] = true
		}
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil || len(raw) > 65536 {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(m.path), ".cmdlib-view-*.json")
	if err != nil {
		m.status = "Category opened; could not remember view preference: " + err.Error()
		return
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(raw, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), m.path+".view-state.json")
	}
	if err != nil {
		m.status = "Category opened; could not remember view preference: " + err.Error()
	}
}
