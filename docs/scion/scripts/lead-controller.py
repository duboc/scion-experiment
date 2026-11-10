#!/usr/bin/env python3
"""Lead cost controller for the psearch Scion factory (runs on the Hub VM, every 5 min by cron).

Design: docs/scion/specs/2026-11-06-lead-cost-controller-design.md
Plan:   docs/plans/2026-11-09-lead-cost-controller.md

Two duties, both without any LLM call:
  1. Context ceiling: when a Claude Code agent (lead, deployer) is idle and its context is above 200k tokens,
     type `/compact <role instructions>` into its tmux pane.
  2. Activity gate: wake the lead with one Hub message only when a deterministic rule finds work
     (new issue, unattended issue activity, unanswered request, safety wake with backoff).

Dry-run by default. Stdlib only, single file: docs/scion/scripts/run-on-hub.sh streams one file to the VM.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import hmac
import json
import os
import re
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Callable, Dict, List, Optional, Tuple

# ---------------------------------------------------------------------------
# Parsing and idle detection
# ---------------------------------------------------------------------------
IDLE_MIN_AGE_S = 90

_TIME_RE = re.compile(
    r"^(?P<date>\d{4}-\d{2}-\d{2})[ T](?P<time>\d{2}:\d{2}:\d{2})(?P<frac>\.\d+)?"
    r"\s*(?P<tz>Z|[+-]\d{2}:?\d{2})?"  # ISO 'Z' / '+00:00' or Go '+0000'
    r"(?:\s+[A-Z]{2,5})?"  # Go zone abbreviation, e.g. 'UTC'
    r"(?:\s+m=[+-]?[\d.]+)?$"  # Go monotonic clock reading
)


def parse_hub_time(value: str | None) -> datetime | None:
    """Parse Hub (Go) and transcript (ISO-8601) timestamps into an aware UTC datetime; None if unparsable."""
    if not value or not isinstance(value, str):
        return None
    m = _TIME_RE.match(value.strip())
    if not m:
        return None
    frac = (m.group("frac") or "")[1:7]
    frac = "." + frac.ljust(6, "0") if frac else ""
    tz = m.group("tz") or "+00:00"
    if tz == "Z":
        tz = "+00:00"
    elif ":" not in tz:
        tz = tz[:3] + ":" + tz[3:]
    try:
        parsed = datetime.fromisoformat(f"{m.group('date')}T{m.group('time')}{frac}{tz}")
    except ValueError:
        return None
    return parsed.astimezone(timezone.utc)


@dataclass
class TranscriptState:
    last_event_at: datetime | None
    last_assistant_at: datetime | None
    last_context_tokens: int | None
    last_compact_at: datetime | None


def parse_transcript(lines: list[str]) -> TranscriptState:
    """Summarize a Claude Code transcript tail: last event, last assistant call and its context, last compaction."""
    st = TranscriptState(None, None, None, None)
    for line in lines:
        try:
            event = json.loads(line)
        except (TypeError, ValueError):
            continue
        if not isinstance(event, dict):
            continue
        ts = parse_hub_time(event.get("timestamp"))
        if ts and (st.last_event_at is None or ts > st.last_event_at):
            st.last_event_at = ts
        if event.get("type") == "assistant":
            usage = (event.get("message") or {}).get("usage") or {}
            st.last_context_tokens = sum(
                int(usage.get(k) or 0)
                for k in ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens")
            )
            if ts:
                st.last_assistant_at = ts
        elif event.get("type") == "system" and event.get("subtype") == "compact_boundary" and ts:
            st.last_compact_at = ts
    return st


def pane_state(text: str) -> str:
    """'busy' while Claude Code works, 'idle' at its input prompt, 'unknown' otherwise."""
    if not text or not text.strip():
        return "unknown"
    if "esc to interrupt" in text.lower():
        return "busy"
    if any(line.lstrip().startswith("❯") for line in text.splitlines()):
        return "idle"
    return "unknown"


def is_idle(activity: str | None, pane: str, last_event_at: datetime | None, now: datetime) -> bool | None:
    """True only when the Hub, the pane and the transcript all agree the agent is idle; None if unknown."""
    if pane == "busy" or activity == "working":
        return False
    if pane != "idle" or last_event_at is None:
        return None
    return (now - last_event_at).total_seconds() > IDLE_MIN_AGE_S


# ---------------------------------------------------------------------------
# Context ceiling
# ---------------------------------------------------------------------------
CEILING_TOKENS = 200_000
COMPACT_COOLDOWN_S = 1800
HEARTBEAT_GUARD_S = 120
TMUX_TARGET = "scion:agent"
TRANSCRIPT_GLOB = "/home/scion/.claude/projects/-workspace/*.jsonl"
COMPACT_POLL_S = 15
COMPACT_WAIT_S = 420

COMPACT_INSTRUCTIONS = {
    "lead": (
        "Keep only what the lead needs to continue: open or in-flight GitHub issues and their current step, "
        "who you are waiting on and since when, standing PO rules and PO decisions not yet recorded on GitHub, "
        "the commit-slot holder, and pending escalations. Drop everything else: GitHub issues, commits and "
        "knowledge-plane/ are the record."
    ),
    "deployer": (
        "Keep only what the deployer needs to continue: an in-progress deploy or migration and its current step, "
        "the last deployed revision and rollback target, and pending live checks. Drop everything else: "
        "DEPLOYMENT.md and the GitHub issues are the record."
    ),
}

# run(argv, timeout_s) -> (returncode, stdout)
Runner = Callable[[List[str], int], Tuple[int, str]]


def read_pane(run: Runner, container: str) -> str:
    """Last lines of the agent's tmux pane ('' when unreadable)."""
    rc, out = run(["docker", "exec", "-u", "scion", container, "tmux", "capture-pane", "-p", "-t", TMUX_TARGET,
                   "-S", "-20"], 30)
    return out if rc == 0 else ""


def read_transcript_lines(run: Runner, container: str, n: int = 400) -> list[str]:
    """Tail of the newest Claude Code transcript in the container ([] when unreadable)."""
    rc, out = run(["docker", "exec", container, "sh", "-c", f"ls -t {TRANSCRIPT_GLOB} | head -1"], 30)
    path = out.strip() if rc == 0 else ""
    if not path:
        return []
    rc, out = run(["docker", "exec", container, "tail", "-n", str(n), path], 120)
    return out.splitlines() if rc == 0 else []


def should_compact(st: TranscriptState, idle: bool | None, now: datetime, last_compact_at: datetime | None,
                   next_heartbeat_at: datetime | None) -> tuple[bool, str]:
    """Decide whether to type /compact now; fail-closed on anything unknown."""
    ctx = st.last_context_tokens
    if ctx is None:
        return False, "no context reading"
    if ctx <= CEILING_TOKENS:
        return False, f"context {ctx} <= {CEILING_TOKENS}"
    if idle is not True:
        return False, "agent not idle" if idle is False else "idle state unknown"
    latest = max((t for t in (last_compact_at, st.last_compact_at) if t is not None), default=None)
    if latest is not None and (now - latest).total_seconds() < COMPACT_COOLDOWN_S:
        return False, "compaction cooldown"
    if next_heartbeat_at is None:
        return False, "next heartbeat unknown"
    if (next_heartbeat_at - now).total_seconds() < HEARTBEAT_GUARD_S:
        return False, "heartbeat due"
    return True, f"context {ctx} > {CEILING_TOKENS}"


def compact(run: Runner, container: str, role: str, since: datetime,
            sleep: Callable[[float], None] = time.sleep) -> bool:
    """Type `/compact <role instructions>` into an idle pane and wait for a compact_boundary newer than `since`."""
    if pane_state(read_pane(run, container)) != "idle":
        return False
    base = ["docker", "exec", "-u", "scion", container, "tmux", "send-keys", "-t", TMUX_TARGET]
    rc, _ = run(base + ["-l", "/compact " + COMPACT_INSTRUCTIONS[role]], 30)
    if rc != 0:
        return False
    sleep(2)
    rc, _ = run(base + ["Enter"], 30)
    if rc != 0:
        return False
    for _ in range(COMPACT_WAIT_S // COMPACT_POLL_S):
        sleep(COMPACT_POLL_S)
        done = parse_transcript(read_transcript_lines(run, container, 200)).last_compact_at
        if done is not None and done > since:
            return True
    return False


# ---------------------------------------------------------------------------
# Hub and GitHub readers
# ---------------------------------------------------------------------------
LEAD_CONTAINER = "psearch--lead"


class SignalError(Exception):
    """A signal source (hub.db, gh, transcript, pane, Hub API) could not be read."""


@dataclass
class Message:
    sender: str
    recipient: str
    body: str
    created_at: datetime


@dataclass
class Issue:
    number: int
    title: str
    created_at: datetime
    updated_at: datetime
    last_comment_at: datetime | None


@dataclass
class HubSnapshot:
    project_id: str
    activity: dict[str, str]
    messages: list[Message]


def read_hub(db_path: str, project_slug: str, since: datetime) -> HubSnapshot:
    """Agent activity and the project's messages created at or after `since`, read-only."""
    if not os.path.exists(db_path):
        raise SignalError(f"hub.db not found: {db_path}")
    try:
        conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
        try:
            row = conn.execute("SELECT id FROM projects WHERE slug = ?", (project_slug,)).fetchone()
            if not row:
                raise SignalError(f"unknown project slug: {project_slug}")
            project_id = row[0]
            activity = {
                name: act or ""
                for name, act in conn.execute(
                    "SELECT name, activity FROM agents WHERE project_id = ? AND deleted_at IS NULL", (project_id,))
            }
            rows = conn.execute(
                "SELECT sender, recipient, msg, created FROM messages WHERE project_id = ? AND substr(created, 1, 10) >= ?",
                (project_id, since.strftime("%Y-%m-%d"))).fetchall()
        finally:
            conn.close()
    except sqlite3.Error as err:
        raise SignalError(f"hub.db read failed: {err}") from err
    messages = []
    for sender, recipient, body, created in rows:
        created_at = parse_hub_time(created)
        if created_at is not None and created_at >= since:
            messages.append(Message(sender or "", recipient or "", body or "", created_at))
    messages.sort(key=lambda m: m.created_at)
    return HubSnapshot(project_id, activity, messages)


def parse_issues(gh_json: str) -> list[Issue]:
    """Parse `gh issue list --json number,title,createdAt,updatedAt,comments` output."""
    try:
        items = json.loads(gh_json)
    except (TypeError, ValueError) as err:
        raise SignalError(f"gh output is not JSON: {str(gh_json)[:120]!r}") from err
    if not isinstance(items, list):
        raise SignalError(f"gh output is not a list: {str(gh_json)[:120]!r}")
    issues = []
    for item in items:
        created_at = parse_hub_time(item.get("createdAt")) if isinstance(item, dict) else None
        updated_at = parse_hub_time(item.get("updatedAt")) if isinstance(item, dict) else None
        if created_at is None or updated_at is None or not isinstance(item.get("number"), int):
            raise SignalError(f"unexpected gh issue item: {str(item)[:120]!r}")
        comment_times = [parse_hub_time(c.get("createdAt")) for c in item.get("comments") or [] if isinstance(c, dict)]
        comment_times = [t for t in comment_times if t is not None]
        issues.append(Issue(item["number"], item.get("title") or "", created_at, updated_at,
                            max(comment_times) if comment_times else None))
    return issues


def read_issues(run: Runner, repo: str, label: str = "psearch", container: str = LEAD_CONTAINER) -> list[Issue]:
    """Open issues with `label`, read with the GitHub token already injected in the lead's container."""
    rc, out = run(["docker", "exec", "-u", "scion", container, "gh", "issue", "list", "-R", repo, "--state",
                   "open", "--label", label, "--json", "number,title,createdAt,updatedAt,comments", "--limit", "50"],
                  60)
    if rc != 0:
        raise SignalError(f"gh issue list exited {rc}")
    return parse_issues(out)


# ---------------------------------------------------------------------------
# Wake rules engine
# ---------------------------------------------------------------------------
GATE_PREFIX = "GATE (automated, not the PO): "
TEAM = ("backend", "frontend", "reviewer", "deployer")
RECENT_TURN_GUARD_S = 300
R2_GRACE_S = 600
R2_COOLDOWN_S = 1800
R3_SILENCE_S = 1800
R3_COOLDOWN_S = 1800
R3_MAX_WAKES = 2
R5_BASE_S = 3600
R5_MAX_S = 14400


@dataclass
class Snapshot:
    now: datetime
    lead_idle: bool | None
    lead_last_turn_at: datetime | None
    issues: list[Issue] | None
    messages: list[Message]
    activity: dict[str, str]


@dataclass
class Wake:
    rule: str
    key: str
    reason: str


def _iso(ts: datetime) -> str:
    return ts.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


def _is_ignored_message(body: str) -> bool:
    stripped = (body or "").lstrip()
    return stripped.startswith(GATE_PREFIX.strip()) or stripped.startswith("HEARTBEAT")


def _clean_rule_state(state: dict) -> tuple[dict, bool]:
    """Copy `state` (preserving non-rule keys like `signal_failures`) and validate rule fields."""
    base = {k: v for k, v in (state.items() if isinstance(state, dict) else [])
            if k not in ("initialized", "known_issues", "open_issue_count", "last_wake", "r3", "r5")}
    if not isinstance(state, dict) or state.get("initialized") is not True:
        return base, False
    known = state.get("known_issues")
    last_wake = state.get("last_wake")
    r3 = state.get("r3")
    r5 = state.get("r5")
    if (not isinstance(known, list) or not all(isinstance(x, int) for x in known)
            or not isinstance(last_wake, dict) or not isinstance(r3, dict) or not isinstance(r5, dict)):
        return base, False
    out = dict(base)
    out["initialized"] = True
    out["known_issues"] = list(known)
    out["open_issue_count"] = int(state.get("open_issue_count") or 0)
    out["last_wake"] = {str(k): str(v) for k, v in last_wake.items() if isinstance(v, str)}
    out["r3"] = {
        str(k): {"streak_start": str(v["streak_start"]), "wakes": int(v["wakes"])}
        for k, v in r3.items()
        if isinstance(v, dict) and isinstance(v.get("streak_start"), str) and isinstance(v.get("wakes"), int)
    }
    interval = int(r5.get("interval_s") or R5_BASE_S)
    lw = r5.get("last_wake_at")
    out["r5"] = {"interval_s": max(R5_BASE_S, min(interval, R5_MAX_S)),
                 "last_wake_at": lw if isinstance(lw, str) else None}
    return out, True


def _cooldown_ok(last_wake: dict[str, str], key: str, now: datetime, cooldown_s: int) -> bool:
    prev = parse_hub_time(last_wake.get(key))
    return prev is None or (now - prev).total_seconds() >= cooldown_s


def evaluate(snap: Snapshot, state: dict) -> tuple[list[Wake], dict]:
    """Pure evaluation of R1, R2, R3 and R5 against `snap`, returning `(wakes, new_state)`."""
    out, ok = _clean_rule_state(state)
    if not ok:
        if snap.issues is None:
            out.update({"initialized": False, "known_issues": [], "open_issue_count": 0, "last_wake": {},
                        "r3": {}, "r5": {"interval_s": R5_BASE_S, "last_wake_at": None}})
            return [], out
        out.update({
            "initialized": True,
            "known_issues": sorted({i.number for i in snap.issues}),
            "open_issue_count": len(snap.issues),
            "last_wake": {},
            "r3": {},
            "r5": {"interval_s": R5_BASE_S, "last_wake_at": None},
        })
        return [], out

    if snap.issues is not None:
        out["open_issue_count"] = len(snap.issues)
    open_count = out["open_issue_count"]

    msgs = [m for m in snap.messages if not _is_ignored_message(m.body)]

    # Reset R5 backoff on any real message or issue update/comment newer than r5.last_wake_at; also clear
    # R3 streaks when a worker has replied, even if the global guard suppresses waking on this tick.
    r5_last = parse_hub_time(out["r5"]["last_wake_at"])
    if r5_last is not None:
        activity_times = [m.created_at for m in msgs]
        for iss in snap.issues or []:
            activity_times.append(iss.updated_at)
            if iss.last_comment_at is not None:
                activity_times.append(iss.last_comment_at)
        if any(t > r5_last for t in activity_times):
            out["r5"]["interval_s"] = R5_BASE_S
    if open_count == 0:
        out["r5"] = {"interval_s": R5_BASE_S, "last_wake_at": None}

    unanswered: dict[str, datetime] = {}
    for agent in TEAM:
        replies = [m.created_at for m in msgs if m.sender == f"agent:{agent}" and m.recipient == "agent:lead"]
        last_reply = max(replies) if replies else None
        reqs = [m.created_at for m in msgs if m.sender == "agent:lead" and m.recipient == f"agent:{agent}"
                and (last_reply is None or m.created_at > last_reply)]
        if not reqs:
            if last_reply is not None:
                out["r3"].pop(agent, None)
            continue
        first_req = min(reqs)
        existing = out["r3"].get(agent)
        prev_start = parse_hub_time(existing["streak_start"]) if existing else None
        if prev_start is None or (last_reply is not None and last_reply > prev_start):
            out["r3"][agent] = {"streak_start": _iso(first_req), "wakes": 0}
            unanswered[agent] = first_req
        else:
            start = min(prev_start, first_req)
            out["r3"][agent]["streak_start"] = _iso(start)
            unanswered[agent] = start

    # Global guard: never wake while the lead is working or within 5 min after its last turn.
    if snap.lead_idle is False:
        return [], out
    if snap.lead_last_turn_at is not None and (snap.now - snap.lead_last_turn_at).total_seconds() < RECENT_TURN_GUARD_S:
        return [], out

    wakes: list[Wake] = []
    known_set = set(out["known_issues"])
    new_numbers: set[int] = set()

    # R1: New issue
    if snap.issues is not None:
        for iss in sorted(snap.issues, key=lambda i: i.number):
            if iss.number not in known_set:
                key = f"r1:{iss.number}"
                known_set.add(iss.number)
                new_numbers.add(iss.number)
                out["last_wake"][key] = _iso(snap.now)
                wakes.append(Wake("r1", key, f'new issue #{iss.number} "{iss.title}" is open and has no plan from you yet'))
        out["known_issues"] = sorted(known_set)

    # R2: Unattended activity on an already-known open issue
    if snap.issues is not None and snap.lead_last_turn_at is not None:
        cutoff = snap.lead_last_turn_at + timedelta(seconds=R2_GRACE_S)
        for iss in sorted(snap.issues, key=lambda i: i.number):
            if iss.number in new_numbers:
                continue
            latest = max(t for t in (iss.updated_at, iss.last_comment_at) if t is not None)
            key = f"r2:{iss.number}"
            r1_key = f"r1:{iss.number}"
            if (latest > cutoff and latest <= snap.now
                    and _cooldown_ok(out["last_wake"], key, snap.now, R2_COOLDOWN_S)
                    and _cooldown_ok(out["last_wake"], r1_key, snap.now, R2_COOLDOWN_S)):
                out["last_wake"][key] = _iso(snap.now)
                wakes.append(Wake("r2", key,
                                  f"issue #{iss.number} has activity since your last turn (latest at {latest.strftime('%H:%M')}Z)"))

    # R3: Unanswered request to a non-working team agent
    if open_count >= 1:
        for agent in TEAM:
            start = unanswered.get(agent)
            entry = out["r3"].get(agent)
            key = f"r3:{agent}"
            if start is None or entry is None or snap.activity.get(agent) == "working":
                continue
            age_s = (snap.now - start).total_seconds()
            if age_s > R3_SILENCE_S and entry["wakes"] < R3_MAX_WAKES and _cooldown_ok(out["last_wake"], key, snap.now, R3_COOLDOWN_S):
                entry["wakes"] += 1
                out["last_wake"][key] = _iso(snap.now)
                wakes.append(Wake("r3", key,
                                  f"{agent} has not replied for {int(age_s // 60)} min to your message at {start.strftime('%H:%M')}Z"))

    # R5: Safety wake with exponential backoff (1h -> 2h -> 4h cap), only when R1-R3 found nothing this tick
    if open_count >= 1 and not wakes:
        interval_s = out["r5"]["interval_s"]
        ref_candidates = [t for t in (snap.lead_last_turn_at, parse_hub_time(out["r5"]["last_wake_at"])) if t is not None]
        ref = max(ref_candidates) if ref_candidates else None
        elapsed_s = (snap.now - ref).total_seconds() if ref is not None else float(interval_s)
        if elapsed_s >= interval_s:
            mins = int(elapsed_s // 60)
            out["r5"]["last_wake_at"] = _iso(snap.now)
            out["r5"]["interval_s"] = min(interval_s * 2, R5_MAX_S)
            out["last_wake"]["r5"] = _iso(snap.now)
            wakes.append(Wake("r5", "r5", f"periodic check: {open_count} open issue(s), no lead turn for {mins} min"))

    return wakes, out


# ---------------------------------------------------------------------------
# Effects — wake message, run log and blind-signal alerts
# ---------------------------------------------------------------------------
SIGNAL_ALERT_THRESHOLD = 3

ApiCall = Callable[[str, str, Optional[Dict]], Tuple[int, Dict]]


def format_wake(wakes: list[Wake]) -> str:
    """Single Hub message body carrying every wake reason from one evaluation."""
    return GATE_PREFIX + "; ".join(w.reason for w in wakes) + ". Follow your wake-up checklist."


def send_wake(api: ApiCall, project_id: str, text: str) -> bool:
    status, _ = api("POST", f"/api/v1/projects/{project_id}/agents/lead/message", {"message": text})
    return status in (200, 204)


def record_signal_health(state: dict, failures: list[str]) -> tuple[dict, list[str]]:
    """Track consecutive failures per signal; return `(new_state, newly_alerted)` when a counter reaches 3."""
    out = dict(state) if isinstance(state, dict) else {}
    prev = out.get("signal_failures")
    counters = {str(k): int(v) for k, v in prev.items()} if isinstance(prev, dict) else {}
    failed_set = {str(f) for f in failures}
    alerted: list[str] = []
    for key in list(counters.keys()):
        if key not in failed_set:
            counters[key] = 0
    for key in sorted(failed_set):
        counters[key] = counters.get(key, 0) + 1
        if counters[key] == SIGNAL_ALERT_THRESHOLD:
            alerted.append(key)
    out["signal_failures"] = counters
    return out, alerted


def log_run(path: str, entry: dict) -> None:
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    with open(path, "a", encoding="utf-8") as f:
        f.write(json.dumps(entry, sort_keys=True) + "\n")


def _default_runner(argv: list[str], timeout: int) -> tuple[int, str]:
    try:
        res = subprocess.run(argv, capture_output=True, text=True, timeout=timeout)
        return res.returncode, res.stdout
    except Exception as err:  # noqa: BLE001
        print(f"WARNING: command {argv[:3]} failed: {err}", file=sys.stderr)
        return 1, ""


def log_alert(event_type: str, payload: dict, severity: str, project_id: str,
              run: Runner | None = None) -> None:
    body = {"event": event_type, **payload}
    print(f"[{severity}] {event_type}: {json.dumps(body, sort_keys=True)}")
    if not project_id:
        return
    runner = run or _default_runner
    rc, _ = runner([
        "gcloud", "logging", "write", "scion-factory-alerts",
        json.dumps(body), "--payload-type=json", f"--severity={severity}", f"--project={project_id}",
    ], 15)
    if rc != 0:
        print(f"WARNING: gcloud logging write failed (code {rc})", file=sys.stderr)


# ---------------------------------------------------------------------------
# CLI, run loop, kill switch and cron install
# ---------------------------------------------------------------------------
TRANSCRIPT_TAIL_LINES = 600
HUB_MESSAGE_LOOKBACK_DAYS = 2
CLAUDE_ROLES = ("lead", "deployer")


@dataclass
class Deps:
    run: Runner
    api: ApiCall
    now: Callable[[], datetime]
    sleep: Callable[[float], None]


def _load_state_file(path: str) -> dict:
    if not os.path.exists(path):
        return {}
    try:
        with open(path, encoding="utf-8") as f:
            loaded = json.load(f)
        return loaded if isinstance(loaded, dict) else {}
    except Exception as err:  # noqa: BLE001
        print(f"WARNING: failed to read {path}: {err}", file=sys.stderr)
        return {}


def _save_state_file(path: str, state: dict) -> None:
    parent = os.path.dirname(path) or "."
    os.makedirs(parent, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=parent, prefix=".lead-controller-state.")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(state, f, indent=2, sort_keys=True)
        os.chmod(tmp, 0o600)
        os.replace(tmp, path)
    except Exception:
        if os.path.exists(tmp):
            os.unlink(tmp)
        raise


def _next_heartbeat(api: ApiCall, project_id: str, project_slug: str) -> datetime | None:
    if not project_id:
        return None
    status, body = api("GET", f"/api/v1/projects/{project_id}/schedules", None)
    if status != 200:
        return None
    schedules = body.get("schedules", []) if isinstance(body, dict) else body
    if not isinstance(schedules, list):
        return None
    want = f"{project_slug}-lead-heartbeat"
    for sched in schedules:
        if isinstance(sched, dict) and sched.get("name") == want:
            return parse_hub_time(sched.get("nextRunAt"))
    return None


def run_once(deps: Deps, cfg: dict, apply_compact: bool, apply_wake: bool) -> dict:
    """One controller pass: kill switch -> signals -> ceiling -> wake rules -> effects -> alerts -> state/log."""
    now = deps.now()
    disabled_path = os.path.join(cfg["home"], "lead-controller.disabled")
    if os.path.exists(disabled_path):
        entry = {"ts": _iso(now), "result": "disabled"}
        log_run(cfg["log_path"], entry)
        return entry

    state = _load_state_file(cfg["state_path"])
    failures: list[str] = []
    project_slug = cfg["project_slug"]
    hub_project_id = cfg.get("hub_project_id") or ""

    try:
        hub = read_hub(cfg["db_path"], project_slug, since=now - timedelta(days=HUB_MESSAGE_LOOKBACK_DAYS))
        project_id, activity, messages = hub.project_id, hub.activity, hub.messages
    except SignalError:
        failures.append("hub")
        project_id, activity, messages = "", {}, []

    lead_container = f"{project_slug}--lead"
    try:
        if not cfg.get("repo"):
            raise SignalError("GITHUB_REPO is empty")
        issues: list[Issue] | None = read_issues(deps.run, cfg["repo"], label=project_slug, container=lead_container)
    except SignalError:
        failures.append("gh")
        issues = None

    transcripts: dict[str, TranscriptState] = {}
    idles: dict[str, bool | None] = {}
    for role in CLAUDE_ROLES:
        container = f"{project_slug}--{role}"
        st = parse_transcript(read_transcript_lines(deps.run, container, TRANSCRIPT_TAIL_LINES))
        if st.last_assistant_at is None:
            failures.append(f"{role}_transcript")
        pane = pane_state(read_pane(deps.run, container))
        if pane == "unknown":
            failures.append(f"{role}_pane")
        transcripts[role] = st
        idles[role] = is_idle(activity.get(role), pane, st.last_event_at, now)

    next_hb = _next_heartbeat(deps.api, project_id, project_slug)
    last_compact = dict(state.get("last_compact")) if isinstance(state.get("last_compact"), dict) else {}
    compacted: list[str] = []
    would_compact: list[str] = []
    for role in CLAUDE_ROLES:
        container = f"{project_slug}--{role}"
        prev_compact = parse_hub_time(last_compact.get(role))
        want, _ = should_compact(transcripts[role], idles[role], now, prev_compact, next_hb)
        if not want:
            continue
        if apply_compact:
            if compact(deps.run, container, role, since=now, sleep=deps.sleep):
                compacted.append(role)
                now = deps.now()
                last_compact[role] = _iso(now)
                log_alert("LEAD_CONTROLLER_COMPACT", {
                    "project": project_slug, "agent": role,
                    "pre_context_tokens": transcripts[role].last_context_tokens,
                }, "INFO", hub_project_id, run=deps.run)
        else:
            would_compact.append(role)

    snap = Snapshot(
        now=now,
        lead_idle=idles["lead"],
        lead_last_turn_at=transcripts["lead"].last_assistant_at,
        issues=issues,
        messages=messages,
        activity=activity,
    )
    wakes, next_state = evaluate(snap, state)
    woke: list[str] = []
    would_wake: list[str] = []
    if wakes:
        if apply_wake:
            if project_id and send_wake(deps.api, project_id, format_wake(wakes)):
                woke = [w.key for w in wakes]
                log_alert("LEAD_CONTROLLER_WAKE", {
                    "project": project_slug, "keys": woke, "reasons": [w.reason for w in wakes],
                }, "INFO", hub_project_id, run=deps.run)
            else:
                failures.append("wake_api")
                next_state = dict(state)
        else:
            would_wake = [w.key for w in wakes]

    next_state["last_compact"] = last_compact
    next_state, alerted = record_signal_health(next_state, failures)
    if alerted:
        log_alert("LEAD_CONTROLLER_SIGNAL_BLIND", {
            "project": project_slug, "signals": alerted, "consecutive_runs": SIGNAL_ALERT_THRESHOLD,
        }, "ERROR", hub_project_id, run=deps.run)

    _save_state_file(cfg["state_path"], next_state)
    entry = {
        "ts": _iso(now),
        "result": "ok",
        "apply_compact": apply_compact,
        "apply_wake": apply_wake,
        "lead_context_tokens": transcripts["lead"].last_context_tokens,
        "deployer_context_tokens": transcripts["deployer"].last_context_tokens,
        "lead_idle": idles["lead"],
        "open_issues": len(issues) if issues is not None else None,
        "compacted": compacted,
        "would_compact": would_compact,
        "woke": woke,
        "would_wake": would_wake,
        "failures": failures,
        "alerted": alerted,
    }
    log_run(cfg["log_path"], entry)
    return entry


def load_env_file(path: str) -> None:
    if not os.path.exists(path):
        return
    with open(path, encoding="utf-8") as f:
        for raw in f:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            k = k.strip()
            v = v.strip().strip("\"'")
            if k and k not in os.environ:
                os.environ[k] = v


def _mint_admin_jwt(db_path: str, hub_env_path: str, admin_email: str) -> str:
    if not os.path.exists(db_path) or not os.path.exists(hub_env_path):
        raise SignalError("hub.db or hub.env missing")
    conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    conn.row_factory = sqlite3.Row
    try:
        user = None
        if admin_email:
            user = conn.execute("SELECT id, email, role FROM users WHERE email = ?", (admin_email,)).fetchone()
        if not user:
            user = conn.execute("SELECT id, email, role FROM users WHERE role = 'admin' ORDER BY created ASC LIMIT 1").fetchone()
        if not user:
            user = conn.execute("SELECT id, email, role FROM users ORDER BY created ASC LIMIT 1").fetchone()
        if not user:
            raise SignalError("no user in hub.db")
    finally:
        conn.close()
    secret = None
    with open(hub_env_path, encoding="utf-8") as f:
        for line in f:
            if line.startswith("SCION_SERVER_SESSION_SECRET="):
                secret = line.split("=", 1)[1].strip()
    if not secret:
        raise SignalError("SCION_SERVER_SESSION_SECRET missing in hub.env")
    key = hashlib.sha256(("scion-hub-signing-key:user_signing_key:" + secret).encode()).digest()

    def b64(data: bytes) -> str:
        return base64.urlsafe_b64encode(data).rstrip(b"=").decode()

    now_ts = int(time.time())
    claims = {
        "iss": "scion-hub", "sub": user["id"], "aud": ["scion-hub-api"], "exp": now_ts + 300,
        "nbf": now_ts - 10, "iat": now_ts, "uid": user["id"], "email": user["email"],
        "name": user["email"], "role": user["role"], "type": "access", "client": "web",
    }
    signing_input = b64(json.dumps({"alg": "HS256", "typ": "JWT"}).encode()) + "." + b64(json.dumps(claims).encode())
    sig = hmac.new(key, signing_input.encode(), hashlib.sha256).digest()
    return signing_input + "." + b64(sig)


def _make_hub_api(hub_url: str, db_path: str, hub_env_path: str, admin_email: str) -> ApiCall:
    token_box: list[str] = []
    base_url = hub_url.rstrip("/")

    def call(method: str, path: str, body: dict | None = None) -> tuple[int, dict]:
        if not token_box:
            try:
                token_box.append(_mint_admin_jwt(db_path, hub_env_path, admin_email))
            except Exception as err:  # noqa: BLE001
                return 500, {"error": str(err)}
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(
            base_url + path, data=data, method=method,
            headers={"Authorization": f"Bearer {token_box[0]}", "Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                raw = resp.read().decode()
                return resp.status, (json.loads(raw) if raw else {})
        except urllib.error.HTTPError as err:
            return err.code, {"error": err.read().decode()[:500]}
        except Exception as err:  # noqa: BLE001
            return 500, {"error": str(err)}

    return call


def _read_system_crontab() -> str:
    res = subprocess.run(["crontab", "-l"], capture_output=True, text=True)
    return res.stdout if res.returncode == 0 else ""


def _write_system_crontab(content: str) -> None:
    subprocess.run(["crontab", "-"], input=content, text=True, check=True)


def install_cron(flags: list[str], cfg: dict,
                 get_crontab: Callable[[], str] = _read_system_crontab,
                 set_crontab: Callable[[str], None] = _write_system_crontab) -> str:
    """Copy script to `<home>/lead-controller.py` (0700), write env (0600) and register the 5-min cron entry."""
    home = cfg["home"]
    os.makedirs(home, exist_ok=True)
    dst_script = os.path.join(home, "lead-controller.py")
    src_script = os.path.abspath(__file__)
    if os.path.realpath(src_script) != os.path.realpath(dst_script):
        shutil.copy2(src_script, dst_script)
    os.chmod(dst_script, 0o700)

    cur_path = os.environ.get("PATH", "/usr/local/bin:/usr/bin:/bin")
    if "/snap/bin" not in cur_path.split(":"):
        cur_path = f"/snap/bin:{cur_path}"
    admin_email = cfg.get("admin_email") or ""
    env_path = os.path.join(home, "lead-controller.env")
    env_content = "\n".join([
        "# Lead cost controller configuration",
        f"HUB_PROJECT_ID={cfg.get('hub_project_id') or ''}",
        f"GITHUB_REPO={cfg.get('repo') or ''}",
        f"ADMIN_EMAIL={admin_email}",
        f"BUILD_PROJECT={cfg.get('project_slug') or 'psearch'}",
        f"PATH={cur_path}",
        "",
    ])
    with open(env_path, "w", encoding="utf-8") as f:
        f.write(env_content)
    os.chmod(env_path, 0o600)

    flag_str = (" " + " ".join(flags)) if flags else ""
    cron_log = os.path.join(home, "lead-controller.cron.log")
    cron_line = f"*/5 * * * * /usr/bin/python3 {dst_script}{flag_str} >> {cron_log} 2>&1"
    kept = [line for line in get_crontab().splitlines() if "lead-controller.py" not in line]
    kept.append(cron_line)
    set_crontab("\n".join(kept).strip() + "\n")
    return cron_line


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Scion lead cost controller (context ceiling + activity gate)")
    parser.add_argument("--apply-compact", action="store_true", help="Send /compact when context > 200k and idle")
    parser.add_argument("--apply-wake", action="store_true", help="Send wake messages when R1-R3/R5 fire")
    parser.add_argument("--install-cron", action="store_true", help="Install to ~/.scion and register 5-min cron")
    args = parser.parse_args(argv)

    home = os.path.expanduser(os.environ.get("SCION_HOME", "~/.scion"))
    for env_name in ("lead-controller.env", "watchdog.env", "sync.env"):
        load_env_file(os.path.join(home, env_name))

    admin_email = os.environ.get("ADMIN_EMAIL") or os.environ.get("PO_EMAIL", "")
    cfg = {
        "home": home,
        "db_path": os.path.join(home, "hub.db"),
        "state_path": os.path.join(home, "lead-controller-state.json"),
        "log_path": os.path.join(home, "lead-controller.log"),
        "repo": os.environ.get("GITHUB_REPO", ""),
        "project_slug": os.environ.get("BUILD_PROJECT", "psearch"),
        "hub_project_id": os.environ.get("HUB_PROJECT_ID") or os.environ.get("HUB_PROJECT", ""),
        "admin_email": admin_email,
    }

    if args.install_cron:
        flags = []
        if args.apply_compact:
            flags.append("--apply-compact")
        if args.apply_wake:
            flags.append("--apply-wake")
        line = install_cron(flags, cfg)
        print(f"Registered lead-controller job in crontab:\n  {line}")
        return 0

    hub_url = os.environ.get("HUB_LOCAL_URL", "http://localhost:8080")
    deps = Deps(
        run=_default_runner,
        api=_make_hub_api(hub_url, cfg["db_path"], os.path.join(home, "hub.env"), admin_email),
        now=lambda: datetime.now(timezone.utc),
        sleep=time.sleep,
    )
    entry = run_once(deps, cfg, apply_compact=args.apply_compact, apply_wake=args.apply_wake)
    print(json.dumps(entry, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
