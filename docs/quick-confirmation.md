# Explicit run confirmation

The focused UI always requires a fresh explicit confirmation after showing the
actual command and execution machine. There is no countdown or automatic execution.
A legacy library-adjacent `.run-policy.json` is left untouched but cannot arm a run.

READ and CONNECT accept Enter, Space, y or 1. WRITE requires typing `WRITE` and
pressing Enter. DANGER requires `RUN <id>` and Enter. Esc cancels; READ and CONNECT
also accept n, 0, Backspace or Delete to cancel. Pasted approvals are rejected.

Selection, search, copying, field navigation and editing never schedule a run.
The library is reloaded before recording a confirmed attempt. If the command,
host, risk or prerequisite changed in another window, it must be reviewed again.

Run `python3 scripts/quick_confirmation_smoke.py ./cmdlib` for native PTY checks
with harmless temporary fixtures, including an old policy that cannot auto-start.
