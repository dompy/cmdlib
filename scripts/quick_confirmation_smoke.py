#!/usr/bin/env python3
"""Native PTY tests. Only synthetic commands write to their temporary test folder."""
import fcntl, hashlib, json, os, pty, select, shlex, struct, subprocess, sys, tempfile, termios, time
from pathlib import Path
binary=str(Path(sys.argv[1]).resolve())
results=[]

def fingerprint(row):
    fields=[row[k] for k in ('id','command','risk','host','prerequisite')]
    raw=json.dumps(fields,ensure_ascii=False,separators=(',',':')).replace('&',r'\u0026').replace('<',r'\u003c').replace('>',r'\u003e').encode()
    return hashlib.sha256(raw).hexdigest()

class Session:
    def __init__(self,env):
        self.master,slave=pty.openpty()
        fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',55,160,0,0))
        self.proc=subprocess.Popen([binary],stdin=slave,stdout=slave,stderr=slave,env=env,start_new_session=True)
        os.close(slave);self.captured=bytearray()
    def pump(self,seconds):
        end=time.monotonic()+seconds
        while time.monotonic()<end:
            if select.select([self.master],[],[],max(0,min(.05,end-time.monotonic())))[0]:
                try:chunk=os.read(self.master,65536)
                except OSError:return
                if not chunk:return
                self.captured.extend(chunk)
                if b'\x1b]11;?' in chunk:os.write(self.master,b'\x1b]11;rgb:0000/0000/0000\x07')
                if b'\x1b[6n' in chunk:os.write(self.master,b'\x1b[1;1R')
    def wait(self,needle,timeout=7):
        end=time.monotonic()+timeout
        while needle not in self.captured and time.monotonic()<end:self.pump(.05)
        if needle not in self.captured:raise AssertionError('Missing '+repr(needle)+' '+repr(bytes(self.captured[-1600:])))
    def send(self,keys,delay=.12):os.write(self.master,keys);self.pump(delay)
    def close(self):
        if self.proc.poll() is None:
            self.send(b'\x03');self.pump(.1)
        if self.proc.poll() is None:self.proc.terminate()
        self.proc.wait(timeout=5);os.close(self.master)

cases=[('space',True,b' ',True),('y',False,b'y',True),('one',False,b'1',True),('enter',False,b'\r',True),
       ('legacy_policy_no_auto',True,None,False),('cancel_n',True,b'n',False),('cancel_zero',True,b'0',False),
       ('cancel_backspace',True,b'\x7f',False),('cancel_delete',True,b'\x1b[3~',False),('cancel_escape',True,b'\x1b',False),
       ('paste_is_not_yes',True,b'\x1b[200~y\x1b[201~',False),('blur',True,b'\x1b[O',False),
       ('not_approved',False,None,False),('write_stays_explicit',True,b' ',False)]
with tempfile.TemporaryDirectory(prefix='cmdlib-quick-pty-') as tmp:
    base=Path(tmp)
    for name,auto,keys,will_run in cases:
        folder=base/name;folder.mkdir();lib=folder/'commands.json';marker=folder/'executed.txt'
        row={'id':'fixture','name':'Synthetic quick confirmation','command':"printf 'ran\\n' >> "+shlex.quote(str(marker)),
             'description':'Harmless test-only local marker','explanation':'Fixture only','tags':['fixture'],'host':'synthetic-local',
             'risk':'WRITE' if name=='write_stays_explicit' else 'READ','prerequisite':'',
             'created_at':'2025-01-01T00:00:00Z','updated_at':'2025-01-01T00:00:00Z','last_used_at':None,'use_count':0}
        lib.write_text(json.dumps([row]))
        if auto:Path(str(lib)+'.run-policy.json').write_text(json.dumps({'version':1,'delay_ms':2500,'approved_fingerprints':[fingerprint(row)]}))
        env=dict(os.environ,CMDLIB_FILE=str(lib),NO_COLOR='1',TERM='xterm-256color')
        session=Session(env)
        try:
            session.wait(b'Synthetic quick confirmation')
            # Merely viewing the fixture must not launch it.
            if name=='legacy_policy_no_auto':session.pump(2.7);assert not marker.exists(),'executed on selection'
            start=time.monotonic();session.send(b'\r')
            session.wait(b'Execution host:')
            if auto: assert b'Start in 2.5s' not in session.captured, 'timer armed'
            assert not marker.exists(),'ran before confirmation'
            if keys is not None:session.send(keys)
            if will_run:
                session.wait(b'Exit status: 0')
                elapsed=time.monotonic()-start
                assert marker.read_text()=='ran\n','wrong or duplicate invocation'
                assert json.loads(lib.read_text())[0]['use_count']==1
                assert elapsed < 7, 'explicit confirmation stalled'
                session.send(b'\r');session.wait(b'Command finished.')
                session.pump(.3);assert marker.read_text()=='ran\n','stale timer duplicate'
                session.send(b'q')
            else:
                session.pump(2.8)
                assert not marker.exists(),'cancelled/unapproved command executed: '+name
                assert json.loads(lib.read_text())[0]['use_count']==0
            results.append({'case':name,'passed':True,'executions':1 if will_run else 0})
        finally:session.close()
print(json.dumps({'native_pty_tests':results,'personal_commands_executed':0},indent=2))