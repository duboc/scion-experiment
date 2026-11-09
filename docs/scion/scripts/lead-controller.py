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
from datetime import datetime, timezone
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
