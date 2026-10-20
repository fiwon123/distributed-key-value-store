#!/usr/bin/env python3
"""Cross-platform dev sandbox launcher (#436)."""
from __future__ import annotations
import os, re, shutil, subprocess, sys, time
from pathlib import Path
REPO_ROOT = Path(__file__).resolve().parents[1]
BASE = ["-f", str(REPO_ROOT / "docker-compose.yml")]
AGENT = ["-f", str(REPO_ROOT / "docker-compose.yml"), "-f", str(REPO_ROOT / "docker-compose.agent.yml")]
IS_WINDOWS = os.name == "nt"
def log(m): print(f"[sandbox] {m}", file=sys.stderr, flush=True)
def err(m): print(f"ERROR: {m}", file=sys.stderr, flush=True)
def host_home(): return Path(os.environ.get("HOST_HOME") or os.environ.get("HOME") or os.environ.get("USERPROFILE") or Path.home()).expanduser()
def opencode_bin(): p = Path(os.environ.get("OPENCODE_BIN", "")).expanduser(); return p if str(p) else host_home() / ".opencode" / "bin" / "opencode"
def is_elf(p): 
    try: return p.open("rb").read(4) == b"\x7fELF"
    except: return False
def agent_avail(): p = opencode_bin(); return p.is_file() and os.access(p, os.X_OK) and is_elf(p)
def compose_files(a): return AGENT if a else BASE
def ensure_mounts():
    h = host_home(); 
    for d in [h/".config/opencode", h/".config/gh"]: d.mkdir(parents=True, exist_ok=True)
    (h/".gitconfig").touch(exist_ok=True)
def uidgid():
    if os.environ.get("HOST_UID") and os.environ.get("HOST_GID"): return os.environ["HOST_UID"], os.environ["HOST_GID"]
    if not IS_WINDOWS and hasattr(os,"getuid"): return str(os.getuid()), str(os.getgid())
    return "1000","1000"
def gh_tok():
    for n in ("GH_TOKEN","GITHUB_TOKEN"):
        t = os.environ.get(n,"")
        if t: return t
    gh = shutil.which("gh")
    if gh:
        try:
            r = subprocess.run([gh,"auth","token"], capture_output=True, text=True, timeout=5, check=False)
            if r.returncode==0 and r.stdout.strip(): return r.stdout.strip()
        except: pass
    return ""
def env_g():
    e = os.environ.copy(); u,g = uidgid(); e["HOST_UID"]=u; e["HOST_GID"]=g; t=gh_tok()
    if t: e["GH_TOKEN"]=e["GITHUB_TOKEN"]=t
    return e
def rc(argv, a=False, cap=False):
    if not shutil.which("docker"): err("docker not found"); raise SystemExit(1)
    try: return subprocess.run(["docker","compose",*compose_files(a),*argv], cwd=REPO_ROOT, env=env_g(), capture_output=cap, text=True, check=False)
    except KeyboardInterrupt: return subprocess.CompletedProcess(["docker","compose"],130,"","")
def preflight(ra):
    ensure_mounts()
    if ra and not agent_avail(): err(f"Linux ELF opencode required at {opencode_bin()}"); raise SystemExit(1)
def cmd_up(): preflight(False); return rc(["up","-d","--build"])[0] if isinstance(rc(["up","-d","--build"]),int)==False else rc(["up","-d","--build"]).returncode
def cmd_down(): return rc(["down"]).returncode
def cmd_log(): return rc(["logs","-f","dev"]).returncode
def cmd_exec(): return rc(["exec","dev","bash"]).returncode
def cmd_opencode(x):
    preflight(True)
    q=" ".join(x) if x else ""
    r = sys.stdin.isatty()
    return rc(["exec","-it" if r else "-T","dev","zsh","-lc",f"cd /workspace && opencode {q}".strip() if shutil.which("zsh") else f"cd /workspace && bash -lc 'opencode {q}'"], True)[0] if False else rc(["exec","-it" if r else "-T","dev","bash","-lc",f"cd /workspace && opencode {q}".strip()], True).returncode
def cmd_shell():
    preflight(True); return rc(["exec","-it","dev","bash"]).returncode
def main(a):
    if not a or a[0] in ("help","-h","--help"): print("usage: python scripts/dev-up.py {dev-up|dev-down|dev-log|dev-exec|opencode|shell}"); return 0
    c=a[0]; rest=a[1:]
    if c=="dev-up": return rc(["up","-d","--build"])[0] if hasattr(rc(["up","-d","--build"]),"returncode")==False else rc(["up","-d","--build"]).returncode
    if c=="dev-down": return rc(["down"]).returncode
    if c=="dev-log": return rc(["logs","-f","dev"]).returncode
    if c=="dev-exec": return rc(["exec","dev","bash"]).returncode
    if c=="shell": preflight(True); return rc(["exec","-it","dev","bash"],True).returncode
    if c=="opencode": preflight(True); q=" ".join(rest); r=sys.stdin.isatty(); return rc(["exec","-it" if r else "-T","dev","bash","-lc",f"cd /workspace && opencode {q}".strip() or "opencode"],True).returncode
    err(f"unknown {c}"); return 2
if __name__=="__main__": raise SystemExit(main(sys.argv[1:]))
