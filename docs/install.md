# Installation and rollback

Release downloads are available at https://github.com/dompy/cmdlib/releases.
Choose `darwin_amd64` for Intel Macs, `darwin_arm64` for Apple Silicon,
`linux_amd64` for x86 Linux, or `linux_arm64` for ARM Linux.
macOS binaries have no Developer ID signature or Apple notarization. The Go arm64 linker may add an
ad-hoc signature; this is not a publisher identity. Follow your organization's macOS policy.
Do not disable Gatekeeper globally. Source builds are an alternative.

The user-local installer is `scripts/install.sh`; pass an explicit version such as `v0.1.0`.
It downloads over HTTPS, verifies SHA-256, runs informational flags on the staged binary, makes private
backups, and atomically replaces `$HOME/.local/bin/cmdlib`. It uses no sudo and edits no shell startup file.
An existing system-wide binary remains untouched. If `$HOME/.local/bin` precedes it on PATH,
the new executable will be selected. Check in a fresh login shell with `command -v cmdlib` and `cmdlib --version`.

Backups are stored in `~/.local/share/cmdlib/backups/<timestamp>-<pid>/` (printed by the installer).
If an executable or wrapper existed there, restore it with:

```sh
sh "$HOME/.local/share/cmdlib/backups/<timestamp>-<pid>/rollback.sh"
```

The backup may also contain `commands.json`, copied from `CMDLIB_FILE` or the platform's default path.
Rollback restores the executable only; data remains untouched. If manually restoring data is necessary,
first preserve the current library, close cmdlib, and copy the private backup to the correct library path.
Do not replace a library with seed data. Existing JSON format and locations are unchanged.

Checksums detect corruption but do not independently authenticate a compromised release account.
