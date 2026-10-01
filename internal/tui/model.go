package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dompy/cmdlib/internal/library"
	"github.com/dompy/cmdlib/internal/search"
	"github.com/dompy/cmdlib/internal/storage"
)

var accent = lipgloss.NewStyle().Foreground(lipgloss.Color("80")).Bold(true)
var muted = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
var warning = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
var actions = []string{"Explain", "Copy", "Run", "Edit"}
var labels = []string{"Name", "Description", "Command", "Tags (comma separated)", "Host", "Risk: READ / WRITE / DANGER / CONNECT", "Prerequisite", "Explanation"}

type Model struct {
	store                                        storage.Store
	all, results                                 []library.Command
	path, host, status, mode, returnMode         string
	selected, action, width, height, field, step int
	search, confirm                              textinput.Model
	fields                                       []textinput.Model
	detail                                       viewport.Model
	target                                       library.Command
}
type copied struct{ err error }
type executed struct{ err error }

func New(s storage.Store, cs []library.Command, path string) Model {
	input := textinput.New()
	input.Prompt = "Search: "
	input.Placeholder = "name, command, tags, host…"
	input.CharLimit = 256
	confirm := textinput.New()
	confirm.Prompt = "> "
	confirm.CharLimit = 256
	host, _ := os.Hostname()
	m := Model{store: s, all: cs, results: search.Filter(cs, ""), path: path, host: host, search: input, confirm: confirm, width: 90, height: 30, detail: viewport.New(50, 18), status: "/ search · ? guided tutorial · n new command"}
	m.refresh()
	return m
}
func (m Model) Init() tea.Cmd { return nil }
func (m Model) current() (library.Command, bool) {
	if len(m.results) == 0 {
		return library.Command{}, false
	}
	return m.results[m.selected], true
}

// Display untrusted library text without terminal escape/control sequences.
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
func (m *Model) refresh() {
	m.results = search.Filter(m.all, m.search.Value())
	m.selected = min(m.selected, max(0, len(m.results)-1))
	m.detail.GotoTop()
	m.layout()
}
func (m *Model) layout() {
	w := max(10, m.width-6)
	h := max(3, m.height-11)
	if m.mode == "" && m.width >= 76 {
		w = max(10, w-max(24, m.width/3)-3)
	}
	m.detail.Width = w
	m.detail.Height = h
	m.search.Width = max(5, m.width-16)
	m.confirm.Width = max(5, m.width-10)
	m.detail.SetContent(lipgloss.NewStyle().Width(w).Render(m.content()))
}
func (m Model) commandDetails(c library.Command) string {
	pre := c.Prerequisite
	if pre == "" {
		pre = "None recorded"
	}
	return accent.Render(safe(c.Name)) + "\n\n" + safe(c.Command) + "\n\n" + muted.Render("Tags: ") + safe(strings.Join(c.Tags, " · ")) + "\nHost: " + safe(c.Host) + "   Risk: " + riskStyle(c.Risk) + "\nRequires: " + safe(pre) + "\n\n" + safe(c.Description)
}
func riskStyle(r string) string {
	if r == "WRITE" || r == "DANGER" {
		return warning.Render(r)
	}
	return accent.Render(r)
}
func (m Model) content() string {
	c, ok := m.current()
	if m.mode != "" {
		c = m.target
		ok = c.ID != ""
	}
	if m.mode == "help" {
		c = m.all[m.step]
		return fmt.Sprintf("Tutorial %d/%d · ←/→ command · e Explain · c Copy · r Run · Esc back\n\n", m.step+1, len(m.all)) + m.commandDetails(c) + "\n\n" + safe(c.Explanation) + "\n\nActions refer to the command above: Explain describes it; Copy copies its exact text; Run opens a confirmation.\n\nLibrary controls: / search; ↑/↓ results; ←/→ actions; Enter activate; Tab details/explanation; PgUp/PgDown scroll; n new; ? tutorial; q quit.\n\nSearch accepts multiple words and fuzzy abbreviations. Esc leaves search before q can quit. Risk and prerequisites are user-maintained labels, not a security check. Commands run locally via /bin/sh.\n\nLibrary: " + safe(m.path)
	}
	if !ok {
		return "No matching commands.\n\nPress / to change search or n to add a command."
	}
	body := m.commandDetails(c)
	switch m.mode {
	case "explain":
		explanation := c.Explanation
		if explanation == "" {
			explanation = "No explanation recorded. Edit this entry to document significant arguments and side effects. cmdlib cannot infer whether an arbitrary shell command changes anything."
		}
		return body + "\n\n" + accent.Render("Explanation") + "\n" + safe(explanation) + "\n\nEsc back"
	case "copy":
		return body + "\n\nClipboard unavailable. Select and copy the raw command above, or save the JSON/export locally. Nothing was executed.\n\nEsc back"
	case "run":
		note := "Review the exact command above."
		if c.Risk == "WRITE" {
			note = "This command is labeled WRITE and may change files or system state."
		}
		if c.Risk == "DANGER" {
			note = "DANGER: this command may irreversibly destroy data or disrupt services. Review every argument and your backups."
		}
		if c.Risk == "CONNECT" {
			note = "This opens a connection and may run commands or start processes remotely."
		}
		return warning.Render(note) + "\nExecution host: " + safe(m.host) + " (" + runtime.GOOS + ") via /bin/sh\nHost labels do not route execution.\nType " + confirmation(c) + " to execute · Esc cancels\n\n" + body
	}
	return body + fmt.Sprintf("\n\nUsed: %d times", c.UseCount)
}
func confirmation(c library.Command) string {
	switch c.Risk {
	case "WRITE":
		return "WRITE"
	case "DANGER":
		return "RUN " + c.ID
	default:
		return "yes"
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil
	case copied:
		if msg.err != nil {
			m.mode = "copy"
			m.status = "Clipboard: " + msg.err.Error()
			m.layout()
		} else {
			m.status = "Copied raw command to clipboard."
		}
		return m, nil
	case executed:
		m.mode = ""
		m.status = "Command finished."
		if msg.err != nil {
			m.status = "Command failed: " + msg.err.Error()
		}
		m.layout()
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.mode == "edit" {
			return m.updateEdit(msg)
		}
		if m.mode == "run" {
			if key == "pgup" || key == "pgdown" {
				var cmd tea.Cmd
				m.detail, cmd = m.detail.Update(msg)
				return m, cmd
			}
			if key == "esc" {
				m.mode = m.returnMode
				m.confirm.Blur()
				m.layout()
				return m, nil
			}
			if key == "enter" {
				if m.confirm.Value() != confirmation(m.target) {
					m.status = "Confirmation does not match; nothing executed."
					return m, nil
				}
				// Persist the confirmed attempt before handing the terminal to the child.
				cs := append([]library.Command(nil), m.all...)
				now := time.Now().UTC()
				for i := range cs {
					if cs[i].ID == m.target.ID {
						cs[i].UseCount++
						cs[i].LastUsedAt = &now
					}
				}
				if err := m.store.Save(context.Background(), cs); err != nil {
					m.status = "Cannot record run; nothing executed: " + err.Error()
					return m, nil
				}
				m.all = cs
				m.refresh()
				cmd := exec.Command("/bin/sh", "-c", `/bin/sh -c "$1"
cmdlib_status=$?
printf '\n[cmdlib] Exit status: %s. Press Enter to return. ' "$cmdlib_status"
read -r cmdlib_reply
exit "$cmdlib_status"`, "cmdlib-run", m.target.Command)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return executed{err} })
			}
			var cmd tea.Cmd
			m.confirm, cmd = m.confirm.Update(msg)
			return m, cmd
		}
		if m.mode == "help" {
			switch key {
			case "esc", "q":
				m.mode = ""
				m.layout()
				return m, nil
			case "left", "right":
				if key == "right" {
					m.step = (m.step + 1) % len(m.all)
				} else {
					m.step = (m.step + len(m.all) - 1) % len(m.all)
				}
				m.detail.GotoTop()
				m.layout()
				return m, nil
			case "e", "c", "r":
				m.target = m.all[m.step]
				m.returnMode = "help"
				a := 0
				if key == "c" {
					a = 1
				}
				if key == "r" {
					a = 2
				}
				return m.activate(a)
			}
		} else if m.mode != "" {
			if key == "esc" || key == "q" || key == "tab" {
				m.mode = m.returnMode
				m.layout()
				return m, nil
			}
		} else if m.search.Focused() {
			switch key {
			case "esc", "enter":
				m.search.Blur()
				return m, nil
			case "up", "down":
				m.search.Blur()
				if key == "down" {
					m.selected = min(m.selected+1, len(m.results)-1)
				} else {
					m.selected = max(0, m.selected-1)
				}
				m.selected = max(0, m.selected)
				m.layout()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.selected = 0
			m.refresh()
			return m, cmd
		} else {
			switch key {
			case "q":
				return m, tea.Quit
			case "/":
				cmd := m.search.Focus()
				return m, cmd
			case "esc":
				m.search.SetValue("")
				m.selected = 0
				m.refresh()
				return m, nil
			case "up":
				m.selected = max(0, m.selected-1)
				m.detail.GotoTop()
				m.layout()
				return m, nil
			case "down":
				m.selected = max(0, min(len(m.results)-1, m.selected+1))
				m.detail.GotoTop()
				m.layout()
				return m, nil
			case "left":
				m.action = (m.action + 3) % 4
				return m, nil
			case "right":
				m.action = (m.action + 1) % 4
				return m, nil
			case "?":
				if len(m.all) > 0 {
					m.mode = "help"
					m.step = 0
					m.layout()
				}
				return m, nil
			case "n":
				m.target = library.Command{ID: fmt.Sprintf("cmd-%d", time.Now().UnixNano()), Risk: "READ", Host: m.host}
				m.beginEdit()
				return m, textinput.Blink
			case "enter", "tab":
				if c, ok := m.current(); ok {
					m.target = c
					m.returnMode = ""
					a := m.action
					if key == "tab" {
						a = 0
					}
					return m.activate(a)
				}
			}
		}
	}
	var cmd tea.Cmd
	if m.mode == "edit" {
		m.fields[m.field], cmd = m.fields[m.field].Update(msg)
		return m, cmd
	}
	if m.mode == "run" {
		m.confirm, cmd = m.confirm.Update(msg)
		return m, cmd
	}
	if m.search.Focused() {
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
	if m.mode == "" || m.mode == "help" || m.mode == "explain" || m.mode == "copy" {
		m.detail, cmd = m.detail.Update(msg)
	}
	return m, cmd
}

func (m Model) activate(a int) (tea.Model, tea.Cmd) {
	m.detail.GotoTop()
	switch a {
	case 0:
		m.mode = "explain"
	case 1:
		raw := m.target.Command
		return m, func() tea.Msg {
			name := ""
			args := []string{}
			if runtime.GOOS == "darwin" {
				name = "pbcopy"
			} else if os.Getenv("WAYLAND_DISPLAY") != "" {
				name = "wl-copy"
			} else if os.Getenv("DISPLAY") != "" {
				name = "xclip"
				args = []string{"-selection", "clipboard"}
			}
			if name == "" {
				return copied{fmt.Errorf("no desktop clipboard detected")}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdin = strings.NewReader(raw)
			return copied{cmd.Run()}
		}
	case 2:
		m.mode = "run"
		m.confirm.SetValue("")
		m.status = "Type " + confirmation(m.target) + " to run · PgUp/PgDown review · Esc cancel"
		m.layout()
		cmd := m.confirm.Focus()
		return m, cmd
	case 3:
		m.beginEdit()
		return m, textinput.Blink
	}
	m.layout()
	return m, nil
}

func (m *Model) beginEdit() {
	m.mode = "edit"
	m.field = 0
	values := []string{m.target.Name, m.target.Description, m.target.Command, strings.Join(m.target.Tags, ", "), m.target.Host, m.target.Risk, m.target.Prerequisite, m.target.Explanation}
	m.fields = make([]textinput.Model, len(values))
	for i, v := range values {
		f := textinput.New()
		f.Prompt = ""
		f.SetValue(v)
		f.Width = max(10, m.width-10)
		m.fields[i] = f
	}
	m.fields[0].Focus()
	m.status = "Tab / Shift+Tab field · Ctrl+S save · Esc cancel"
}
func (m Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = ""
		m.status = "Edit cancelled."
		m.layout()
		return m, nil
	case "tab", "shift+tab", "enter":
		m.fields[m.field].Blur()
		d := 1
		if msg.String() == "shift+tab" {
			d = -1
		}
		m.field = (m.field + d + len(m.fields)) % len(m.fields)
		cmd := m.fields[m.field].Focus()
		return m, cmd
	case "ctrl+s":
		c := m.target
		c.Name = strings.TrimSpace(m.fields[0].Value())
		c.Description = m.fields[1].Value()
		c.Command = m.fields[2].Value()
		c.Tags = nil
		for _, t := range strings.Split(m.fields[3].Value(), ",") {
			if t = strings.TrimSpace(t); t != "" {
				c.Tags = append(c.Tags, t)
			}
		}
		c.Host = m.fields[4].Value()
		c.Risk = strings.ToUpper(strings.TrimSpace(m.fields[5].Value()))
		c.Prerequisite = m.fields[6].Value()
		c.Explanation = m.fields[7].Value()
		c.UpdatedAt = time.Now().UTC()
		if c.CreatedAt.IsZero() {
			c.CreatedAt = c.UpdatedAt
		}
		if c.Command != m.target.Command && c.Explanation == m.target.Explanation {
			c.Explanation = ""
		}
		if err := c.Validate(); err != nil {
			m.status = err.Error()
			return m, nil
		}
		cs := append([]library.Command(nil), m.all...)
		found := false
		for i := range cs {
			if cs[i].ID == c.ID {
				cs[i] = c
				found = true
			}
		}
		if !found {
			cs = append(cs, c)
		}
		if err := m.store.Save(context.Background(), cs); err != nil {
			m.status = "Save failed: " + err.Error()
			return m, nil
		}
		m.all = cs
		m.mode = ""
		m.status = "Saved."
		m.refresh()
		return m, nil
	}
	var cmd tea.Cmd
	m.fields[m.field], cmd = m.fields[m.field].Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.width < 36 || m.height < 14 {
		return "cmdlib\nTerminal too small: resize to at least 36×14.\nCtrl+C quits."
	}
	w := m.width - 4
	header := accent.Render("cmdlib") + muted.Render("  / by ai-mate.ai") + "\n" + muted.Render("host: "+safe(m.host))
	var body string
	if m.mode == "edit" {
		// Show a scrolling window of fields on short terminals.
		count := max(1, (m.height-12)/3)
		start := max(0, m.field-count+1)
		end := min(len(m.fields), start+count)
		lines := []string{accent.Render("Edit command · Ctrl+S save · Esc cancel")}
		for i := start; i < end; i++ {
			f := m.fields[i]
			f.Width = max(10, w-4)
			label := labels[i]
			if i == m.field {
				label = accent.Render(label)
			}
			lines = append(lines, label+"\n"+f.View())
		}
		body = strings.Join(lines, "\n\n")
	} else {
		body = m.detail.View()
		if m.mode == "" && m.width >= 76 {
			lw := max(24, m.width/3)
			lines := []string{accent.Render(fmt.Sprintf("Results (%d)", len(m.results))), ""}
			count := max(1, m.detail.Height-2)
			start := max(0, m.selected-count+1)
			for i := start; i < min(len(m.results), start+count); i++ {
				name := safe(m.results[i].Name)
				style := lipgloss.NewStyle().MaxWidth(lw - 2)
				if i == m.selected {
					lines = append(lines, accent.Render("› ")+style.Bold(true).Render(name))
				} else {
					lines = append(lines, "  "+style.Render(name))
				}
			}
			left := lipgloss.NewStyle().Width(lw).Height(m.detail.Height).Render(strings.Join(lines, "\n"))
			body = lipgloss.JoinHorizontal(lipgloss.Top, left, " │ ", body)
		}
	}
	actionLine := ""
	for i, a := range actions {
		label := "[" + a + "]"
		if i == m.action {
			label = accent.Render(label)
		} else {
			label = muted.Render(label)
		}
		actionLine += label + " "
	}
	if m.mode == "run" {
		actionLine = m.confirm.View()
	}
	if m.mode == "help" {
		actionLine = "←/→ tutorial step · e Explain · c Copy · r Run"
	}
	if m.mode == "edit" {
		actionLine = "Tab next field · Shift+Tab previous"
	}
	if m.mode == "explain" || m.mode == "copy" {
		actionLine = "Esc back · PgUp/PgDown scroll"
	}
	footer := muted.Render("↑↓ result  ←→ action  Enter select  / search  ? help")
	content := header + "\n" + m.search.View() + "\n" + strings.Repeat("─", w) + "\n" + body + "\n" + actionLine + "\n" + lipgloss.NewStyle().MaxWidth(w).Render(safe(m.status)) + "\n" + footer
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1).Width(w + 2).MaxWidth(m.width).MaxHeight(m.height).Render(content)
}
