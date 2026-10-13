#!/usr/bin/env python3
"""Tests für cmdlib-add (Go-Schema von github.com/dompy/cmdlib).

Ausführen:  python3 test_cmdlib_add.py
"""
import importlib.util
import json
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("ca", os.path.join(HERE, "cmdlib-add.py"))
ca = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ca)


def risk(command):
    entry, _ = ca.analyze_local(command)
    return entry["risk"]


class RiskHeuristics(unittest.TestCase):
    # --- die vier ursprünglich gemeldeten Fehler (Regression) ---
    def test_redirect_is_write(self):
        self.assertEqual(risk("cat input > output"), "WRITE")

    def test_find_delete_is_danger(self):
        self.assertEqual(risk("find . -delete"), "DANGER")

    def test_sudo_user_flag_detects_program(self):
        self.assertEqual(ca.programs_in("sudo -u root rm -rf /tmp/x"), ["rm"])
        self.assertEqual(risk("sudo -u root rm -rf /tmp/x"), "DANGER")

    def test_ai_cannot_downgrade_risk(self):
        entry, _ = ca.analyze_local("sudo -u root rm -rf /tmp/x")
        ca.merge_codex(entry, {"risk": "READ"})
        self.assertEqual(entry["risk"], "DANGER")

    # --- weiteres erwartetes Verhalten ---
    def test_ai_can_upgrade_risk(self):
        entry, _ = ca.analyze_local("df -h")
        ca.merge_codex(entry, {"risk": "WRITE"})
        self.assertEqual(entry["risk"], "WRITE")

    def test_plain_cat_is_read(self):
        self.assertEqual(risk("cat datei.txt"), "READ")

    def test_journalctl_u_is_not_sudo_option(self):
        self.assertEqual(ca.programs_in("sudo journalctl -u ssh -f"), ["journalctl"])
        self.assertEqual(risk("sudo journalctl -u ssh -f"), "READ")

    def test_ssh_is_connect(self):
        self.assertEqual(risk("ssh ai-mate-prod-01"), "CONNECT")

    def test_connect_with_redirect_is_write(self):
        self.assertEqual(risk("curl https://example.org > seite.html"), "WRITE")

    def test_pipe_with_redirect_is_write(self):
        self.assertEqual(risk("tail -f log | grep ERROR > treffer.txt"), "WRITE")

    def test_rsync_delete_is_danger(self):
        self.assertEqual(risk("rsync -a --delete quelle/ ziel/"), "DANGER")

    def test_rm_behind_and_is_danger(self):
        self.assertEqual(risk("rm alt.txt && df -h"), "DANGER")

    def test_unknown_program_defaults_to_write(self):
        self.assertEqual(risk("frobnicate --now"), "WRITE")

    def test_risk_values_match_go_validation(self):
        for r in ("READ", "CONNECT", "WRITE", "DANGER"):
            self.assertIn(r, ca.RANK)


class LibraryWriting(unittest.TestCase):
    def entry(self, name, command, category):
        return {"id": "", "name": name, "command": command, "description": "x",
                "explanation": "", "tags": [], "host": "local",
                "category": category, "risk": "READ", "prerequisite": ""}

    def test_insert_after_same_category_case_insensitive(self):
        data = [self.entry("A", "a", "System"),
                self.entry("B", "b", "FFmpeg"),
                self.entry("C", "c", "System")]
        e = self.entry("D", "d", "system")
        ca.match_category(e, data)
        self.assertEqual(e["category"], "System")  # vorhandene Schreibweise übernehmen
        self.assertEqual(ca.insert_sorted(data, e), 3)
        e2 = self.entry("E", "e", "")
        self.assertEqual(ca.insert_sorted(data, e2), len(data) - 1)

    def test_unique_ids(self):
        e = {"name": "SSH Live Logs!", "command": "x"}
        self.assertEqual(ca.unique_id(e, set()), "ssh-live-logs")
        second = ca.unique_id(e, {"ssh-live-logs"})
        self.assertNotEqual(second, "ssh-live-logs")
        self.assertTrue(second.startswith("ssh-live-logs-"))

    def test_save_backup_roundtrip(self):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "cmdlib", "commands.json")
            data = [self.entry("A", "a", "System")]
            data[0]["id"] = "a"
            ca.save_db(path, data)
            raw = open(path, encoding="utf-8").read()
            self.assertTrue(raw.endswith("\n"))  # wie die Go-Version
            self.assertEqual(ca.load_db(path), data)
            b = ca.backup(path)
            self.assertTrue(os.path.exists(b))
            self.assertIn(os.path.join("cmdlib", "backups"), b)
            self.assertEqual(json.load(open(b)), data)


if __name__ == "__main__":
    unittest.main(verbosity=2)
