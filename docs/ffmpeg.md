# FFmpeg in cmdlib

The optional `scripts/cmdlib-ffmpeg.py` toolkit provides nine keyboard-friendly
actions. It uses Python's standard library and an existing FFmpeg installation;
it does not install packages or start an action merely by being installed.
The installation's private command library can place these entries in the
expandable **FFmpeg** category. The public template contains no personal paths.

| Action | Purpose |
| --- | --- |
| `devices` | List the current macOS audio input devices; no recording |
| `record` | Record the explicitly shown microphone for 30 seconds by default |
| `play-latest` | Play the last recording created by this toolkit |
| `open-recordings` | Open the private recording folder in Finder |
| `probe` | Show a local file's format, duration, size and media streams |
| `convert` | Save a new WAV, M4A, MP3 or MP4 file |
| `extract-audio` | Save the first audio track as a new WAV file |
| `trim` | Save a chosen time interval in a new file |
| `volume` | Change volume in a new file; default −3 dB |

Run the toolkit with `python3 /installed/path/cmdlib-ffmpeg.py ACTION`.
The last five actions ask for a file path. Drag a file into the terminal and
press Enter; quoted and shell-escaped paths with spaces are accepted. A path
may also be supplied as a quoted argument. These are local-file operations;
network URLs are not accepted.

## Microphone recording

`record` enumerates devices afresh and selects the uniquely named built-in
MacBook microphone when present, rather than relying on a stale numeric index.
Otherwise it asks which explicitly named microphone to use. It prints source,
duration and destination, then waits for Enter. Anything typed before that Enter
cancels. It never records a camera, screen, virtual system-output device or an
unidentified default device.

The default duration is 30 seconds. `record --seconds 60` changes it; the range
is 1–300 seconds. `--device 'Exact microphone name'` selects a freshly listed,
explicitly named microphone. macOS may ask for microphone permission; the helper
does not bypass that prompt or change audio routing. Play Alex over speakers if
his output should be captured through the microphone.

The recording is mono 48 kHz PCM16 WAV, without loudness or speed processing.
It is stored under `~/Library/Application Support/cmdlib/recordings`, with a
private folder (0700) and recording (0600). Each filename is unique. A visible
approximate countdown starts when FFmpeg starts; if macOS pauses for permission,
the displayed countdown can reach zero before the actual recording has finished.
FFmpeg's duration limit remains authoritative. A wall-clock limit ends stalled
attempts after the requested duration plus 30 seconds. Ctrl+C asks FFmpeg to
finish the file; a shorter recording or a partial file remains available and is
not silently deleted. A failed first permission attempt can be started again
by the owner after permitting access.

`play-latest` only selects toolkit-owned `Mikrofon-*.wav` files inside that folder,
ignores symbolic links, and uses the standard macOS `afplay` player. Ctrl+C stops
playback. `open-recordings` uses Finder. No recording is uploaded automatically.

## File conversion

All outputs use unique names next to the original. FFmpeg is always passed `-n`;
existing files are never overwritten. The original is retained. `trim` re-encodes
the range rather than making an imprecise keyframe-only cut. Audio-only inputs
produce WAV by default for trim and volume; other media defaults to MP4. Video
MP4 output uses H.264/AAC, and MP3 uses libmp3lame, which must be present in the
installed FFmpeg build. Unsupported codecs produce an error, not an automatic
installation or an altered original. Loudness changes accept −60 to +24 dB;
positive gain may clip, so listen to the new file before using it.

These conversion actions create new files and can take time; the helper bounds
each process to one hour and stops on Ctrl+C. No cloud service, new account,
microphone capture during maintenance, or automatic media operation is involved.

## Development checks

```sh
python3 scripts/test_cmdlib_ffmpeg.py
python3 scripts/cmdlib-ffmpeg.py --help
```

These pure tests exercise command construction, path parsing, device selection,
bounded durations and cancellation. They never record or play audio. Real
recording requires an owner-operated microphone-permission check on the Mac.

Primary documentation:
[FFmpeg AVFoundation devices](https://ffmpeg.org/ffmpeg-devices.html#avfoundation)
and [FFmpeg command options](https://ffmpeg.org/ffmpeg.html).
