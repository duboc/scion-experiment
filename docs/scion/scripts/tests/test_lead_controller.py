"""Unit tests for docs/scion/scripts/lead-controller.py (stdlib unittest; no Hub, Docker or network needed).

Run: python3 -m unittest discover -s docs/scion/scripts/tests -v
"""
import importlib.util
import json
import pathlib
import sys
import unittest
from datetime import datetime, timedelta, timezone

SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "lead-controller.py"


def load_controller():
    spec = importlib.util.spec_from_file_location("lead_controller", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module  # dataclasses resolve postponed annotations through sys.modules
    spec.loader.exec_module(module)
    return module


lc = load_controller()

UTC = timezone.utc


def assistant_line(ts, cache_write, cache_read=0, input_tokens=2, msg_id="msg_1"):
    return json.dumps({
        "type": "assistant", "timestamp": ts,
        "message": {"id": msg_id, "model": "claude-opus-5-5",
                    "usage": {"input_tokens": input_tokens, "cache_creation_input_tokens": cache_write,
                              "cache_read_input_tokens": cache_read, "output_tokens": 106}},
    })


def compact_line(ts):
    return json.dumps({"type": "system", "subtype": "compact_boundary", "timestamp": ts,
                       "compactMetadata": {"trigger": "manual", "preTokens": 764580, "postTokens": 2326}})


FIXTURE_LINES = [
    json.dumps({"type": "user", "timestamp": "2026-11-06T21:53:42.100Z",
                "message": {"role": "user", "content": "/compact Keep only what the lead needs to continue"}}),
    compact_line("2026-11-06T21:54:32.525Z"),
    "this line is not json",
    json.dumps({"type": "user", "timestamp": "2026-11-06T22:00:30.000Z",
                "message": {"role": "user", "content": "HEARTBEAT (every 15 min): ..."}}),
    assistant_line("2026-11-06T22:00:34.000Z", cache_write=12745),
]

# Tail of the lead's pane captured right after the step-0 /compact on 2026-11-06.
PANE_AFTER_STEP0 = "\n".join([
    "❯ /compact Keep only what the lead needs to continue: open or in-flight GitHub issues and their current step,",
    "  ⎿  Compacted (ctrl+o to see full summary)",
    "  ⎿  Referenced file src/psearch/serving/internal/api/routes.go",
    "",
    "─" * 60,
    "❯ ",
    "─" * 60,
    "  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
])

PANE_BUSY = "✻ Thinking… (esc to interrupt)"


class ParsingTest(unittest.TestCase):
    def test_parse_hub_time_go_format(self):
        self.assertEqual(lc.parse_hub_time("2026-11-06 18:00:18.665550575 +0000 UTC m=+1979146.354969246"),
                         datetime(2026, 11, 6, 18, 0, 18, 665550, tzinfo=UTC))

    def test_parse_hub_time_iso_z(self):
        self.assertEqual(lc.parse_hub_time("2026-11-06T21:54:32.525Z"),
                         datetime(2026, 11, 6, 21, 54, 32, 525000, tzinfo=UTC))

    def test_parse_hub_time_garbage(self):
        self.assertIsNone(lc.parse_hub_time("not a time"))
        self.assertIsNone(lc.parse_hub_time(""))
        self.assertIsNone(lc.parse_hub_time(None))

    def test_parse_transcript_last_context_and_compact(self):
        st = lc.parse_transcript(FIXTURE_LINES)
        self.assertEqual(st.last_context_tokens, 12747)
        self.assertEqual(st.last_compact_at, datetime(2026, 11, 6, 21, 54, 32, 525000, tzinfo=UTC))
        self.assertEqual(st.last_assistant_at, datetime(2026, 11, 6, 22, 0, 34, tzinfo=UTC))
        self.assertEqual(st.last_event_at, datetime(2026, 11, 6, 22, 0, 34, tzinfo=UTC))

    def test_parse_transcript_empty(self):
        st = lc.parse_transcript([])
        self.assertIsNone(st.last_context_tokens)
        self.assertIsNone(st.last_event_at)

    def test_pane_state(self):
        self.assertEqual(lc.pane_state(PANE_BUSY), "busy")
        self.assertEqual(lc.pane_state(PANE_AFTER_STEP0), "idle")
        self.assertEqual(lc.pane_state(""), "unknown")
        self.assertEqual(lc.pane_state("some unrelated output"), "unknown")

    def test_is_idle_requires_all_three(self):
        now = datetime(2026, 11, 9, 12, 0, tzinfo=UTC)
        old, recent = now - timedelta(seconds=120), now - timedelta(seconds=60)
        self.assertTrue(lc.is_idle("completed", "idle", old, now))
        self.assertFalse(lc.is_idle("working", "idle", old, now))
        self.assertFalse(lc.is_idle("completed", "busy", old, now))
        self.assertFalse(lc.is_idle("completed", "idle", recent, now))
        self.assertIsNone(lc.is_idle("completed", "unknown", old, now))
        self.assertIsNone(lc.is_idle("completed", "idle", None, now))


if __name__ == "__main__":
    unittest.main()
