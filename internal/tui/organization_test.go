package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dompy/cmdlib/internal/library"
)

func typedKey(m Model, value string) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)})
	return next.(Model)
}

func TestNavigationDefaultsToRun(t *testing.T) {
	cs := library.Seeds()
	s := &memory{cs: cs}
	m := New(s, cs, "")
	if m.action != 1 {
		t.Fatal("Run is not the initial action")
	}
	m = key(m, tea.KeyLeft)
	if m.action != 0 {
		t.Fatal("Copy action unavailable")
	}
	m = key(m, tea.KeyDown)
	if m.action != 1 {
		t.Fatal("new command did not reset to Run")
	}
	m = key(m, tea.KeyEnter)
	if m.mode != "run" || m.target.ID != cs[1].ID || s.saves != 0 {
		t.Fatal("Enter did not review the selected command without executing")
	}
	m = key(m, tea.KeyEsc)
	if s.saves != 0 || m.mode != "" {
		t.Fatal("Run cancellation changed data")
	}
}

func TestPasteOutsideSearchDoesNotActivateAnAction(t *testing.T) {
	cs := library.Seeds()
	s := &memory{cs: cs}
	m := New(s, cs, "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d"), Paste: true})
	m = next.(Model)
	if m.mode != "" || s.saves != 0 || !m.search.Focused() || m.search.Value() != "d" {
		t.Fatal("pasted text activated a shortcut instead of search")
	}
}

func TestCategoryExpansionAndClosedSearch(t *testing.T) {
	cs := library.Seeds()
	cs[0].Category = "FFmpeg"
	cs[1].Category = "FFmpeg"
	s := &memory{cs: cs}
	m := New(s, cs, "")
	if len(m.rows) != 4 || m.rows[0].category != "FFmpeg" {
		t.Fatal("category did not group its two children")
	}
	if _, ok := m.current(); ok {
		t.Fatal("category is an execution target")
	}
	m = key(m, tea.KeyEnter)
	if len(m.rows) != 6 || !m.expanded["FFmpeg"] || m.mode != "" || s.saves != 0 {
		t.Fatal("Enter did not expand without execution")
	}
	m = key(m, tea.KeyEnter)
	if len(m.rows) != 4 || m.expanded["FFmpeg"] {
		t.Fatal("Enter did not collapse")
	}
	m.search.SetValue("working directory")
	m.refresh()
	c, ok := m.current()
	if !ok || c.ID != cs[0].ID || len(m.rows) != 1 {
		t.Fatal("closed category hid matching child from search")
	}
	m.search.SetValue("ffmpeg")
	m.refresh()
	if len(m.results) != 2 || len(m.rows) != 2 {
		t.Fatal("category name is not searchable")
	}
}

func TestDeletionConfirmationAndUndo(t *testing.T) {
	cs := library.Seeds()
	s := &memory{cs: cs}
	m := New(s, cs, "")
	m = typedKey(m, "d")
	if m.mode != "delete" || s.saves != 0 || !strings.Contains(m.content(), cs[0].Command) {
		t.Fatal("delete did not show the exact selected command")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y"), Paste: true})
	m = next.(Model)
	if s.saves != 0 {
		t.Fatal("paste approved deletion")
	}
	m = key(m, tea.KeyEsc)
	if s.saves != 0 || m.mode != "" {
		t.Fatal("cancellation deleted")
	}
	m = typedKey(m, "d")
	m = key(m, tea.KeyEnter)
	if len(s.cs) != 4 || s.saves != 1 || commandIndex(s.cs, cs[0].ID) >= 0 {
		t.Fatal("confirmed deletion did not save")
	}
	m = typedKey(m, "u")
	if !reflect.DeepEqual(s.cs, cs) || s.saves != 2 || m.undoLabel != "" {
		t.Fatal("undo did not restore exact command and order")
	}
}

func TestMovePersistsOrderAndCategoryOnlyOnSave(t *testing.T) {
	cs := library.Seeds()
	s := &memory{cs: cs}
	m := New(s, cs, "")
	m = typedKey(m, "m")
	m = key(m, tea.KeyDown)
	if s.saves != 0 || m.moveDraft[1].ID != cs[0].ID {
		t.Fatal("move preview saved too early or did not move")
	}
	m = key(m, tea.KeyTab)
	m.moveCategory.SetValue("Tools")
	m = key(m, tea.KeyEnter)
	if s.saves != 1 || s.cs[1].ID != cs[0].ID || s.cs[1].Category != "Tools" {
		t.Fatal("move/category was not persisted")
	}
	c, ok := m.current()
	if !ok || c.ID != cs[0].ID || m.action != 1 {
		t.Fatal("moved command not selected with default Run")
	}
	m = typedKey(m, "u")
	if !reflect.DeepEqual(s.cs, cs) {
		t.Fatal("move undo did not restore original order/category")
	}
	m = typedKey(m, "m")
	m = key(m, tea.KeyDown)
	m = key(m, tea.KeyEsc)
	if s.saves != 2 || !reflect.DeepEqual(s.cs, cs) {
		t.Fatal("cancelled move saved")
	}
}

func TestOrganizationDoesNotOverwriteConcurrentChange(t *testing.T) {
	cs := library.Seeds()
	s := &memory{cs: cs}
	m := New(s, cs, "")
	m = typedKey(m, "m")
	m = key(m, tea.KeyDown)
	s.cs = append([]library.Command(nil), s.cs...)
	s.cs[1].Description = "A newer edit"
	m = key(m, tea.KeyEnter)
	if s.saves != 0 || s.cs[1].Description != "A newer edit" || m.mode != "" {
		t.Fatal("organization overwrote concurrent data")
	}
}

func TestCategoryEditorAndEmptyLibraryDeleteUndo(t *testing.T) {
	cs := library.Seeds()[:1]
	s := &memory{cs: cs}
	m := New(s, cs, "")
	m = typedKey(m, "e")
	if len(m.fields) != len(labels) || len(m.fields) != 9 {
		t.Fatal("category editor field missing")
	}
	m.fields[8].SetValue("FFmpeg")
	m = key(m, tea.KeyCtrlS)
	if s.cs[0].Category != "FFmpeg" {
		t.Fatal("category edit not saved")
	}
	// Locate the child explicitly; category headers cannot be deleted.
	m.selectCommand(cs[0].ID)
	m = typedKey(m, "d")
	m = key(m, tea.KeyEnter)
	if len(s.cs) != 0 || len(m.rows) != 0 {
		t.Fatal("last command deletion failed")
	}
	m = typedKey(m, "u")
	if len(s.cs) != 1 || s.cs[0].Category != "FFmpeg" {
		t.Fatal("undo from empty library failed")
	}
}

func TestCategoryExpansionPreferenceSurvivesRestart(t *testing.T) {
	cs := library.Seeds()
	cs[0].Category = "FFmpeg"
	path := filepath.Join(t.TempDir(), "commands.json")
	m := New(&memory{cs: cs}, cs, path)
	m = key(m, tea.KeyEnter)
	reopened := New(&memory{cs: cs}, cs, path)
	if !reopened.expanded["FFmpeg"] || len(reopened.rows) != 6 {
		t.Fatal("category expansion preference was not saved")
	}
	info, err := os.Stat(path + ".view-state.json")
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("preference file is not private")
	}
	if err = os.WriteFile(path+".view-state.json", []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	reopened = New(&memory{cs: cs}, cs, path)
	if reopened.expanded["FFmpeg"] {
		t.Fatal("invalid preference should safely default to closed")
	}
}

func TestCategoryRenamePreservesCommandsExpansionAndUndo(t *testing.T) {
	cs := library.Seeds()
	cs[0].Category, cs[1].Category = "FFmpeg", "FFmpeg"
	s := &memory{cs: append([]library.Command(nil), cs...)}
	path := filepath.Join(t.TempDir(), "commands.json")
	m := New(s, cs, path)
	m = key(m, tea.KeyEnter)
	m = typedKey(m, "e")
	if m.mode != "rename-category" || !strings.Contains(m.View(), "Kategorie umbenennen") || s.saves != 0 {
		t.Fatal("category edit did not open without changing the library")
	}
	m = typedKey(m, "Audio & Aufnahme")
	m = key(m, tea.KeyEnter)
	if s.saves != 1 || m.mode != "" || !m.expanded["Audio & Aufnahme"] || m.expanded["FFmpeg"] {
		t.Fatal("rename did not preserve the expanded category")
	}
	row, ok := m.selectedRow()
	if !ok || row.category != "Audio & Aufnahme" || m.action != 1 {
		t.Fatal("renamed category did not stay selected")
	}
	for i, saved := range s.cs {
		expected := cs[i]
		if i < 2 {
			expected.Category = "Audio & Aufnahme"
			expected.UpdatedAt = saved.UpdatedAt
		}
		if !reflect.DeepEqual(saved, expected) {
			t.Fatal("rename changed command text, order, history or unrelated metadata")
		}
	}
	reopened := New(&memory{cs: s.cs}, s.cs, path)
	if !reopened.expanded["Audio & Aufnahme"] || reopened.rows[0].category != "Audio & Aufnahme" {
		t.Fatal("renamed category did not survive restart")
	}
	m = typedKey(m, "u")
	row, ok = m.selectedRow()
	if !reflect.DeepEqual(s.cs, cs) || !m.expanded["FFmpeg"] || !ok || row.category != "FFmpeg" {
		t.Fatal("undo did not restore original data, category and expansion")
	}
}

func TestCategoryRenameRejectsEmptyCollisionAndAllowsCancel(t *testing.T) {
	cs := library.Seeds()
	cs[0].Category, cs[1].Category = "FFmpeg", "Tools"
	s := &memory{cs: cs}
	m := New(s, cs, "")
	m = typedKey(m, "e")
	m = key(m, tea.KeyEnter)
	if m.mode != "rename-category" || s.saves != 0 {
		t.Fatal("empty name changed the category")
	}
	m = typedKey(m, "Tools")
	m = key(m, tea.KeyEnter)
	if m.mode != "rename-category" || s.saves != 0 || !strings.Contains(m.status, "bereits") {
		t.Fatal("rename silently merged two categories")
	}
	m = key(m, tea.KeyEsc)
	if m.mode != "" || s.saves != 0 || !reflect.DeepEqual(s.cs, cs) {
		t.Fatal("cancelled category rename changed data")
	}
}

func TestCategoryRenameReloadsConcurrentEdits(t *testing.T) {
	cs := library.Seeds()
	cs[0].Category = "FFmpeg"
	s := &memory{cs: cs}
	m := New(s, cs, "")
	m = typedKey(m, "e")
	m = typedKey(m, "Audio")
	s.cs = append([]library.Command(nil), s.cs...)
	s.cs[1].Name = "Newer command name"
	m = key(m, tea.KeyEnter)
	if m.mode != "" || s.saves != 0 || s.cs[0].Category != "FFmpeg" || s.cs[1].Name != "Newer command name" {
		t.Fatal("rename overwrote a concurrent edit")
	}
}
