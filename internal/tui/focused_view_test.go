package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dompy/cmdlib/internal/library"
)

func focusedFixture() (Model, *memory) {
	cs := library.Seeds()
	cs[0].Description = "Synthetic description"
	cs[0].Explanation = "Synthetic explanation"
	s := &memory{cs: append([]library.Command(nil), cs...)}
	m := New(s, cs, "")
	m.host = "example-machine"
	return m, s
}

func TestFocusLayersDoNotExecute(t *testing.T) {
	m, s := focusedFixture()
	v := m.View()
	if strings.Count(v, m.all[0].Name) != 1 || strings.Count(v, m.all[0].Command) != 1 {
		t.Fatal("selected name or command repeated")
	}
	for _, hidden := range []string{"Requires:", "Host:", "example-machine", "[Copy]", "[Run]", "by ai-mate.ai"} {
		if strings.Contains(v, hidden) {
			t.Fatalf("default view includes %q", hidden)
		}
	}
	if strings.Count(v, "Synthetic description") != 1 || strings.Index(v, m.all[0].Command) > strings.Index(v, "Synthetic description") {
		t.Fatal("command and description are not shown once in order")
	}
	content := m.fieldContent(m.all[0])
	for _, label := range []string{"Command", "Description"} {
		if strings.Contains(content, label) {
			t.Fatalf("main view includes field heading %q", label)
		}
	}
	m = key(m, tea.KeyTab)
	m = key(m, tea.KeyDown)
	if !m.fieldFocus || m.detailField != 1 || !strings.Contains(m.View(), "Synthetic description") {
		t.Fatal("description field unavailable")
	}
	m = key(m, tea.KeyRight)
	for _, field := range layerFields[1] {
		m.detailField = field
		m.layout()
		if !strings.Contains(m.View(), labels[field]) {
			t.Fatalf("field %d unavailable", field)
		}
	}
	if s.saves != 0 {
		t.Fatal("navigation changed library")
	}
}

func TestInlineEditSaveCancelAndConcurrentChanges(t *testing.T) {
	for _, field := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8} {
		t.Run(labels[field], func(t *testing.T) {
			m, s := focusedFixture()
			original := m.all[0]
			m.detailField = field
			if field == 1 {
				m.layer = 0
			} else if field != 2 {
				m.layer = 1
			}
			next, _ := m.Update(singleKey("e"))
			m = next.(Model)
			if m.mode != "edit" || m.field != field {
				t.Fatal("wrong field editor")
			}
			value := "Synthetic replacement"
			if field == 2 {
				value = "printf 'SYNTHETIC_ONLY\\n'"
			}
			if field == 5 {
				value = "WRITE"
			}
			m.fields[field].SetValue(value)
			m.layout()
			cancelled := key(m, tea.KeyEsc)
			if s.saves != 0 || fieldValue(cancelled.all[0], field) != fieldValue(original, field) {
				t.Fatal("cancel persisted draft")
			}
			m = key(m, tea.KeyEnter)
			if m.mode != "" || s.saves != 1 {
				t.Fatal("field not saved")
			}
			got := s.cs[0]
			if fieldValue(got, field) != value {
				t.Fatal("wrong value saved")
			}
			for other := 0; other < 9; other++ {
				if other == field || field == 2 && other == 7 {
					continue
				}
				if fieldValue(got, other) != fieldValue(original, other) {
					t.Fatalf("changed unrelated field %d", other)
				}
			}
		})
	}
	m, s := focusedFixture()
	m.target = m.all[0]
	m.beginEdit()
	m.fields[2].SetValue("printf changed")
	s.cs[1].Description = "Concurrent edit"
	m = key(m, tea.KeyEnter)
	if s.saves != 0 || !strings.Contains(m.status, "another window") || m.all[1].Description != "Concurrent edit" {
		t.Fatal("concurrent edit overwritten")
	}
}

func TestInlinePasteAndCreation(t *testing.T) {
	m, s := focusedFixture()
	m.target = m.all[0]
	m.beginEdit()
	m.fields[2].SetValue("")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("printf pasted"), Paste: true})
	m = next.(Model)
	if m.fields[2].Value() != "printf pasted" || s.saves != 0 || m.mode != "edit" {
		t.Fatal("paste executed or was intercepted")
	}
	m = key(m, tea.KeyEsc)
	next, _ = m.Update(singleKey("n"))
	m = next.(Model)
	m.fields[0].SetValue("New synthetic command")
	m = key(m, tea.KeyTab)
	m.fields[1].SetValue("Synthetic new description")
	m = key(m, tea.KeyTab)
	m.fields[2].SetValue("printf new")
	m = key(m, tea.KeyEnter)
	if m.mode != "" || s.saves != 1 || len(s.cs) != len(library.Seeds())+1 {
		t.Fatal("new command not saved")
	}
}

func TestFocusedPreview(t *testing.T) {
	if directory := os.Getenv("CMDLIB_FOCUSED_PREVIEW"); directory != "" {
		m, _ := focusedFixture()
		for _, mode := range []string{"command", "description", "details", "edit", "confirm"} {
			switch mode {
			case "description":
				m.fieldFocus = true
				m.layer = 0
				m.detailField = 1
			case "details":
				m.layer = 1
				m.detailField = 4
			case "edit":
				m.layer = 0
				m.detailField = 1
				m.target = m.all[0]
				m.beginEdit()
			case "confirm":
				m.mode = ""
				m.target = m.all[0]
				next, _ := m.beginConfirmation()
				m = next.(Model)
			}
			m.layout()
			if err := os.WriteFile(filepath.Join(directory, mode+".txt"), []byte(m.View()), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestFocusedControlsRemainVisible(t *testing.T) {
	for _, size := range [][2]int{{36, 14}, {60, 24}, {90, 30}, {140, 40}} {
		m, _ := focusedFixture()
		m.width, m.height = size[0], size[1]
		m.search.SetValue("directory")
		m.search.Focus()
		m.refresh()
		if !strings.Contains(strings.Join(strings.Fields(m.View()), " "), "leave search") {
			t.Fatalf("search controls clipped at %v: %q; viewport=%d", size, m.View(), m.detail.Height)
		}
		m.search.Blur()
		m.search.SetValue("")
		m.refresh()
		m.target = m.all[0]
		m.target.Risk = "WRITE"
		next, _ := m.beginConfirmation()
		m = next.(Model)
		if !strings.Contains(strings.Join(strings.Fields(m.View()), " "), "Esc cancel") || !strings.Contains(strings.Join(strings.Fields(m.View()), " "), "Execution host: example-machine") {
			t.Fatalf("confirmation review or controls clipped at %v", size)
		}
		m.mode = ""
		m.fieldFocus = true
		m.layer = 1
		m.detailField = 7
		m.layout()
		if !strings.Contains(m.View(), "Explanation") || lipgloss.Height(m.View()) > size[1] {
			t.Fatalf("selected field clipped at %v", size)
		}
	}
}

func TestInlineMultilineContentIsPreserved(t *testing.T) {
	m, s := focusedFixture()
	m.all[0].Command = "printf first\nprintf second"
	m.all[0].Explanation = "First line\nSecond line"
	s.cs = append([]library.Command(nil), m.all...)
	m.refresh()
	m.target = m.all[0]
	m.layer = 0
	m.detailField = 1
	m.beginEdit()
	m.fields[1].SetValue("Updated description")
	m = key(m, tea.KeyEnter)
	if s.cs[0].Command != "printf first\nprintf second" || s.cs[0].Explanation != "First line\nSecond line" {
		t.Fatal("editing description changed multiline content")
	}
	m.layer = 0
	m.detailField = 2
	m.target = m.all[0]
	m.beginEdit()
	m.fields[2].SetValue("")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("printf alpha\nprintf beta"), Paste: true})
	m = next.(Model)
	m = key(m, tea.KeyEnter)
	if s.cs[0].Command != "printf alpha\nprintf beta" {
		t.Fatal("multiline paste changed command")
	}
}
