#!/usr/bin/env python3
"""Make Antigravity's Scion hooks run with telemetry off. Runs ON the Hub VM, as the Hub's OS user.

Why (lessons-learned.md L6): with Hub telemetry and cloud export enabled, `sciontool hook --dialect=antigravity`
prints its decision but doesn't exit. Antigravity (`agy`) kills the hook after its 10 s timeout and aborts the
tool call, so the agent can't run any tool. The Hub injects SCION_TELEMETRY_ENABLED=true into every agent and that
wins over template settings, so the fix has to be in the hook command itself:

    ... | SCION_TELEMETRY_ENABLED=false sciontool hook --dialect=antigravity

Places checked and patched:
  1. All provision.py locations under ~/.scion:
     - ~/.scion/harness-configs/antigravity/provision.py
     - ~/.scion/workspace-storage/hubs/*/harness-configs/global/antigravity/provision.py
     - ~/.scion/project-configs/*/.scion/agents/*/home/.scion/harness/provision.py
  2. All hooks.json files:
     - ~/.scion/projects/*/.agents/hooks.json
  3. Pre-start hooks staged across all agent homes:
     - ~/.scion/project-configs/*/.scion/agents/*/home/.scion/hooks/pre-start.d/30-project-custom
  4. Project pre-start hooks registered and active in Hub DB / API:
     - agy-telemetry-off for each active project in hub.db

  python3 patch-antigravity-hooks.py           # check only (default): report what is patched
  python3 patch-antigravity-hooks.py --apply   # patch all idempotently; exit 0

Run through run-on-hub.sh: scripts/run-on-hub.sh scripts/patch-antigravity-hooks.py --apply
"""
import argparse
import base64
import datetime
import glob
import hashlib
import hmac
import json
import os
import py_compile
import shutil
import sqlite3
import sys
import tempfile
import time
import urllib.error
import urllib.request

SCION_HOME = os.path.expanduser(os.environ.get("SCION_HOME", "~/.scion"))
HUB_DB = os.path.join(SCION_HOME, "hub.db")
HUB_ENV = os.path.join(SCION_HOME, "hub.env")
HUB_URL = os.environ.get("HUB_LOCAL_URL", "http://localhost:8080").rstrip("/")

PROVISIONER_PATTERNS = [
    os.path.join(SCION_HOME, "harness-configs", "antigravity", "provision.py"),
    os.path.join(SCION_HOME, "workspace-storage", "hubs", "*", "harness-configs", "global", "antigravity", "provision.py"),
    os.path.join(SCION_HOME, "project-configs", "*", ".scion", "agents", "*", "home", ".scion", "harness", "provision.py"),
]
HOOK_FILES_PATTERN = os.path.join(SCION_HOME, "projects", "*", ".agents", "hooks.json")
AGENT_HOMES_PATTERN = os.path.join(SCION_HOME, "project-configs", "*", ".scion", "agents", "*", "home")

OLD = '"| sciontool hook --dialect=antigravity"'
NEW = '"| SCION_TELEMETRY_ENABLED=false sciontool hook --dialect=antigravity"'
FIXED = "SCION_TELEMETRY_ENABLED=false sciontool hook"

CUSTOM_HOOK_SCRIPT = """#!/bin/sh
f=/workspace/.agents/hooks.json
if [ -f "$f" ]; then
  sed -i 's/| sciontool hook --dialect=antigravity/| SCION_TELEMETRY_ENABLED=false sciontool hook --dialect=antigravity/g' "$f"
fi
exit 0
"""


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


def check_or_patch_provisioner(path, apply):
    if not os.path.exists(path):
        return f"{path}: not found"
    with open(path, "r", encoding="utf-8") as f:
        src = f.read()
    if NEW in src:
        return f"{path}: patched"
    if OLD in src:
        if not apply:
            return f"{path}: NOT patched"
        bak = backup(path)
        atomic_write(path, src.replace(OLD, NEW), check=lambda p: py_compile.compile(p, doraise=True))
        return f"{path}: patched now (backup {bak})"
    if "antigravity" in path:
        return f"{path}: UNEXPECTED, antigravity provisioner missing {OLD}"
    return f"{path}: skipped (non-antigravity)"


def hook_commands(data):
    for entries in data.get("scion-hooks", {}).values():
        for entry in entries:
            for hook in entry.get("hooks", [entry]):
                if "| sciontool hook" in hook.get("command", ""):
                    yield hook


def check_or_patch_hooks_file(path, apply):
    if not os.path.exists(path):
        return f"{path}: not found"
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
    return f"{path}: patched {len(pending)} commands now (backup {bak})"


def check_or_stage_pre_start_hook(home_path, apply):
    hook_dir = os.path.join(home_path, ".scion", "hooks", "pre-start.d")
    hook_path = os.path.join(hook_dir, "30-project-custom")
    if os.path.exists(hook_path):
        try:
            content = open(hook_path, "r", encoding="utf-8").read()
            is_exec = (os.stat(hook_path).st_mode & 0o700) == 0o700
            if "SCION_TELEMETRY_ENABLED=false" in content and is_exec:
                return f"{hook_path}: patched"
        except Exception:
            pass
    if not apply:
        return f"{hook_path}: NOT patched (missing or invalid)"
    os.makedirs(hook_dir, exist_ok=True)
    with open(hook_path, "w", encoding="utf-8") as f:
        f.write(CUSTOM_HOOK_SCRIPT)
    os.chmod(hook_path, 0o700)
    return f"{hook_path}: patched now"


_TOKEN = None


def _token():
    email = os.environ.get("PO_EMAIL")
    conn = sqlite3.connect(f"file:{HUB_DB}?mode=ro", uri=True)
    conn.row_factory = sqlite3.Row
    if email:
        user = conn.execute("SELECT id, email, role FROM users WHERE email = ?", (email,)).fetchone()
    else:
        user = conn.execute("SELECT id, email, role FROM users WHERE role = 'admin' LIMIT 1").fetchone()
    if not user:
        user = conn.execute("SELECT id, email, role FROM users LIMIT 1").fetchone()
    if not user:
        sys.exit("no user found in hub.db")
    secret = None
    if os.path.exists(HUB_ENV):
        with open(HUB_ENV) as f:
            for line in f:
                if line.startswith("SCION_SERVER_SESSION_SECRET="):
                    secret = line.split("=", 1)[1].strip()
    if not secret:
        sys.exit(f"SCION_SERVER_SESSION_SECRET not found in {HUB_ENV}")
    key = hashlib.sha256(("scion-hub-signing-key:user_signing_key:" + secret).encode()).digest()

    def b64(data):
        return base64.urlsafe_b64encode(data).rstrip(b"=").decode()

    now = int(time.time())
    claims = {
        "iss": "scion-hub", "sub": user["id"], "aud": ["scion-hub-api"], "exp": now + 300, "nbf": now - 10,
        "iat": now, "uid": user["id"], "email": user["email"], "name": user["email"], "role": user["role"],
        "type": "access", "client": "web",
    }
    signing_input = b64(json.dumps({"alg": "HS256", "typ": "JWT"}).encode()) + "." + b64(json.dumps(claims).encode())
    signature = hmac.new(key, signing_input.encode(), hashlib.sha256).digest()
    return signing_input + "." + b64(signature)


def api(method, path, body=None):
    global _TOKEN
    if _TOKEN is None:
        _TOKEN = _token()
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(HUB_URL + path, data=data, method=method,
                                 headers={"Authorization": "Bearer " + _TOKEN, "Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read().decode()
            return resp.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as err:
        sys.exit(f"{method} {path} -> HTTP {err.code}: {err.read().decode()[:500]}")


def ensure_project_pre_start_hooks(apply):
    lines = []
    if not os.path.exists(HUB_DB):
        return ["hub.db: not found"]

    conn = sqlite3.connect(f"file:{HUB_DB}?mode=ro", uri=True)
    conn.row_factory = sqlite3.Row
    cursor = conn.cursor()
    cursor.execute("SELECT name FROM sqlite_master WHERE type='table' AND name='project_pre_start_hooks'")
    if not cursor.fetchone():
        return ["hub.db table project_pre_start_hooks: NOT FOUND"]

    projects = conn.execute("SELECT id, slug FROM projects WHERE slug <> 'global' ORDER BY slug").fetchall()
    for p in projects:
        pid = p["id"]
        pslug = p["slug"]
        row = conn.execute(
            "SELECT id, slug, status, script FROM project_pre_start_hooks WHERE project_id = ? AND slug = 'agy-telemetry-off'",
            (pid,),
        ).fetchone()

        is_ok = row and row["status"] == "active" and "SCION_TELEMETRY_ENABLED=false" in (row["script"] or "")
        if is_ok:
            lines.append(f"project {pslug}: hook agy-telemetry-off active")
            continue

        if not apply:
            lines.append(f"project {pslug}: hook agy-telemetry-off NOT active")
            continue

        # Apply via Hub API
        status, res = api("GET", f"/api/v1/projects/{pid}/pre-start-hooks")
        existing_hooks = res.get("hooks", []) if isinstance(res, dict) else []
        matching = next((h for h in existing_hooks if h.get("slug") == "agy-telemetry-off"), None)

        hook_id = None
        if matching:
            hook_id = matching["id"]
            api("PUT", f"/api/v1/projects/{pid}/pre-start-hooks/{hook_id}", {
                "script": CUSTOM_HOOK_SCRIPT,
            })
        else:
            status, created = api("POST", f"/api/v1/projects/{pid}/pre-start-hooks", {
                "name": "agy-telemetry-off",
                "slug": "agy-telemetry-off",
                "description": "Disable telemetry in antigravity hooks",
                "script": CUSTOM_HOOK_SCRIPT,
            })
            hook_id = created.get("id")

        if hook_id:
            api("POST", f"/api/v1/projects/{pid}/pre-start-hooks/{hook_id}/activate")
            lines.append(f"project {pslug}: hook agy-telemetry-off created/activated now")
        else:
            lines.append(f"project {pslug}: hook agy-telemetry-off UNEXPECTED failed to create")

    return lines


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--apply", action="store_true", help="patch (default is check only)")
    args = parser.parse_args()

    lines = []

    # 1. Provisioners
    prov_files = []
    for pat in PROVISIONER_PATTERNS:
        prov_files.extend(glob.glob(pat))
    prov_files = sorted(set(prov_files))
    if not prov_files:
        lines.append("provisioners: none found")
    else:
        for p in prov_files:
            lines.append(check_or_patch_provisioner(p, args.apply))

    # 2. Hooks files
    hooks_files = sorted(glob.glob(HOOK_FILES_PATTERN))
    if not hooks_files:
        lines.append(f"no hooks.json under {HOOK_FILES_PATTERN}")
    else:
        for h in hooks_files:
            lines.append(check_or_patch_hooks_file(h, args.apply))

    # 3. Agent home pre-start hooks
    agent_homes = sorted(glob.glob(AGENT_HOMES_PATTERN))
    if not agent_homes:
        lines.append(f"no agent homes under {AGENT_HOMES_PATTERN}")
    else:
        for ah in agent_homes:
            lines.append(check_or_stage_pre_start_hook(ah, args.apply))

    # 4. Project pre-start hooks via Hub API / DB
    lines.extend(ensure_project_pre_start_hooks(args.apply))

    print("\n".join(lines))

    failed = any("UNEXPECTED" in line or (not args.apply and ("NOT patched" in line or "NOT active" in line)) for line in lines)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
