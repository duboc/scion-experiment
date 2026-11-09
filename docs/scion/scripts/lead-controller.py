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
import re
from dataclasses import dataclass
from datetime import datetime, timezone

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
