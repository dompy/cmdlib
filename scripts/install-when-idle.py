#!/usr/bin/env python3
"""One-shot personal update: wait for cmdlib to close, then install tested build."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import subprocess
import sys

from importlib.util import module_from_spec, spec_from_file_location

ROOT = Path(__file__).resolve().parent.parent
SPEC = spec_from_file_location("local_install", ROOT / "scripts/install-local-ux.py")
INSTALL = module_from_spec(SPEC)
SPEC.loader.exec_module(INSTALL)
LABEL = "ai.mate.cmdlib.install-usability"
REQUEST = ROOT / "verification/pending-install.json"


def register(args):
    binary = Path.home() / ".local/bin/cmdlib"
    if hashlib.sha256(binary.read_bytes()).hexdigest() != args.expected_binary_sha:
        raise SystemExit("Current installation changed; deferred upgrade not registered.")
    # Registration happens only AFTER source tests and native smoke tests pass.
    INSTALL.atomic_bytes(REQUEST, (json.dumps({
        "expected_binary_sha": args.expected_binary_sha,
        "remove_duplicate_name": args.remove_duplicate_name,
        "keep_duplicate_name": args.keep_duplicate_name,
        "queue_advisor": args.queue_advisor,
    }, indent=2) + "\n").encode())
    agents = Path.home() / "Library/LaunchAgents"
    agents.mkdir(parents=True, exist_ok=True)
    path = agents / (LABEL + ".plist")
    plist = {
        "Label": LABEL,
        "ProgramArguments": [sys.executable, str(Path(__file__).resolve()), "--once"],
        "RunAtLoad": True,
        "StartInterval": 20,
        "ProcessType": "Background",
        "StandardOutPath": str(ROOT / "verification/deferred-install.log"),
        "StandardErrorPath": str(ROOT / "verification/deferred-install-error.log"),
    }
    if path.exists():
        raise SystemExit("A deferred update is already registered; inspect it first.")
    INSTALL.atomic_bytes(path, plistlib.dumps(plist))
    subprocess.run(["/bin/launchctl", "bootstrap", "gui/" + str(os.getuid()), str(path)], check=True)
    print("Tested update queued. Installation waits until all cmdlib windows are closed.")


def once():
    receipt_path = ROOT / "verification/installation.json"
    if receipt_path.exists():
        receipt = json.loads(receipt_path.read_text())
        if receipt.get("installed"):
            return finish()
    active = subprocess.run(["/usr/bin/pgrep", "-x", "cmdlib"], capture_output=True)
    if active.returncode == 0:
        print("cmdlib still open; installation deferred.")
        return
    if active.returncode != 1:
        raise SystemExit("Cannot verify idle state; update not applied.")
    request = json.loads(REQUEST.read_text())
    command = [sys.executable, str(ROOT / "scripts/install-local-ux.py"),
               "--expected-binary-sha", request["expected_binary_sha"]]
    if request.get("remove_duplicate_name"):
        command.extend(["--remove-duplicate-name", request["remove_duplicate_name"],
                        "--keep-duplicate-name", request["keep_duplicate_name"]])
    if request.get("queue_advisor"):
        command.append("--queue-advisor")
    subprocess.run(command, check=True)
    finish()


def finish():
    path = Path.home() / "Library/LaunchAgents" / (LABEL + ".plist")
    if path.exists():
        path.unlink()  # Only this task's one-shot LaunchAgent, never user data.
    subprocess.run(["/bin/launchctl", "bootout", "gui/" + str(os.getuid()) + "/" + LABEL], check=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--once", action="store_true")
    parser.add_argument("--expected-binary-sha")
    parser.add_argument("--remove-duplicate-name")
    parser.add_argument("--keep-duplicate-name")
    parser.add_argument("--queue-advisor", action="store_true")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise SystemExit("macOS only.")
    if args.once:
        once()
    elif not args.expected_binary_sha:
        parser.error("--expected-binary-sha is required to register an update")
    else:
        register(args)


if __name__ == "__main__":
    main()
