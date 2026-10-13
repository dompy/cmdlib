package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dompy/cmdlib/internal/library"
)

// Field indices match the existing editor and on-disk command model.
var layerFields = [][]int{{2}, {1}, {0, 8, 3, 4, 5, 6, 7}}
var layerNames = []string{"Command", "Description", "Details"}

func (m Model) listWidth() int { return max(22, min(30, m.width/3)) }

func fieldValue(c library.Command, field int) string {
	return []string{c.Name, c.Description, c.Command, strings.Join(c.Tags, ", "), c.Host, c.Risk, c.Prerequisite, c.Explanation, c.Category}[field]
}

func (m *Model) navigateField(key string) {
	if key == "left" || key == "right" {
		d := 1
		if key == "left" {
			d = -1
		}
		m.layer = max(0, min(len(layerFields)-1, m.layer+d))
		m.detailField = layerFields[m.layer][0]
		return
	}
	fields := layerFields[m.layer]
	pos := 0
	for i, field := range fields {
		if field == m.detailField {
			pos = i
		}
	}
	d := 1
	if key == "up" {
		d = -1
	}
	m.detailField = fields[max(0, min(len(fields)-1, pos+d))]
}

func (m Model) fieldContent(c library.Command) string {
	if m.creating {
		return accent.Render("New command") + "\n\n" + muted.Render(labels[m.field]) + "\n" + m.fields[m.field].View()
	}
	fields := layerFields[m.layer]
	var lines []string
	if m.layer > 0 {
		lines = append(lines, muted.Render(layerNames[m.layer]), "")
	}
	for _, field := range fields {
		selected := m.fieldFocus && field == m.detailField
		value := safe(fieldValue(c, field))
		editing := m.mode == "edit" && field == m.field
		if m.layer == 2 && !selected && (value == "" || field == 0) {
			continue
		}
		if editing {
			value = m.fields[field].View()
		}
		if value == "" {
			value = muted.Render("—")
		}
		marker := "  "
		if selected {
			marker = accent.Render("› ")
		}
		if m.layer == 2 {
			lines = append(lines, marker+muted.Render(labels[field])+"\n  "+value, "")
		} else {
			if !editing {
				value = accent.Render(value)
			}
			lines = append(lines, marker+value)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) listView(height int, showSelectedName bool) string {
	lw := m.listWidth()
	start := max(0, m.selected-height+1)
	var lines []string
	for i := start; i < min(len(m.rows), start+height); i++ {
		row := m.rows[i]
		name := safe(row.command.Name)
		if row.category != "" {
			marker := "▸"
			if m.expanded[row.category] {
				marker = "▾"
			}
			name = marker + " " + safe(row.category)
		} else if row.command.Category != "" && strings.TrimSpace(m.search.Value()) == "" {
			name = "  " + name
		}
		if i == m.selected && !showSelectedName {
			name = ""
		}
		style := lipgloss.NewStyle().MaxWidth(lw - 2)
		if i == m.selected {
			lines = append(lines, accent.Render("› ")+style.Bold(true).Render(name))
		} else {
			lines = append(lines, "  "+muted.Render(style.Render(name)))
		}
	}
	return lipgloss.NewStyle().Width(lw).Height(height).Render(strings.Join(lines, "\n"))
}

func (m Model) controls() string {
	switch m.mode {
	case "run":
		if quickConfirmation(m.target) {
			return "Enter / Space start · Esc cancel · PgUp/PgDown review"
		}
		return "Type " + confirmation(m.target) + " · Enter start · Esc cancel"
	case "edit":
		if m.creating {
			return "Tab field · Enter save · Esc cancel"
		}
		if m.field == 5 {
			return "READ / WRITE / DANGER / CONNECT · Enter save · Esc cancel"
		}
		return "Enter save · Alt+Enter newline · Esc cancel"
	case "help":
		return "←/→ tutorial · ? explain · c copy · r run · Esc back"
	case "explain":
		return "? / Esc back · PgUp/PgDown scroll"
	case "copy":
		return "Esc back · PgUp/PgDown scroll"
	case "move":
		return "↑↓ position · Tab category · Enter save · Esc cancel"
	case "rename-category":
		return "Enter save · Alt+Enter newline · Esc cancel"
	case "delete":
		return "Enter delete · Esc cancel"
	}
	if m.search.Focused() {
		return "↑↓ results · Enter / Esc leave search"
	}
	if row, ok := m.selectedRow(); ok && row.category != "" {
		return "Enter expand/collapse · e rename · / search · h help"
	}
	if m.fieldFocus {
		return "←/→ level · ↑↓ field · e edit · Tab list · r run"
	}
	a := append(append([]string{}, actions...), organizationActions...)[m.action]
	return a + " · Enter · ←/→ action · Tab fields · / search · h help"
}

func (m Model) View() string {
	if m.width < 36 || m.height < 14 {
		return "cmdlib\nTerminal too small: resize to at least 36×14.\nCtrl+C quits."
	}
	w := m.width - 4
	header := muted.Render("cmdlib")
	if m.search.Value() != "" {
		header += muted.Render(fmt.Sprintf("  %d matches", len(m.results)))
	}
	if m.search.Focused() || m.search.Value() != "" {
		header += "\n" + m.search.View()
	}
	body := m.detail.View()
	browse := m.mode == "" || m.mode == "edit" && !m.creating
	if browse && m.width >= 76 {
		showName := !(m.layer == 2 && m.fieldFocus && m.detailField == 0)
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.listView(m.detail.Height, showName), "   ", body)
	} else if browse {
		// Narrow terminals show only the selected row, keeping the detail usable.
		if row, ok := m.selectedRow(); ok && !(m.layer == 2 && m.fieldFocus && m.detailField == 0) {
			name := row.command.Name
			if row.category != "" {
				name = row.category
			}
			body = accent.Render(safe(name)) + "\n\n" + body
		}
	}
	status := ""
	if m.status != "" {
		status = "\n" + lipgloss.NewStyle().MaxWidth(w).Render(safe(m.status))
	}
	input := ""
	if m.mode == "run" && !quickConfirmation(m.target) {
		input = "\n" + m.confirm.View()
	}
	return lipgloss.NewStyle().Padding(0, 2).Width(m.width).MaxWidth(m.width).MaxHeight(m.height).Render(header + "\n\n" + body + input + status + "\n" + muted.Render(lipgloss.NewStyle().Width(w).Render(m.controls())))
}
