#!/usr/bin/env python3
"""cmdlib-add — Beihilfe zur Go-Version von cmdlib (github.com/dompy/cmdlib).

Analysiert einen fertigen Shell-Befehl und trägt ihn im Go-Schema in die
Bibliothek ein (commands.json). Erkennung zuerst lokal (bekannte Tools,
Risiko-Heuristik, Abhängigkeiten via `which`), optional verfeinert über den
vorhandenen Codex-Zugang. Die KI darf das Risiko nur erhöhen, nie senken.

Bibliothekspfad wie in der Go-Version:
  $CMDLIB_FILE, sonst <UserConfigDir>/cmdlib/commands.json
  (macOS: ~/Library/Application Support/cmdlib/commands.json)

Aufruf:
    cmdlib-add "ffmpeg -i in.mp4 out.mp3"
    cmdlib-add --no-ai "df -h"       # rein lokal
    cmdlib-add --ai "..."            # Codex ohne Rückfrage nutzen
    cmdlib-add --db PFAD "..."       # andere Bibliotheksdatei
    cmdlib-add                       # fragt interaktiv nach dem Befehl
"""
import datetime
import hashlib
import json
import os
import re
import shlex
import shutil
import subprocess
import sys

RISKS = ("READ", "CONNECT", "WRITE", "DANGER")
RANK = {r: i for i, r in enumerate(RISKS)}

# --------------------------------------------------------------------- Pfade
def default_db_path():
    if os.environ.get("CMDLIB_FILE"):
        return os.environ["CMDLIB_FILE"]
    if sys.platform == "darwin":
        base = os.path.expanduser("~/Library/Application Support")
    else:
        base = os.environ.get("XDG_CONFIG_HOME") or os.path.expanduser("~/.config")
    return os.path.join(base, "cmdlib", "commands.json")

# --------------------------------------------------------- lokales Wissen
# program -> (kategorie, risiko, beschreibung, erklärung, voraussetzung)
KNOWN = {
    "ffmpeg":    ("FFmpeg", "WRITE",  "Audio/Video verarbeiten mit FFmpeg.", "Erzeugt neue Ausgabedateien; Originale bleiben unverändert, sofern nicht -y gesetzt ist.", "FFmpeg installiert (z. B. brew install ffmpeg)."),
    "ffprobe":   ("FFmpeg", "READ",   "Mediendatei analysieren.", "Liest nur Metadaten der Datei; ändert nichts.", "FFmpeg/ffprobe installiert."),
    "journalctl":("System", "READ",   "System-Logs anzeigen.", "Liest das Journal; ändert keine Logs.", "systemd-Journal (Linux)."),
    "systemctl": ("System", "WRITE",  "Dienst steuern.", "Kann Dienste starten, stoppen oder neu laden.", "systemd (Linux), ggf. sudo."),
    "launchctl": ("System", "WRITE",  "macOS-LaunchAgent steuern.", "Lädt oder entlädt Launch-Agents des Benutzers.", "macOS."),
    "df":        ("System", "READ",   "Freien Speicherplatz anzeigen.", "Liest Dateisystem-Statistiken; ändert nichts.", None),
    "du":        ("System", "READ",   "Belegten Speicher pro Ordner anzeigen.", "Liest Verzeichnisgrößen; ändert nichts.", None),
    "top":       ("System", "READ",   "Laufende Prozesse anzeigen.", "Zeigt Prozessliste live; ändert nichts.", None),
    "ps":        ("System", "READ",   "Prozessliste anzeigen.", "Liest Prozessinformationen; ändert nichts.", None),
    "kill":      ("System", "WRITE",  "Prozess beenden.", "Sendet ein Signal an einen laufenden Prozess.", None),
    "brew":      ("System", "WRITE",  "Homebrew-Paketverwaltung.", "Kann Software installieren oder aktualisieren.", "Homebrew installiert."),
    "ls":        ("Dateien","READ",   "Dateien auflisten.", "Liest Verzeichniseinträge; ändert nichts.", None),
    "find":      ("Dateien","READ",   "Dateien suchen.", "Durchsucht Verzeichnisse; ohne -delete/-exec nur lesend.", None),
    "grep":      ("Dateien","READ",   "Text in Dateien suchen.", "Liest Dateien und gibt Treffer aus.", None),
    "tar":       ("Dateien","WRITE",  "Archiv packen/entpacken.", "Erstellt oder entpackt Dateien im Zielordner.", None),
    "rsync":     ("Dateien","WRITE",  "Dateien synchronisieren.", "Kopiert Dateien; mit --delete werden Ziel-Dateien entfernt.", "rsync installiert."),
    "open":      ("Dateien","READ",   "Datei/Ordner öffnen (macOS).", "Öffnet mit der Standard-App; ändert die Datei nicht.", "macOS."),
    "curl":      ("Netzwerk","CONNECT","HTTP-Anfrage senden.", "Baut eine Netzwerkverbindung auf; mit -o wird eine Datei geschrieben.", None),
    "ping":      ("Netzwerk","CONNECT","Erreichbarkeit prüfen.", "Sendet ICMP-Pakete; ändert nichts lokal.", None),
    "ssh":       ("Netzwerk","CONNECT","SSH-Verbindung öffnen.", "Verbindet zu einem entfernten Host; dortige Befehle wirken remote.", "SSH-Zugang zum Zielhost."),
    "scp":       ("Netzwerk","CONNECT","Dateien per SSH kopieren.", "Überträgt Dateien über das Netzwerk.", "SSH-Zugang zum Zielhost."),
    "git":       ("Dev",    "WRITE",  "Git-Aktion ausführen.", "Wirkt im aktuellen Repository.", "Git installiert; im Repository ausführen."),
    "docker":    ("Dev",    "WRITE",  "Docker-Aktion ausführen.", "Startet oder verwaltet Container.", "Docker installiert und gestartet."),
    "python3":   ("Dev",    "WRITE",  "Python-Skript ausführen.", "Führt das angegebene Skript aus.", "Python 3 installiert."),
    "cat":       ("Dateien","READ",   "Dateiinhalt anzeigen.", "Gibt Dateiinhalt aus; mit > wird eine Datei geschrieben.", None),
}
READ_ONLY = {"cat", "head", "tail", "less", "stat", "which", "whoami", "date",
             "uptime", "ifconfig", "uname", "pwd", "printf", "echo", "wc"}
DANGEROUS = {"rm", "dd", "mkfs", "shred", "diskutil", "format", "mkfs.ext4"}
DANGEROUS_FLAGS = {"-delete", "--delete", "--delete-after", "--delete-before"}
PREFIXES = {"sudo", "env", "nohup", "nice", "time", "caffeinate", "doas", "xargs"}
PREFIX_ARG_OPTS = {"-u", "-g", "--user", "--group"}  # Prefix-Optionen mit eigenem Argument (z. B. sudo -u root)

def tokenize(text):
    try:
        return shlex.split(text)
    except ValueError:
        return text.split()

def strip_prefixes(toks):
    """sudo/env/… samt ihrer Optionen (z. B. `sudo -u root`) überspringen."""
    while toks:
        t = toks[0]
        if t in PREFIXES or ("=" in t and not t.startswith("-")):
            toks = toks[1:]
        elif t.startswith("-"):
            toks = toks[2:] if t in PREFIX_ARG_OPTS and len(toks) > 1 else toks[1:]
        else:
            break
    return toks

def programs_in(command):
    """Alle aufgerufenen Programme (auch hinter | ; && ||), Prefixe übersprungen."""
    progs = []
    for part in re.split(r"\|\||&&|[|;]", command):
        toks = strip_prefixes(tokenize(part.strip()))
        if toks:
            progs.append(os.path.basename(toks[0]))
    return progs

def redirects_output(command):
    """Erkennt Ausgabe-Umleitungen wie `> datei`, `>>datei`, `2>fehler.log`."""
    return bool(re.search(r"(^|\s)\d?>{1,2}", command))

def prog_risk(p):
    if p in KNOWN:
        return KNOWN[p][1]
    if p in READ_ONLY:
        return "READ"
    return "WRITE"  # Unbekanntes konservativ einstufen

def local_risk(command, progs):
    all_toks = tokenize(command)
    if any(p in DANGEROUS for p in progs) or any(t in DANGEROUS_FLAGS for t in all_toks):
        return "DANGER"  # löscht/überschreibt Daten (rm, dd, find -delete, rsync --delete …)
    risk = max((prog_risk(p) for p in progs), key=lambda r: RANK[r], default="WRITE")
    if redirects_output(command) and RANK[risk] < RANK["WRITE"]:
        risk = "WRITE"  # z. B. `cat input > output` schreibt eine Datei
    return risk

def analyze_local(command):
    progs = programs_in(command)
    main = progs[0] if progs else (command.split() or ["?"])[0]
    known = KNOWN.get(main)
    # Für den Namen nur den ersten Abschnitt bis zur ersten Umleitung/Pipe nutzen
    first = re.split(r"\|\||&&|[|;]", command)[0]
    head = []
    for t in strip_prefixes(tokenize(first)):
        if re.match(r"^\d?>{1,2}$|^<$", t):
            break
        head.append(t)
    args = [t for t in head if not t.startswith("-")][1:3]
    name = f"{main} {' '.join(args)}".strip() if args else \
           (known[2].rstrip(".") if known else f"{main} ausführen")
    missing = []
    for p in dict.fromkeys(progs):
        if shutil.which(p) is None:
            missing.append(p)
    prereq_parts = [k[4] for p in dict.fromkeys(progs) if (k := KNOWN.get(p)) and k[4]]
    if missing:
        prereq_parts.append("Fehlt hier noch: " + ", ".join(missing) + ".")
    entry = {
        "id": "",  # wird beim Eintragen eindeutig vergeben
        "name": name,
        "command": command,
        "description": known[2] if known else f"Führt aus: {command}",
        "explanation": known[3] if known else "",
        "tags": sorted({t for t in (main, (known[0].lower() if known else "")) if t}),
        "host": "local",
        "category": known[0] if known else "",
        "risk": local_risk(command, progs),
        "prerequisite": " ".join(dict.fromkeys(prereq_parts)),
    }
    return entry, missing

# ----------------------------------------------------------------- Codex
def analyze_codex(command):
    """Verfeinert den Eintrag über den vorhandenen Codex-Zugang.
    Gibt None zurück, wenn Codex fehlt oder etwas schiefgeht (kein Retry)."""
    if shutil.which("codex") is None:
        return None
    prompt = (
        "Analysiere diesen Shell-Befehl für eine Befehlsbibliothek. Antworte NUR "
        "mit einem JSON-Objekt (Deutsch, ohne Markdown):\n"
        f"Befehl: {command}\n"
        '{"name": "kurzer Titel", "description": "ein Satz, was er tut", '
        '"explanation": "1-2 Sätze: was er genau ändert bzw. NICHT ändert", '
        '"category": "ein Wort, z. B. System/Dateien/Netzwerk/FFmpeg/Dev", '
        '"risk": "READ|CONNECT|WRITE|DANGER", "tags": ["..."], '
        '"prerequisite": "benötigte Software/Zugänge in einem Satz"}'
    )
    try:
        out = subprocess.run(["codex", "exec", prompt], capture_output=True,
                             text=True, timeout=90).stdout
        match = re.search(r"\{.*\}", out, re.S)
        return json.loads(match.group(0)) if match else None
    except Exception:
        return None

def merge_codex(entry, ai):
    try:
        for field in ("name", "description", "explanation", "prerequisite", "category"):
            val = ai.get(field)
            if isinstance(val, str) and val.strip():
                entry[field] = val.strip()
        if isinstance(ai.get("tags"), list):
            entry["tags"] = sorted({str(t).strip().lower() for t in ai["tags"] if str(t).strip()})
        # KI darf das Risiko nur erhöhen, nie herunterstufen (DANGER bleibt DANGER)
        if ai.get("risk") in RANK and RANK[ai["risk"]] > RANK.get(entry.get("risk", "WRITE"), RANK["WRITE"]):
            entry["risk"] = ai["risk"]
    except Exception:
        pass

# -------------------------------------------------------------- Bibliothek
def load_db(path):
    with open(path, encoding="utf-8") as f:
        data = json.load(f)
    if not isinstance(data, list):
        raise ValueError("commands.json muss eine Liste sein")
    return data

def backup(path):
    """Wie die Go-Version: backups/ neben der Bibliothek, 0600/0700."""
    bdir = os.path.join(os.path.dirname(path), "backups")
    os.makedirs(bdir, mode=0o700, exist_ok=True)
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
    dest = os.path.join(bdir, f"{os.path.basename(path)}.{stamp}.cmdlib-add.bak")
    with open(path, "rb") as src, open(os.open(dest, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as out:
        out.write(src.read())
    return dest

def save_db(path, data):
    os.makedirs(os.path.dirname(path), mode=0o700, exist_ok=True)
    tmp = os.path.join(os.path.dirname(path), ".cmdlib-add.tmp.json")
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
        f.write("\n")
        f.flush()
        os.fsync(f.fileno())
    os.replace(tmp, path)

def unique_id(entry, existing_ids):
    base = re.sub(r"[^a-z0-9]+", "-", entry["name"].lower()).strip("-") or "command"
    cand = base
    if cand in existing_ids:
        h = hashlib.sha256(entry["command"].encode()).hexdigest()[:6]
        cand = f"{base}-{h}"
    n = 2
    while cand in existing_ids:
        cand = f"{base}-{n}"
        n += 1
    return cand

def match_category(entry, data):
    """Vorhandene Kategorien in ihrer Schreibweise wiederverwenden."""
    want = (entry.get("category") or "").lower()
    if not want:
        return
    for item in data:
        cat = item.get("category") or ""
        if cat.lower() == want:
            entry["category"] = cat
            return

def insert_sorted(data, entry):
    """Hinter den letzten Eintrag derselben Kategorie, sonst ans Ende."""
    cat = (entry.get("category") or "").lower()
    pos = len(data)
    for i, item in enumerate(data):
        if (item.get("category") or "").lower() == cat:
            pos = i + 1
    data.insert(pos, entry)
    return pos

# --------------------------------------------------------------------- Main
def main():
    argv = sys.argv[1:]
    use_ai, db_path = None, default_db_path()
    if "--ai" in argv:
        use_ai = True; argv.remove("--ai")
    if "--no-ai" in argv:
        use_ai = False; argv.remove("--no-ai")
    if "--db" in argv:
        i = argv.index("--db")
        db_path = argv[i + 1]
        del argv[i:i + 2]
    command = " ".join(argv).strip()
    if not command:
        try:
            command = input("Befehl: ").strip()
        except (EOFError, KeyboardInterrupt):
            sys.exit(1)
    if not command:
        sys.exit("Kein Befehl angegeben.")

    if os.path.exists(db_path):
        try:
            data = load_db(db_path)
        except Exception as exc:
            sys.exit(f"FEHLER: {db_path} ist keine gültige Bibliothek ({exc}). Nichts geändert.")
    else:
        print(f"Hinweis: {db_path} existiert noch nicht und wird neu angelegt.")
        data = []

    dup = next((i for i in data if i.get("command") == command), None)
    if dup:
        print(f"Hinweis: Befehl existiert bereits als „{dup.get('name')}“.")
        if input("Trotzdem anlegen? [j/N] ").strip().lower() not in ("j", "y"):
            sys.exit(0)

    entry, missing = analyze_local(command)

    if use_ai is None and shutil.which("codex"):
        use_ai = input("Mit Codex verfeinern? [J/n] ").strip().lower() not in ("n", "no")
    if use_ai:
        ai = analyze_codex(command)
        if ai:
            merge_codex(entry, ai)
        else:
            print("Codex nicht verfügbar oder Antwort unbrauchbar – lokale Erkennung verwendet.")

    match_category(entry, data)
    entry["id"] = unique_id(entry, {i.get("id") for i in data})
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    entry["created_at"] = now
    entry["updated_at"] = now
    entry["last_used_at"] = None
    entry["use_count"] = 0

    print("\nVorschau")
    print(f"  Name:        {entry['name']}   (id: {entry['id']})")
    print(f"  Befehl:      {entry['command']}")
    print(f"  Beschreibung: {entry['description']}")
    if entry["explanation"]:
        print(f"  Erklärung:   {entry['explanation']}")
    print(f"  Kategorie:   {entry['category'] or '(ohne)'} · Risiko: {entry['risk']} · Tags: {', '.join(entry['tags']) or '-'}")
    if entry["prerequisite"]:
        print(f"  Voraussetzung: {entry['prerequisite']}")
    if missing:
        print(f"  Auf diesem Rechner fehlt: {', '.join(missing)}")
    try:
        if input("\nEintragen? [J/n] ").strip().lower() in ("n", "no"):
            sys.exit(0)
    except (EOFError, KeyboardInterrupt):
        sys.exit(1)

    backed_up = backup(db_path) if os.path.exists(db_path) else None
    pos = insert_sorted(data, entry)
    save_db(db_path, data)
    print(f"Eingetragen an Position {pos + 1} (Kategorie „{entry['category'] or 'ohne'}“) in {db_path}.")
    if backed_up:
        print("Backup liegt im Ordner backups/ daneben.")

if __name__ == "__main__":
    main()
