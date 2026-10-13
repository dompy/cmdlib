#!/usr/bin/env python3
"""Pure toolkit checks; never access a microphone or play sound."""

import importlib.util
import io
from pathlib import Path
import signal
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("cmdlib_ffmpeg", Path(__file__).with_name("cmdlib-ffmpeg.py"))
tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tool)


class FFmpegToolkitTests(unittest.TestCase):
    def test_device_parse_ignores_video_and_uses_fresh_name(self):
        listing = """[AVFoundation @ 0x1] AVFoundation video devices:
[AVFoundation @ 0x1] [0] FaceTime HD Camera
[AVFoundation @ 0x1] AVFoundation audio devices:
[AVFoundation @ 0x1] [0] Virtual Device
[AVFoundation @ 0x1] [2] MacBook Pro-Mikrofon
Error opening input file .
"""
        devices = tool.parse_audio_devices(listing)
        self.assertEqual(devices, [(0, "Virtual Device"), (2, "MacBook Pro-Mikrofon")])
        self.assertEqual(tool.choose_microphone(devices), "MacBook Pro-Mikrofon")
        with self.assertRaises(tool.ToolkitError):
            tool.choose_microphone(devices, "Virtual Device")

    def test_record_arguments_are_bounded_audio_only_nonoverwrite(self):
        args = tool.record_arguments("ffmpeg", "MacBook Pro-Mikrofon", 30, Path("/tmp/audio.wav"))
        self.assertEqual(args[args.index("-i") + 1], "none:MacBook Pro-Mikrofon")
        self.assertEqual(args[args.index("-t") + 1], "30")
        self.assertIn("-n", args)
        self.assertIn("-vn", args)
        self.assertNotIn("-y", args)
        for invalid in (0, -1, 301, "nan", "inf", "1:99", "$(touch bad)"):
            with self.subTest(invalid=invalid), self.assertRaises(tool.ToolkitError):
                tool.record_arguments("ffmpeg", "Mic", invalid, Path("audio.wav"))

    def test_time_and_volume_validation(self):
        self.assertEqual(tool.seconds("01:30"), 90)
        self.assertEqual(tool.seconds("1,5"), 1.5)
        for invalid in (float("nan"), float("inf"), -61, 25):
            with self.subTest(invalid=invalid), self.assertRaises(tool.ToolkitError):
                tool.file_arguments("ffmpeg", Path("source.wav"), Path("out.wav"), kind="volume", decibels=invalid)

    def test_file_commands_preserve_originals_and_map_audio_explicitly(self):
        source, target = Path("/tmp/source with space.mp4"), Path("/tmp/target.wav")
        args = tool.file_arguments("ffmpeg", source, target, kind="extract-audio")
        self.assertIn("-n", args)
        self.assertNotIn("-y", args)
        self.assertEqual(args[args.index("-i") + 1], str(source))
        self.assertIn("0:a:0", args)
        self.assertIn("-vn", args)
        self.assertEqual(args[-1], str(target))
        trim = tool.file_arguments("ffmpeg", source, Path("out.mp4"), kind="trim", start="00:02", duration="3.5")
        self.assertGreater(trim.index("-ss"), trim.index("-i"))
        self.assertEqual(trim[trim.index("-ss") + 1], "2")

    def test_dragged_paths_and_unique_outputs(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "a space.wav"
            source.touch()
            self.assertEqual(tool.input_path(str(source)), source.resolve())
            self.assertEqual(tool.input_path(str(source).replace(" ", "\\ ")), source.resolve())
            self.assertEqual(tool.input_path("'" + str(source) + "'"), source.resolve())
            first = tool.unique_output(Path(directory), "recording", "wav")
            second = tool.unique_output(Path(directory), "recording", "wav")
            self.assertNotEqual(first, second)
            self.assertFalse(first.exists())

    def test_latest_is_scoped_and_ignores_symlinks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with self.assertRaises(tool.ToolkitError):
                tool.latest_recording(root)
            other = root / "other.wav"
            other.touch()
            own = root / "Mikrofon-1.wav"
            own.touch()
            (root / "Mikrofon-2.wav").symlink_to(other)
            self.assertEqual(tool.latest_recording(root), own)

    def test_record_cancel_does_not_spawn_or_create_audio(self):
        with tempfile.TemporaryDirectory() as directory:
            with patch.object(tool, "list_audio_devices", return_value=[(2, "MacBook Pro-Mikrofon")]), \
                 patch.object(tool, "recordings_directory", return_value=Path(directory)), \
                 patch("builtins.input", return_value="n"), patch.object(tool, "execute") as execute:
                tool.record("ffmpeg")
            execute.assert_not_called()
            self.assertEqual(list(Path(directory).iterdir()), [])

    def test_ctrl_c_closes_child_without_recording(self):
        class FakeProcess:
            returncode = None

            def __init__(self):
                self.signals = []

            def poll(self):
                return self.returncode

            def send_signal(self, value):
                self.signals.append(value)

            def wait(self, timeout):
                self.returncode = 255

        process = FakeProcess()
        with patch.object(tool.subprocess, "Popen", return_value=process) as spawn, \
             patch.object(tool.time, "sleep", side_effect=KeyboardInterrupt), \
             patch("sys.stdout", new_callable=io.StringIO):
            self.assertFalse(tool.execute(["fake-ffmpeg", "-n", "out.wav"]))
        spawn.assert_called_once_with(["fake-ffmpeg", "-n", "out.wav"], start_new_session=True)
        self.assertEqual(process.signals, [signal.SIGINT])

    def test_stalled_child_escalates_with_bounded_waits(self):
        class FakeProcess:
            def __init__(self):
                self.calls = []

            def poll(self):
                return None

            def send_signal(self, value):
                self.calls.append(("signal", value))

            def wait(self, timeout):
                self.calls.append(("wait", timeout))
                if len([call for call in self.calls if call[0] == "wait"]) < 3:
                    raise subprocess.TimeoutExpired("fake", timeout)

            def terminate(self):
                self.calls.append(("terminate",))

            def kill(self):
                self.calls.append(("kill",))

        process = FakeProcess()
        tool.stop_process(process)
        self.assertEqual(process.calls, [("signal", signal.SIGINT), ("wait", 8), ("terminate",), ("wait", 3), ("kill",), ("wait", 3)])


if __name__ == "__main__":
    unittest.main()
