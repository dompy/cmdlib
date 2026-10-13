# cmdlib — by ai-mate.ai

A small, offline terminal command library for commands you use too rarely to remember.
Find a command, read its explanation, copy its exact text, or deliberately run it.
Built with Go, Bubble Tea, Bubbles, and Lip Gloss. [ai-mate.ai](https://ai-mate.ai) is the project attribution.

This personal UX build adds default Run, collapsible and renameable categories, move/delete/undo,
an owner-operated FFmpeg toolkit, idle-only background checks and a deliberately
queued Codex advisor. See [current controls](CMDLIB_BEDIENUNG.md) and
[native verification and installation state](CMDLIB_PRUEFERGEBNIS.md).

## Install

Download the matching archive and `checksums.txt` from [Releases](https://github.com/dompy/cmdlib/releases).
Archives cover macOS Intel / Apple Silicon and Linux amd64 / arm64.
Verify with `shasum -a 256` on macOS or `sha256sum` on Linux, extract, and put `cmdlib` on your PATH.

For a backed-up, user-local installation, download and inspect the versioned installer:

```sh
curl -fsSL https://raw.githubusercontent.com/dompy/cmdlib/v0.1.1/scripts/install.sh -o /tmp/install-cmdlib.sh
sh /tmp/install-cmdlib.sh v0.1.1
cmdlib --version
cmdlib
```

The installer verifies the archive against the release checksums and installs to `$HOME/.local/bin/cmdlib`.
Add that directory to PATH if needed. Existing executables or wrappers at that path and an existing
command library are backed up privately; the library is never changed by installation.
See [installation and rollback](docs/install.md).

To build from source with the Go version in `go.mod` or later:

```sh
git clone https://github.com/dompy/cmdlib.git
cd cmdlib
go build -trimpath -o cmdlib .
./cmdlib
```

## Use

Search matches names, command text, descriptions, tags, host labels, and risk labels.
Multiple words must all match; fuzzy abbreviations are supported.
The selected command and details stay visible in the wide terminal layout.

| Key | Action |
| --- | --- |
| `/` | Focus search; normal typing and pasted text work |
| `↑` / `↓` | Select a result |
| `←` / `→` | Select Copy / Run / Edit |
| `Enter` | Activate the selected action; Run is selected by default on command navigation, category headers expand/collapse |
| `?` | Toggle the selected command's contextual explanation |
| `PgUp` / `PgDown` | Scroll details |
| `n` | Add a command |
| `m` / Alt+↑ / Alt+↓ | Move a command; the dialog also edits its category |
| `d` / Delete | Review and confirm deletion |
| `e` / F2 on a category | Rename the whole category; Enter saves, Esc cancels |
| `u` | Undo the last move/delete/category rename in this session; private backups survive restart |
| `p` | Local checks and stored background/AI results |
| `a` | Queue one Codex metadata-only check for after cmdlib closes |
| `h` | Guided tutorial, one visible command per step |
| `Esc` | Leave search, clear search, or cancel the active view |
| `q` / `Ctrl+C` | Quit (`q` is normal text while searching) |

In the editor, use Tab / Shift+Tab to select a field, Ctrl+S to save, and Esc to cancel.
Changing command text clears an unchanged explanation so stale documentation does not silently remain.
Tutorial controls are ←/→ for steps, `?` for contextual explanation, `c` Copy, and `r` Run.

Selection and copying never execute a command. Run shows the actual local execution machine and
exact command before confirmation. For READ / CONNECT, press Enter, Space, `y` or `1`
to start; Esc, `n`, `0`, Backspace or Delete cancels. A pasted approval is ignored.
WRITE still requires `WRITE` and DANGER requires `RUN <id>` followed by Enter.
Neither category permits shortcut approval or automatic execution.

Optional local countdowns require a separate `<library-path>.run-policy.json` file.
With version 1, delay_ms 2500 and a matching approved_fingerprints entry, consciously
selecting Run opens a visible 2.5-second countdown for that exact READ / CONNECT entry.
Merely selecting, searching or copying never arms it. Changes to the command, ID,
risk, host or prerequisite invalidate that approval. New commands have no countdown.
Focus loss (where reported by the terminal), window resizing, review keys, pasted
text or a delayed event loop pause the countdown; explicit confirmation remains
available. A command too large to review on screen does not count down.
This user-maintained opt-in is not a guarantee that a command or referenced script
is safe; code reached by a command can change independently of its text.
See `docs/quick-confirmation.md` for policy format and local rollback.

Esc cancels. Commands run locally through `/bin/sh` with your permissions
and current working directory. The explicit command may itself open a remote connection;
a Host label does not route execution. READ and other labels are authored metadata, not safety checks.
Confirmed run attempts are recorded before execution; the count includes failed attempts.

Copy uses `pbcopy` on macOS, `wl-copy` on Wayland, or `xclip` on X11.
Without a working clipboard, the exact command is shown for manual copying.
`?` opens a compact local explanation beside the selected command. Known command tokens get concise built-in hints; stored explanation text remains the fallback. No AI service or network connection is involved.

## Data and privacy

Your library is a local JSON array. Default locations:

- macOS: `~/Library/Application Support/cmdlib/commands.json`
- Linux: `$XDG_CONFIG_HOME/cmdlib/commands.json`, or `~/.config/cmdlib/commands.json`
- Override: `CMDLIB_FILE=/path/to/commands.json cmdlib`

Five harmless examples are created only when the file is absent. Existing libraries, including empty
ones, are preserved. Writes use a private temporary file and atomic replacement. There is no account,
telemetry, server synchronization, credential storage, or LLM API requirement. Remote sync remains
future work. The optional advisor uses the owner's existing ChatGPT Codex CLI session;
queued checks run only while cmdlib is closed and never modify commands or install updates.

Back up the JSON file before editing or upgrading. For example on macOS:

```sh
cp "$HOME/Library/Application Support/cmdlib/commands.json" "$HOME/Library/Application Support/cmdlib/commands.json.bak"
chmod 600 "$HOME/Library/Application Support/cmdlib/commands.json.bak"
```

Treat your library and exports as private: shell commands can contain secrets. Do not commit them,
upload them with bug reports, or include them in screenshots. cmdlib does not encrypt the file.

## Navi export

```sh
cmdlib export navi > commands.cheat
```

Export writes to stdout and never executes commands. Commands containing newlines, carriage returns,
`<` or `>`, or beginning (after whitespace) with `#`, `%`, or `$` are skipped because Navi may interpret
those as syntax. The skipped count is reported on stderr. Explanations, prerequisites, and usage
history are not exported. Import is not implemented. Navi has its own execution behavior; cmdlib's
confirmation does not apply once commands are exported.

## Development

```sh
gofmt -w main.go main_test.go internal
go test ./...
go vet ./...
go build -trimpath -o cmdlib .
python3 scripts/smoke.py ./cmdlib
```

CI uses hosted runners for native tests on all four release targets and checks formatting, race tests,
vet, dependency vulnerabilities, and secrets in Git history. See [release instructions](docs/releases.md),
[contributing](CONTRIBUTING.md), and [security](SECURITY.md). Original code is MIT licensed;
[third-party notices](THIRD_PARTY_NOTICES.md) retain dependency license terms.
