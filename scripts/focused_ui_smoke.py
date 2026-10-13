#!/usr/bin/env python3
"""Native macOS PTY checks with isolated fixtures and explicit confirmations."""
import fcntl, json, os, pty, select, struct, subprocess, tempfile, termios, time, socket
from pathlib import Path
fixture = tempfile.TemporaryDirectory(prefix="cmdlib-focused-native-")
root = Path(fixture.name)
db = root / "commands.json"
marker = root / "executed"
commands = [
 dict(id="anchor", name="Anchor Fixture", command="printf 'SYNTHETIC_ONLY\\n'", description="Synthetic description", explanation="Synthetic explanation", host="example-label", risk="READ", tags=["fixture"], created_at="2025-01-01T00:00:00Z", updated_at="2025-01-01T00:00:00Z", use_count=0),
 dict(id="write", name="Write Fixture", command="printf SYNTHETIC_EXECUTED > "+str(marker), description="Synthetic write", explanation="", host="example-label", risk="WRITE", tags=["fixture"], created_at="2025-01-01T00:00:00Z", updated_at="2025-01-01T00:00:00Z", use_count=0)
]
db.write_text(json.dumps(commands))
master, slave = pty.openpty()
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack("HHHH",30,100,0,0))
env = dict(os.environ,CMDLIB_FILE=str(db),NO_COLOR="1",TERM="xterm-256color")
process = subprocess.Popen(["/bin/zsh","-lc", "exec "+__import__("shlex").quote(str(Path(__import__("sys").argv[1]).resolve()))],stdin=slave,stdout=slave,stderr=slave,env=env,start_new_session=True)
os.close(slave)
captured=bytearray()
def pump(seconds=.4):
 deadline=time.monotonic()+seconds
 while time.monotonic()<deadline:
  if not select.select([master],[],[],.05)[0]: continue
  try: chunk=os.read(master,65536)
  except OSError: break
  captured.extend(chunk)
  if b"\x1b]11;?" in chunk: os.write(master,b"\x1b]11;rgb:0000/0000/0000\x07")
  if b"\x1b[6n" in chunk: os.write(master,b"\x1b[1;1R")
def send(data):
 captured.clear()
 if data == b"WRITE\r":
  for ch in data: os.write(master,bytes([ch]));pump(.08)
 else: os.write(master,data);pump()
def state():
 text=captured.decode(errors="replace").replace(socket.gethostname(),"example-machine").replace(str(root),"<isolated-fixture>")
 return dict(alive=process.poll() is None, names=[n for n in ["Anchor Fixture","Write Fixture"] if n in text], confirmation="Execution host:" in text, error="Save failed" in text, executed=marker.exists(), uses=[c["use_count"] for c in json.loads(db.read_text())], description=json.loads(db.read_text())[0]["description"])
pump(1)
sequence = json.loads("[{\"keys\":\"/\\u001b[200~Write\\u001b[201~\\r\",\"expect\":{\"executed\":false,\"uses\":[0,0]}},{\"keys\":\"\\r\",\"expect\":{\"confirmation\":true,\"executed\":false,\"uses\":[0,0]}},{\"keys\":\"\\u001b[200~WRITE\\u001b[201~\\r\",\"expect\":{\"executed\":false,\"uses\":[0,0]}},{\"keys\":\"\\u001b\",\"expect\":{\"executed\":false,\"uses\":[0,0]}},{\"keys\":\"\\r\",\"expect\":{\"confirmation\":true,\"executed\":false,\"uses\":[0,0]}},{\"keys\":\"WRITE\\r\",\"expect\":{\"executed\":true,\"uses\":[0,1]}},{\"keys\":\"\\r\",\"expect\":{\"alive\":true,\"executed\":true,\"uses\":[0,1]}},{\"keys\":\"\\u001b\",\"expect\":{\"alive\":true,\"uses\":[0,1]}},{\"keys\":\"\\u001b[A\\t\\u001b[C\",\"expect\":{\"alive\":true,\"uses\":[0,1]}},{\"keys\":\"e\\u0015\\u001b[200~Edited synthetic description\\u001b[201~\\r\",\"expect\":{\"description\":\"Edited synthetic description\",\"uses\":[0,1]}},{\"keys\":\"e\\u0015Cancelled draft\\u001b\",\"expect\":{\"description\":\"Edited synthetic description\",\"uses\":[0,1]}},{\"keys\":\"\\tq\",\"expect\":{\"alive\":false,\"uses\":[0,1]}}]")
try:
 for req in sequence:
  send(req['keys'].encode())
  result=state()
  for name,value in req.get('expect',{}).items(): assert result[name]==value,(name,result)
 print(json.dumps({'native_focused_ui': 'passed', 'steps': len(sequence), 'personal_commands_executed': 0},indent=2))
finally:
 if process.poll() is None: process.terminate();process.wait(timeout=3)
 os.close(master);fixture.cleanup()