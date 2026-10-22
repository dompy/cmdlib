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

	keybinding "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
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

var explainViolet = lipgloss.AdaptiveColor{Light: "#6D28D9", Dark: "#C4A5F5"}
var explainBorder = lipgloss.AdaptiveColor{Light: "#A78BCA", Dark: "#8F7AAE"}
var explainToken = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#4C1D95", Dark: "#F5F3FF"}).
	Background(lipgloss.AdaptiveColor{Light: "#EDE9FE", Dark: "#5B3B74"}).
	Bold(true).
	Padding(0, 1)
var explainTitle = lipgloss.NewStyle().Foreground(explainViolet).Bold(true)
var explainBox = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(explainBorder).
	Padding(0, 1)

var actions = []string{"Copy", "Run", "Edit"}
var organizationActions = []string{"Move", "Delete"}
var labels = []string{"Name", "Description", "Command", "Tags", "Host", "Risk", "Prerequisite", "Explanation", "Category"}

type Model struct {
	mobile                                       bool
	store                                        storage.Store
	all, results                                 []library.Command
	path, host, status, mode, returnMode         string
	selected, action, width, height, field, step int
	layer, detailField                           int
	fieldFocus, creating                         bool
	search, confirm                              textinput.Model
	fields                                       []textarea.Model
	detail                                       viewport.Model
	rows                                         []libraryRow
	expanded                                     map[string]bool
	moveDraft, undoAll                           []library.Command
	moveCategory                                 textinput.Model
	renameCategory                               textinput.Model
	renameOriginal, undoCategory                 string
	undoExpanded                                 map[string]bool
	undoLabel                                    string
	target                                       library.Command
	runGeneration                                uint64
	runDeadline                                  time.Time
	runFingerprint                               string
}
type copied struct{ err error }
type executed struct{ err error }

func New(s storage.Store, cs []library.Command, path string) Model {
	input := textinput.New()
	input.Prompt = "Search: "
	input.Placeholder = "Search"
	input.CharLimit = 256
	confirm := textinput.New()
	confirm.Prompt = "> "
	confirm.CharLimit = 256
	host, _ := os.Hostname()
	m := Model{store: s, all: append([]library.Command(nil), cs...), results: search.Filter(cs, ""), path: path, host: host, search: input, confirm: confirm, action: 1, expanded: map[string]bool{}, width: 90, height: 30, detail: viewport.New(50, 18), detailField: 2}
	m.mobile = os.Getenv("CMDLIB_MOBILE") == "1"
	m.loadViewState()
	m.refresh()
	return m
}
func (m Model) Init() tea.Cmd { return nil }
func (m Model) current() (library.Command, bool) {
	if m.selected < 0 || m.selected >= len(m.rows) || m.rows[m.selected].category != "" {
		return library.Command{}, false
	}
	return m.rows[m.selected].command, true
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
	m.rebuildRows()
	m.selected = min(m.selected, max(0, len(m.rows)-1))
	m.detail.GotoTop()
	m.layout()
}
func (m *Model) layout() {
	if m.mobile {
		m.mobileLayout()
		return
	}
	w := max(10, m.width-4)
	reserved := 2
	if m.search.Focused() || m.search.Value() != "" {
		reserved++
	}
	reserved += lipgloss.Height(lipgloss.NewStyle().Width(m.width - 4).Render(m.controls()))
	if m.status != "" {
		reserved += lipgloss.Height(lipgloss.NewStyle().Width(m.width - 4).Render(safe(m.status)))
	}
	if m.mode == "run" && !quickConfirmation(m.target) {
		reserved++
	}
	if (m.mode == "" || m.mode == "edit" && !m.creating) && m.width < 76 {
		reserved += 2
	}
	h := max(3, m.height-reserved)
	if (m.mode == "" || m.mode == "edit" && !m.creating) && m.width >= 76 {
		w -= m.listWidth() + 3
	}
	m.detail.Width = w
	m.detail.Height = h
	m.search.Width = max(5, m.width-6-len(m.search.Prompt))
	m.confirm.Width = max(5, m.width-10)
	m.moveCategory.Width = max(10, w-14)
	m.renameCategory.Width = max(10, w-14)
	if m.mode == "edit" {
		m.fields[m.field].SetWidth(max(5, w-4))
		m.fields[m.field].SetHeight(max(1, min(m.detail.Height-4, m.fields[m.field].LineCount()+1)))
	}
	content := lipgloss.NewStyle().Width(w).Render(m.content())
	m.detail.SetContent(content)
	if m.fieldFocus && (m.mode == "" || m.mode == "edit") {
		for i, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, accent.Render("› ")) {
				if i < m.detail.YOffset {
					m.detail.SetYOffset(i)
				}
				if i+2 >= m.detail.YOffset+h {
					m.detail.SetYOffset(i + 3 - h)
				}
				break
			}
		}
	}
}
func (m Model) commandDetails(c library.Command) string {
	lines := []string{accent.Render(safe(c.Name)), "", accent.Render(safe(c.Command))}
	if c.Host != "" {
		lines = append(lines, "", "Host: "+safe(c.Host)+"   Risk: "+riskStyle(c.Risk))
	}
	if c.Prerequisite != "" {
		lines = append(lines, "Requires: "+safe(c.Prerequisite))
	}
	return strings.Join(lines, "\n")
}
func riskStyle(r string) string {
	if r == "WRITE" || r == "DANGER" {
		return warning.Render(r)
	}
	return accent.Render(r)
}

type tokenMeaning struct {
	token   string
	meaning string
}

var toolMeanings = map[string]string{
	"tmutil": "Time Machine utility · macOS",
	"pwd":    "print working directory · shell",
	"ls":     "list directory contents · Unix",
	"date":   "date and time utility · Unix",
	"git":    "Git version-control tool",
	"printf": "formatted output utility · shell",
	"ssh":    "secure shell client",
	"arp":    "address-resolution table utility",
}

var argumentMeanings = map[string]map[string]string{
	"tmutil": {
		"status": "show current backup state",
	},
	"ls": {
		"-l":   "long listing",
		"-a":   "include hidden entries",
		"-h":   "human-readable sizes",
		"-lah": "long listing · hidden entries · readable sizes",
	},
	"date": {
		"-u": "use UTC",
	},
	"git": {
		"status":  "show working tree status",
		"--short": "use compact output",
	},
}

func commandBase(token string) string {
	if i := strings.LastIndex(token, "/"); i >= 0 {
		return token[i+1:]
	}
	return token
}

func meaningsFor(c library.Command) []tokenMeaning {
	fields := strings.Fields(c.Command)
	if len(fields) == 0 {
		return nil
	}
	base := commandBase(fields[0])
	var out []tokenMeaning
	if meaning, ok := toolMeanings[base]; ok {
		out = append(out, tokenMeaning{token: base, meaning: meaning})
	}
	for _, token := range fields[1:] {
		if meaning, ok := argumentMeanings[base][token]; ok {
			out = append(out, tokenMeaning{token: token, meaning: meaning})
		}
	}
	return out
}

func explanationRiskLabel(risk string) string {
	switch risk {
	case "READ":
		return "READ ONLY"
	case "CONNECT":
		return "CONNECTS"
	case "WRITE":
		return "WRITES"
	case "DANGER":
		return "DANGER"
	default:
		return risk
	}
}

func (m Model) explanationPopover(c library.Command) string {
	var lines []string
	if summary := strings.TrimSpace(c.Description); summary != "" {
		lines = append(lines, safe(summary))
	}

	meanings := meaningsFor(c)
	if len(meanings) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		for _, item := range meanings {
			lines = append(lines, explainToken.Render(safe(item.token))+muted.Render("  →  ")+safe(item.meaning))
		}
	} else if explanation := strings.TrimSpace(c.Explanation); explanation != "" {
		if len(lines) > 0 && explanation != strings.TrimSpace(c.Description) {
			lines = append(lines, "")
		}
		if explanation != strings.TrimSpace(c.Description) {
			lines = append(lines, safe(explanation))
		}
	}

	if len(lines) == 0 {
		lines = append(lines, muted.Render("No explanation recorded."))
	}
	lines = append(lines, "", explainTitle.Render(explanationRiskLabel(c.Risk)))

	content := explainTitle.Render("what it is") + "\n\n" + strings.Join(lines, "\n")
	width := max(24, min(m.detail.Width-2, 64))
	return explainBox.Width(width).Render(content)
}
func (m Model) content() string {
	if m.mode == "rename-category" {
		return m.renameCategoryContent()
	}
	if m.mode == "move" {
		return m.moveContent()
	}
	if m.mode == "" {
		if row, ok := m.selectedRow(); ok && row.category != "" {
			return ""
		}
	}
	c, ok := m.current()
	if m.mode != "" {
		c = m.target
		ok = c.ID != ""
	}
	if m.mode == "edit" {
		return m.fieldContent(m.target)
	}
	if m.mode == "help" {
		c = m.all[m.step]
		return fmt.Sprintf("Tutorial %d/%d · ←/→ command · ? explain · c Copy · r Run · Esc back\n\n", m.step+1, len(m.all)) + m.commandDetails(c) + "\n\n" + safe(c.Description) + "\n\n" + safe(c.Explanation) + "\n\nActions refer to the command above: ? explains it; Copy copies its exact text; Run opens a confirmation.\n\ncmdlib — by ai-mate.ai\n\nLibrary controls: Tab fields/list; ←/→ detail level; ↑/↓ field; e edit field; / search; ↑/↓ results; ←/→ actions; Enter activate; PgUp/PgDown scroll; n new; ? explain; h tutorial; q quit.\n\nSearch accepts multiple words and fuzzy abbreviations. Esc leaves search before q can quit. Risk and prerequisites are user-maintained labels, not a security check. Commands run locally via /bin/sh.\n\nLibrary: " + safe(m.path)
	}
	if !ok {
		return muted.Render("No matching commands.")
	}
	body := m.commandDetails(c)
	switch m.mode {
	case "delete":
		return warning.Render("Delete this command from the library?") + "\n\n" + body + "\n\nNothing will execute. After deletion, u restores it."
	case "explain":
		return accent.Render(safe(c.Name)) + "\n\n" + accent.Render(safe(c.Command)) + "\n\n" + m.explanationPopover(c)
	case "copy":
		return body + "\n\nClipboard unavailable. Select and copy the raw command above, or save the JSON/export locally. Nothing was executed.\n\nEsc back"
	case "run":
		note := "Review this command before starting."
		if c.Risk == "WRITE" {
			note = "This command is labeled WRITE and may change files or system state."
		}
		if c.Risk == "DANGER" {
			note = "DANGER: this command may irreversibly destroy data or disrupt services. Review every argument and your backups."
		}
		if c.Risk == "CONNECT" {
			note = "This opens a connection and may run commands or start processes remotely."
		}

		return warning.Render(note) + "\nExecution host: " + safe(m.host) + " (" + runtime.GOOS + ") via /bin/sh\nHost labels do not route execution.\n" + "\n" + body
	}
	return m.fieldContent(c)
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
	if m.mobile {
		if next, cmd, handled := m.updateMobile(msg); handled {
			return next, cmd
		}
	}
	switch msg := msg.(type) {
	case confirmationTick:
		return m.confirmTick(msg)
	case tea.BlurMsg:
		if m.mode == "run" {
			m.pauseCountdown("Countdown paused: window lost focus; confirm explicitly.")
		}
		return m, nil
	case tea.WindowSizeMsg:
		if m.mode == "run" {
			m.pauseCountdown("Countdown paused: window resized; confirm explicitly.")
		}
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
		m.action = 1
		m.status = "Command finished."
		if msg.err != nil {
			m.status = "Command failed: " + msg.err.Error()
		}
		m.layout()
		return m, nil
	case maintenanceFinished:
		m.mode = ""
		m.action = 1
		m.status = "Check finished."
		if msg.err != nil {
			m.status = "Check: " + msg.err.Error()
		}
		if latest, err := m.store.Load(context.Background()); err == nil {
			m.all = latest
		}
		m.refresh()
		return m, nil
	case queuedMaintenance:
		m.status = "KI-Prüfung vorgemerkt; nach dem Schließen im Hintergrund."
		if msg.err != nil {
			m.status = "KI-Prüfung konnte nicht vorgemerkt werden: " + msg.err.Error()
		}
		return m, nil
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			m.runGeneration++
			m.runDeadline = time.Time{}
			return m, tea.Quit
		}
		if msg.Paste && m.mode == "" && !m.search.Focused() {
			focus := m.search.Focus()
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.selected = 0
			m.action = 1
			m.refresh()
			return m, tea.Batch(focus, cmd)
		}
		if m.mode == "edit" {
			return m.updateEdit(msg)
		}
		if m.mode == "run" {
			return m.confirmKey(msg)
		}
		if m.mode == "move" {
			return m.updateMove(msg)
		}
		if m.mode == "rename-category" {
			return m.updateRenameCategory(msg)
		}
		if m.mode == "delete" {
			return m.updateDelete(msg)
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
			case "?", "c", "r":
				m.target = m.all[m.step]
				m.returnMode = "help"
				if key == "?" {
					m.mode = "explain"
					m.layout()
					return m, nil
				}
				a := 0
				if key == "r" {
					a = 1
				}
				return m.activate(a)
			}
		} else if m.mode != "" {
			if key == "esc" || key == "q" || (m.mode == "explain" && key == "?") {
				m.mode = m.returnMode
				m.layout()
				return m, nil
			}
		} else if m.search.Focused() {
			switch key {
			case "esc", "enter":
				m.search.Blur()
				m.action = 1
				m.layout()
				return m, nil
			case "up", "down":
				m.search.Blur()
				if key == "down" {
					m.selected = min(m.selected+1, len(m.rows)-1)
				} else {
					m.selected = max(0, m.selected-1)
				}
				m.selected = max(0, m.selected)
				m.action = 1
				m.layout()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.selected = 0
			m.action = 1
			m.refresh()
			return m, cmd
		} else {
			if key == "tab" && m.mode == "" {
				m.fieldFocus = !m.fieldFocus
				m.status = ""
				m.detail.GotoTop()
				m.layout()
				return m, nil
			}
			if m.fieldFocus && (key == "up" || key == "down" || key == "left" || key == "right") {
				m.navigateField(key)
				m.status = ""
				m.detail.GotoTop()
				m.layout()
				return m, nil
			}
			switch key {
			case "q":
				return m, tea.Quit
			case "/":
				cmd := m.search.Focus()
				return m, cmd
			case "esc":
				m.search.SetValue("")
				m.selected = 0
				m.action = 1
				m.refresh()
				return m, nil
			case "up":
				m.selected = max(0, m.selected-1)
				m.status = ""
				m.action = 1
				m.detail.GotoTop()
				m.layout()
				return m, nil
			case "down":
				m.selected = max(0, min(len(m.rows)-1, m.selected+1))
				m.status = ""
				m.action = 1
				m.detail.GotoTop()
				m.layout()
				return m, nil
			case "left":
				m.action = (m.action + len(actions) + len(organizationActions) - 1) % (len(actions) + len(organizationActions))
				return m, nil
			case "right":
				m.action = (m.action + 1) % (len(actions) + len(organizationActions))
				return m, nil
			case "?":
				if c, ok := m.current(); ok {
					m.target = c
					m.returnMode = ""
					m.mode = "explain"
					m.layout()
				}
				return m, nil
			case "h":
				if len(m.all) > 0 {
					m.mode = "help"
					m.step = 0
					m.layout()
				}
				return m, nil
			case "n":
				m.target = library.Command{ID: fmt.Sprintf("cmd-%d", time.Now().UnixNano()), Risk: "READ", Host: m.host}
				m.creating = true
				m.detailField = 0
				m.beginEdit()
				return m, textinput.Blink
			case "r", "c", "e", "m", "d", "delete", "f2":
				if row, ok := m.selectedRow(); ok && row.category != "" && (key == "e" || key == "f2") {
					return m.beginRenameCategory(row.category)
				}
				if key == "f2" {
					return m, nil
				}
				if c, ok := m.current(); ok {
					m.target = c
					m.returnMode = ""
					a := map[string]int{"r": 1, "c": 0, "e": 2, "m": 3, "d": 4, "delete": 4}[key]
					return m.activate(a)
				}
				return m, nil
			case "alt+up", "alt+down":
				if c, ok := m.current(); ok {
					m.target = c
					m.beginMove()
					direction := -1
					if key == "alt+down" {
						direction = 1
					}
					m.shiftMove(direction)
					return m.saveMove()
				}
				return m, nil
			case "u":
				return m.undoOrganization()
			case "p":
				return m.openMaintenance("doctor")
			case "a":
				return m.openMaintenance("assist")
			case "enter":
				if row, ok := m.selectedRow(); ok && row.category != "" {
					m.expanded[row.category] = !m.expanded[row.category]
					m.saveViewState()
					m.refresh()
					return m, nil
				}
				if c, ok := m.current(); ok {
					m.target = c
					m.returnMode = ""
					return m.activate(m.action)
				}
			}
		}
	}
	var cmd tea.Cmd
	if m.mode == "rename-category" {
		m.renameCategory, cmd = m.renameCategory.Update(msg)
		m.layout()
		return m, cmd
	}
	if m.mode == "edit" {
		m.fields[m.field], cmd = m.fields[m.field].Update(msg)
		m.layout()
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
	case 1:
		return m.beginConfirmation()
	case 2:
		m.beginEdit()
		return m, textinput.Blink
	case 3:
		m.beginMove()
		return m, nil
	case 4:
		m.mode = "delete"
		m.status = ""
	}
	m.layout()
	return m, nil
}

func (m *Model) beginEdit() {
	m.mode = "edit"
	m.field = m.detailField
	m.fieldFocus = true
	m.status = ""
	values := []string{m.target.Name, m.target.Description, m.target.Command, strings.Join(m.target.Tags, ", "), m.target.Host, m.target.Risk, m.target.Prerequisite, m.target.Explanation, m.target.Category}
	m.fields = make([]textarea.Model, len(values))
	for i, v := range values {
		f := textarea.New()
		f.Prompt = ""
		f.ShowLineNumbers = false
		f.CharLimit = 0
		f.KeyMap.InsertNewline = keybinding.NewBinding(keybinding.WithKeys("alt+enter", "ctrl+j"))
		f.SetValue(v)
		f.SetWidth(max(10, m.width-10))
		f.SetHeight(max(1, min(6, f.LineCount()+1)))
		m.fields[i] = f
	}
	m.fields[m.field].Focus()
	m.layout()
}
func (m Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = ""
		m.creating = false
		m.status = "Edit cancelled."
		m.layout()
		return m, nil
	case "tab", "shift+tab":
		if !m.creating {
			return m, nil
		}
		m.fields[m.field].Blur()
		d := 1
		if msg.String() == "shift+tab" {
			d = -1
		}
		m.field = (m.field + d + len(m.fields)) % len(m.fields)
		cmd := m.fields[m.field].Focus()
		m.layout()
		return m, cmd
	case "enter", "ctrl+s":
		c := m.target
		fields := []int{m.field}
		if m.creating {
			fields = []int{0, 1, 2, 3, 4, 5, 6, 7, 8}
		}
		for _, field := range fields {
			value := m.fields[field].Value()
			switch field {
			case 0:
				c.Name = strings.TrimSpace(value)
			case 1:
				c.Description = value
			case 2:
				c.Command = value
			case 3:
				c.Tags = nil
				for _, tag := range strings.Split(value, ",") {
					if tag = strings.TrimSpace(tag); tag != "" {
						c.Tags = append(c.Tags, tag)
					}
				}
			case 4:
				c.Host = value
			case 5:
				c.Risk = strings.ToUpper(strings.TrimSpace(value))
			case 6:
				c.Prerequisite = value
			case 7:
				c.Explanation = value
			case 8:
				c.Category = strings.TrimSpace(value)
			}
		}
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
		if !m.saveOrganization(cs, "edit") {
			return m, nil
		}
		m.mode = ""
		m.action = 1
		m.status = "Saved."
		m.creating = false
		m.selectCommand(c.ID)
		return m, nil
	}
	var cmd tea.Cmd
	m.fields[m.field], cmd = m.fields[m.field].Update(msg)
	m.layout()
	return m, cmd
}
