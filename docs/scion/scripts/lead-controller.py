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

import json
import os
import re
import sqlite3
import time
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Callable, List, Tuple

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


def read_issues(run: Runner, repo: str, label: str = "psearch") -> list[Issue]:
    """Open issues with `label`, read with the GitHub token already injected in the lead's container."""
    rc, out = run(["docker", "exec", "-u", "scion", LEAD_CONTAINER, "gh", "issue", "list", "-R", repo, "--state",
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
