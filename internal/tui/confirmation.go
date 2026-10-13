package tui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dompy/cmdlib/internal/library"
)

const confirmationDelay = 2500 * time.Millisecond

type confirmationTick struct {
	generation uint64
	at         time.Time
}

type runPolicy struct {
	Version  int      `json:"version"`
	DelayMS  int      `json:"delay_ms"`
	Approved []string `json:"approved_fingerprints"`
}

func quickConfirmation(c library.Command) bool { return c.Risk == "READ" || c.Risk == "CONNECT" }

// A risk label is NOT approval for a timer. Approval belongs to the exact entry.
func commandFingerprint(c library.Command) string {
	fields := []string{c.ID, c.Command, c.Risk, c.Host, c.Prerequisite}
	raw, _ := json.Marshal(fields)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func approvedCountdown(path string, c library.Command) bool {
	if path == "" || !quickConfirmation(c) {
		return false
	}
	f, err := os.Open(path + ".run-policy.json")
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return false
	}
	var policy runPolicy
	dec := json.NewDecoder(io.LimitReader(f, 65537))
	dec.DisallowUnknownFields()
	if dec.Decode(&policy) != nil || policy.Version != 1 || policy.DelayMS != 2500 {
		return false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return false
	}
	fingerprint := commandFingerprint(c)
	for _, allowed := range policy.Approved {
		if allowed == fingerprint {
			return true
		}
	}
	return false
}

func countdownTick(generation uint64) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return confirmationTick{generation, t} })
}

func (m *Model) pauseCountdown(message string) {
	if !m.runDeadline.IsZero() {
		m.runGeneration++
		m.runDeadline = time.Time{}
		m.status = message
	}
}

func (m Model) beginConfirmation() (tea.Model, tea.Cmd) {
	m.mode = "run"
	m.runGeneration++
	m.runDeadline = time.Time{}
	m.runFingerprint = commandFingerprint(m.target)
	m.confirm.SetValue("")
	m.status = "Type " + confirmation(m.target) + " to run · Esc cancels"
	if quickConfirmation(m.target) {
		m.status = "Enter / Space / y / 1: start · Esc / n / 0 / Backspace / Del: cancel"
	}
	m.layout()
	focus := m.confirm.Focus()
	if approvedCountdown(m.path, m.target) {
		// The review must fit on screen; never count down behind a clipped command.
		if m.width < 60 || m.height < 18 || m.detail.TotalLineCount() > m.detail.Height {
			m.status = "Auto-start paused: enlarge/review the command, then confirm explicitly."
			return m, focus
		}
		m.runDeadline = time.Now().Add(confirmationDelay)
		m.status = "Start in 2.5s · Enter / Space / y / 1: now · Esc / n / 0 / Backspace / Del: cancel"
		return m, tea.Batch(focus, countdownTick(m.runGeneration))
	}
	return m, focus
}

func (m Model) cancelConfirmation() (tea.Model, tea.Cmd) {
	m.runGeneration++
	m.runDeadline = time.Time{}
	m.mode = m.returnMode
	m.confirm.SetValue("")
	m.confirm.Blur()
	m.status = "Cancelled. Nothing executed."
	m.layout()
	return m, nil
}

func (m Model) confirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	// A paste, including a pasted y, is never a deliberate single-key approval.
	if msg.Paste || (msg.Type == tea.KeyRunes && len(msg.Runes) != 1) {
		m.pauseCountdown("Countdown paused: pasted text is not confirmation.")
		return m, nil
	}
	if k == "esc" {
		return m.cancelConfirmation()
	}
	if quickConfirmation(m.target) {
		switch k {
		case "n", "N", "0", "backspace", "ctrl+h", "delete":
			return m.cancelConfirmation()
		case "enter", " ", "space", "y", "Y", "1":
			return m.executeConfirmed(false)
		}
	} else if m.confirm.Value() == "" {
		switch k {
		case "n", "0", "backspace", "ctrl+h", "delete":
			return m.cancelConfirmation()
		}
	}
	// Any other key pauses the timer, including navigation/review keys.
	m.pauseCountdown("Countdown paused; confirm explicitly when ready.")
	if k == "pgup" || k == "pgdown" {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}
	if quickConfirmation(m.target) {
		return m, nil
	}
	if k == "enter" {
		if m.confirm.Value() != confirmation(m.target) {
			m.status = "Confirmation does not match; nothing executed."
			return m, nil
		}
		return m.executeConfirmed(false)
	}
	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)
	return m, cmd
}

func (m Model) confirmTick(t confirmationTick) (tea.Model, tea.Cmd) {
	if m.mode != "run" || m.runDeadline.IsZero() || t.generation != m.runGeneration {
		return m, nil
	}
	// Sleep/suspend/backlog must not cause a surprise execution upon returning.
	if t.at.After(m.runDeadline.Add(500 * time.Millisecond)) {
		m.pauseCountdown("Countdown paused after a delay; confirm explicitly.")
		return m, nil
	}
	remaining := m.runDeadline.Sub(t.at)
	if remaining <= 0 {
		if !approvedCountdown(m.path, m.target) {
			m.pauseCountdown("Auto-start approval changed; confirm explicitly.")
			return m, nil
		}
		return m.executeConfirmed(true)
	}
	m.status = fmt.Sprintf("Start in %.1fs · Enter / Space / y / 1: now · Esc / n / 0 / Backspace / Del: cancel", remaining.Seconds())
	return m, countdownTick(m.runGeneration)
}

func (m Model) executeConfirmed(automatic bool) (tea.Model, tea.Cmd) {
	if m.mode != "run" || commandFingerprint(m.target) != m.runFingerprint {
		return m, nil
	}
	m.runGeneration++
	m.runDeadline = time.Time{}
	// Refresh before recording use: do not overwrite edits made in another window.
	latest, err := m.store.Load(context.Background())
	if err != nil {
		m.status = "Cannot reload library; nothing executed: " + err.Error()
		return m, nil
	}
	cs := append([]library.Command(nil), latest...)
	found := -1
	for i, c := range cs {
		if c.ID == m.target.ID {
			found = i
			break
		}
	}
	if found < 0 || commandFingerprint(cs[found]) != m.runFingerprint {
		m.all = cs
		m.mode = ""
		m.confirm.Blur()
		m.status = "Command changed in another window; review it again. Nothing executed."
		m.refresh()
		return m, nil
	}
	if automatic && !approvedCountdown(m.path, cs[found]) {
		m.status = "Automatic approval revoked; confirm explicitly."
		return m, nil
	}
	now := time.Now().UTC()
	cs[found].UseCount++
	cs[found].LastUsedAt = &now
	if err = m.store.Save(context.Background(), cs); err != nil {
		m.status = "Cannot record run; nothing executed: " + err.Error()
		return m, nil
	}
	m.all = cs
	m.mode = "executing" // Invalidate queued timer/keypress events before launching.
	m.confirm.Blur()
	m.refresh()
	cmd := exec.Command("/bin/sh", "-c", `/bin/sh -c "$1"
cmdlib_status=$?
printf '\n[cmdlib] Exit status: %s. Press Enter to return. ' "$cmdlib_status"
read -r cmdlib_reply
exit "$cmdlib_status"`, "cmdlib-run", m.target.Command)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return executed{err} })
}
