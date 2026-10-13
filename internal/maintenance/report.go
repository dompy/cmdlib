// Package maintenance provides local checks and a deliberately opened AI advisor.
// Neither diagnosis nor idle maintenance executes commands from the library.
package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dompy/cmdlib/internal/library"
)

type Entry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
	Risk     string `json:"risk"`
	Host     string `json:"host"`
}

type Duplicate struct {
	Kind    string  `json:"kind"`
	Entries []Entry `json:"entries"`
}

type Tool struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
}

type Release struct {
	Name      string `json:"name"`
	Installed string `json:"installed,omitempty"`
	Latest    string `json:"latest,omitempty"`
	URL       string `json:"url,omitempty"`
	Status    string `json:"status"`
}

type Report struct {
	CheckedAt  time.Time   `json:"checked_at"`
	Version    string      `json:"cmdlib_version"`
	Platform   string      `json:"platform"`
	Valid      bool        `json:"library_valid"`
	Count      int         `json:"command_count"`
	Categories []string    `json:"categories"`
	Issues     []string    `json:"issues"`
	Duplicates []Duplicate `json:"duplicates"`
	Tools      []Tool      `json:"tools"`
	Releases   []Release   `json:"releases,omitempty"`
}

// Runner only runs hard-coded version and compatibility queries, never library text.
type Runner func(context.Context, string, ...string) (string, error)

func RunTool(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 200 * time.Millisecond
	b, err := cmd.CombinedOutput()
	return string(b), err
}

func Discover(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}
	// LaunchAgents have a small PATH; support the normal private and Homebrew paths.
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return "", err
	}
	for _, base := range []string{home + "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"} {
		p := base + "/" + name
		if fi, e := os.Stat(p); e == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
			return p, nil
		}
	}
	return "", err
}

var secretLike = regexp.MustCompile(`(?i)(sk-[a-z0-9_-]+|bearer\s+[^\s]+|[a-z0-9_-]{40,})`)

func clean(s string) string {
	s = secretLike.ReplaceAllString(s, "[redacted]")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	r := []rune(strings.TrimSpace(s))
	if len(r) > 160 {
		r = append(r[:160], '…')
	}
	return string(r)
}

func metadata(c library.Command) Entry {
	return Entry{ID: clean(c.ID), Name: clean(c.Name), Category: clean(c.Category), Risk: clean(c.Risk), Host: clean(c.Host)}
}

// ReadLibrary is intentionally not storage.Load: diagnosis must never create seeds.
func ReadLibrary(path string) ([]library.Command, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 8*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 8*1024*1024 {
		return nil, fmt.Errorf("library exceeds 8 MiB diagnosis limit")
	}
	var cs []library.Command
	if err := json.Unmarshal(b, &cs); err != nil {
		return nil, fmt.Errorf("library is not a JSON command array")
	}
	return cs, nil
}

func Inspect(ctx context.Context, path, version string, lookup func(string) (string, error), runner Runner) (Report, []library.Command) {
	r := Report{CheckedAt: time.Now().UTC(), Version: clean(version), Platform: runtime.GOOS + "/" + runtime.GOARCH, Valid: true}
	cs, err := ReadLibrary(path)
	if err != nil {
		r.Valid = false
		if os.IsNotExist(err) {
			r.Issues = append(r.Issues, "Bibliothek fehlt; Diagnose hat keine Beispieldatei angelegt.")
		} else {
			r.Issues = append(r.Issues, "Bibliothek nicht lesbar oder ungültiges JSON; Datei bleibt unverändert.")
		}
	}
	r.Count = len(cs)
	ids, categories := map[string]bool{}, map[string]bool{}
	fingerprints, names := map[string][]Entry{}, map[string][]Entry{}
	for i, c := range cs {
		if c.Validate() != nil || ids[c.ID] {
			r.Valid = false
			r.Issues = append(r.Issues, fmt.Sprintf("Eintrag %d: Pflichtfelder/Risikowert ungültig oder ID doppelt.", i+1))
		}
		ids[c.ID] = true
		category := clean(c.Category)
		if category == "" {
			category = "Allgemein"
		}
		categories[category] = true
		// Preserve internal whitespace: shell text is not safe to rewrite heuristically.
		fp := sha256.Sum256([]byte(strings.TrimSpace(c.Command) + "\x00" + strings.TrimSpace(c.Host)))
		fingerprints[hex.EncodeToString(fp[:])] = append(fingerprints[hex.EncodeToString(fp[:])], metadata(c))
		name := strings.ToLower(strings.TrimSpace(c.Name))
		names[name] = append(names[name], metadata(c))
	}
	for c := range categories {
		r.Categories = append(r.Categories, c)
	}
	sort.Strings(r.Categories)
	for _, group := range []struct {
		kind string
		m    map[string][]Entry
	}{{"gleicher Befehl und Host", fingerprints}, {"gleicher Name", names}} {
		keys := make([]string, 0, len(group.m))
		for k := range group.m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if len(group.m[k]) > 1 {
				r.Duplicates = append(r.Duplicates, Duplicate{Kind: group.kind, Entries: group.m[k]})
			}
		}
	}
	queries := []struct {
		name string
		args []string
	}{{"ffmpeg", []string{"-version"}}, {"ffprobe", []string{"-version"}}, {"codex", []string{"--version"}}, {"git", []string{"--version"}}, {"ssh", []string{"-V"}}, {"python3", []string{"--version"}}, {"brew", []string{"--version"}}}
	for _, q := range queries {
		p, err := lookup(q.name)
		t := Tool{Name: q.name, Available: err == nil}
		if err == nil {
			toolCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			out, runErr := runner(toolCtx, p, q.args...)
			cancel()
			if runErr == nil {
				t.Version = clean(strings.SplitN(out, "\n", 2)[0])
			} else {
				t.Version = "Versionsabfrage nicht verfügbar"
			}
		}
		r.Tools = append(r.Tools, t)
	}
	if runtime.GOOS != "darwin" {
		r.Issues = append(r.Issues, "Mac-Mikrofon-Befehle mit avfoundation sind nur unter macOS kompatibel.")
	} else if p, err := lookup("ffmpeg"); err == nil {
		toolCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		out, runErr := runner(toolCtx, p, "-hide_banner", "-devices")
		cancel()
		if runErr != nil || !strings.Contains(out, "avfoundation") {
			r.Issues = append(r.Issues, "Dieses FFmpeg meldet kein avfoundation-Eingabegerät; Mac-Mikrofonaufnahme prüfen.")
		}
	}
	return r, cs
}

func Print(w io.Writer, r Report) {
	fmt.Fprintf(w, "cmdlib · Ordnung & Kompatibilität\nVersion: %s · %s\n", r.Version, r.Platform)
	state := "gültig"
	if !r.Valid {
		state = "prüfen"
	}
	fmt.Fprintf(w, "Bibliothek: %s · %d Commands · %d Kategorien\n", state, r.Count, len(r.Categories))
	for _, issue := range r.Issues {
		fmt.Fprintln(w, "Hinweis:", issue)
	}
	if len(r.Duplicates) == 0 {
		fmt.Fprintln(w, "Keine gleichen Befehle/Hosts oder doppelten Namen gefunden.")
	}
	for _, d := range r.Duplicates {
		fmt.Fprintf(w, "Doppelt (%s):", d.Kind)
		for _, e := range d.Entries {
			fmt.Fprintf(w, " %s [%s]", e.Name, e.ID)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "\nInstallierte Werkzeuge:")
	for _, t := range r.Tools {
		state := "nicht gefunden"
		if t.Available {
			state = t.Version
		}
		fmt.Fprintf(w, "  %-9s %s\n", t.Name, state)
	}
	for _, release := range r.Releases {
		fmt.Fprintf(w, "Release %s: %s", release.Name, release.Status)
		if release.Latest != "" {
			fmt.Fprintf(w, " · %s", release.Latest)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "\nKeine Commands ausgeführt, nichts installiert, umsortiert oder migriert.\nKI-Beratung: cmdlib assist (bewusst öffnen; vorhandener Codex-Zugang).")
}
