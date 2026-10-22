#!/usr/bin/env python3
"""Build an iSH-only Linux/386 binary without modifying the installed Go toolchain."""
import argparse
import json
import os
import shutil
import pathlib
import subprocess
import tempfile

p=argparse.ArgumentParser()
p.add_argument("--output",default="cmdlib-linux-386")
args=p.parse_args()
repo=pathlib.Path(__file__).resolve().parent.parent
goroot=pathlib.Path(subprocess.check_output(["go","env","GOROOT"],cwd=repo,text=True).strip())
with tempfile.TemporaryDirectory(prefix="cmdlib-ish-overlay-") as temp:
    work=pathlib.Path(temp)
    # Auto-downloaded toolchains live in GOMODCACHE, where overlays are forbidden.
    # Make a private hard-linked copy; the overlay never writes to those files.
    isolated=work/"toolchain"
    def copy_file(src,dst):
        try:
            return os.link(src,dst)
        except OSError:
            return shutil.copy2(src,dst)
    shutil.copytree(goroot,isolated,copy_function=copy_file,symlinks=True)
    goroot=isolated
    pidfd=goroot/"src/os/pidfd_linux.go"
    source=pidfd.read_text()
    old="func pidfdWorks() bool {\n\treturn checkPidfdOnce() == nil\n}"
    if source.count(old)!=1:
        raise SystemExit("Go pidfd implementation changed; inspect before building.")
    # iSH raises SIGSYS for this optional probe, rather than returning ENOSYS.
    patched=source.replace(old,"func pidfdWorks() bool { return false }")
    (work/"pidfd_linux.go").write_text(patched)
    waitid=goroot/"src/os/wait_waitid.go"
    if "func (p *Process) blockUntilWaitable() (bool, error)" not in waitid.read_text():
        raise SystemExit("Go wait implementation changed; inspect before building.")
    (work/"wait_waitid.go").write_text("//go:build linux\n\npackage os\n\n// iSH uses the existing wait4 fallback.\nfunc (p *Process) blockUntilWaitable() (bool, error) { return false, nil }\n")
    overlay={"Replace":{str(pidfd):str(work/"pidfd_linux.go"),str(waitid):str(work/"wait_waitid.go")}}
    (work/"overlay.json").write_text(json.dumps(overlay))
    env=dict(os.environ,GOOS="linux",GOARCH="386",GO386="softfloat",CGO_ENABLED="0",GOROOT=str(goroot),GOTOOLCHAIN="local")
    commit=subprocess.check_output(["git","rev-parse","HEAD"],cwd=repo,text=True).strip()
    subprocess.run([str(goroot/"bin/go"),"build","-overlay",str(work/"overlay.json"),"-trimpath","-ldflags",
        "-s -w -X main.version=ish-mobile-2 -X main.commit="+commit,
        "-o",str(pathlib.Path(args.output).resolve()),"."],cwd=repo,env=env,check=True)
