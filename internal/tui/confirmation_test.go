package tui

import (
	"context"
	"encoding/json"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dompy/cmdlib/internal/library"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func quickFixture(t *testing.T, risk string, automatic bool) (Model, *memory) {
	t.Helper()
	cs := library.Seeds()
	cs[0].Risk = risk
	store := &memory{cs: cs}
	path := filepath.Join(t.TempDir(), "commands.json")
	if automatic {
		writePolicy(t, path, cs[0])
	}
	m := New(store, cs, path)
	m.width = 160
	m.height = 55
	m.layout()
	m.target = cs[0]
	next, _ := m.activate(1)
	return next.(Model), store
}
func writePolicy(t *testing.T, path string, c library.Command) {
	t.Helper()
	b, err := json.Marshal(runPolicy{Version: 1, DelayMS: 2500, Approved: []string{commandFingerprint(c)}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path+".run-policy.json", b, 0600); err != nil {
		t.Fatal(err)
	}
}
func singleKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestQuickAffirmativeKeys(t *testing.T) {
	for _, risk := range []string{"READ", "CONNECT"} {
		for _, msg := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeySpace}, singleKey("y"), singleKey("Y"), singleKey("1")} {
			t.Run(risk+"/"+msg.String(), func(t *testing.T) {
				m, s := quickFixture(t, risk, false)
				next, cmd := m.Update(msg)
				m = next.(Model)
				if cmd == nil || s.saves != 1 || s.cs[0].UseCount != 1 || m.mode != "executing" {
					t.Fatal("single key did not schedule exactly one run")
				}
				next, cmd = m.Update(confirmationTick{m.runGeneration - 1, time.Now().Add(time.Hour)})
				if cmd != nil || s.saves != 1 {
					t.Fatal("stale timer duplicated run")
				}
			})
		}
	}
}
func TestQuickCancellationKeys(t *testing.T) {
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyEsc}, {Type: tea.KeyBackspace}, {Type: tea.KeyCtrlH}, {Type: tea.KeyDelete}, singleKey("n"), singleKey("N"), singleKey("0")} {
		t.Run(msg.String(), func(t *testing.T) {
			m, s := quickFixture(t, "CONNECT", true)
			old := m.runGeneration
			deadline := m.runDeadline
			if !deadline.IsZero() {
				t.Fatal("confirmation armed a timer")
			}
			next, cmd := m.Update(msg)
			m = next.(Model)
			if m.mode != "" || cmd != nil || s.saves != 0 || !m.runDeadline.IsZero() {
				t.Fatal("cancel failed")
			}
			_, cmd = m.Update(confirmationTick{old, deadline})
			if cmd != nil || s.saves != 0 {
				t.Fatal("timer ran after cancel")
			}
		})
	}
}
func TestSavedCountdownPolicyCannotExecute(t *testing.T) {
	m, s := quickFixture(t, "READ", true)
	if !m.runDeadline.IsZero() {
		t.Fatal("saved policy armed a timer")
	}
	for _, at := range []time.Time{time.Now(), time.Now().Add(time.Hour)} {
		next, cmd := m.Update(confirmationTick{m.runGeneration, at})
		m = next.(Model)
		if cmd != nil || s.saves != 0 || m.mode != "run" {
			t.Fatal("timer executed without confirmation")
		}
	}
	if _, cmd := m.executeConfirmed(true); cmd != nil || s.saves != 0 {
		t.Fatal("automatic execution accepted")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || s.saves != 1 || next.(Model).mode != "executing" {
		t.Fatal("explicit confirmation failed")
	}
}
func TestNoCountdownFromRiskLabelAlone(t *testing.T) {
	for _, risk := range []string{"READ", "CONNECT", "WRITE", "DANGER"} {
		m, s := quickFixture(t, risk, false)
		if !m.runDeadline.IsZero() || s.saves != 0 {
			t.Fatal("unexpected automatic approval")
		}
	}
	for _, risk := range []string{"WRITE", "DANGER"} {
		m, s := quickFixture(t, risk, true)
		if !m.runDeadline.IsZero() {
			t.Fatal("dangerous command got timer")
		}
		for _, msg := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeySpace}, singleKey("y"), singleKey("1")} {
			next, _ := m.Update(msg)
			m = next.(Model)
			if s.saves != 0 {
				t.Fatal("shortcut bypassed strong confirmation")
			}
		}
	}
}
func TestTimerPausesOnReviewPasteFocusResizeAndDelay(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.Msg
	}{
		{"review", tea.KeyMsg{Type: tea.KeyPgDown}},
		{"paste", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y"), Paste: true}},
		{"multiple runes", singleKey("yes")},
		{"blur", tea.BlurMsg{}},
		{"resize", tea.WindowSizeMsg{Width: 100, Height: 40}},
		{"other key", singleKey("x")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, s := quickFixture(t, "READ", true)
			gen := m.runGeneration
			deadline := m.runDeadline
			next, _ := m.Update(c.msg)
			m = next.(Model)
			if !m.runDeadline.IsZero() || s.saves != 0 {
				t.Fatal("timer not paused")
			}
			_, cmd := m.Update(confirmationTick{gen, deadline})
			if cmd != nil || s.saves != 0 {
				t.Fatal("paused timer ran")
			}
		})
	}
	m, s := quickFixture(t, "READ", true)
	next, cmd := m.Update(confirmationTick{m.runGeneration, m.runDeadline.Add(time.Second)})
	if cmd != nil || s.saves != 0 || !next.(Model).runDeadline.IsZero() {
		t.Fatal("overdue timer ran after stall")
	}
}
func TestEditedEntryInvalidatesTimerApproval(t *testing.T) {
	m, s := quickFixture(t, "READ", true)
	for _, field := range []string{"command", "risk", "host", "prerequisite", "id"} {
		c := m.target
		switch field {
		case "command":
			c.Command += " changed"
		case "risk":
			c.Risk = "CONNECT"
		case "host":
			c.Host = "other"
		case "prerequisite":
			c.Prerequisite = "new"
		case "id":
			c.ID = "other"
		}
		if approvedCountdown(m.path, c) {
			t.Fatal("approval survived changed " + field)
		}
	}
	s.cs[0].Command = "printf changed"
	next, cmd := m.Update(singleKey("y"))
	if cmd != nil || s.saves != 0 || next.(Model).mode != "" {
		t.Fatal("changed on-disk command executed")
	}
}
func TestPolicyRevocationAndMalformedPolicy(t *testing.T) {
	m, s := quickFixture(t, "READ", true)
	if err := os.Remove(m.path + ".run-policy.json"); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.Update(confirmationTick{m.runGeneration, m.runDeadline})
	if cmd != nil || s.saves != 0 || !next.(Model).runDeadline.IsZero() {
		t.Fatal("revoked timer executed")
	}
	for _, raw := range []string{"{}", "not-json", `{"version":2,"delay_ms":2500}`, `{"version":1,"delay_ms":2500,"unknown":true}`, strings.Repeat("x", 65537)} {
		os.WriteFile(m.path+".run-policy.json", []byte(raw), 0600)
		if approvedCountdown(m.path, m.target) {
			t.Fatal("invalid policy accepted")
		}
	}
}
func TestReviewMustBeVisibleBeforeCountdown(t *testing.T) {
	m, s := quickFixture(t, "READ", true)
	m.width = 36
	m.height = 14
	next, _ := m.beginConfirmation()
	if !next.(Model).runDeadline.IsZero() || s.saves != 0 {
		t.Fatal("countdown with hidden command")
	}
	m.width = 160
	m.height = 55
	m.target.Command = strings.Repeat("printf long ", 2000)
	writePolicy(t, m.path, m.target)
	next, _ = m.beginConfirmation()
	if !next.(Model).runDeadline.IsZero() {
		t.Fatal("countdown with clipped command")
	}
}
func TestExecutionUsesFreshLibraryAndPreservesOtherChanges(t *testing.T) {
	m, s := quickFixture(t, "READ", false)
	s.cs[1].Description = "Changed in another window"
	next, cmd := m.Update(singleKey("y"))
	if cmd == nil || s.saves != 1 || s.cs[1].Description != "Changed in another window" || next.(Model).mode != "executing" {
		t.Fatal("lost concurrent edit")
	}
}

type brokenStore struct {
	cs               []library.Command
	loadErr, saveErr bool
}

func (s *brokenStore) Load(context.Context) ([]library.Command, error) {
	if s.loadErr {
		return nil, errors.New("test load failure")
	}
	return s.cs, nil
}
func (s *brokenStore) Save(context.Context, []library.Command) error {
	if s.saveErr {
		return errors.New("test save failure")
	}
	return nil
}
func TestStorageFailureNeverRuns(t *testing.T) {
	for _, loadErr := range []bool{false, true} {
		m, _ := quickFixture(t, "READ", true)
		m.store = &brokenStore{cs: m.all, loadErr: loadErr, saveErr: !loadErr}
		next, cmd := m.Update(singleKey("y"))
		if cmd != nil || next.(Model).mode == "executing" || !next.(Model).runDeadline.IsZero() {
			t.Fatal("execution despite storage failure")
		}
	}
}

func TestControlCInvalidatesPendingTimer(t *testing.T) {
	m, s := quickFixture(t, "READ", true)
	gen := m.runGeneration
	deadline := m.runDeadline
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(Model)
	if !m.runDeadline.IsZero() {
		t.Fatal("ctrl-c left timer armed")
	}
	_, cmd := m.Update(confirmationTick{gen, deadline})
	if cmd != nil || s.saves != 0 {
		t.Fatal("timer ran after quit")
	}
}
