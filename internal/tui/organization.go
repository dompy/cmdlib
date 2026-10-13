package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dompy/cmdlib/internal/library"
)

// Category headers are view rows, never commands and never execution targets.
type libraryRow struct {
	command  library.Command
	category string
	count    int
}

func (m Model) selectedRow() (libraryRow, bool) {
	if m.selected < 0 || m.selected >= len(m.rows) {
		return libraryRow{}, false
	}
	return m.rows[m.selected], true
}

func (m *Model) rebuildRows() {
	m.rows = nil
	if strings.TrimSpace(m.search.Value()) != "" {
		for _, c := range m.results {
			m.rows = append(m.rows, libraryRow{command: c})
		}
		return
	}
	groups := map[string][]library.Command{}
	for _, c := range m.results {
		if c.Category != "" {
			groups[c.Category] = append(groups[c.Category], c)
		}
	}
	seen := map[string]bool{}
	for _, c := range m.results {
		if c.Category == "" {
			m.rows = append(m.rows, libraryRow{command: c})
			continue
		}
		if seen[c.Category] {
			continue
		}
		seen[c.Category] = true
		m.rows = append(m.rows, libraryRow{category: c.Category, count: len(groups[c.Category])})
		if m.expanded[c.Category] {
			for _, child := range groups[c.Category] {
				m.rows = append(m.rows, libraryRow{command: child})
			}
		}
	}
}

func commandIndex(cs []library.Command, id string) int {
	for i, c := range cs {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func sameLibrary(a, b []library.Command) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func (m *Model) selectCommand(id string) {
	if i := commandIndex(m.all, id); i >= 0 && m.all[i].Category != "" {
		m.expanded[m.all[i].Category] = true
		m.saveViewState()
	}
	m.refresh()
	for i, row := range m.rows {
		if row.category == "" && row.command.ID == id {
			m.selected = i
			break
		}
	}
	m.action = 1
	m.layout()
}

func copyExpansion(state map[string]bool) map[string]bool {
	copy := make(map[string]bool, len(state))
	for name, expanded := range state {
		copy[name] = expanded
	}
	return copy
}

func (m *Model) selectCategory(name string) {
	m.refresh()
	for i, row := range m.rows {
		if row.category == name {
			m.selected = i
			break
		}
	}
	m.action = 1
	m.layout()
}

func (m Model) beginRenameCategory(name string) (tea.Model, tea.Cmd) {
	m.mode = "rename-category"
	m.renameOriginal = name
	m.renameCategory = textinput.New()
	m.renameCategory.Prompt = "Neuer Name: "
	m.renameCategory.Placeholder = "z. B. Audio & Aufnahme"
	m.renameCategory.CharLimit = 80
	cmd := m.renameCategory.Focus()
	m.status = ""
	m.detail.GotoTop()
	m.layout()
	return m, cmd
}

func (m Model) renameCategoryContent() string {
	count := 0
	for _, c := range m.all {
		if c.Category == m.renameOriginal {
			count++
		}
	}
	return accent.Render("Kategorie umbenennen") + "\n\n" + safe(m.renameOriginal) +
		fmt.Sprintf(" · %d commands\n\n", count) + m.renameCategory.View()
}

func (m Model) updateRenameCategory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = ""
		m.renameCategory.Blur()
		m.status = "Umbenennen abgebrochen."
		m.selectCategory(m.renameOriginal)
		return m, nil
	case "enter":
		return m.saveCategoryName()
	}
	var cmd tea.Cmd
	m.renameCategory, cmd = m.renameCategory.Update(msg)
	m.layout()
	return m, cmd
}

func (m Model) saveCategoryName() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.renameCategory.Value())
	if name == "" {
		m.status = "Bitte einen Namen eingeben."
		return m, nil
	}
	if utf8.RuneCountInString(name) > 80 || strings.ContainsFunc(name, unicode.IsControl) {
		m.status = "Bitte höchstens 80 Zeichen ohne Zeilenumbrüche verwenden."
		return m, nil
	}
	if name == m.renameOriginal {
		m.mode = ""
		m.renameCategory.Blur()
		m.status = "Name unverändert."
		m.selectCategory(name)
		return m, nil
	}
	for _, c := range m.all {
		if c.Category == name {
			m.status = "Diese Kategorie gibt es bereits. Bitte einen anderen Namen wählen."
			return m, nil
		}
	}
	cs := append([]library.Command(nil), m.all...)
	count := 0
	now := time.Now().UTC()
	for i := range cs {
		if cs[i].Category == m.renameOriginal {
			cs[i].Category = name
			cs[i].UpdatedAt = now
			count++
		}
	}
	if count == 0 {
		m.status = "Die Kategorie ist nicht mehr vorhanden."
		return m, nil
	}
	if m.saveOrganization(cs, "rename category") {
		m.expanded[name] = m.expanded[m.renameOriginal]
		delete(m.expanded, m.renameOriginal)
		m.saveViewState()
		m.mode = ""
		m.renameCategory.Blur()
		m.status = "Kategorie heißt jetzt „" + name + "“. u macht es rückgängig."
		m.selectCategory(name)
	}
	return m, nil
}

func (m *Model) beginMove() {
	m.mode = "move"
	m.moveDraft = append([]library.Command(nil), m.all...)
	m.moveCategory = textinput.New()
	m.moveCategory.Prompt = "Category: "
	m.moveCategory.Placeholder = "main list (or type a category name)"
	m.moveCategory.SetValue(m.target.Category)
	m.moveCategory.CharLimit = 80
	m.moveCategory.Width = max(10, m.detail.Width-14)
	m.status = ""
	m.layout()
}

func (m *Model) shiftMove(direction int) {
	i := commandIndex(m.moveDraft, m.target.ID)
	j := i + direction
	if i < 0 || j < 0 || j >= len(m.moveDraft) {
		return
	}
	m.moveDraft[i], m.moveDraft[j] = m.moveDraft[j], m.moveDraft[i]
	m.layout()
}

func (m Model) moveContent() string {
	i := commandIndex(m.moveDraft, m.target.ID)
	if i < 0 {
		return "This command is no longer available. Esc returns to the library."
	}
	lines := []string{accent.Render("Move · " + safe(m.target.Name)), fmt.Sprintf("\nSaved position: %d / %d", i+1, len(m.moveDraft)), ""}
	for j := max(0, i-2); j < min(len(m.moveDraft), i+3); j++ {
		name := fmt.Sprintf("%d. %s", j+1, safe(m.moveDraft[j].Name))
		if j == i {
			name = accent.Render("› " + name)
		} else {
			name = "  " + name
		}
		lines = append(lines, name)
	}
	lines = append(lines, "", m.moveCategory.View())
	return strings.Join(lines, "\n")
}

func (m Model) updateMove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = ""
		m.moveDraft = nil
		m.status = "Move cancelled."
		m.action = 1
		m.layout()
		return m, nil
	case "tab", "shift+tab":
		if m.moveCategory.Focused() {
			m.moveCategory.Blur()
			m.layout()
			return m, nil
		}
		cmd := m.moveCategory.Focus()
		m.layout()
		return m, cmd
	case "enter":
		return m.saveMove()
	case "up", "down":
		if !m.moveCategory.Focused() {
			direction := -1
			if msg.String() == "down" {
				direction = 1
			}
			m.shiftMove(direction)
			return m, nil
		}
	}
	if m.moveCategory.Focused() {
		var cmd tea.Cmd
		m.moveCategory, cmd = m.moveCategory.Update(msg)
		m.layout()
		return m, cmd
	}
	return m, nil
}

// Reload before an organization write, rather than overwriting another window.
// JSON Store still performs the existing atomic validated save.
func (m *Model) saveOrganization(cs []library.Command, label string) bool {
	latest, err := m.store.Load(context.Background())
	if err != nil {
		m.status = "Cannot reload library: " + err.Error()
		return false
	}
	if !sameLibrary(latest, m.all) {
		m.all = append([]library.Command(nil), latest...)
		m.mode = ""
		m.status = "Library changed in another window. Reloaded; please try again."
		m.refresh()
		return false
	}
	if backup, ok := m.store.(interface {
		Backup(context.Context, string) (string, error)
	}); ok {
		reason := label
		if reason == "" {
			reason = "undo"
		}
		if _, err = backup.Backup(context.Background(), reason); err != nil {
			m.status = "Cannot create recovery copy; unchanged: " + err.Error()
			return false
		}
	}
	if err = m.store.Save(context.Background(), cs); err != nil {
		m.status = "Save failed: " + err.Error()
		return false
	}
	m.undoAll = append([]library.Command(nil), m.all...)
	m.undoExpanded = copyExpansion(m.expanded)
	m.undoCategory = ""
	if row, ok := m.selectedRow(); ok {
		m.undoCategory = row.category
	}
	m.undoLabel = label
	m.all = append([]library.Command(nil), cs...)
	return true
}

func (m Model) saveMove() (tea.Model, tea.Cmd) {
	cs := append([]library.Command(nil), m.moveDraft...)
	i := commandIndex(cs, m.target.ID)
	if i < 0 {
		m.status = "Command no longer available."
		return m, nil
	}
	cs[i].Category = strings.TrimSpace(m.moveCategory.Value())
	if sameLibrary(cs, m.all) {
		m.mode = ""
		m.status = "Position unchanged."
		m.selectCommand(m.target.ID)
		return m, nil
	}
	cs[i].UpdatedAt = time.Now().UTC()
	if m.saveOrganization(cs, "move") {
		m.mode = ""
		m.moveDraft = nil
		m.status = "Moved “" + m.target.Name + "”. u undoes."
		m.selectCommand(m.target.ID)
	}
	return m, nil
}

func (m Model) updateDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Paste || (msg.Type == tea.KeyRunes && len(msg.Runes) != 1) {
		return m, nil
	}
	switch msg.String() {
	case "esc", "n", "N", "0":
		m.mode = ""
		m.action = 1
		m.status = "Deletion cancelled."
		m.layout()
		return m, nil
	case "enter", "y", "Y":
		cs := make([]library.Command, 0, len(m.all))
		for _, c := range m.all {
			if c.ID != m.target.ID {
				cs = append(cs, c)
			}
		}
		if len(cs) == len(m.all) {
			m.status = "Command no longer available."
			return m, nil
		}
		if m.saveOrganization(cs, "delete") {
			m.mode = ""
			m.action = 1
			m.status = "Deleted “" + m.target.Name + "”. Press u to restore it."
			m.refresh()
		}
	}
	return m, nil
}

func (m Model) undoOrganization() (tea.Model, tea.Cmd) {
	if m.undoLabel == "" {
		m.status = "Nothing to undo in this session."
		return m, nil
	}
	previous := append([]library.Command(nil), m.undoAll...)
	label := m.undoLabel
	expanded := copyExpansion(m.undoExpanded)
	category := m.undoCategory
	if m.saveOrganization(previous, "") {
		m.undoAll = nil
		m.undoExpanded = nil
		m.undoCategory = ""
		m.undoLabel = ""
		m.expanded = expanded
		m.saveViewState()
		m.status = "Undid " + label + "."
		if category != "" {
			m.selectCategory(category)
		} else {
			m.selectCommand(m.target.ID)
		}
	}
	return m, nil
}

type maintenanceFinished struct{ err error }
type queuedMaintenance struct{ err error }

func maintenanceEnvironment(path string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "CMDLIB_FILE=") {
			env = append(env, v)
		}
	}
	if path != "" {
		env = append(env, "CMDLIB_FILE="+path)
	}
	return env
}

func (m Model) openMaintenance(action string) (tea.Model, tea.Cmd) {
	if action != "doctor" && action != "assist" {
		return m, nil
	}
	executable, err := os.Executable()
	if err != nil {
		m.status = "Cannot locate cmdlib: " + err.Error()
		return m, nil
	}
	if action == "assist" {
		cmd := exec.Command(executable, "assist", "--queue")
		cmd.Env = maintenanceEnvironment(m.path)
		m.status = "KI-Prüfung wird vorgemerkt …"
		return m, func() tea.Msg { return queuedMaintenance{cmd.Run()} }
	}
	// Arguments, not interpolated command text, keep paths and filenames literal.
	cmd := exec.Command("/bin/sh", "-c", `"$1" "$2"
cmdlib_status=$?
printf '\n[cmdlib] Press Enter to return. '
read -r cmdlib_reply
exit "$cmdlib_status"`, "cmdlib-maintenance", executable, action)
	cmd.Env = maintenanceEnvironment(m.path)
	m.mode = "maintenance"
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return maintenanceFinished{err} })
}
