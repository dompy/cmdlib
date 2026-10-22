#!/usr/bin/env python3
"""Package a checked iSH build into a private, self-contained shell installer."""
import argparse
import base64
import hashlib
import io
import json
import pathlib
import tarfile

p=argparse.ArgumentParser()
p.add_argument("--binary",required=True)
p.add_argument("--output",required=True)
p.add_argument("--seed",help="Optional private JSON, never committed or published")
args=p.parse_args()
binary=pathlib.Path(args.binary).read_bytes()
if not binary.startswith(b"\x7fELF\x01"):
    raise SystemExit("Expected a 32-bit ELF binary")
files={"cmdlib-linux-386":binary}
if args.seed:
    seed=pathlib.Path(args.seed).read_bytes()
    if not isinstance(json.loads(seed),list):
        raise SystemExit("Seed must be a JSON array")
    files["commands.json"]=seed
archive=io.BytesIO()
with tarfile.open(fileobj=archive,mode="w:gz") as tf:
    for name,data in files.items():
        entry=tarfile.TarInfo(name)
        entry.size=len(data);entry.mode=0o700 if name.startswith("cmdlib") else 0o600
        tf.addfile(entry,io.BytesIO(data))
digest=hashlib.sha256(binary).hexdigest()
head=r"""#!/bin/sh
set -eu
umask 077
case "$(uname -m)" in
  i386|i486|i586|i686) ;;
  *) printf 'Bitte lokal in iSH ausführen (localhost), nicht auf Hetzner.\n' >&2; exit 1 ;;
esac
[ "$(id -u)" = 0 ] || { printf 'Dieser Installer erwartet den lokalen iSH-Benutzer root.\n' >&2; exit 1; }
task_tmp=$(mktemp -d /tmp/cmdlib-ish.XXXXXX)
trap 'rm -rf "$task_tmp"' EXIT HUP INT TERM
base64 -d <<'CMDLIB_PRIVATE_ARCHIVE' | tar -xzf - -C "$task_tmp"
"""
tail=r"""CMDLIB_PRIVATE_ARCHIVE
printf '%s  %s\n' '__DIGEST__' "$task_tmp/cmdlib-linux-386" | sha256sum -c -
printf 'Prüfe Ausführen und Zurückkehren ...\n'
timeout 30 env GOMAXPROCS=1 GODEBUG=asyncpreemptoff=1 "$task_tmp/cmdlib-linux-386" --ish-check
task_stamp=$(date -u +%Y%m%dT%H%M%SZ)-$$
task_backup="$HOME/.local/share/cmdlib/backups/ish-$task_stamp"
mkdir -p "$task_backup" /usr/local/bin /usr/local/lib/cmdlib
chmod 700 "$task_backup"
for task_file in /usr/local/bin/cmdlib /usr/local/lib/cmdlib/cmdlib-linux-386; do
    if [ -e "$task_file" ] || [ -L "$task_file" ]; then
        cp -p "$task_file" "$task_backup/$(basename "$task_file")"
    fi
done
task_library="${CMDLIB_FILE:-${XDG_CONFIG_HOME:-$HOME/.config}/cmdlib/commands.json}"
if [ -f "$task_library" ]; then
    cp -p "$task_library" "$task_backup/commands.json"
    chmod 600 "$task_backup/commands.json"
elif [ ! -e "$task_library" ] && [ ! -L "$task_library" ] && [ -f "$task_tmp/commands.json" ]; then
    mkdir -p "$(dirname "$task_library")"
    cp "$task_tmp/commands.json" "$task_library"
    chmod 600 "$task_library"
fi
cat > "$task_tmp/cmdlib" <<'CMDLIB_LAUNCHER'
#!/bin/sh
export GOMAXPROCS=1
export GODEBUG=asyncpreemptoff=1
export CMDLIB_MOBILE=1
export CMDLIB_ACTIVE=1
exec /usr/local/lib/cmdlib/cmdlib-linux-386 "$@"
CMDLIB_LAUNCHER
cp "$task_tmp/cmdlib-linux-386" /usr/local/lib/cmdlib/.cmdlib-new
chmod 755 /usr/local/lib/cmdlib/.cmdlib-new
mv /usr/local/lib/cmdlib/.cmdlib-new /usr/local/lib/cmdlib/cmdlib-linux-386
cp "$task_tmp/cmdlib" /usr/local/bin/.cmdlib-new
chmod 755 /usr/local/bin/.cmdlib-new
mv /usr/local/bin/.cmdlib-new /usr/local/bin/cmdlib
if ! grep -Fq '# cmdlib iSH menu (managed)' "$HOME/.profile" 2>/dev/null; then
    if [ -e "$HOME/.profile" ]; then cp -p "$HOME/.profile" "$task_backup/profile"; fi
    cat >> "$HOME/.profile" <<'CMDLIB_PROFILE'

# cmdlib iSH menu (managed)
if [ -t 0 ] && [ -t 1 ] && [ -z "${SSH_CONNECTION:-}" ] && [ "${CMDLIB_ACTIVE:-0}" != 1 ] && [ "${CMDLIB_AUTOSTART:-1}" = 1 ] && [ -x /usr/local/bin/cmdlib ]; then
    CMDLIB_ACTIVE=1 /usr/local/bin/cmdlib
fi
CMDLIB_PROFILE
fi
/usr/local/bin/cmdlib --version
printf '\nFertig. Befehle beibehalten. Start: cmdlib\n'
"""
# Python raw strings keep shell backslashes literal; shell parameters need no escaping.
tail=tail.replace("\\$","$")
script=head+base64.encodebytes(archive.getvalue()).decode()+tail.replace("__DIGEST__",digest)
path=pathlib.Path(args.output)
path.write_text(script)
path.chmod(0o700)
print("Packaged installer; binary SHA256="+digest)
