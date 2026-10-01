#!/bin/sh
# Install a checksum-verified public release without touching command libraries.
set -eu
umask 077
version=${1:-v0.1.0}
case "$version" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo 'Expected a version such as v0.1.0' >&2; exit 1;; esac
os=$(uname -s); arch=$(uname -m)
case "$os" in Darwin) os=darwin;; Linux) os=linux;; *) echo 'Supported: macOS and Linux' >&2; exit 1;; esac
case "$arch" in x86_64|amd64) arch=amd64;; arm64|aarch64) arch=arm64;; *) echo 'Unsupported architecture' >&2; exit 1;; esac
base="https://github.com/dompy/cmdlib/releases/download/$version"
asset="cmdlib_${version}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
curl -fL --proto '=https' --tlsv1.2 "$base/$asset" -o "$tmp/$asset"
curl -fL --proto '=https' --tlsv1.2 "$base/checksums.txt" -o "$tmp/checksums.txt"
expected=$(awk -v name="./$asset" '$2==name {print $1}' "$tmp/checksums.txt")
[ ${#expected} -eq 64 ] || { echo 'Missing or invalid release checksum' >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp/$asset"); else actual=$(shasum -a 256 "$tmp/$asset"); fi
[ "$expected" = "${actual%% *}" ] || { echo 'Checksum mismatch' >&2; exit 1; }
tar -xzf "$tmp/$asset" -C "$tmp" cmdlib
"$tmp/cmdlib" --version
"$tmp/cmdlib" --help >/dev/null
NO_COLOR=1 "$tmp/cmdlib" --tea >/dev/null
bin="$HOME/.local/bin"
backup="$HOME/.local/share/cmdlib/backups/$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$bin" "$backup"
chmod 700 "$backup"
if [ -e "$bin/cmdlib" ] || [ -L "$bin/cmdlib" ]; then
    cp -p "$bin/cmdlib" "$backup/previous-cmdlib"
    # Restore the backed-up executable or wrapper atomically.
    cat > "$backup/rollback.sh" <<'ROLLBACK'
#!/bin/sh
set -eu
umask 077
backup=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
target="$HOME/.local/bin/cmdlib"
cp -p "$backup/previous-cmdlib" "$target.rollback-$$"
mv -f "$target.rollback-$$" "$target"
printf 'Previous cmdlib restored. Command data was not changed.\n'
ROLLBACK
    chmod 700 "$backup/rollback.sh"
fi
if [ -n "${CMDLIB_FILE:-}" ]; then library=$CMDLIB_FILE
elif [ "$os" = darwin ]; then library="$HOME/Library/Application Support/cmdlib/commands.json"
else library="${XDG_CONFIG_HOME:-$HOME/.config}/cmdlib/commands.json"; fi
if [ -f "$library" ]; then cp -p "$library" "$backup/commands.json"; chmod 600 "$backup/commands.json"; fi
staged=$(mktemp "$bin/.cmdlib-install.XXXXXX")
cp "$tmp/cmdlib" "$staged"
chmod 755 "$staged"
mv -f "$staged" "$bin/cmdlib"
printf 'Installed %s to %s\nPrivate backups: %s\n' "$version" "$bin/cmdlib" "$backup"
printf 'Ensure $HOME/.local/bin is in PATH, then run: cmdlib --version\n'
