package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type releaseCache struct {
	CheckedAt time.Time `json:"checked_at"`
	Releases  []Release `json:"releases"`
}

func writePrivate(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".cmdlib-maintenance-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
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

func fetchRelease(ctx context.Context, client *http.Client, name, repo, installed string) Release {
	r := Release{Name: name, Installed: installed, Status: "Aktuelle Release-Version unbekannt (keine Installation)."}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return r
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cmdlib-idle-maintenance")
	resp, err := client.Do(req)
	if err != nil {
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return r
	}
	var release struct {
		Tag        string `json:"tag_name"`
		URL        string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&release) != nil || release.Draft || release.Prerelease || release.Tag == "" {
		return r
	}
	if !strings.HasPrefix(release.URL, "https://github.com/"+repo+"/releases/") {
		return r
	}
	r.Latest, r.URL = clean(release.Tag), release.URL
	r.Status = "Offizieller Release zur Prüfung; Custom-Build bleibt unverändert."
	return r
}

func releases(ctx context.Context, path string, r Report) []Release {
	cachePath := path + ".release-cache.json"
	var cache releaseCache
	if b, err := os.ReadFile(cachePath); err == nil && json.Unmarshal(b, &cache) == nil {
		age := time.Since(cache.CheckedAt)
		if age >= 0 && age < 24*time.Hour {
			return cache.Releases
		}
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.Host != "api.github.com" || len(via) > 2 {
			return fmt.Errorf("unexpected release redirect")
		}
		return nil
	}}
	codexVersion := "nicht gefunden"
	for _, t := range r.Tools {
		if t.Name == "codex" && t.Available {
			codexVersion = t.Version
		}
	}
	cache = releaseCache{CheckedAt: time.Now().UTC(), Releases: []Release{
		fetchRelease(ctx, client, "cmdlib", "dompy/cmdlib", r.Version),
		fetchRelease(ctx, client, "Codex", "openai/codex", codexVersion),
		{Name: "FFmpeg", Status: "Installierte Version geprüft; neueste Version hier nicht online geprüft."},
	}}
	_ = writePrivate(cachePath, cache)
	return cache.Releases
}

// RunIdle skips atomically if any TUI/advisor owns a shared usage lock. It only
// writes private reports, never a library, executable, configuration or migration.
func RunIdle(ctx context.Context, path, version string, out io.Writer) error {
	return runIdle(ctx, path, out, func(idleCtx context.Context) Report {
		r, _ := Inspect(idleCtx, path, version, Discover, RunTool)
		r.Releases = releases(idleCtx, path, r)
		if idleCtx.Err() == nil {
			_ = runQueuedAdvisor(idleCtx, path, r, out)
		}
		return r
	})
}

func runIdle(ctx context.Context, path string, out io.Writer, inspect func(context.Context) Report) error {
	marker := path + ".foreground-request"
	if fi, err := os.Stat(marker); err == nil {
		if time.Since(fi.ModTime()) < 30*time.Second {
			fmt.Fprintln(out, "cmdlib wird gerade geöffnet; Hintergrundwartung übersprungen.")
			return nil
		}
		_ = os.Remove(marker) // Stale request from a terminated startup, never a live lock.
	}
	lock, err := acquireIdleLock(path)
	if errors.Is(err, ErrInUse) {
		fmt.Fprintln(out, "cmdlib ist geöffnet; Hintergrundwartung übersprungen.")
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	idleCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if _, err := os.Stat(marker); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	r := inspect(idleCtx)
	if idleCtx.Err() != nil {
		fmt.Fprintln(out, "cmdlib wird geöffnet; Hintergrundprüfung sofort beendet.")
		return nil
	}
	if err := writePrivate(path+".maintenance.json", r); err != nil {
		return fmt.Errorf("private Wartungsübersicht speichern: %w", err)
	}
	fmt.Fprintln(out, "Hintergrundprüfung abgeschlossen; private Übersicht aktualisiert. Keine Installation oder Migration; KI nur bei einmalig vorgemerktem Auftrag.")
	return nil
}
