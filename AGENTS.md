# cmdlib

## Product
A small terminal command library: find, understand, copy, and deliberately
run infrequently used commands. Public identity: cmdlib — by ai-mate.ai.
Keep the executable name `cmdlib`. Preserve the working personal installation.

## Code
Go with Bubble Tea, Bubbles, and Lip Gloss. The executable entry point is
`main.go` at the repository root; do not assume a `cmd/cmdlib` build target.
`internal/library`, `storage`, `search`, `navi`, and `tui` hold the main features.
Choose simple, idiomatic solutions; justify dependencies by their actual value.

## Behavior
Preserve the keyboard-first search/detail layout and normal text-input/paste behavior.
Actions must visibly refer to their selected command. Selection and copying never execute.
Show the actual execution machine and command before explicit confirmation.
Risk and host labels are metadata, not proof of safety or remote-routing instructions.
Preserve concise explanations and the tutorial's one-visible-command-per-step design.

## Data
Keep personal libraries, histories, credentials, and infrastructure details out of
public source, examples, screenshots, binaries, test fixtures, and release assets.
Never overwrite an existing user's library with seed data. Migrations need backup
and rollback. Retain the current on-disk format and locations unless migration is necessary.
Document only implemented features; do not claim server sync exists because an interface does.

## Validation
Format changed Go source with gofmt. Normally run `go test ./...` and `go vet ./...`.
Build the executable from `.` and verify relevant supported targets.
Use isolated test libraries and harmless commands; never run personal operational
commands merely to test the application.
Test the actual installation path, architecture, fresh-shell startup, command discovery,
confirmation/cancellation, and data preservation when access permits.
A cross-build alone is not a native runtime or installation test.
