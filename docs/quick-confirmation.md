# Quick confirmation — local extension

This build adds Enter, Space, y and 1 as single-key approvals for READ/CONNECT
commands after the user deliberately selects Run. n, 0, Esc, Backspace and Delete
cancel. WRITE and DANGER retain their exact typed confirmations. Nothing changes
in the Studio editor, SSH permissions, Codex approval policy, or website access.

## Optional countdown

A private file next to the selected library has the path
`<library-path>.run-policy.json`. Example with no approvals:

```json
{"version":1,"delay_ms":2500,"approved_fingerprints":[]}
```

Each approved value is the lowercase SHA-256 hex digest of Go JSON-marshaled
`[]string{ID, Command, Risk, Host, Prerequisite}` for one deliberately reviewed entry.
There is no blanket approval for a risk label and no timer for new/changed entries.
Only READ/CONNECT entries can use this policy. The policy is loaded at confirmation
and checked again at expiry. Removing the file disables countdowns, not shortcuts.

Countdown starts only after Run is deliberately opened, with the exact command and
actual local execution machine visible. The whole review must fit on screen.
It is invalidated on cancel, execution, changed target, resize, focus loss reported
by the terminal, any non-approval key, pasted text, and excessive scheduling delay.
Missing or malformed policy means no countdown. Command labels are not safety checks.

The actual command is reloaded from the library before execution; changes in another
window prevent execution and require a new review. Usage is persisted before launch.
The execution state is changed before returning the process command, so stale timer
messages cannot schedule a second invocation. No test runs a personal command.

## Local installation

Only the user's local executable is replaced after native tests. The previous
binary and library bytes are backed up privately. Public GitHub files are not pushed
or released. The personal countdown policy stays outside source and version control.
Already-open cmdlib instances continue running their old binary and should be closed
normally before reopening; do not kill unrelated terminals or unsaved editors.

## Known boundaries

A command can call changing scripts or external executables; fingerprints do not
freeze their implementation. Focus handling depends on terminal support. This is
local command-launch convenience, not a security approval for actions inside Codex.
The unrelated Studio mouse-positioning issue is unchanged by this patch.
