#!/usr/bin/env python3
"""Native terminal checks with isolated, synthetic command data only."""
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import sys
import tempfile
import termios
import time

binary = str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix="cmdlib-smoke-") as tmp:
    path = Path(tmp) / "commands.json"
    env = dict(os.environ, CMDLIB_FILE=str(path), NO_COLOR="1", TERM="xterm-256color")
    for flag in ["--version", "--help", "--tea"]:
        out = subprocess.check_output([binary, flag], env=env)
        assert b"cmdlib" in out and b"\x1b" not in out
    assert not path.exists(), "Informational flags created data"
    subprocess.run([binary, "export", "navi"], env=env, stdout=subprocess.DEVNULL, check=True)
    assert len(json.loads(path.read_text())) == 5, "Fresh install did not seed"
    fixture = [{"id": "fixture", "name": "Synthetic Fixture", "command": "printf 'CMDLIB_SMOKE_OK\\n'",
                "description": "Harmless terminal test", "explanation": "Print a word", "tags": ["fixture"],
                "host": "example-remote-label", "risk": "READ", "prerequisite": "",
                "created_at": "2025-01-01T00:00:00Z", "updated_at": "2025-01-01T00:00:00Z",
                "last_used_at": None, "use_count": 0}]
    raw = (json.dumps(fixture, indent=2) + "\n").encode()
    path.write_bytes(raw)
    output = subprocess.check_output([binary, "export", "navi"], env=env)
    assert b"CMDLIB_SMOKE_OK" in output and path.read_bytes() == raw

    def session(confirm):
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 36, 110, 0, 0))
        proc = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave, env=env, start_new_session=True)
        os.close(slave)
        captured = bytearray()

        def pump(timeout):
            if select.select([master], [], [], timeout)[0]:
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    return False
                if not chunk:
                    return False
                captured.extend(chunk)
                if b"\x1b]11;?" in chunk:
                    os.write(master, b"\x1b]11;rgb:0000/0000/0000\x07")
                if b"\x1b[6n" in chunk:
                    os.write(master, b"\x1b[1;1R")
            return True

        def wait_for(needle, timeout=12):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                if needle in captured:
                    return
                if not pump(0.1):
                    break
            # This terminal contains only the synthetic fixture and runner metadata.
            print("Synthetic terminal tail:", repr(bytes(captured[-4096:])), file=sys.stderr)
            raise AssertionError("TUI did not show " + needle.decode())

        def send(keys):
            os.write(master, keys)
            # Drain output while the application handles input. macOS PTYs have
            # small buffers; sleeping here can block a render and merge keys.
            deadline = time.monotonic() + 0.2
            while time.monotonic() < deadline:
                if not pump(min(0.05, deadline - time.monotonic())):
                    break

        try:
            wait_for(b"Synthetic Fixture")
            send(b"/")
            send(b"\x1b[200~synthetic fixture\x1b[201~")
            wait_for(b"synthetic fixture")
            send(b"\x1b")
            send(b"\x1b[C")
            send(b"\x1b[C")
            send(b"\r")
            wait_for(b"Execution host:")
            wait_for(b"example-remote-label")
            assert path.read_bytes() == raw, "Selection changed data"
            if confirm:
                send(b"yes\r")
                wait_for(b"Exit status: 0")
                send(b"\r")
                wait_for(b"Command finished.")
            else:
                send(b"\x1b")
            send(b"q")
            proc.wait(timeout=12)
            assert proc.returncode == 0
        finally:
            if proc.poll() is None:
                proc.terminate()
                proc.wait(timeout=5)
            os.close(master)

    session(False)
    assert path.read_bytes() == raw, "Cancellation changed library"
    session(True)
    used = json.loads(path.read_text())[0]
    assert used["use_count"] == 1 and used["last_used_at"]
    for k, v in fixture[0].items():
        if k not in ["use_count", "last_used_at"]:
            assert used[k] == v, "Run changed unrelated data"
    path.write_text("[]\n")
    subprocess.run([binary, "export", "navi"], env=env, stdout=subprocess.DEVNULL, check=True)
    assert path.read_text() == "[]\n", "Empty library reseeded"
print("Native smoke passed: flags, fresh install, upgrade, export, pasted search, confirmation, cancel, harmless run, empty data.")
