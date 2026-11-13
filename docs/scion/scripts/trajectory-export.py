#!/usr/bin/env python3
"""Export agent transcripts and Hub messages to BigQuery (runs on the Hub VM).

Design: docs/scion/specs/2026-11-12-trajectory-export-to-bigquery-design.md

Dry-run by default: reads the sources and prints counts; loads nothing and saves no cursor.
With --apply it appends every new record, unchanged, to BigQuery and saves the cursor after each batch.

v1 reads the JSONL transcripts (Claude Code, Antigravity) and the Hub message table. The SQLite stores of
OpenCode and Hermes are not read yet. Stdlib only, single file.
"""
from __future__ import annotations

import argparse
import fcntl
import hashlib
import json
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Callable, List, Tuple

VERSION = "1"
READ_BYTES = 32 * 1024 * 1024
MAX_LINE_BYTES = 512 * 1024 * 1024
HEAD_BYTES = 256
HUB_BATCH = 5000
CLAUDE_DIR = "/home/scion/.claude/projects"
AGY_DIR = "/home/scion/.gemini/antigravity-cli/brain"
AGY_RE = re.compile(r"/brain/([^/]+)/\.system_generated/logs/(transcript_full|transcript)\.jsonl$")
TS_RE = re.compile(r"^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}")
EVENTS_SCHEMA = ("project_slug:STRING,agent_name:STRING,harness:STRING,source_path:STRING,source_kind:STRING,"
                 "session_id:STRING,seq:INTEGER,event_ts:TIMESTAMP,event_type:STRING,role:STRING,raw:JSON,"
                 "ingested_at:TIMESTAMP,exporter_version:STRING")
MESSAGES_SCHEMA = "message_rowid:INTEGER,project_slug:STRING,raw:JSON,ingested_at:TIMESTAMP,exporter_version:STRING"

# run(argv, timeout_s) -> (returncode, stdout bytes)
Runner = Callable[[List[str], int], Tuple[int, bytes]]


class SourceError(Exception):
    """A source could not be read, or a load failed."""


# ---------------------------------------------------------------------------
# Pure core
# ---------------------------------------------------------------------------
def classify(path: str) -> dict | None:
    """Harness, session id and kind of a transcript path; None for any other file."""
    if path.startswith(CLAUDE_DIR + "/") and path.endswith(".jsonl"):
        return {"harness": "claude", "session_id": os.path.basename(path)[:-6], "kind": "claude_jsonl"}
    m = AGY_RE.search(path)
    if m and path.startswith(AGY_DIR + "/"):
        return {"harness": "antigravity", "session_id": m.group(1), "kind": "antigravity_" + m.group(2)}
    return None


def select_sources(listing: list[tuple[int, str]]) -> list[dict]:
    """Transcript files to export. An Antigravity conversation uses its full transcript when it has one."""
    chosen: dict[tuple, dict] = {}
    for size, path in listing:
        info = classify(path)
        if info is None:
            continue
        key = ("agy", info["session_id"]) if info["harness"] == "antigravity" else ("file", path)
        cur = chosen.get(key)
        if cur is None or (info["kind"].endswith("_full") and not cur["kind"].endswith("_full")):
            chosen[key] = {**info, "path": path, "size": size}
    return sorted(chosen.values(), key=lambda s: s["path"])


def split_complete(data: bytes) -> tuple[list[bytes], int]:
    """Complete lines in `data` and the number of bytes they use; a trailing partial line is left out."""
    end = data.rfind(b"\n")
    if end < 0:
        return [], 0
    return data[:end].split(b"\n"), end + 1


def head_hash(head: bytes) -> str:
    return hashlib.sha1(head).hexdigest()


def plan_read(entry: dict | None, size: int, head: bytes) -> tuple[int, int]:
    """Byte offset and line number to resume at; restart from zero if the file shrank or its start changed.

    The start is compared over the bytes that were hashed last time, so a short file that only grew still matches.
    """
    if not entry or size < int(entry.get("offset", 0)):
        return 0, 0
    if head_hash(head[:int(entry.get("head_len", 0))]) != entry.get("head"):
        return 0, 0
    return int(entry["offset"]), int(entry["lines"])


def event_fields(harness: str, obj: object) -> tuple[str | None, str | None, str | None]:
    """Timestamp, type and role lifted from a record, when it has them."""
    if not isinstance(obj, dict):
        return None, None, None
    if harness == "claude":
        msg = obj.get("message") if isinstance(obj.get("message"), dict) else {}
        ts, etype, role = obj.get("timestamp"), obj.get("type"), msg.get("role")
    else:
        ts, etype, role = obj.get("created_at"), obj.get("type"), obj.get("source")
    ts = ts if isinstance(ts, str) and TS_RE.match(ts) else None
    return ts, (etype if isinstance(etype, str) else None), (role if isinstance(role, str) else None)


def make_rows(meta: dict, lines: list[bytes], first_line: int, ingested_at: str) -> tuple[list[dict], int]:
    """One row per valid JSON line (seq = 1-based line number); returns (rows, invalid line count)."""
    rows, invalid = [], 0
    for i, line in enumerate(lines):
        text = line.strip()
        if not text:
            continue
        try:
            obj = json.loads(text)
        except ValueError:
            invalid += 1
            continue
        ts, etype, role = event_fields(meta["harness"], obj)
        rows.append({
            "project_slug": meta["project_slug"], "agent_name": meta["agent_name"], "harness": meta["harness"],
            "source_path": meta["path"], "source_kind": meta["kind"], "session_id": meta["session_id"],
            "seq": first_line + i + 1, "event_ts": ts, "event_type": etype, "role": role,
            "raw": json.dumps(obj, ensure_ascii=False), "ingested_at": ingested_at, "exporter_version": VERSION,
        })
    return rows, invalid


def job_id(source_key: str, start: int, end: int) -> str:
    """Same source and range always give the same load job id, so a repeated load is refused by BigQuery."""
    return "traj_%s_%d_%d" % (hashlib.sha1(source_key.encode()).hexdigest()[:16], start, end)


# ---------------------------------------------------------------------------
# Adapters (read-only on the sources)
# ---------------------------------------------------------------------------
def list_containers(run: Runner, projects: list[str]) -> list[str]:
    rc, out = run(["docker", "ps", "--format", "{{.Names}}"], 30)
    if rc != 0:
        raise SourceError("docker ps failed")
    names = out.decode("utf-8", "replace").split()
    return sorted(n for n in names if "--" in n and n.split("--", 1)[0] in projects)


def list_files(run: Runner, container: str) -> list[tuple[int, str]]:
    listing = []
    for root in (CLAUDE_DIR, AGY_DIR):
        rc, out = run(["docker", "exec", "-u", "scion", container, "find", root, "-type", "f", "-name", "*.jsonl",
                       "-printf", "%s\\t%p\\n"], 60)
        if rc != 0:
            continue  # this agent does not use that harness
        for line in out.decode("utf-8", "replace").splitlines():
            size, _, path = line.partition("\t")
            if size.isdigit() and path:
                listing.append((int(size), path))
    return listing


def read_head(run: Runner, container: str, path: str) -> bytes:
    rc, out = run(["docker", "exec", "-u", "scion", container, "head", "-c", str(HEAD_BYTES), path], 30)
    if rc != 0:
        raise SourceError(f"cannot read {container}:{path}")
    return out


def read_range(run: Runner, container: str, path: str, offset: int, count: int) -> bytes:
    rc, out = run(["docker", "exec", "-u", "scion", container, "dd", f"if={path}", "iflag=skip_bytes,count_bytes",
                   f"skip={offset}", f"count={count}", "bs=1M", "status=none"], 300)
    if rc != 0:
        raise SourceError(f"cannot read {container}:{path}")
    return out


def read_hub_messages(db_path: str, projects: list[str], after_rowid: int, limit: int = HUB_BATCH) -> list[dict]:
    """Rows of the Hub message table newer than `after_rowid`, as dicts, oldest first."""
    if not os.path.exists(db_path):
        raise SourceError(f"hub.db not found: {db_path}")
    marks = ",".join("?" for _ in projects)
    query = ("SELECT m.rowid AS _rowid, p.slug AS _project, m.* FROM messages m "
             "JOIN projects p ON p.id = m.project_id "
             f"WHERE m.rowid > ? AND p.slug IN ({marks}) ORDER BY m.rowid LIMIT ?")
    try:
        conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
        conn.row_factory = sqlite3.Row
        try:
            rows = conn.execute(query, (after_rowid, *projects, limit)).fetchall()
        finally:
            conn.close()
    except sqlite3.Error as err:
        raise SourceError(f"hub.db read failed: {err}") from err
    return [dict(r) for r in rows]


def load_rows(run: Runner, cfg: dict, table: str, schema: str, rows: list[dict], jid: str,
              partition_field: str | None = None) -> None:
    """Append `rows` with one load job. A job id that already exists means the batch was loaded before."""
    fd, tmp = tempfile.mkstemp(prefix="trajectory-export.", suffix=".ndjson")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            for row in rows:
                f.write(json.dumps(row, ensure_ascii=False, default=str) + "\n")
        argv = ["bq", f"--project_id={cfg['bq_project']}", "load", "--source_format=NEWLINE_DELIMITED_JSON",
                f"--job_id={jid}"]
        if partition_field:
            argv.append(f"--time_partitioning_field={partition_field}")
        argv += [f"{cfg['dataset']}.{table}", tmp, schema]
        rc, out = run(argv, 600)
        if rc != 0 and b"Already Exists" not in out:
            raise SourceError(f"bq load failed for {table} ({jid}): {out[-300:].decode('utf-8', 'replace')}")
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)


def _default_runner(argv: list[str], timeout: int) -> tuple[int, bytes]:
    try:
        res = subprocess.run(argv, capture_output=True, timeout=timeout)
        return res.returncode, res.stdout
    except Exception as err:  # noqa: BLE001
        print(f"WARNING: command {argv[:3]} failed: {err}", file=sys.stderr)
        return 1, b""


# ---------------------------------------------------------------------------
# Run loop
# ---------------------------------------------------------------------------
@dataclass
class Deps:
    run: Runner
    now: Callable[[], datetime]


def _iso(ts: datetime) -> str:
    return ts.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


def load_cursor(path: str) -> dict:
    try:
        with open(path, encoding="utf-8") as f:
            data = json.load(f)
        return data if isinstance(data, dict) else {}
    except (OSError, ValueError):
        return {}


def save_cursor(path: str, cursor: dict) -> None:
    parent = os.path.dirname(path) or "."
    os.makedirs(parent, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=parent, prefix=".trajectory-export-cursor.")
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        json.dump(cursor, f, indent=1, sort_keys=True)
    os.chmod(tmp, 0o600)
    os.replace(tmp, path)


def export_file(deps: Deps, cfg: dict, apply: bool, container: str, src: dict, cursor: dict, stats: dict) -> None:
    """Read one transcript from its cursor to the last complete line; count, and load when `apply`."""
    key = f"{container}:{src['path']}"
    project, agent = container.split("--", 1)
    meta = {**src, "project_slug": project, "agent_name": agent}
    head = read_head(deps.run, container, src["path"])
    offset, line_no = plan_read(cursor["files"].get(key), src["size"], head)
    stats["files"] += 1
    count = READ_BYTES
    while offset < src["size"]:
        data = read_range(deps.run, container, src["path"], offset, count)
        lines, used = split_complete(data)
        if used == 0:
            if len(data) < count:
                break  # only a partial last line is left; wait for the harness to finish it
            count *= 4
            if count > MAX_LINE_BYTES:
                raise SourceError(f"line longer than {MAX_LINE_BYTES} bytes in {key}")
            continue
        rows, invalid = make_rows(meta, lines, line_no, _iso(deps.now()))
        if apply and rows:
            load_rows(deps.run, cfg, cfg["events_table"], EVENTS_SCHEMA, rows, job_id(key, offset, offset + used),
                      partition_field="event_ts")
            stats["loaded_rows"] += len(rows)
        stats["bytes"] += used
        stats["records"] += len(rows)
        stats["invalid_lines"] += invalid
        for row in rows:
            ts = row["event_ts"]
            if ts:
                stats["first_ts"] = min(stats["first_ts"] or ts, ts)
                stats["last_ts"] = max(stats["last_ts"] or ts, ts)
        offset, line_no, count = offset + used, line_no + len(lines), READ_BYTES
        cursor["files"][key] = {"offset": offset, "lines": line_no, "head": head_hash(head), "head_len": len(head)}
        if apply:
            save_cursor(cfg["cursor_path"], cursor)


def export_hub_messages(deps: Deps, cfg: dict, apply: bool, cursor: dict) -> int:
    total = 0
    while True:
        after = int(cursor.get("hub_rowid", 0))
        msgs = read_hub_messages(cfg["db_path"], cfg["projects"], after)
        if not msgs:
            return total
        last = int(msgs[-1]["_rowid"])
        if apply:
            now = _iso(deps.now())
            rows = [{"message_rowid": m["_rowid"], "project_slug": m["_project"],
                     "raw": json.dumps(m, ensure_ascii=False, default=str), "ingested_at": now,
                     "exporter_version": VERSION} for m in msgs]
            load_rows(deps.run, cfg, cfg["messages_table"], MESSAGES_SCHEMA, rows, job_id("hub_messages", after, last))
        total += len(msgs)
        cursor["hub_rowid"] = last
        if apply:
            save_cursor(cfg["cursor_path"], cursor)


def run_once(deps: Deps, cfg: dict, apply: bool) -> dict:
    """One pass over every source. Dry-run counts only; nothing is loaded and no cursor is saved."""
    started = deps.now()
    cursor = load_cursor(cfg["cursor_path"])
    cursor.setdefault("files", {})
    agents: dict[str, dict] = {}
    failures: list[str] = []
    try:
        containers = list_containers(deps.run, cfg["projects"])
    except SourceError as err:
        containers, failures = [], [str(err)]
    for container in containers:
        stats = {"harness": None, "files": 0, "bytes": 0, "records": 0, "invalid_lines": 0, "loaded_rows": 0,
                 "first_ts": None, "last_ts": None}
        agents[container] = stats
        try:
            for src in select_sources(list_files(deps.run, container)):
                stats["harness"] = src["harness"]
                export_file(deps, cfg, apply, container, src, cursor, stats)
        except SourceError as err:
            failures.append(str(err))
    hub_messages = 0
    try:
        hub_messages = export_hub_messages(deps, cfg, apply, cursor)
    except SourceError as err:
        failures.append(str(err))
    return {
        "ts": _iso(started), "result": "ok" if not failures else "partial", "apply": apply,
        "agents": agents, "hub_messages": hub_messages,
        "records": sum(a["records"] for a in agents.values()),
        "loaded_rows": sum(a["loaded_rows"] for a in agents.values()),
        "failures": failures, "duration_s": round((deps.now() - started).total_seconds(), 1),
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Export agent transcripts and Hub messages to BigQuery")
    parser.add_argument("--apply", action="store_true", help="Load into BigQuery and save the cursor")
    parser.add_argument("--dataset", default=os.environ.get("BQ_DATASET", ""), help="BigQuery dataset")
    args = parser.parse_args(argv)

    home = os.path.expanduser(os.environ.get("SCION_HOME", "~/.scion"))
    listed = os.environ.get("FACTORY_PROJECTS") or ",".join(
        [os.environ.get("BUILD_PROJECT", ""), os.environ.get("EVAL_PROJECT", "")])
    projects = [p.strip() for p in listed.split(",") if p.strip()]
    cfg = {
        "db_path": os.path.join(home, "hub.db"),
        "cursor_path": os.path.join(home, "trajectory-export-cursor.json"),
        "projects": projects, "bq_project": os.environ.get("SANDBOX_PROJECT", ""), "dataset": args.dataset,
        "events_table": "agent_transcript_events", "messages_table": "hub_messages",
    }
    if not projects:
        print("set FACTORY_PROJECTS, or BUILD_PROJECT and EVAL_PROJECT", file=sys.stderr)
        return 2
    if args.apply and not (cfg["bq_project"] and cfg["dataset"]):
        print("--apply needs SANDBOX_PROJECT and --dataset (or BQ_DATASET)", file=sys.stderr)
        return 2
    if os.path.exists(os.path.join(home, "trajectory-export.disabled")):
        print(json.dumps({"result": "disabled"}))
        return 0
    os.makedirs(home, exist_ok=True)
    with open(os.path.join(home, "trajectory-export.lock"), "w", encoding="utf-8") as lock:
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError:
            print(json.dumps({"result": "locked"}))
            return 0
        deps = Deps(run=_default_runner, now=lambda: datetime.now(timezone.utc))
        print(json.dumps(run_once(deps, cfg, args.apply), sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
