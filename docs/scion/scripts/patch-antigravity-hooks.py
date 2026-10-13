#!/usr/bin/env python3
"""Make Antigravity's Scion hooks run with telemetry off. Runs ON the Hub VM, as the Hub's OS user.

Why (lessons-learned.md L6): with Hub telemetry and cloud export enabled, `sciontool hook --dialect=antigravity`
prints its decision but doesn't exit. Antigravity (`agy`) kills the hook after its 10 s timeout and aborts the
tool call, so the agent can't run any tool. The Hub injects SCION_TELEMETRY_ENABLED=true into every agent and that
wins over template settings, so the fix has to be in the hook command itself:

    ... | SCION_TELEMETRY_ENABLED=false sciontool hook --dialect=antigravity

Two places generate or hold that command:
  1. ~/.scion/harness-configs/antigravity/provision.py   writes hooks.json at every Antigravity agent start
                                                         (a nightly auto-update of Scion can revert this patch)
  2. ~/.scion/projects/*/.agents/hooks.json               the copy that running agents read (shared workspaces)

  python3 patch-antigravity-hooks.py           # check only (default): report what is patched
  python3 patch-antigravity-hooks.py --apply   # patch both, with .bak-<date> backups; idempotent

Run through run-on-hub.sh: scripts/run-on-hub.sh scripts/patch-antigravity-hooks.py --apply
"""
import argparse
import datetime
import glob
import json
import os
import py_compile
import shutil
import sys
import tempfile

SCION_HOME = os.path.expanduser(os.environ.get("SCION_HOME", "~/.scion"))
PROVISIONER = os.path.join(SCION_HOME, "harness-configs", "antigravity", "provision.py")
HOOK_FILES = os.path.join(SCION_HOME, "projects", "*", ".agents", "hooks.json")
OLD = '"| sciontool hook --dialect=antigravity"'
NEW = '"| SCION_TELEMETRY_ENABLED=false sciontool hook --dialect=antigravity"'
FIXED = "SCION_TELEMETRY_ENABLED=false sciontool hook"


def backup(path):
    bak = f"{path}.bak-{datetime.date.today():%Y%m%d}"
    if not os.path.exists(bak):
        shutil.copy2(path, bak)
    return bak


def atomic_write(path, text, check=None):
    fd, tmp = tempfile.mkstemp(dir=os.path.dirname(path), prefix="." + os.path.basename(path) + ".")
    with os.fdopen(fd, "w") as f:
        f.write(text)
    if check:
        check(tmp)
    os.chmod(tmp, os.stat(path).st_mode & 0o777)
    os.replace(tmp, path)


def provisioner(apply):
    if not os.path.exists(PROVISIONER):
        return f"provisioner: not found ({PROVISIONER})"
    src = open(PROVISIONER).read()
    if NEW in src:
        return "provisioner: patched"
    if src.count(OLD) != 1:
        return f"provisioner: UNEXPECTED, found {src.count(OLD)} copies of {OLD}; patch it by hand"
    if not apply:
        return "provisioner: NOT patched"
    bak = backup(PROVISIONER)
    atomic_write(PROVISIONER, src.replace(OLD, NEW), check=lambda p: py_compile.compile(p, doraise=True))
    return f"provisioner: patched now (backup {bak}); applies to the next Antigravity agent start"


def hook_commands(data):
    for entries in data.get("scion-hooks", {}).values():
        for entry in entries:
            for hook in entry.get("hooks", [entry]):
                if "| sciontool hook" in hook.get("command", ""):
                    yield hook


def hooks_file(path, apply):
    data = json.load(open(path))
    pending = [h for h in hook_commands(data) if FIXED not in h["command"]]
    if not pending:
        return f"{path}: patched"
    if not apply:
        return f"{path}: NOT patched ({len(pending)} commands)"
    bak = backup(path)
    for hook in pending:
        hook["command"] = hook["command"].replace("| sciontool hook", "| " + FIXED)
    atomic_write(path, json.dumps(data, indent=2) + "\n")
    return f"{path}: patched {len(pending)} commands now (backup {bak}); running agents pick it up on the next tool call"


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--apply", action="store_true", help="patch (default is check only)")
    args = parser.parse_args()
    lines = [provisioner(args.apply)]
    files = sorted(glob.glob(HOOK_FILES))
    lines += [hooks_file(p, args.apply) for p in files] or [f"no hooks.json under {HOOK_FILES}"]
    print("\n".join(lines))
    return 1 if any("UNEXPECTED" in line or (not args.apply and "NOT patched" in line) for line in lines) else 0


if __name__ == "__main__":
    sys.exit(main())
