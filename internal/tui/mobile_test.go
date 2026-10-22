package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dompy/cmdlib/internal/library"
	"strings"
	"testing"
)

func mobileFixture(t *testing.T) (Model, *memory) {
	t.Helper()
	t.Setenv("CMDLIB_MOBILE", "1")
	cs := library.Seeds()
	cs[0].Category = "Examples"
	s := &memory{cs: cs}
	m := New(s, cs, "")
	m.width = 36
	m.height = 22
	m.layout()
	return m, s
}

func TestMobileSelectionReviewCancelAndRun(t *testing.T) {
	m, s := mobileFixture(t)
	if len(m.rows) != len(m.all) {
		t.Fatal("mobile hid category commands")
	}
	next, cmd := m.Update(singleKey("1"))
	m = next.(Model)
	if m.mode != "run" || s.saves != 0 {
		t.Fatal("selection ran instead of reviewing")
	}
	_ = cmd
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1"), Paste: true})
	m = next.(Model)
	if s.saves != 0 || cmd != nil {
		t.Fatal("paste confirmed execution")
	}
	m = key(m, tea.KeyEsc)
	if m.mode != "" || s.saves != 0 {
		t.Fatal("cancel failed")
	}
	next, _ = m.Update(singleKey("1"))
	m = next.(Model)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.mode != "executing" || cmd == nil || s.saves != 1 {
		t.Fatal("explicit run not scheduled")
	}
}

func TestMobileCreationWithoutTabOrMetadataTyping(t *testing.T) {
	m, s := mobileFixture(t)
	original := len(s.cs)
	next, _ := m.Update(singleKey("n"))
	m = next.(Model)
	m.fields[0].SetValue("Synthetic entry")
	m = key(m, tea.KeyEnter)
	if m.field != 2 || s.saves != 0 {
		t.Fatal("name did not advance to command")
	}
	m.fields[2].SetValue("printf 'synthetic only\\n'")
	m = key(m, tea.KeyEnter)
	if m.field != 5 || s.saves != 0 {
		t.Fatal("command did not advance to type")
	}
	next, cmd := m.Update(singleKey("2"))
	m = next.(Model)
	if cmd != nil || m.mode != "" || len(s.cs) != original+1 || s.saves != 1 {
		t.Fatal("type choice did not save only")
	}
	c := s.cs[len(s.cs)-1]
	if c.Name != "Synthetic entry" || c.Risk != "CONNECT" || c.UseCount != 0 {
		t.Fatal("wrong creation values")
	}
}

func TestMobileNewCancelPreservesLibrary(t *testing.T) {
	m, s := mobileFixture(t)
	next, _ := m.Update(singleKey("n"))
	m = next.(Model)
	m.fields[0].SetValue("Discard")
	m = key(m, tea.KeyEnter)
	m.fields[2].SetValue("printf discard")
	m = key(m, tea.KeyEsc)
	if s.saves != 0 || m.mode != "" || len(m.all) != len(library.Seeds()) {
		t.Fatal("cancel changed library")
	}
}

func TestMobileStrongConfirmationStillRequired(t *testing.T) {
	for _, risk := range []string{"WRITE", "DANGER"} {
		m, s := mobileFixture(t)
		m.all[0].Risk = risk
		s.cs[0].Risk = risk
		m.refresh()
		next, _ := m.Update(singleKey("1"))
		m = next.(Model)
		for _, k := range []tea.KeyMsg{{Type: tea.KeyEnter}, singleKey("1"), {Type: tea.KeySpace}} {
			next, _ = m.Update(k)
			m = next.(Model)
		}
		if s.saves != 0 || m.mode != "run" {
			t.Fatal("mobile bypassed stronger confirmation")
		}
	}
}

func TestMobileMenuFitsAndPages(t *testing.T) {
	m, _ := mobileFixture(t)
	for i := 0; i < 12; i++ {
		c := m.all[0]
		c.ID = "synthetic-" + strings.Repeat("x", i+1)
		c.Name = "Another synthetic entry"
		m.all = append(m.all, c)
	}
	m.refresh()
	for _, size := range [][2]int{{28, 18}, {36, 22}, {80, 25}} {
		m.width = size[0]
		m.height = size[1]
		m.layout()
		v := m.View()
		for _, label := range []string{"[N]", "[E]", "[Q]"} {
			if !strings.Contains(v, label) {
				t.Fatalf("%dx%d clipped %s", size[0], size[1], label)
			}
		}
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = next.(Model)
		want := m.rows[(m.selected/m.mobilePageSize())*m.mobilePageSize()].command.ID
		next, _ = m.Update(singleKey("1"))
		m = next.(Model)
		if m.mode != "run" || m.target.ID != want {
			t.Fatal("number selected wrong page")
		}
		m = key(m, tea.KeyEsc)
	}
}
