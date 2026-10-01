package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dompy/cmdlib/internal/library"
)

type memory struct {
	cs    []library.Command
	saves int
}

func (s *memory) Load(context.Context) ([]library.Command, error) { return s.cs, nil }
func (s *memory) Save(_ context.Context, cs []library.Command) error {
	s.cs = cs
	s.saves++
	return nil
}
func key(m Model, k tea.KeyType) Model { next, _ := m.Update(tea.KeyMsg{Type: k}); return next.(Model) }
func TestRunConfirmation(t *testing.T) {
	for _, risk := range []string{"READ", "WRITE", "DANGER", "CONNECT"} {
		cs := library.Seeds()
		cs[0].Risk = risk
		s := &memory{cs: cs}
		m := New(s, cs, "")
		m.action = 1
		m = key(m, tea.KeyEnter)
		if m.mode != "run" || !m.confirm.Focused() || s.saves != 0 {
			t.Fatal("selection executed")
		}
		m = key(m, tea.KeyEnter)
		if s.saves != 0 {
			t.Fatal("empty executed")
		}
		m.confirm.SetValue("wrong")
		m = key(m, tea.KeyEnter)
		if s.saves != 0 {
			t.Fatal("wrong executed")
		}
		m.confirm.SetValue(confirmation(cs[0]))
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil || s.saves != 1 || s.cs[0].UseCount != 1 {
			t.Fatal("run not scheduled")
		}
	}
}
func TestSearchCancel(t *testing.T) {
	cs := library.Seeds()
	s := &memory{}
	m := New(s, cs, "")
	focused, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = focused.(Model)
	if !m.search.Focused() {
		t.Fatal("slash did not focus search")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("files")})
	m = next.(Model)
	if len(m.results) != 3 {
		t.Fatal("live filter")
	}
	m = key(m, tea.KeyEsc)
	if m.search.Focused() {
		t.Fatal("blur")
	}
	m.target = cs[0]
	m.beginEdit()
	m.fields[0].SetValue("changed")
	m = key(m, tea.KeyEsc)
	if s.saves != 0 || m.all[0].Name == "changed" {
		t.Fatal("cancel saved")
	}
}
func TestResponsive(t *testing.T) {
	cs := library.Seeds()
	m := New(&memory{}, cs, "")
	for _, size := range [][2]int{{36, 14}, {60, 24}, {90, 30}, {140, 40}} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		for _, mode := range []string{"", "explain", "run", "help"} {
			m.mode = mode
			m.target = cs[0]
			m.layout()
			v := m.View()
			if lipgloss.Width(v) > size[0] || lipgloss.Height(v) > size[1] {
				t.Errorf("%s %v overflow %dx%d", mode, size, lipgloss.Width(v), lipgloss.Height(v))
			}
		}
	}
	m.width = 90
	m.height = 30
	m.mode = "help"
	m.layout()
	if !strings.Contains(m.View(), "pwd") {
		t.Fatal("tutorial missing command")
	}
}

func TestCancelRunAndHost(t *testing.T) {
	cs := library.Seeds()
	cs[0].Host = "example-remote-label"
	s := &memory{cs: cs}
	m := New(s, cs, "")
	m.host = "actual-machine"
	m.action = 1
	m = key(m, tea.KeyEnter)
	v := m.content()
	for _, want := range []string{"Execution host: actual-machine", "Host: example-remote-label", cs[0].Command, "Host labels do not route execution"} {
		if !strings.Contains(v, want) {
			t.Fatalf("confirmation missing %q", want)
		}
	}
	m.confirm.SetValue("yes")
	m = key(m, tea.KeyEsc)
	if m.mode != "" || s.saves != 0 {
		t.Fatal("cancel changed library")
	}
}
func TestContextualExplain(t *testing.T) {
	cs := []library.Command{{
		ID:          "tm-status",
		Name:        "Time Machine Status",
		Command:     "tmutil status",
		Description: "Shows the current Time Machine backup status.",
		Explanation: "Fallback explanation.",
		Host:        "Mac",
		Risk:        "READ",
	}}
	m := New(&memory{}, cs, "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if m.mode != "explain" {
		t.Fatal("question mark did not open explanation")
	}
	view := m.content()
	for _, want := range []string{"what it is", "tmutil", "Time Machine utility · macOS", "status", "show current backup state", "READ ONLY"} {
		if !strings.Contains(view, want) {
			t.Fatalf("explanation missing %q", want)
		}
	}
	m = key(m, tea.KeyEsc)
	if m.mode != "" {
		t.Fatal("escape did not close explanation")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = next.(Model)
	if m.mode != "help" {
		t.Fatal("h did not open help")
	}
}

func TestExplanationFallback(t *testing.T) {
	cs := []library.Command{{
		ID:          "mystery",
		Name:        "Mystery",
		Command:     "mystery --foo",
		Description: "Runs a custom local command.",
		Explanation: "Stored explanation stays available when token meanings are unknown.",
		Host:        "local",
		Risk:        "READ",
	}}
	m := New(&memory{}, cs, "")
	m.target = cs[0]
	m.mode = "explain"
	m.layout()
	view := m.content()
	if !strings.Contains(view, "Stored explanation stays available") {
		t.Fatal("stored explanation fallback missing")
	}
	if !strings.Contains(view, "READ ONLY") {
		t.Fatal("risk label missing")
	}
}

func TestPrimaryActions(t *testing.T) {
	if got := strings.Join(actions, ","); got != "Copy,Run,Edit" {
		t.Fatalf("actions = %q", got)
	}
}

func TestPasteAndTutorial(t *testing.T) {
	cs := library.Seeds()
	m := New(&memory{}, cs, "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("git"), Paste: true})
	m = next.(Model)
	if m.search.Value() != "git" || len(m.results) != 1 {
		t.Fatal("paste intercepted")
	}
	m = key(m, tea.KeyEsc)
	m.mode = "help"
	m.step = 0
	for i, c := range cs {
		if i != 0 && strings.Contains(m.content(), c.Command) {
			t.Fatal("multiple tutorial commands")
		}
	}
}
func TestPreview(t *testing.T) {
	// Optional documentation capture uses the real view and synthetic data only.
	if path := os.Getenv("CMDLIB_PREVIEW"); path != "" {
		m := New(&memory{}, library.Seeds(), "example-library.json")
		m.host = "example-machine"
		if err := os.WriteFile(path, []byte(fmt.Sprint(m.View())), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
