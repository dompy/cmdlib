#!/usr/bin/env python3
"""Linux/386 process regression with iSH-like SIGSYS traps, plus a real TUI round trip."""
import ctypes
import errno
import fcntl
import json
import os
import pathlib
import pty
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time

def restrict():
    # Only the 32-bit guest is filtered. Missing optional iSH probes raise SIGSYS.
    class Filter(ctypes.Structure):
        _fields_=[("code",ctypes.c_ushort),("jt",ctypes.c_ubyte),("jf",ctypes.c_ubyte),("k",ctypes.c_uint)]
    class Program(ctypes.Structure):
        _fields_=[("length",ctypes.c_ushort),("filter",ctypes.POINTER(Filter))]
    rules=[(0x20,0,0,4),(0x15,0,6,0x40000003),(0x20,0,0,0),
           (0x15,3,0,434),(0x15,2,0,424),(0x15,1,0,435),(0x15,0,1,284),
           (0x06,0,0,0x00030000),(0x06,0,0,0x7fff0000)]
    filters=(Filter*len(rules))(*(Filter(*r) for r in rules))
    program=Program(len(rules),filters)
    libc=ctypes.CDLL(None,use_errno=True)
    if libc.prctl(38,1,0,0,0) or libc.prctl(22,2,ctypes.byref(program),0,0):
        raise OSError(ctypes.get_errno(),"seccomp setup")

if len(sys.argv)>1 and sys.argv[1]=="--trap-exec":
    restrict()
    os.execv(sys.argv[2],sys.argv[2:])

binary=str(pathlib.Path(sys.argv[1]).resolve())
env=dict(os.environ,GOMAXPROCS="1",GODEBUG="asyncpreemptoff=1",CMDLIB_MOBILE="1",TERM="xterm-256color")
self_check=subprocess.run([sys.executable,__file__,"--trap-exec",binary,"--ish-check"],env=env)
if self_check.returncode!=0:
    raise SystemExit("Process compatibility failed: "+str(self_check.returncode))
with tempfile.TemporaryDirectory(prefix="cmdlib-ish-smoke-") as temp:
    path=pathlib.Path(temp)/"commands.json"
    path.write_text(json.dumps([dict(id="synthetic",name="Synthetic check",command="printf 'CMDLIB_CHILD_OK\\n'",description="Harmless test",explanation="",tags=[],host="test",risk="CONNECT",prerequisite="",created_at="2026-01-01T00:00:00Z",updated_at="2026-01-01T00:00:00Z",last_used_at=None,use_count=0)]))
    env["CMDLIB_FILE"]=str(path)
    pid,fd=pty.fork()
    if pid==0:
        os.execve(sys.executable,[sys.executable,__file__,"--trap-exec",binary],env)
    fcntl.ioctl(fd,termios.TIOCSWINSZ,struct.pack("HHHH",22,36,0,0))
    output=bytearray(); phase=0; result=None; deadline=time.monotonic()+15
    while time.monotonic()<deadline:
        ready,_,_=select.select([fd],[],[],0.1)
        if ready:
            try: chunk=os.read(fd,65536)
            except OSError as error:
                if error.errno==errno.EIO: break
                raise
            output.extend(chunk)
            if b"\x1b[6n" in chunk: os.write(fd,b"\x1b[1;1R")
        raw=bytes(output)
        if phase==0 and b"Synthetic check" in raw:
            os.write(fd,b"1");phase=1;output.clear()
        elif phase==1 and b"Verbinden?" in raw:
            os.write(fd,b"\r");phase=2;output.clear()
        elif phase==2 and b"CMDLIB_CHILD_OK" in raw and b"[N]" in raw:
            os.write(fd,b"q");phase=3
        finished,status=os.waitpid(pid,os.WNOHANG)
        if finished: result=os.waitstatus_to_exitcode(status);break
    if result is None:
        finished,status=os.waitpid(pid,os.WNOHANG)
        if finished: result=os.waitstatus_to_exitcode(status)
    if result is None:
        os.kill(pid,signal.SIGTERM);os.waitpid(pid,0)
        raise SystemExit("TUI round trip timed out; phase="+str(phase)+"; tail="+repr(bytes(output)[-600:]))
    if result!=0 or phase!=3: raise SystemExit("TUI failed; exit="+str(result)+"; phase="+str(phase))
    data=json.loads(path.read_text())
    assert len(data)==1 and data[0]["use_count"]==1 and data[0]["command"]=="printf 'CMDLIB_CHILD_OK\\n'"
print("PASS: blocked pidfd/clone3/waitid probes; spawn, wait, mobile confirmation, child execution, return, library preserved")
