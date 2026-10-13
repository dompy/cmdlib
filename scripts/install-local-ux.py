#!/usr/bin/env python3
"""Install a tested personal build without replacing private command data."""

import argparse
from datetime import datetime, timezone
import fcntl
import hashlib
import json
import os
from pathlib import Path
import plistlib
import shlex
import shutil
import subprocess
import sys
import tempfile


def atomic_bytes(path, data, mode=0o600):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd, temp = tempfile.mkstemp(prefix=".cmdlib-install-", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.chmod(temp, mode)
        os.replace(temp, path)
    finally:
        if os.path.exists(temp):
            os.unlink(temp)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--expected-binary-sha", required=True)
    parser.add_argument("--remove-duplicate-name")
    parser.add_argument("--keep-duplicate-name")
    parser.add_argument("--queue-advisor", action="store_true")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("This installer targets a personal macOS installation.")
    root = Path(__file__).resolve().parent.parent
    home = Path.home()
    binary = home / ".local/bin/cmdlib"
    base = home / "Library/Application Support/cmdlib"
    library = base / "commands.json"
    staged = root / "verification/cmdlib"
    if not staged.is_file() or not binary.is_file() or not library.is_file():
        raise SystemExit("Tested build, existing binary, or existing library missing; no changes.")
    if hashlib.sha256(binary.read_bytes()).hexdigest() != args.expected_binary_sha:
        raise SystemExit("Installed executable changed since inspection; no changes.")
    active = subprocess.run(["/usr/bin/pgrep", "-x", "cmdlib"], capture_output=True)
    if active.returncode == 0:
        raise SystemExit("cmdlib is open. Close it before this upgrade; no changes.")
    if active.returncode != 1:
        raise SystemExit("Could not verify whether cmdlib is closed; no changes.")
    base.mkdir(mode=0o700, parents=True, exist_ok=True)
    lock = (base / "commands.json.in-use.lock").open("a+b")
    os.chmod(lock.name, 0o600)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        raise SystemExit("cmdlib is in use; no changes.")
    raw = library.read_bytes()
    commands = json.loads(raw)
    if not isinstance(commands, list):
        raise SystemExit("Existing library is not an array; no changes.")
    removed = []
    if args.remove_duplicate_name:
        candidates = [row for row in commands if row.get("name") == args.remove_duplicate_name]
        keep = [row for row in commands if row.get("name") == args.keep_duplicate_name]
        if len(candidates) != 1 or len(keep) != 1 or candidates[0].get("command") != keep[0].get("command"):
            raise SystemExit("Duplicate target is not exact and unique; no changes.")
        removed = candidates
        commands = [row for row in commands if row is not candidates[0]]
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
    backup = base / "backups" / ("usability-" + stamp)
    backup.mkdir(mode=0o700, parents=True)
    shutil.copy2(binary, backup / "cmdlib-before")
    (backup / "cmdlib-before").chmod(0o700)
    atomic_bytes(backup / "commands-before.json", raw)
    tools = base / "tools"
    tools.mkdir(mode=0o700, exist_ok=True)
    helper = tools / "ffmpeg.py"
    atomic_bytes(helper, (root / "scripts/cmdlib-ffmpeg.py").read_bytes(), 0o700)
    timestamp = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    templates = json.loads((root / "templates/ffmpeg-commands.json").read_text())
    installed_ids = {row.get("id") for row in commands}
    added = []
    for row in templates:
        if row["id"] in installed_ids:
            continue  # Never replace owner-edited entries on a later install.
        action = shlex.split(row["command"])[-1]
        row["command"] = "python3 " + shlex.quote(str(helper)) + " " + shlex.quote(action)
        row.update(created_at=timestamp, updated_at=timestamp, last_used_at=None, use_count=0)
        commands.append(row)
        added.append(row["id"])
    atomic_bytes(library, (json.dumps(commands, ensure_ascii=False, indent=2) + "\n").encode())
    atomic_bytes(binary, staged.read_bytes(), 0o755)
    lock.close()
    queued = False
    if args.queue_advisor:
        subprocess.run([str(binary), "assist", "--queue"], check=True)
        queued = True
    launch_dir = home / "Library/LaunchAgents"
    launch_dir.mkdir(parents=True, exist_ok=True)
    log_dir = base / "maintenance"
    log_dir.mkdir(mode=0o700, exist_ok=True)
    label = "ai.mate.cmdlib.maintenance"
    launch_path = launch_dir / (label + ".plist")
    if launch_path.exists():
        shutil.copy2(launch_path, backup / "launch-agent-before.plist")
    agent = {
        "Label": label,
        "ProgramArguments": [str(binary), "maintain", "--idle"],
        "RunAtLoad": True,
        "StartInterval": 300,
        "ProcessType": "Background",
        "EnvironmentVariables": {
            "CMDLIB_FILE": str(library),
            "PATH": str(home / ".local/bin") + ":/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
        },
        "StandardOutPath": str(log_dir / "background.log"),
        "StandardErrorPath": str(log_dir / "background-error.log"),
    }
    atomic_bytes(launch_path, plistlib.dumps(agent))
    domain = "gui/" + str(os.getuid())
    existing = subprocess.run(["/bin/launchctl", "print", domain + "/" + label], capture_output=True)
    if existing.returncode == 0:
        subprocess.run(["/bin/launchctl", "bootout", domain + "/" + label], check=True)
    subprocess.run(["/bin/launchctl", "bootstrap", domain, str(launch_path)], check=True)
    result = {
        "installed": True,
        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
        "commands_before": len(json.loads(raw)),
        "commands_after": len(commands),
        "duplicates_removed": len(removed),
        "ffmpeg_entries_added": len(added),
        "original_remaining_entries_unchanged": all(row == next(old for old in json.loads(raw) if old["id"] == row["id"]) for row in commands if row["id"] not in added),
        "backup": str(backup),
        "launch_agent": label,
        "interval_seconds": 300,
        "advisor_queued_once": queued,
        "microphone_recorded": False,
    }
    atomic_bytes(root / "verification/installation.json", (json.dumps(result, indent=2) + "\n").encode())
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
