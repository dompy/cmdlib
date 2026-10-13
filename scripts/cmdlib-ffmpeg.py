#!/usr/bin/env python3
"""Small owner-operated FFmpeg toolkit. No recording or playback on import."""

import argparse
from datetime import datetime
import json
import math
import os
from pathlib import Path
import re
import shlex
import shutil
import signal
import subprocess
import sys
import time
import uuid


class ToolkitError(Exception):
    pass


def executable(name):
    candidate = shutil.which(name)
    if candidate:
        return candidate
    for directory in ("/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"):
        candidate = Path(directory) / name
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return str(candidate)
    raise ToolkitError(f"{name} fehlt. Bitte zuerst installieren; es wird nichts automatisch installiert.")


def seconds(value, *, minimum=0, maximum=86400):
    """Parse decimal seconds or mm:ss / hh:mm:ss, without a shell."""
    try:
        parts = str(value).strip().replace(",", ".").split(":")
        if not 1 <= len(parts) <= 3 or any(not part for part in parts):
            raise ValueError
        numbers = [float(part) for part in parts]
        if any(not math.isfinite(number) or number < 0 for number in numbers):
            raise ValueError
        if len(parts) > 1 and any(number >= 60 for number in numbers[1:]):
            raise ValueError
        result = sum(number * (60 ** index) for index, number in enumerate(reversed(numbers)))
        if not minimum <= result <= maximum:
            raise ValueError
        return result
    except (TypeError, ValueError):
        raise ToolkitError(f"Zeit muss zwischen {minimum:g} und {maximum:g} Sekunden liegen (z. B. 30 oder 01:30).") from None


def number_text(value):
    return f"{value:g}"


def recordings_directory():
    if sys.platform != "darwin":
        raise ToolkitError("Die Mikrofon- und Wiedergabeaktionen sind für macOS eingerichtet.")
    directory = Path.home() / "Library" / "Application Support" / "cmdlib" / "recordings"
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    directory.chmod(0o700)
    return directory


def unique_output(directory, stem, extension):
    # FFmpeg -n additionally protects against even an unlikely name collision.
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    return directory / f"{stem}-{stamp}-{uuid.uuid4().hex[:8]}.{extension.lstrip('.')}"


def parse_audio_devices(text):
    devices = []
    in_audio = False
    for line in text.splitlines():
        if "AVFoundation audio devices:" in line:
            in_audio = True
            continue
        if "AVFoundation video devices:" in line:
            in_audio = False
        if in_audio:
            match = re.search(r"\[(\d+)\]\s+(.+?)\s*$", line)
            if match:
                devices.append((int(match.group(1)), match.group(2)))
    return devices


def list_audio_devices(ffmpeg):
    if sys.platform != "darwin":
        raise ToolkitError("AVFoundation-Geräte werden nur auf macOS angeboten.")
    result = subprocess.run(
        [ffmpeg, "-hide_banner", "-nostdin", "-f", "avfoundation", "-list_devices", "true", "-i", ""],
        capture_output=True, text=True, timeout=15,
    )
    # FFmpeg intentionally exits nonzero after listing devices with an empty input.
    devices = parse_audio_devices(result.stderr + "\n" + result.stdout)
    if not devices:
        raise ToolkitError("Keine Audiogeräte gefunden. FFmpeg/AVFoundation und die macOS-Berechtigungen prüfen.")
    return devices


def microphone_devices(devices):
    return [(index, name) for index, name in devices if re.search(r"microphone|mikrofon", name, re.I)]


def choose_microphone(devices, requested=None):
    microphones = microphone_devices(devices)
    if requested:
        found = [name for _, name in microphones if name == requested]
        if len(found) == 1:
            return found[0]
        raise ToolkitError("Das gewünschte Mikrofon ist nicht eindeutig in der aktuellen Geräteliste vorhanden.")
    built_in = [name for _, name in microphones if "macbook" in name.lower()]
    if len(built_in) == 1:
        return built_in[0]
    if len(microphones) == 1:
        return microphones[0][1]
    if not microphones:
        raise ToolkitError("Kein eindeutig benanntes Mikrofon gefunden. Mit 'devices' die Geräte prüfen.")
    print("Welches Mikrofon?")
    for position, (_, name) in enumerate(microphones, 1):
        print(f"  {position}. {name}")
    answer = input("Nummer: ").strip()
    if answer.isdecimal() and 1 <= int(answer) <= len(microphones):
        return microphones[int(answer) - 1][1]
    raise ToolkitError("Keine gültige Mikrofonwahl; es wurde nichts aufgenommen.")


def record_arguments(ffmpeg, microphone, duration, output):
    if not microphone or ":" in microphone or "\x00" in microphone:
        raise ToolkitError("Ungültiger Mikrofonname.")
    duration = seconds(duration, minimum=1, maximum=300)
    return [
        ffmpeg, "-hide_banner", "-loglevel", "warning", "-nostdin", "-n", "-f", "avfoundation",
        "-i", f"none:{microphone}", "-t", number_text(duration),
        "-vn", "-ac", "1", "-ar", "48000", "-c:a", "pcm_s16le", str(output),
    ]


def stop_process(process):
    """Give FFmpeg an opportunity to finish its container before escalation."""
    if process.poll() is not None:
        return
    process.send_signal(signal.SIGINT)
    try:
        process.wait(timeout=8)
    except subprocess.TimeoutExpired:
        process.terminate()
        try:
            process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=3)


def execute(arguments, *, timeout=3600, countdown=None):
    print("\n" + shlex.join(arguments), flush=True)
    process = subprocess.Popen(arguments, start_new_session=True)
    deadline = time.monotonic() + timeout
    started = time.monotonic()
    shown = None
    try:
        while process.poll() is None:
            if time.monotonic() >= deadline:
                stop_process(process)
                raise ToolkitError("Zeitlimit erreicht; FFmpeg wurde beendet. Eine mögliche Teildatei bleibt erhalten.")
            if countdown is not None:
                remaining = max(0, math.ceil(countdown - (time.monotonic() - started)))
                if remaining != shown:
                    print(f"\rAufnahme läuft · etwa {remaining:3d} Sekunden · Ctrl+C beendet", end="", flush=True)
                    shown = remaining
            time.sleep(0.1)
        if countdown is not None:
            print()
        if process.returncode:
            raise ToolkitError(f"FFmpeg/Programm meldet Fehler {process.returncode}; Originaldateien bleiben unverändert.")
    except KeyboardInterrupt:
        stop_process(process)
        print("\nBeendet. Eine bereits geschriebene Teildatei bleibt erhalten.")
        return False
    finally:
        if process.poll() is None:
            stop_process(process)
    return True


def record(ffmpeg, duration=30, device=None):
    duration = seconds(duration, minimum=1, maximum=300)
    microphone = choose_microphone(list_audio_devices(ffmpeg), device)
    output = unique_output(recordings_directory(), "Mikrofon", "wav")
    print(f"Quelle: {microphone}\nDauer: {number_text(duration)} Sekunden\nZiel: {output}")
    print("Nur Mikrofon; keine Kamera, kein Bildschirm, kein System-Audio. Noch läuft keine Aufnahme.")
    print("Falls macOS fragt: Mikrofonzugriff erlauben. Lass Alex über Lautsprecher sprechen.")
    if input("Enter startet · 'n' + Enter bricht ab: ").strip():
        print("Abgebrochen; keine Aufnahme gestartet.")
        return
    previous_umask = os.umask(0o077)
    try:
        completed = execute(record_arguments(ffmpeg, microphone, duration, output), timeout=duration + 30, countdown=duration)
    finally:
        os.umask(previous_umask)
        if output.is_file():
            output.chmod(0o600)
            print(f"\nAufnahme gespeichert: {output}")
    if not output.is_file() or output.stat().st_size <= 44:
        raise ToolkitError("Keine verwertbare WAV-Datei entstanden. Bitte Mikrofonfreigabe und Quelle prüfen.")
    if completed:
        print("Fertig. 'play-latest' spielt sie ab; 'open-recordings' öffnet den Ordner.")


def input_path(raw=None):
    value = raw if raw is not None else input("Datei ins Terminal ziehen oder Pfad eingeben, dann Enter: ")
    value = value.strip()
    candidate = Path(value).expanduser()
    if not candidate.is_file():
        try:
            parts = shlex.split(value)
        except ValueError:
            raise ToolkitError("Der Dateipfad konnte nicht gelesen werden.") from None
        if len(parts) != 1:
            raise ToolkitError("Bitte genau eine bestehende lokale Datei auswählen.")
        candidate = Path(parts[0]).expanduser()
    if not candidate.is_file():
        raise ToolkitError("Diese lokale Datei existiert nicht.")
    return candidate.resolve()


def file_arguments(ffmpeg, source, output, *, kind, start=0, duration=30, decibels=0):
    args = [ffmpeg, "-hide_banner", "-nostdin", "-n", "-i", str(source)]
    if kind == "trim":
        # Output-side seek re-encodes the selected range instead of keyframe-only cuts.
        args += ["-ss", number_text(seconds(start)), "-t", number_text(seconds(duration, minimum=0.01))]
    if kind == "volume":
        if not math.isfinite(decibels) or not -60 <= decibels <= 24:
            raise ToolkitError("Lautstärkeänderung muss zwischen -60 und +24 dB liegen.")
        args += ["-af", f"volume={number_text(decibels)}dB"]
    extension = output.suffix.lower()
    audio = {
        ".wav": ["-vn", "-map", "0:a:0", "-c:a", "pcm_s16le"],
        ".m4a": ["-vn", "-map", "0:a:0", "-c:a", "aac", "-b:a", "192k"],
        ".mp3": ["-vn", "-map", "0:a:0", "-c:a", "libmp3lame", "-b:a", "192k"],
    }
    if extension in audio:
        args += audio[extension]
    elif extension == ".mp4":
        args += ["-map", "0:v:0?", "-map", "0:a:0?", "-c:v", "libx264", "-preset", "medium",
                 "-crf", "23", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart"]
    else:
        raise ToolkitError("Unterstützte Zielformate: wav, m4a, mp3, mp4.")
    return args + [str(output)]


def report_output(output):
    if output.is_file():
        print(f"\nNeue Datei: {output}\nDas Original ist unverändert.")


def convert_file(ffmpeg, action, raw_path=None):
    source = input_path(raw_path)
    start, duration, decibels = 0, 30, 0
    audio_extension = source.suffix.lower() in (".wav", ".m4a", ".mp3", ".flac", ".aiff", ".aif", ".ogg", ".opus")
    if action == "extract-audio":
        extension = "wav"
    elif action in ("trim", "volume"):
        extension = "wav" if audio_extension else "mp4"
    else:
        default = "wav" if audio_extension else "mp4"
        extension = (input(f"Zielformat wav / m4a / mp3 / mp4 [{default}]: ").strip().lower() or default).lstrip(".")
        if extension not in ("wav", "m4a", "mp3", "mp4"):
            raise ToolkitError("Unbekanntes Zielformat.")
    if action == "trim":
        start = seconds(input("Beginn in Sekunden oder mm:ss [0]: ").strip() or "0")
        duration = seconds(input("Länge in Sekunden oder mm:ss [30]: ").strip() or "30", minimum=0.01)
    if action == "volume":
        raw = input("Lautstärkeänderung in dB [-3] (negativ = leiser): ").strip() or "-3"
        try:
            decibels = float(raw.replace(",", "."))
        except ValueError:
            raise ToolkitError("Bitte eine Zahl für die dB-Änderung eingeben.") from None
    label = {"convert": "converted", "extract-audio": "audio", "trim": "trimmed", "volume": "volume"}[action]
    output = unique_output(source.parent, f"{source.stem}.{label}", extension)
    arguments = file_arguments(ffmpeg, source, output, kind=action, start=start, duration=duration, decibels=decibels)
    execute(arguments)
    report_output(output)


def probe(raw_path=None):
    source = input_path(raw_path)
    result = subprocess.run(
        [executable("ffprobe"), "-v", "error", "-show_format", "-show_streams", "-of", "json", str(source)],
        capture_output=True, text=True, timeout=30,
    )
    if result.returncode:
        raise ToolkitError(result.stderr.strip() or "ffprobe konnte die Datei nicht lesen.")
    data = json.loads(result.stdout)
    info = data.get("format", {})
    print(f"Datei: {source}\nFormat: {info.get('format_long_name', 'unbekannt')}\nDauer: {info.get('duration', '?')} s\nGröße: {source.stat().st_size:,} Bytes")
    for index, stream in enumerate(data.get("streams", []), 1):
        kind = stream.get("codec_type", "?")
        details = f"{stream.get('width', '?')}×{stream.get('height', '?')}" if kind == "video" else f"{stream.get('sample_rate', '?')} Hz · {stream.get('channels', '?')} Kanäle"
        print(f"  Spur {index}: {kind} · {stream.get('codec_name', '?')} · {details}")


def latest_recording(directory):
    candidates = [path for path in directory.glob("Mikrofon-*.wav") if path.is_file() and not path.is_symlink()]
    if not candidates:
        raise ToolkitError("Noch keine cmdlib-Mikrofonaufnahme vorhanden.")
    return max(candidates, key=lambda path: (path.stat().st_mtime_ns, path.name))


def main(argv=None):
    parser = argparse.ArgumentParser(description="cmdlib · FFmpeg: aufnehmen, ansehen und verlustfrei das Original behalten.")
    sub = parser.add_subparsers(dest="action", required=True)
    sub.add_parser("devices", help="Aktuelle Audiogeräte anzeigen; nimmt nichts auf")
    recording = sub.add_parser("record", help="Mikrofon aufnehmen; Enter startet ausdrücklich")
    recording.add_argument("--seconds", default="30", help="Dauer 1–300 Sekunden; Standard 30")
    recording.add_argument("--device", help="Exakter Mikrofonname aus der aktuellen Liste")
    sub.add_parser("play-latest", help="Letzte cmdlib-Mikrofonaufnahme abspielen")
    sub.add_parser("open-recordings", help="Privaten Aufnahmeordner im Finder öffnen")
    for action, help_text in (
        ("probe", "Dateiinformationen anzeigen"), ("convert", "In WAV, M4A, MP3 oder MP4 umwandeln"),
        ("extract-audio", "Erste Tonspur als WAV speichern"), ("trim", "Zeitabschnitt als neue Datei speichern"),
        ("volume", "Lautstärke in einer neuen Datei ändern"),
    ):
        command = sub.add_parser(action, help=help_text)
        command.add_argument("path", nargs="?", help="Lokaler Dateipfad; sonst interaktive Eingabe")
    args = parser.parse_args(argv)
    try:
        if args.action == "devices":
            print("Aktuelle AVFoundation-Audiogeräte (keine Aufnahme):")
            for index, name in list_audio_devices(executable("ffmpeg")):
                print(f"  {index}: {name}")
        elif args.action == "record":
            record(executable("ffmpeg"), args.seconds, args.device)
        elif args.action == "open-recordings":
            directory = recordings_directory()
            print(f"Aufnahmeordner: {directory}")
            subprocess.run([executable("open"), str(directory)], check=True, timeout=15)
        elif args.action == "play-latest":
            source = latest_recording(recordings_directory())
            print(f"Wiedergabe: {source}\nCtrl+C beendet.")
            execute([executable("afplay"), str(source)], timeout=330)
        elif args.action == "probe":
            probe(args.path)
        else:
            convert_file(executable("ffmpeg"), args.action, args.path)
        return 0
    except (ToolkitError, OSError, subprocess.SubprocessError, json.JSONDecodeError) as error:
        print(f"\nHinweis: {error}", file=sys.stderr)
        return 1
    except (KeyboardInterrupt, EOFError):
        print("\nAbgebrochen; kein weiterer Vorgang gestartet.")
        return 130


if __name__ == "__main__":
    sys.exit(main())
