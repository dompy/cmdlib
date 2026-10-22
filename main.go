package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dompy/cmdlib/internal/maintenance"
	"github.com/dompy/cmdlib/internal/navi"
	"github.com/dompy/cmdlib/internal/storage"
	"github.com/dompy/cmdlib/internal/tui"
)

var version = "dev"
var commit = "unknown"

func run(args []string) error {
	// The iSH installer checks spawning AND waiting before replacing a working build.
	if len(args) == 1 && args[0] == "--ish-check" {
		return exec.Command("/bin/sh", "-c", "exit 0").Run()
	}
	if len(args) == 1 && args[0] == "--version" {
		fmt.Printf("cmdlib %s (%s; %s/%s) — by ai-mate.ai\n", version, commit, runtime.GOOS, runtime.GOARCH)
		return nil
	}

	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("cmdlib — by ai-mate.ai\nA terminal command library\n\n  cmdlib                 Open the TUI\n  cmdlib doctor          Local checks + last background advice\n  cmdlib assist --queue  Queue one AI review for after closing\n  cmdlib assist          Deliberately open the Codex advisor\n  cmdlib maintain --idle Background checks only while closed\n  cmdlib export navi      Export to stdout\n  cmdlib --version       Show version and build target\n\nSet CMDLIB_FILE to override the JSON library path.")
		return nil
	}
	isDoctor := len(args) == 1 && args[0] == "doctor"
	isAssist := len(args) == 1 && args[0] == "assist"
	isQueue := len(args) == 2 && args[0] == "assist" && args[1] == "--queue"
	isIdle := len(args) == 2 && args[0] == "maintain" && args[1] == "--idle"
	if len(args) > 0 && !isDoctor && !isAssist && !isQueue && !isIdle && !(len(args) == 2 && args[0] == "export" && args[1] == "navi") {
		return fmt.Errorf("unknown arguments; use cmdlib --help")
	}
	path, err := storage.DefaultPath()
	if err != nil {
		return err
	}
	if isDoctor {
		r, _ := maintenance.Inspect(context.Background(), path, version, maintenance.Discover, maintenance.RunTool)
		maintenance.Print(os.Stdout, r)
		maintenance.PrintStoredAdvisor(os.Stdout, path)
		return nil
	}
	if isAssist {
		return maintenance.Assist(context.Background(), path, version, os.Stdin, os.Stdout, os.Stderr)
	}
	if isQueue {
		return maintenance.QueueAdvisor(path, os.Stdout)
	}
	if isIdle {
		return maintenance.RunIdle(context.Background(), path, version, os.Stdout)
	}
	var usageLock *maintenance.UsageLock
	if len(args) == 0 {
		usageLock, err = maintenance.AcquireUsageLock(path)
		if err != nil {
			return fmt.Errorf("Bibliothek für diese Sitzung sperren: %w", err)
		}
		defer usageLock.Close()
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
	_, err = tea.NewProgram(tui.New(store, cs, path), tea.WithAltScreen(), tea.WithReportFocus()).Run()
	return err
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cmdlib:", err)
		os.Exit(1)
	}
}
