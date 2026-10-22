# iSH on iPhone

The normal release archives do not run in iSH. Build the dedicated Linux/386 target:

```sh
python3 scripts/build-ish.py --output /tmp/cmdlib-linux-386
python3 scripts/package-ish.py --binary /tmp/cmdlib-linux-386 --output /tmp/install-cmdlib-ish.sh
```

Transfer the installer through your existing authenticated connection, then run it
**locally in iSH**, not on the remote machine. It checks the architecture and starts
and waits for a harmless child process before replacing the installed executable.
Existing executables, the command library and an edited login profile are backed up.
Existing command JSON is kept. An optional `--seed /private/path/commands.json` fills
only a missing library; do not commit or publicly distribute personalized installers.

The iSH launcher enables a single-column mobile interface. A number opens the
matching command on the visible page. Enter reviews the current selection.
On the review screen Enter/Space/1 starts READ or CONNECT; 0/Esc cancels.
WRITE and DANGER keep their existing typed confirmation.

- N: add a command with Name → Command → type selection; no Tab or Ctrl+S needed.
- E: edit Name, Command, Description or type with a number.
- /: search. Arrow keys change selection; left/right change pages.
- Q or 0: return to the local shell.
- After a CONNECT command ends, the menu returns immediately.

These are terminal controls operated with number keys/Enter, not native iOS touch
buttons. No terminal mouse reporting is enabled, so iSH text selection is preserved.

The installer adds one guarded menu launch to `~/.profile`. Opening a new interactive
login shell opens cmdlib; Q returns to the shell without a restart loop. Noninteractive
shells and SSH sessions do not autostart it. `CMDLIB_AUTOSTART=0` disables this behavior.
The launcher itself prevents nested autostarts.

## Compatibility regression

iSH can signal SIGSYS for unimplemented Linux calls instead of returning ENOSYS.
Go's optional pidfd probe crashes when starting a child process. The iSH build
uses a private Go source overlay to select the existing PID/wait4 fallback and skip
the optional waitid probe. The installed toolchain and other release targets are
not modified. The builder checks the expected Go functions and stops if they change.

On Linux, test both child execution and the UI round trip under the same blocked
optional syscalls:

```sh
python3 scripts/ish_process_smoke.py /path/to/cmdlib-linux-386
```

This trap-based Linux test complements a physical iPhone test; it does not claim
to reproduce all iSH system calls, terminal behavior or iOS lifecycle behavior.
