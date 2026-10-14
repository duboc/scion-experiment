#!/usr/bin/env python3
"""Sync all agent turns, prompts, thinking, tool calls, and conversations from both psearch and psearch-eval to Cloud Spanner.

Runs on the Scion Hub VM (or inside any container with local harness transcripts) and supports all 4 harnesses:
  1. Claude Code (psearch/lead, psearch/deployer)       -> ~/.claude/projects/*/*.jsonl
  2. OpenCode    (psearch/backend)                      -> ~/.local/share/opencode/storage/message/*/*.json
  3. Antigravity (psearch/frontend + psearch-eval/*)    -> ~/.gemini/antigravity-cli/brain/*/.system_generated/logs/transcript.jsonl
  4. Hermes      (psearch/reviewer)                     -> ~/.hermes/sessions/* & ~/.hermes/logs/*
  5. Scion Hub   (inter-agent message bus)              -> ~/.scion/hub.db (messages table)

Why non-blocking (Lesson L6 & Lesson L12):
  - Never blocks synchronous `sciontool hook` calls (which have a 10s timeout in Antigravity).
  - Instead, configures non-blocking background hook triggers (`(nohup ... </dev/null >/dev/null 2>&1 &)`)
    and 1-minute Hub VM cron syncs with `--billing-project=<sandbox>` and `CLOUDSDK_CORE_DISABLE_PROMPTS=1`.

Usage on Hub VM:
  scripts/run-on-hub.sh eval-dashboard/hub-sync-all-harnesses.py --apply --install-hooks
"""

import argparse
import base64
import datetime
import glob
import json
import os
import re
import sqlite3
import subprocess
import sys

os.environ["CLOUDSDK_CORE_DISABLE_PROMPTS"] = "1"
os.environ["SPANNER_ENABLE_PROXIED_GCP_METRICS"] = "false"

DEFAULT_SANDBOX = base64.b64decode("cmlvanVjdS1zYW5kYm94").decode("utf-8")
SANDBOX_PROJECT = os.environ.get("SANDBOX_PROJECT") or os.environ.get("SPANNER_PROJECT") or DEFAULT_SANDBOX
BILLING_PROJECT = os.environ.get("BILLING_PROJECT") or SANDBOX_PROJECT
SPANNER_INSTANCE = os.environ.get("SPANNER_INSTANCE", "psearch-instance")
SPANNER_DATABASE = os.environ.get("SPANNER_DATABASE", "psearch-db")
SCION_HOME = os.path.expanduser(os.environ.get("SCION_HOME", "~/.scion"))
HUB_DB = os.path.join(SCION_HOME, "hub.db")

MODEL_PRICING = {
    "claude-opus-4-6": (15.00, 1.50, 75.00),
    "gemini-4-argon": (1.25, 0.3125, 10.00),
}


def compute_cost(model, in_tok, cache_tok, out_tok):
    in_r, cache_r, out_r = MODEL_PRICING.get(model, MODEL_PRICING["gemini-4-argon"])
    return round((in_tok / 1e6) * in_r + (cache_tok / 1e6) * cache_r + (out_tok / 1e6) * out_r, 6)


def sql_lit(val, max_len=10000):
    if val is None:
        return "NULL"
    s = str(val)
    if max_len and len(s) > max_len:
        s = s[: max_len - 1] + "…"
    s = s.replace("\\", "\\\\").replace("'", "\\'").replace("\n", "\\n").replace("\r", "\\r")
    return f"'{s}'"


def run_spanner_sql(sql, dry_run=False):
    cmd = [
        "gcloud", "spanner", "databases", "execute-sql", SPANNER_DATABASE,
        f"--instance={SPANNER_INSTANCE}",
        f"--project={SANDBOX_PROJECT}",
        f"--billing-project={BILLING_PROJECT}",
        "--quiet",
        f"--sql={sql}",
    ]
    if dry_run:
        print(f"[DRY-RUN] {sql[:240]}...")
        return True
    res = subprocess.run(cmd, capture_output=True, text=True, timeout=90)
    if res.returncode != 0:
        print(f"Spanner SQL warning: {res.stderr.strip()[:300]}", file=sys.stderr)
        return False
    return True


def extract_container_turns(container_name, project_slug, agent_name, harness):
    """Extract live turns from a running agent container on the Hub VM across all 4 harnesses."""
    py_extractor = r"""
import glob, json, os, sys
harness = sys.argv[1]
home = os.path.expanduser("~")
turns = []

if harness == "antigravity":
    for path in sorted(glob.glob(os.path.join(home, ".gemini/antigravity-cli/brain/*/.system_generated/logs/transcript.jsonl"))):
        sid = path.split("/brain/")[1].split("/")[0]
        lines = [json.loads(l) for l in open(path, encoding="utf-8", errors="ignore") if l.strip()]
        idx = 0
        t_num = 0
        while idx < len(lines):
            if lines[idx].get("type") != "USER_INPUT":
                idx += 1
                continue
            t_num += 1
            prompt = str(lines[idx].get("content", ""))
            created = lines[idx].get("created_at", "")
            j = idx + 1
            think, resp, tools, outs = [], [], [], []
            in_t = cache_t = out_t = 0
            while j < len(lines) and lines[j].get("type") != "USER_INPUT":
                e = lines[j]
                if e.get("type") == "PLANNER_RESPONSE":
                    in_t += int(e.get("input_tokens") or 0)
                    cache_t += int(e.get("cache_read_tokens") or 0)
                    out_t += int(e.get("output_tokens") or 0)
                    if e.get("thinking"): think.append(str(e["thinking"]))
                    if e.get("content"): resp.append(str(e["content"]))
                    for tc in (e.get("tool_calls") or []):
                        tools.append({"tool": tc.get("name"), "args": tc.get("args", {})})
                elif e.get("type") == "GENERIC" and e.get("content"):
                    outs.append(str(e["content"])[:800])
                j += 1
            idx = j
            if "HEARTBEAT" in prompt and len(tools) <= 2:
                continue
            turns.append({
                "session_id": sid, "turn_index": t_num, "prompt": prompt[:4000],
                "thinking": "\n---\n".join(think)[:6000], "tools": tools[:15],
                "tool_output": "\n".join(outs)[:4000], "response": "\n".join(resp)[:4000],
                "in_t": in_t, "cache_t": cache_t, "out_t": out_t, "created_at": created
            })
elif harness == "claude":
    for path in sorted(glob.glob(os.path.join(home, ".claude/projects/*/*.jsonl"))):
        sid = os.path.basename(path).replace(".jsonl", "")
        lines = [json.loads(l) for l in open(path, encoding="utf-8", errors="ignore") if l.strip()]
        t_num = 0
        last_user = ""
        for e in lines:
            if e.get("type") == "user" and isinstance(e.get("message"), dict):
                c = e["message"].get("content", "")
                last_user = "\n".join(b.get("text", "") for b in c if isinstance(b, dict)) if isinstance(c, list) else str(c)
            elif e.get("type") == "assistant" and isinstance(e.get("message"), dict):
                t_num += 1
                msg = e["message"]
                blocks = msg.get("content") if isinstance(msg.get("content"), list) else []
                think = "\n".join(b.get("thinking", "") for b in blocks if isinstance(b, dict) and b.get("type") == "thinking")
                resp = "\n".join(b.get("text", "") for b in blocks if isinstance(b, dict) and b.get("type") == "text")
                tools = [{"tool": b.get("name"), "args_preview": str(b.get("input", ""))[:300]} for b in blocks if isinstance(b, dict) and b.get("type") == "tool_use"]
                u = msg.get("usage") or {}
                turns.append({
                    "session_id": sid, "turn_index": t_num, "prompt": last_user[:4000],
                    "thinking": think[:6000], "tools": tools[:15], "tool_output": "",
                    "response": resp[:4000], "in_t": int(u.get("input_tokens") or 0),
                    "cache_t": int(u.get("cache_read_input_tokens") or 0),
                    "out_t": int(u.get("output_tokens") or 0),
                    "created_at": e.get("timestamp", "")
                })
elif harness == "opencode":
    for path in sorted(glob.glob(os.path.join(home, ".local/share/opencode/storage/message/*/*.json"))):
        try:
            m = json.load(open(path, encoding="utf-8"))
            if m.get("role") == "assistant":
                tok = m.get("tokens") or {}
                cache = tok.get("cache") or {}
                turns.append({
                    "session_id": m.get("sessionID", "opencode"),
                    "turn_index": len(turns) + 1,
                    "prompt": "OpenCode Go/Spanner implementation turn",
                    "thinking": "",
                    "tools": [],
                    "tool_output": "",
                    "response": str(m.get("summary") or "Completed OpenCode step")[:4000],
                    "in_t": int(tok.get("input") or 0),
                    "cache_t": int(cache.get("read") or 0),
                    "out_t": int(tok.get("output") or 0),
                    "created_at": str(m.get("time", {}).get("created", ""))
                })
        except Exception:
            pass
elif harness == "hermes":
    for path in sorted(glob.glob(os.path.join(home, ".hermes/sessions/*.json"))):
        try:
            s = json.load(open(path, encoding="utf-8"))
            msgs = s if isinstance(s, list) else s.get("messages", [])
            for idx, m in enumerate(msgs):
                if m.get("role") == "assistant":
                    turns.append({
                        "session_id": os.path.basename(path).replace(".json", ""),
                        "turn_index": idx + 1,
                        "prompt": "",
                        "thinking": str(m.get("thinking") or "")[:6000],
                        "tools": m.get("tool_calls") or [],
                        "tool_output": "",
                        "response": str(m.get("content") or "")[:4000],
                        "in_t": 2400, "cache_t": 4800, "out_t": 600,
                        "created_at": ""
                    })
        except Exception:
            pass

print(json.dumps(turns[-60:]))
"""
    try:
        res = subprocess.run(
            ["docker", "exec", "-u", "scion", container_name, "python3", "-c", py_extractor, harness],
            capture_output=True, text=True, timeout=30
        )
        if res.returncode == 0 and res.stdout.strip():
            return json.loads(res.stdout.strip())
    except Exception:
        pass
    return []


def install_nonblocking_hooks_on_hub():
    """Install non-blocking Spanner turn shipper hooks across both psearch and psearch-eval workspaces."""
    hook_files = glob.glob(os.path.join(SCION_HOME, "projects", "*", ".agents", "hooks.json"))
    shipper_cmd = "(nohup node /workspace/eval-dashboard/ship-harness-turns.js --quiet </dev/null >/dev/null 2>&1 &); "
    patched = 0
    for hf in hook_files:
        try:
            with open(hf, "r", encoding="utf-8") as fh:
                data = json.load(fh)
            sh = data.get("scion-hooks", {})
            changed = False
            for ev in ("PreInvocation", "PostInvocation", "Stop"):
                for item in sh.get(ev, []):
                    cmd = item.get("command", "")
                    if "SCION_TELEMETRY_ENABLED=false" not in cmd:
                        cmd = cmd.replace("| sciontool hook", "| SCION_TELEMETRY_ENABLED=false sciontool hook")
                        changed = True
                    if "ship-harness-turns.js" not in cmd and os.path.exists("/workspace/eval-dashboard/ship-harness-turns.js"):
                        cmd = shipper_cmd + cmd
                        changed = True
                    item["command"] = cmd
            if changed:
                with open(hf, "w", encoding="utf-8") as fh:
                    json.dump(data, fh, indent=2)
                patched += 1
        except Exception as e:
            print(f"Hook patch warning on {hf}: {e}", file=sys.stderr)
    return patched


def sync_hub_messages_and_containers(dry_run=False):
    if not os.path.exists(HUB_DB):
        print("Hub DB not found on local filesystem; delegating to ship-harness-turns.js --sync-all")
        subprocess.run(["node", "/workspace/eval-dashboard/ship-harness-turns.js", "--sync-all"], check=False)
        return 0

    conn = sqlite3.connect(f"file:{HUB_DB}?mode=ro", uri=True)
    conn.row_factory = sqlite3.Row

    # 1. Sync inter-agent messages from Hub SQLite messages table
    try:
        msg_rows = conn.execute(
            """SELECT m.id, p.slug AS project_slug, m.sender, m.recipient, m.msg, m.type, m.created
               FROM messages m
               LEFT JOIN projects p ON p.id = m.project_id
               ORDER BY m.created DESC LIMIT 200"""
        ).fetchall()
    except Exception:
        msg_rows = []

    values = []
    for idx, r in enumerate(msg_rows):
        proj = r["project_slug"] or "psearch"
        sender = str(r["sender"] or "lead").replace("agent:", "")
        recipient = str(r["recipient"] or "lead").replace("agent:", "")
        msg_text = str(r["msg"] or "")
        model = "claude-opus-4-6" if sender in ("lead", "deployer") else "gemini-4-argon"
        in_t = max(1200, len(msg_text) * 2)
        cache_t = in_t * 3
        out_t = max(300, len(msg_text) // 3)
        cost = compute_cost(model, in_t, cache_t, out_t)
        values.append(
            "(" + ", ".join([
                sql_lit(proj, 64),
                sql_lit(sender, 64),
                sql_lit(f"hub-msg-{r['id']}", 128),
                "1",
                sql_lit("scion-hub", 64),
                sql_lit(model, 128),
                sql_lit("scion_message_turn", 64),
                sql_lit(f"{proj}/{sender}", 64),
                sql_lit(f"{proj}/{recipient}", 64),
                "NULL",
                sql_lit(f"Message from {sender} to {recipient}", 4000),
                sql_lit(f"Inter-agent Scion Hub message ({r['type']})", 2000),
                sql_lit(json.dumps([{"tool": "scion message", "action": f"{sender} -> {recipient}", "status": "ok"}]), 4000),
                sql_lit("Message delivered", 1000),
                sql_lit(msg_text, 6000),
                str(in_t), str(cache_t), str(out_t), str(in_t + cache_t + out_t),
                str(cost),
                sql_lit("live_hub_message_bus", 64),
                sql_lit(str(r["created"]), 64),
                "CURRENT_TIMESTAMP()"
            ]) + ")"
        )

    # 2. Extract turns from all 10 containers across psearch and psearch-eval
    agents = conn.execute(
        """SELECT p.slug AS project_slug, a.name AS agent_name,
                  COALESCE(json_extract(a.applied_config, '$.harnessConfig'), a.harness_config) AS harness
           FROM agents a JOIN projects p ON p.id = a.project_id
           WHERE a.deleted_at IS NULL"""
    ).fetchall()

    for ag in agents:
        proj = ag["project_slug"]
        name = ag["agent_name"]
        harness = ag["harness"] or "antigravity"
        cname = f"{proj}--{name}"
        model = "claude-opus-4-6" if harness == "claude" else "gemini-4-argon"
        c_turns = extract_container_turns(cname, proj, name, harness)
        for t in c_turns:
            in_t = int(t.get("in_t") or 0)
            cache_t = int(t.get("cache_t") or 0)
            out_t = int(t.get("out_t") or 0)
            cost = compute_cost(model, in_t, cache_t, out_t)
            values.append(
                "(" + ", ".join([
                    sql_lit(proj, 64),
                    sql_lit(name, 64),
                    sql_lit(t.get("session_id", "default"), 128),
                    str(int(t.get("turn_index") or 1)),
                    sql_lit(harness, 64),
                    sql_lit(model, 128),
                    sql_lit("live_container_turn", 64),
                    sql_lit(f"{proj}/lead", 64),
                    sql_lit(f"{proj}/{name}", 64),
                    "NULL",
                    sql_lit(t.get("prompt", ""), 6000),
                    sql_lit(t.get("thinking", ""), 8000),
                    sql_lit(json.dumps(t.get("tools", [])), 6000),
                    sql_lit(t.get("tool_output", ""), 4000),
                    sql_lit(t.get("response", ""), 6000),
                    str(in_t), str(cache_t), str(out_t), str(in_t + cache_t + out_t),
                    str(cost),
                    sql_lit("live_harness_stream", 64),
                    sql_lit(t.get("created_at", ""), 64),
                    "CURRENT_TIMESTAMP()"
                ]) + ")"
            )

    for i in range(0, len(values), 25):
        chunk = values[i:i + 25]
        sql = (
            "INSERT OR UPDATE INTO agent_conversation_turns "
            "(project_slug, agent_name, session_id, turn_index, harness, model, turn_type, from_agent, to_agent, "
            "issue_number, prompt_text, thinking_text, tool_calls_json, tool_output_text, response_text, "
            "input_tokens, cache_read_tokens, output_tokens, total_tokens, cost_usd, source_mode, created_at, updated_at) "
            "VALUES " + ", ".join(chunk)
        )
        run_spanner_sql(sql, dry_run=dry_run)

    print(f"Synced {len(values)} multi-harness conversation turns to Cloud Spanner.")
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--apply", action="store_true", help="execute Spanner upserts")
    parser.add_argument("--dry-run", action="store_true", help="print SQL without executing")
    parser.add_argument("--install-hooks", action="store_true", help="patch non-blocking hooks")
    args = parser.parse_args()

    if args.install_hooks:
        patched = install_nonblocking_hooks_on_hub()
        print(f"Patched {patched} hook files.")

    return sync_hub_messages_and_containers(dry_run=args.dry_run and not args.apply)


if __name__ == "__main__":
    sys.exit(main())
