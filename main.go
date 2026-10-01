package main

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
	"os"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dompy/cmdlib/internal/navi"
	"github.com/dompy/cmdlib/internal/storage"
	"github.com/dompy/cmdlib/internal/tui"
)

var version = "dev"
var commit = "unknown"

func renderTea() string {
	renderer := lipgloss.NewRenderer(os.Stdout)
	_, noColor := os.LookupEnv("NO_COLOR")
	if noColor || !term.IsTerminal(os.Stdout.Fd()) {
		renderer.SetColorProfile(termenv.Ascii)
	}

	main := renderer.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6D28D9", Dark: "#C4A5F5"}).
		Bold(true).
		Render("Go dress cmdlib in Lip Gloss.\nServe Bubble Tea with Bubbles.")
	ready := renderer.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#706677", Dark: "#A69BAE"}).
		Render("ready.")

	return renderer.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#A78BCA", Dark: "#8F7AAE"}).
		Padding(1, 3).
		Align(lipgloss.Center).
		Render(main + "\n\n" + ready)
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Printf("cmdlib %s (%s; %s/%s) — by ai-mate.ai\n", version, commit, runtime.GOOS, runtime.GOARCH)
		return nil
	}

	if len(args) == 1 && args[0] == "--tea" {
		fmt.Println(renderTea())
		return nil
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("cmdlib — by ai-mate.ai\nA terminal command library\n\n  cmdlib              Open the TUI\n  cmdlib export navi  Export to stdout\n  cmdlib --tea        Serve tea\n  cmdlib --version    Show version and build target\n\nSet CMDLIB_FILE to override the JSON library path.")
		return nil
	}
	if len(args) > 0 && !(len(args) == 2 && args[0] == "export" && args[1] == "navi") {
		return fmt.Errorf("unknown arguments; use cmdlib --help")
	}
	path, err := storage.DefaultPath()
	if err != nil {
		return err
	}
	store := storage.JSON{Path: path}
	cs, err := store.Load(context.Background())
	if err != nil {
		return err
	}
	if len(args) > 0 {
		n, err := navi.Export(os.Stdout, cs)
		if n > 0 {
			fmt.Fprintf(os.Stderr, "Skipped %d commands incompatible with Navi syntax.\n", n)
		}
		return err
	}
	_, err = tea.NewProgram(tui.New(store, cs, path), tea.WithAltScreen()).Run()
	return err
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cmdlib:", err)
		os.Exit(1)
	}
}
