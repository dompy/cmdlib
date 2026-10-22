# cmdlib — by ai-mate.ai

A small, offline terminal command library for commands you use too rarely to remember.
Find a command, read its explanation, copy its exact text, or deliberately run it.
Built with Go, Bubble Tea, Bubbles, and Lip Gloss. [ai-mate.ai](https://ai-mate.ai) is the project attribution.

This focused UI build restores the tested local category and maintenance features,
then adds command/description/details layers and direct editing of the selected field.
See [focused controls and previews](docs/focused-ui.md). The Python helper in PR #3
is separate work and is not changed here.

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

For local use on iPhone in iSH, see [iSH installation and mobile controls](docs/ish.md).

## Use

The selected command is prominent; other results are muted. Description and metadata
are revealed only when requested. Wide terminals retain the search/detail layout;
narrow terminals show the selected row above its detail.

| Key | Action |
| --- | --- |
| `/` | Search by name, command, category, tags, host or risk; ordinary paste works |
| `↑` / `↓` | Select a result; in field focus, select a field |
| `Tab` | Switch between results and field focus |
| `←` / `→` | Choose an action; in field focus, choose Command / Description / Details |
| `e` | Edit the marked field in place; Enter saves and Esc discards |
| `Enter` | Activate the selected action; category headers expand/collapse |
| `r` / `c` | Review a run / copy the exact command |
| `?` / `h` | Contextual explanation / tutorial and full controls |
| `n` | New command; Tab switches fields, Enter saves, Esc cancels |
| `m` / Alt+↑ / Alt+↓ | Move a command |
| `d` / Delete | Review deletion; Enter confirms, Esc cancels |
| `e` / F2 on a category | Rename the whole category |
| `u` | Undo the last organization change or field edit in this session |
| `p` / `a` | Local checks / queue one Codex metadata-only review |
| `Esc` | Leave search or clear the query; cancel an open view |
| `q` / Ctrl+C | Quit; q remains ordinary text while searching or editing |

Field editing uses the existing storage format, validation, backups and conflict
checks. Changing command text clears an unchanged explanation. No existing personal
library is migrated or replaced. On creation, fields start with the existing defaults.

Selection and copying never execute. Run always opens a separate review showing
the real command and actual local execution machine. READ / CONNECT accept an explicit
Enter, Space, y or 1; WRITE requires typing WRITE; DANGER requires RUN followed by the
command ID. Esc cancels. Pasted approval is ignored. This build has no automatic
execution, even when a legacy `.run-policy.json` exists.

Commands run through `/bin/sh` with the current user's permissions and working directory.
A Host label does not route execution, and risk labels are authored metadata, not
security checks. The confirmed run is recorded before execution; failed attempts also
count. Clipboard handling remains pbcopy on macOS, wl-copy on Wayland or xclip on X11,
with an exact-text fallback if no clipboard is available.

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
python3 scripts/focused_ui_smoke.py ./cmdlib
```

CI uses hosted runners for native tests on all four release targets and checks formatting, race tests,
vet, dependency vulnerabilities, and secrets in Git history. See [release instructions](docs/releases.md),
[contributing](CONTRIBUTING.md), and [security](SECURITY.md). Original code is MIT licensed;
[third-party notices](THIRD_PARTY_NOTICES.md) retain dependency license terms.
