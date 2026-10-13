package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type AIRequest struct {
	RequestedAt time.Time  `json:"requested_at"`
	AttemptedAt *time.Time `json:"attempted_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Status      string     `json:"status"`
}

func QueueAdvisor(path string, out io.Writer) error {
	lock, err := AcquireUsageLock(path)
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := Discover("codex"); err != nil {
		return fmt.Errorf("Codex CLI nicht gefunden; lokale Hintergrundprüfung bleibt verfügbar")
	}
	if b, err := os.ReadFile(path + ".assistant-request.json"); err == nil {
		var previous AIRequest
		if json.Unmarshal(b, &previous) == nil && previous.Status == "pending" {
			fmt.Fprintln(out, "Eine KI-Prüfung ist bereits vorgemerkt; keine doppelte Anfrage. Sie läuft nach dem Schließen von cmdlib.")
			return nil
		}
	}
	r := AIRequest{RequestedAt: time.Now().UTC(), Status: "pending"}
	if err := writePrivate(path+".assistant-request.json", r); err != nil {
		return err
	}
	fmt.Fprintln(out, "Eine KI-Prüfung vorgemerkt. Sie läuft erst nach dem Schließen von cmdlib im Hintergrund, einmalig mit deinem vorhandenen Codex-Kontingent. Keine Änderung, Installation oder Migration; bei Unterbrechung kein automatischer Neuversuch.")
	return nil
}

func PrintStoredAdvisor(out io.Writer, path string) {
	if b, err := os.ReadFile(path + ".maintenance.json"); err == nil && len(b) <= 1024*1024 {
		var report Report
		if json.Unmarshal(b, &report) == nil {
			fmt.Fprintf(out, "\nLetzte Hintergrundprüfung: %s\n", report.CheckedAt.Local().Format("02.01.2006 15:04"))
			for _, release := range report.Releases {
				fmt.Fprintf(out, "  %s: %s %s\n", clean(release.Name), clean(release.Status), clean(release.Latest))
			}
		}
	}
	b, err := os.ReadFile(path + ".assistant-request.json")
	if err != nil || len(b) > 16*1024 {
		return
	}
	var request AIRequest
	if json.Unmarshal(b, &request) != nil {
		return
	}
	status := "Nicht abgeschlossen; kein automatischer Neuversuch. Mit a erneut bewusst vormerken."
	switch request.Status {
	case "pending":
		status = "Vorgemerkt; startet erst nach dem Schließen von cmdlib."
	case "attempted":
		status = "Gestartet oder Status unklar; wird nicht automatisch erneut gestartet."
	case "completed":
		status = "Abgeschlossen; Vorschläge ändern nichts automatisch."
	case "invalid-library-no-model-call", "failed-before-model-call":
		status = "Vor dem KI-Aufruf gestoppt; Bibliothek/CLI prüfen, danach neu vormerken."
	}
	fmt.Fprintln(out, "KI-Prüfung:", status)
	if request.Status != "completed" {
		return
	}
	f, err := os.Open(path + ".assistant.txt")
	if err != nil {
		return
	}
	defer f.Close()
	advice, err := io.ReadAll(io.LimitReader(f, 16*1024))
	if err != nil {
		return
	}
	text := strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' || r == 127 {
			return -1
		}
		return r
	}, string(advice))
	fmt.Fprintln(out, "\nKI-Vorschläge:\n"+text)
}

func runQueuedAdvisor(ctx context.Context, path string, report Report, out io.Writer) error {
	return runQueued(ctx, path, report, out, func(ctx context.Context, prompt string) (string, error) {
		cli, err := Discover("codex")
		if err != nil {
			return "", err
		}
		cwd, err := os.MkdirTemp("", "cmdlib-idle-advisor-")
		if err != nil {
			return "", err
		}
		defer os.Remove(cwd)
		command, err := AdvisorCommand(ctx, cli, cwd, prompt, RunTool)
		if err != nil {
			return "", err
		}
		execHelp, err := RunTool(ctx, cli, "exec", "--help")
		if err != nil || !strings.Contains(execHelp, "--ignore-user-config") || !strings.Contains(execHelp, "--ephemeral") {
			return "", fmt.Errorf("installed CLI does not support isolated ephemeral advisor")
		}
		outputPath := filepath.Join(cwd, "result.txt")
		defer os.Remove(outputPath)
		if err := os.WriteFile(outputPath, nil, 0600); err != nil {
			return "", err
		}
		// Global flags precede exec. Prompt via stdin avoids a giant process argument.
		args := append([]string{}, command.Args[1:len(command.Args)-1]...)
		args = append(args, "exec", "--ignore-user-config", "--ephemeral", "--skip-git-repo-check", "--color", "never", "--output-last-message", outputPath, "-")
		cmd := exec.CommandContext(ctx, cli, args...)
		cmd.Env = advisorEnvironment()
		cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = cwd, strings.NewReader(prompt), io.Discard, io.Discard
		cmd.WaitDelay = 200 * time.Millisecond
		if err := cmd.Run(); err != nil {
			return "", err
		}
		f, err := os.Open(outputPath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, 64*1024))
		if err != nil || strings.TrimSpace(string(b)) == "" {
			return "", fmt.Errorf("Codex returned no advisor result")
		}
		return string(b), nil
	})
}

// runQueued consumes one explicit owner request before any model invocation.
// Failed, cancelled or uncertain attempts are never automatically retried.
func runQueued(ctx context.Context, path string, report Report, out io.Writer, analyze func(context.Context, string) (string, error)) error {
	requestPath := path + ".assistant-request.json"
	b, err := os.ReadFile(requestPath)
	if os.IsNotExist(err) {
		return nil
	}
	var request AIRequest
	if err != nil || json.Unmarshal(b, &request) != nil || request.Status != "pending" {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	now := time.Now().UTC()
	request.AttemptedAt, request.Status = &now, "attempted"
	if err := writePrivate(requestPath, request); err != nil {
		return err
	}
	cs, err := ReadLibrary(path)
	if err != nil || !report.Valid {
		request.Status = "invalid-library-no-model-call"
	} else {
		prompt, promptErr := AdvisorPrompt(report, cs)
		if promptErr != nil {
			request.Status = "failed-before-model-call"
		} else {
			aiCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			text, analyzeErr := analyze(aiCtx, prompt)
			cancel()
			if analyzeErr != nil {
				request.Status = "interrupted-or-failed-no-auto-retry"
			} else if err := writePrivateText(path+".assistant.txt", text); err != nil {
				request.Status = "result-save-failed-no-auto-retry"
			} else {
				request.Status = "completed"
			}
		}
	}
	finished := time.Now().UTC()
	request.FinishedAt = &finished
	if err := writePrivate(requestPath, request); err != nil {
		return err
	}
	fmt.Fprintf(out, "Vorgemerkte KI-Prüfung: %s. Ergebnis bei Erfolg privat als .assistant.txt gespeichert.\n", request.Status)
	return nil
}

func writePrivateText(path, text string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".cmdlib-advice-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
