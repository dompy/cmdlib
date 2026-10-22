package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/dompy/cmdlib/internal/library"
)

// Mobile mode is explicitly enabled by the iSH launcher; desktop controls stay intact.
func (m Model) mobilePageSize() int {
	return max(1, min(9, (m.height-9)/3))
}

func (m *Model) mobileLayout() {
	m.detail.Width = max(10, m.width-4)
	m.detail.Height = max(2, m.height-10)
	m.search.Width = max(5, m.width-12)
	m.confirm.Width = max(5, m.width-6)
	if m.mode == "edit" && len(m.fields) > m.field {
		m.fields[m.field].SetWidth(max(8, m.width-6))
		m.fields[m.field].SetHeight(max(1, min(5, m.height-10)))
	}
	var body string
	switch m.mode {
	case "run", "explain", "copy":
		c := m.target
		body = accent.Render(safe(c.Name)) + "\n\n" + safe(c.Command)
		if m.mode == "run" {
			body += "\n\nLokal: " + safe(m.host) + " · iSH"
			if c.Risk == "WRITE" {
				body += "\n" + warning.Render("Dieser Befehl ändert Daten.")
			}
			if c.Risk == "DANGER" {
				body += "\n" + warning.Render("Kritischer Befehl: genau prüfen.")
			}
		} else {
			if c.Description != "" {
				body += "\n\n" + safe(c.Description)
			}
			if c.Explanation != "" {
				body += "\n\n" + safe(c.Explanation)
			}
		}
	default:
		body = m.content()
	}
	m.detail.SetContent(lipgloss.NewStyle().Width(m.detail.Width).Render(body))
}

func (m Model) mobileOpen() (tea.Model, tea.Cmd) {
	if c, ok := m.current(); ok {
		m.target = c
		m.returnMode = ""
		return m.beginConfirmation()
	}
	return m, nil
}

func (m Model) updateMobile(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	k, ok := msg.(tea.KeyMsg)
	if !ok || k.String() == "ctrl+c" {
		return m, nil, false
	}
	key := k.String()
	if m.mode == "edit" && (m.creating || m.field == 5) {
		next, cmd := m.mobileEditKey(k)
		return next, cmd, true
	}
	if k.Paste || (k.Type == tea.KeyRunes && len(k.Runes) != 1) {
		return m, nil, false
	}
	if m.mode == "mobile-edit" {
		if key == "esc" || key == "0" || key == "q" {
			m.mode = ""
			m.layout()
			return m, nil, true
		}
		fields := map[string]int{"1": 0, "2": 2, "3": 1, "4": 5}
		if field, ok := fields[key]; ok {
			m.detailField = field
			m.creating = false
			m.beginEdit()
			return m, textinput.Blink, true
		}
		return m, nil, true
	}
	if m.mode == "run" {
		if key == "q" {
			next, cmd := m.cancelConfirmation()
			return next, cmd, true
		}
		if key == "up" || key == "down" {
			if key == "up" {
				m.detail.LineUp(1)
			} else {
				m.detail.LineDown(1)
			}
			return m, nil, true
		}
		return m, nil, false
	}
	if m.mode != "" || m.search.Focused() {
		return m, nil, false
	}
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		page := m.mobilePageSize()
		index := (m.selected/page)*page + int(key[0]-'1')
		if index < len(m.rows) && index < (m.selected/page+1)*page {
			m.selected = index
			next, cmd := m.mobileOpen()
			return next, cmd, true
		}
		return m, nil, true
	}
	switch key {
	case "enter", " ", "r":
		next, cmd := m.mobileOpen()
		return next, cmd, true
	case "up", "down", "left", "right":
		delta := -1
		if key == "down" {
			delta = 1
		}
		if key == "left" {
			delta = -m.mobilePageSize()
		}
		if key == "right" {
			delta = m.mobilePageSize()
		}
		m.selected = max(0, min(len(m.rows)-1, m.selected+delta))
		m.fieldFocus = false
		m.status = ""
		m.layout()
		return m, nil, true
	case "e":
		if c, ok := m.current(); ok {
			m.target = c
			m.mode = "mobile-edit"
			m.status = ""
			m.layout()
		}
		return m, nil, true
	case "n":
		m.target = library.Command{ID: fmt.Sprintf("cmd-%d", time.Now().UnixNano()), Risk: "WRITE", Host: m.host}
		m.creating = true
		m.detailField = 0
		m.beginEdit()
		return m, textinput.Blink, true
	case "tab":
		return m, nil, true
	case "0":
		return m, tea.Quit, true
	}
	return m, nil, false
}

func (m Model) mobileEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "esc" {
		return m.updateEdit(msg)
	}
	if m.field == 5 {
		if msg.Paste {
			return m, nil
		}
		risks := map[string]string{"1": "READ", "2": "CONNECT", "3": "WRITE", "4": "DANGER"}
		if risk, ok := risks[key]; ok {
			m.fields[5].SetValue(risk)
			return m.updateEdit(tea.KeyMsg{Type: tea.KeyEnter})
		}
		// Saving a new command requires an explicit choice of type.
		if !m.creating && key == "enter" {
			return m.updateEdit(msg)
		}
		return m, nil
	}
	if key == "enter" || key == "tab" {
		if strings.TrimSpace(m.fields[m.field].Value()) == "" {
			m.status = "Bitte ausfüllen."
			return m, nil
		}
		m.fields[m.field].Blur()
		if m.field == 0 {
			m.field = 2
		} else {
			m.field = 5
		}
		cmd := m.fields[m.field].Focus()
		m.status = ""
		m.layout()
		return m, cmd
	}
	if key == "shift+tab" && m.field == 2 {
		m.fields[2].Blur()
		m.field = 0
		cmd := m.fields[0].Focus()
		m.layout()
		return m, cmd
	}
	var cmd tea.Cmd
	m.fields[m.field], cmd = m.fields[m.field].Update(msg)
	m.layout()
	return m, cmd
}

func (m Model) mobileView() string {
	if m.width < 20 || m.height < 10 {
		return "cmdlib\nBitte Terminal vergrössern.\nCtrl+C: Terminal"
	}
	w := m.width - 2
	title := muted.Render("cmdlib · iPhone")
	var body, footer string
	switch m.mode {
	case "run":
		title = accent.Render("Befehl starten?")
		action := "Starten"
		if m.target.Risk == "CONNECT" {
			title = accent.Render("Verbinden?")
			action = "Verbinden"
		}
		body = m.detail.View()
		if quickConfirmation(m.target) {
			footer = accent.Render("[Enter / 1] "+action) + "\n" + muted.Render("[0 / Esc] Zurück")
		} else {
			footer = "Bestätigen: " + confirmation(m.target) + "\n" + m.confirm.View() + "\n" + muted.Render("Enter: Starten · Esc: Zurück")
		}
	case "mobile-edit":
		title = accent.Render("Bearbeiten")
		body = safe(m.target.Name) + "\n\n[1] Name\n\n[2] Befehl\n\n[3] Beschreibung\n\n[4] Typ"
		footer = "[0 / Esc] Zurück"
	case "edit":
		if m.creating {
			title = accent.Render("Neuer Befehl")
		} else {
			title = accent.Render("Bearbeiten")
		}
		if m.field == 5 {
			body = "Was macht der Befehl?\n\n[1] Lesen\n[2] Verbinden\n[3] Ändern\n[4] Kritisch"
			footer = "Zahl: Speichern · Esc: Abbrechen"
		} else {
			label := map[int]string{0: "Name", 1: "Beschreibung", 2: "Befehl"}[m.field]
			body = label + "\n\n" + m.fields[m.field].View()
			footer = "[Enter] Speichern · [Esc] Abbrechen"
			if m.creating {
				footer = "[Enter] Weiter · [Esc] Abbrechen"
			}
		}
	case "explain", "copy", "help":
		body = m.detail.View()
		footer = "[Esc] Zurück · ↑↓ Scrollen"
	case "":
		if m.search.Focused() || m.search.Value() != "" {
			title += "\n" + m.search.View()
		}
		if len(m.rows) == 0 {
			body = "Noch kein passender Befehl."
		} else {
			count := m.mobilePageSize()
			start := (m.selected / count) * count
			for i := start; i < min(len(m.rows), start+count); i++ {
				c := m.rows[i].command
				label := fmt.Sprintf("[%d] %s", i-start+1, ansi.Truncate(safe(strings.ReplaceAll(c.Name, "\n", " ")), max(4, w-10), "…"))
				style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(w - 4)
				if i == m.selected {
					style = style.BorderForeground(lipgloss.Color("80")).Bold(true)
				} else {
					style = style.Foreground(lipgloss.Color("245"))
				}
				body += style.Render(label) + "\n"
			}
			if len(m.rows) > count {
				body += fmt.Sprintf("Seite %d/%d · ← →", start/count+1, (len(m.rows)+count-1)/count)
			}
		}
		footer = "Zahl / Enter: Öffnen · ↑↓ Auswahl\n[N] Neu  [/] Suchen\n[E] Bearbeiten  [?] Info  [Q] Terminal"
	default:
		body = m.detail.View()
		footer = "[Esc] Zurück"
	}
	status := ""
	if m.status != "" {
		status = "\n" + muted.Render(safe(m.status))
	}
	view := title + "\n\n" + body + status + "\n" + footer
	return lipgloss.NewStyle().Padding(0, 1).Width(m.width).MaxWidth(m.width).MaxHeight(m.height).Render(view)
}
