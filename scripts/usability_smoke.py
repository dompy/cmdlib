#!/usr/bin/env python3
"""Native PTY checks of new controls, using only isolated harmless fixtures."""
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
with tempfile.TemporaryDirectory(prefix="cmdlib-ux-smoke-") as temporary:
    directory = Path(temporary)
    library = directory / "commands.json"
    commands = [dict(id=name.lower().replace(" ", "-"), name=name,
                     command="printf 'SYNTHETIC_ONLY\\n'", description="Synthetic harmless fixture",
                     explanation="No microphone, network or real operational command", tags=["fixture"],
                     category=category, host="synthetic-local", risk="READ", prerequisite="",
                     created_at="2025-01-01T00:00:00Z", updated_at="2025-01-01T00:00:00Z",
                     last_used_at=None, use_count=0)
                for name, category in [("Anchor Fixture", ""), ("Recording Fixture", "FFmpeg"),
                                       ("Device Fixture", "FFmpeg")]]
    for command in commands:
        if not command["category"]:
            del command["category"]  # Match the optional Go JSON field's omitempty format.
    library.write_text(json.dumps(commands))
    env = dict(os.environ, CMDLIB_FILE=str(library), NO_COLOR="1", TERM="xterm-256color")
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 38, 120, 0, 0))
    process = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave,
                               env=env, start_new_session=True)
    os.close(slave)
    captured = bytearray()

    def pump(seconds):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if not select.select([master], [], [], min(.05, max(0, deadline-time.monotonic())))[0]:
                continue
            try:
                chunk = os.read(master, 65536)
            except OSError:
                return
            if not chunk:
                return
            captured.extend(chunk)
            if b"\x1b]11;?" in chunk:
                os.write(master, b"\x1b]11;rgb:0000/0000/0000\x07")
            if b"\x1b[6n" in chunk:
                os.write(master, b"\x1b[1;1R")

    def wait(needle):
        deadline = time.monotonic() + 8
        while needle not in captured and time.monotonic() < deadline:
            pump(.05)
        if needle not in captured:
            raise AssertionError("Missing UI text " + repr(needle) + repr(bytes(captured[-2000:])))

    def send(keys):
        captured.clear()
        os.write(master, keys)
        pump(.2)

    try:
        wait(b"Anchor Fixture")
        send(b"\r")
        wait(b"Execution host:")
        assert json.loads(library.read_text()) == commands
        send(b"\x1b")
        send(b"\x1b[B")
        send(b"\r")
        wait(b"Recording Fixture")
        assert json.loads(library.read_text()) == commands
        assert json.loads(Path(str(library)+".view-state.json").read_text())["FFmpeg"]
        send(b"e")
        wait(b"Kategorie umbenennen")
        send(b"Audio & Aufnahme")
        send(b"\r")
        renamed = json.loads(library.read_text())
        assert renamed[0] == commands[0]
        for index in [1, 2]:
            expected = dict(commands[index], category="Audio & Aufnahme", updated_at=renamed[index]["updated_at"])
            assert renamed[index] == expected
        assert json.loads(Path(str(library)+".view-state.json").read_text())["Audio & Aufnahme"]
        assert list((directory / "backups").glob("*.bak"))
        send(b"u")
        assert json.loads(library.read_text()) == commands
        assert json.loads(Path(str(library)+".view-state.json").read_text())["FFmpeg"]
        send(b"\x1b[B")
        send(b"d")
        wait(b"Delete this command")
        send(b"\r")
        assert [r["id"] for r in json.loads(library.read_text())] == [commands[0]["id"], commands[2]["id"]]
        assert list((directory / "backups").glob("*.bak"))
        send(b"u")
        assert json.loads(library.read_text()) == commands
        send(b"m")
        wait(b"Move")
        send(b"\x1b[B")
        send(b"\r")
        assert [r["id"] for r in json.loads(library.read_text())] == [commands[0]["id"], commands[2]["id"], commands[1]["id"]]
        send(b"u")
        assert json.loads(library.read_text()) == commands
        output = subprocess.check_output([binary, "maintain", "--idle"], env=env, timeout=5)
        assert "geöffnet" in output.decode()
        assert not Path(str(library)+".maintenance.json").exists()
        send(b"q")
        process.wait(timeout=5)
        assert process.returncode == 0
        assert json.loads(library.read_text()) == commands
    finally:
        if process.poll() is None:
            process.terminate()
            process.wait(timeout=5)
        os.close(master)
print(json.dumps({"native_pty": {"enter_defaults_run": True, "category_expands": True,
     "category_rename_preserve_undo": True,
     "delete_confirm_undo": True, "move_persist_undo": True, "private_backup": True,
     "idle_skips_open_ui": True}, "personal_commands_executed": 0,
     "microphone_recordings": 0, "model_calls": 0}, indent=2))
