"""Unit tests for docs/scion/scripts/lead-controller.py (stdlib unittest; no Hub, Docker or network needed).

Run: python3 -m unittest discover -s docs/scion/scripts/tests -v
"""
import importlib.util
import json
import os
import pathlib
import sqlite3
import sys
import tempfile
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


NOW = datetime(2026, 11, 9, 12, 7, tzinfo=UTC)


def st(ctx, compact_at=None, last_event_at=None):
    return lc.TranscriptState(last_event_at=last_event_at, last_assistant_at=last_event_at,
                              last_context_tokens=ctx, last_compact_at=compact_at)


class FakeRunner:
    """Stands in for `docker exec ...`: records argv lists and answers tmux / ls / tail like the Hub would."""

    def __init__(self, pane="", boundary_after_send=False, transcript=None):
        self.pane = pane
        self.boundary_after_send = boundary_after_send
        self.transcript = list(transcript) if transcript is not None else [
            assistant_line("2026-11-09T11:45:30.000Z", cache_write=250_000)]
        self.calls = []

    def __call__(self, argv, timeout):
        self.calls.append(list(argv))
        if "capture-pane" in argv:
            return 0, self.pane
        if "send-keys" in argv:
            return 0, ""
        if "sh" in argv and any("ls -t" in a for a in argv):
            return 0, "/home/scion/.claude/projects/-workspace/session.jsonl\n"
        if "tail" in argv:
            lines = list(self.transcript)
            if self.boundary_after_send and any("send-keys" in c for c in self.calls):
                lines.append(compact_line("2026-11-09T12:08:10.000Z"))
            return 0, "\n".join(lines) + "\n"
        return 1, ""


class CeilingTest(unittest.TestCase):
    def test_should_compact_above_ceiling_when_idle(self):
        ok, _ = lc.should_compact(st(ctx=200_001), True, NOW, None, NOW + timedelta(minutes=30))
        self.assertTrue(ok)

    def test_no_compact_at_or_below_ceiling(self):
        self.assertFalse(lc.should_compact(st(ctx=200_000), True, NOW, None, NOW + timedelta(minutes=30))[0])

    def test_no_compact_when_busy_or_unknown(self):
        later = NOW + timedelta(minutes=30)
        self.assertFalse(lc.should_compact(st(ctx=500_000), False, NOW, None, later)[0])
        self.assertFalse(lc.should_compact(st(ctx=500_000), None, NOW, None, later)[0])

    def test_no_compact_within_cooldown(self):
        later = NOW + timedelta(minutes=30)
        self.assertFalse(lc.should_compact(st(ctx=500_000), True, NOW, NOW - timedelta(minutes=20), later)[0])
        # the transcript's own compact_boundary counts too
        self.assertFalse(lc.should_compact(st(ctx=500_000, compact_at=NOW - timedelta(minutes=5)), True, NOW,
                                           None, later)[0])

    def test_no_compact_near_heartbeat(self):
        self.assertFalse(lc.should_compact(st(ctx=500_000), True, NOW, None, NOW + timedelta(seconds=90))[0])

    def test_no_compact_when_next_heartbeat_unknown(self):
        self.assertFalse(lc.should_compact(st(ctx=500_000), True, NOW, None, None)[0])

    def test_compact_aborts_if_pane_busy_at_send_time(self):
        run = FakeRunner(pane="✻ Working… (esc to interrupt)")
        self.assertFalse(lc.compact(run, "psearch--lead", "lead", NOW, sleep=lambda s: None))
        self.assertFalse(any("send-keys" in c for c in run.calls))

    def test_compact_sends_literal_text_then_enter(self):
        run = FakeRunner(pane=PANE_AFTER_STEP0, boundary_after_send=True)
        self.assertTrue(lc.compact(run, "psearch--lead", "lead", NOW, sleep=lambda s: None))
        sends = [c for c in run.calls if "send-keys" in c]
        self.assertEqual(sends[0][-2:], ["-l", "/compact " + lc.COMPACT_INSTRUCTIONS["lead"]])
        self.assertEqual(sends[1][-1], "Enter")
        self.assertIn("scion:agent", sends[0])

    def test_compact_reports_failure_without_boundary(self):
        run = FakeRunner(pane=PANE_AFTER_STEP0, boundary_after_send=False)
        self.assertFalse(lc.compact(run, "psearch--lead", "lead", NOW, sleep=lambda s: None))

    def test_compact_instructions_exact_text(self):
        self.assertTrue(lc.COMPACT_INSTRUCTIONS["lead"].startswith("Keep only what the lead needs to continue: "))
        self.assertTrue(lc.COMPACT_INSTRUCTIONS["deployer"].endswith(
            "Drop everything else: DEPLOYMENT.md and the GitHub issues are the record."))


GH_JSON_TWO_ISSUES = json.dumps([
    {"number": 55, "title": "Add facet X", "createdAt": "2026-11-09T09:00:00Z", "updatedAt": "2026-11-09T11:30:00Z",
     "comments": [{"createdAt": "2026-11-09T10:00:00Z", "body": "plan"},
                  {"createdAt": "2026-11-09T11:30:00Z", "body": "deployed"}]},
    {"number": 56, "title": "Fix Y", "createdAt": "2026-11-09T11:00:00Z", "updatedAt": "2026-11-09T11:00:00Z",
     "comments": []},
])


def make_hub_db(path):
    """Minimal hub.db with the columns the controller reads (values in the Hub's Go time format)."""
    conn = sqlite3.connect(path)
    conn.executescript("""
        CREATE TABLE projects (id TEXT, slug TEXT);
        CREATE TABLE agents (id TEXT, project_id TEXT, name TEXT, slug TEXT, activity TEXT, deleted_at TEXT);
        CREATE TABLE messages (id TEXT, project_id TEXT, sender TEXT, recipient TEXT, msg TEXT, created TEXT);
        INSERT INTO projects VALUES ('p1', 'psearch'), ('p2', 'psearch-eval');
        INSERT INTO agents VALUES ('a1', 'p1', 'lead', 'lead', 'stalled', NULL),
                                  ('a2', 'p1', 'backend', 'backend', 'completed', NULL),
                                  ('a3', 'p1', 'old-worker', 'old-worker', 'working', '2026-10-01 00:00:00 +0000 UTC'),
                                  ('a4', 'p2', 'eval-lead', 'eval-lead', 'working', NULL);
        INSERT INTO messages VALUES
          ('m0', 'p1', 'agent:lead', 'agent:backend', 'yesterday', '2026-11-08 22:00:00.5 +0000 UTC m=+10.0'),
          ('m2', 'p1', 'agent:backend', 'agent:lead', 'backend -> lead: DONE', '2026-11-09 10:20:00 +0000 UTC'),
          ('m1', 'p1', 'agent:lead', 'agent:backend', 'lead -> backend: #55',
           '2026-11-09 10:00:00.123456789 +0000 UTC m=+1979146.354969246'),
          ('m3', 'p2', 'agent:eval-lead', 'agent:factory-analyst', 'other project', '2026-11-09 10:30:00 +0000 UTC');
    """)
    conn.commit()
    conn.close()


class ReadersTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.tmp.name, "hub.db")
        make_hub_db(self.db)

    def tearDown(self):
        self.tmp.cleanup()

    def test_read_hub_from_fixture_db(self):
        snap = lc.read_hub(self.db, "psearch", since=datetime(2026, 11, 9, tzinfo=UTC))
        self.assertEqual(snap.project_id, "p1")
        self.assertEqual(snap.activity, {"lead": "stalled", "backend": "completed"})
        self.assertEqual([m.sender for m in snap.messages], ["agent:lead", "agent:backend"])
        self.assertEqual(snap.messages[0].created_at, datetime(2026, 11, 9, 10, 0, 0, 123456, tzinfo=UTC))
        self.assertEqual(snap.messages[1].body, "backend -> lead: DONE")

    def test_read_hub_missing_db_raises_signal_error(self):
        with self.assertRaises(lc.SignalError):
            lc.read_hub(os.path.join(self.tmp.name, "nonexistent.db"), "psearch", since=NOW)

    def test_read_hub_unknown_project_raises_signal_error(self):
        with self.assertRaises(lc.SignalError):
            lc.read_hub(self.db, "nope", since=NOW)

    def test_parse_issues_last_comment(self):
        issues = lc.parse_issues(GH_JSON_TWO_ISSUES)
        self.assertEqual([i.number for i in issues], [55, 56])
        self.assertEqual(issues[0].last_comment_at, datetime(2026, 11, 9, 11, 30, tzinfo=UTC))
        self.assertEqual(issues[0].updated_at, datetime(2026, 11, 9, 11, 30, tzinfo=UTC))
        self.assertIsNone(issues[1].last_comment_at)

    def test_parse_issues_invalid_json_raises(self):
        with self.assertRaises(lc.SignalError):
            lc.parse_issues("HTTP 403: API rate limit exceeded")
        with self.assertRaises(lc.SignalError):
            lc.parse_issues('{"message": "Bad credentials"}')

    def test_read_issues_command_and_error(self):
        calls = []

        def ok_runner(argv, timeout):
            calls.append(argv)
            return 0, GH_JSON_TWO_ISSUES

        self.assertEqual(len(lc.read_issues(ok_runner, "owner/repo")), 2)
        self.assertEqual(calls[0], ["docker", "exec", "-u", "scion", "psearch--lead", "gh", "issue", "list",
                                    "-R", "owner/repo", "--state", "open", "--label", "psearch", "--json",
                                    "number,title,createdAt,updatedAt,comments", "--limit", "50"])
        with self.assertRaises(lc.SignalError):
            lc.read_issues(lambda argv, timeout: (1, ""), "owner/repo")


T0 = datetime(2026, 11, 9, 12, 0, tzinfo=UTC)


def issue(number, title="Issue", created_at=T0 - timedelta(hours=2), updated_at=None, last_comment_at=None):
    return lc.Issue(number=number, title=title, created_at=created_at,
                    updated_at=updated_at or last_comment_at or created_at, last_comment_at=last_comment_at)


def msg(sender, recipient, at, body="work message"):
    return lc.Message(sender=sender, recipient=recipient, body=body, created_at=at)


def snap(now=T0 + timedelta(minutes=15), lead_idle=True, lead_last_turn_at=T0, issues=None, messages=None,
         activity=None):
    return lc.Snapshot(
        now=now,
        lead_idle=lead_idle,
        lead_last_turn_at=lead_last_turn_at,
        issues=[] if issues is None else issues,
        messages=list(messages or []),
        activity=dict(activity or {"lead": "stalled", "backend": "completed", "frontend": "completed",
                                   "reviewer": "stalled", "deployer": "stalled"}),
    )


class WakeRulesTest(unittest.TestCase):
    def test_first_run_records_issues_without_waking(self):
        wakes, state = lc.evaluate(snap(issues=[issue(55)]), {})
        self.assertEqual(wakes, [])
        self.assertTrue(state["initialized"])
        self.assertEqual(state["known_issues"], [55])
        self.assertEqual(state["open_issue_count"], 1)

    def test_missing_or_corrupt_state_behaves_as_first_run(self):
        wakes, state = lc.evaluate(snap(issues=[issue(55)]), {"garbage": 1, "r3": "corrupt"})
        self.assertEqual(wakes, [])
        self.assertEqual(state["known_issues"], [55])

    def test_first_run_with_unreadable_issues_stays_uninitialized(self):
        s_unreadable = lc.Snapshot(now=T0 + timedelta(minutes=15), lead_idle=True, lead_last_turn_at=T0,
                                   issues=None, messages=[], activity={})
        wakes, state = lc.evaluate(s_unreadable, {})
        self.assertEqual(wakes, [])
        self.assertFalse(state["initialized"])
        # Next run when gh recovers still treats existing open issues as baseline (no wake storm)
        wakes2, state2 = lc.evaluate(snap(issues=[issue(55), issue(56)]), state)
        self.assertEqual(wakes2, [])
        self.assertEqual(state2["known_issues"], [55, 56])

    def test_r1_new_issue_wakes_once(self):
        s1 = lc.evaluate(snap(issues=[issue(55)]), {})[1]
        i56 = issue(56, title="Add vector cache", created_at=T0 + timedelta(minutes=12))
        wakes, s2 = lc.evaluate(snap(now=T0 + timedelta(minutes=15), issues=[issue(55), i56]), s1)
        self.assertEqual([w.key for w in wakes], ["r1:56"])
        self.assertEqual(wakes[0].reason, 'new issue #56 "Add vector cache" is open and has no plan from you yet')
        self.assertEqual(lc.evaluate(snap(now=T0 + timedelta(minutes=20), issues=[issue(55), i56]), s2)[0], [])

    def test_r2_after_grace_wakes_then_cooldown(self):
        s = lc.evaluate(snap(now=T0 - timedelta(minutes=5), lead_last_turn_at=T0 - timedelta(minutes=20),
                             issues=[issue(55)]), {})[1]
        commented = [issue(55, last_comment_at=T0 + timedelta(minutes=11))]
        w1, s = lc.evaluate(snap(now=T0 + timedelta(minutes=15), lead_last_turn_at=T0, issues=commented), s)
        self.assertEqual([w.key for w in w1], ["r2:55"])
        self.assertIn("latest at 12:11Z", w1[0].reason)
        w2, s = lc.evaluate(snap(now=T0 + timedelta(minutes=25), lead_last_turn_at=T0, issues=commented), s)
        self.assertEqual(w2, [])
        w3, s = lc.evaluate(snap(now=T0 + timedelta(minutes=46), lead_last_turn_at=T0, issues=commented), s)
        self.assertEqual([w.key for w in w3], ["r2:55"])

    def test_r2_within_grace_does_not_wake(self):
        s = lc.evaluate(snap(now=T0 - timedelta(minutes=5), lead_last_turn_at=T0 - timedelta(minutes=20),
                             issues=[issue(55)]), {})[1]
        commented = [issue(55, last_comment_at=T0 + timedelta(minutes=9))]
        wakes, _ = lc.evaluate(snap(now=T0 + timedelta(minutes=15), lead_last_turn_at=T0, issues=commented), s)
        self.assertEqual(wakes, [])

    def test_r3_unanswered_request_wakes_after_30_min(self):
        s = lc.evaluate(snap(issues=[issue(55)]), {})[1]
        m1 = msg("agent:lead", "agent:backend", T0)
        wakes, _ = lc.evaluate(snap(now=T0 + timedelta(minutes=31), issues=[issue(55)], messages=[m1]), s)
        self.assertEqual([w.key for w in wakes], ["r3:backend"])
        self.assertEqual(wakes[0].reason, "backend has not replied for 31 min to your message at 12:00Z")

    def test_r3_not_when_agent_working(self):
        s = lc.evaluate(snap(issues=[issue(55)]), {})[1]
        m1 = msg("agent:lead", "agent:backend", T0)
        wakes, _ = lc.evaluate(snap(now=T0 + timedelta(minutes=31), issues=[issue(55)], messages=[m1],
                                    activity={"backend": "working"}), s)
        self.assertEqual(wakes, [])

    def test_r3_stops_after_two_wakes_followups_do_not_reset(self):
        s = lc.evaluate(snap(issues=[issue(55)]), {})[1]
        m1 = msg("agent:lead", "agent:backend", T0)
        w1, s = lc.evaluate(snap(now=T0 + timedelta(minutes=31), issues=[issue(55)], messages=[m1]), s)
        self.assertEqual([w.key for w in w1], ["r3:backend"])
        followup = msg("agent:lead", "agent:backend", T0 + timedelta(minutes=40), body="lead -> backend: status?")
        w2, s = lc.evaluate(snap(now=T0 + timedelta(minutes=62), issues=[issue(55)], messages=[m1, followup]), s)
        self.assertEqual([w.key for w in w2], ["r3:backend"])
        w3, s = lc.evaluate(snap(now=T0 + timedelta(minutes=93), issues=[issue(55)], messages=[m1, followup]), s)
        self.assertEqual([w.key for w in w3 if w.rule == "r3"], [])
        self.assertEqual(s["r3"]["backend"]["wakes"], 2)

    def test_r3_reply_ends_streak(self):
        s = lc.evaluate(snap(issues=[issue(55)]), {})[1]
        m1 = msg("agent:lead", "agent:backend", T0)
        _, s = lc.evaluate(snap(now=T0 + timedelta(minutes=31), issues=[issue(55)], messages=[m1]), s)
        reply = msg("agent:backend", "agent:lead", T0 + timedelta(minutes=35), body="backend -> lead: DONE")
        wakes, s = lc.evaluate(snap(now=T0 + timedelta(minutes=65), lead_last_turn_at=T0 + timedelta(minutes=36),
                                    issues=[issue(55)], messages=[m1, reply]), s)
        self.assertEqual([w.key for w in wakes if w.rule == "r3"], [])
        self.assertNotIn("backend", s["r3"])

    def test_r5_backoff_1h_2h_4h_cap(self):
        s = lc.evaluate(snap(now=T0, lead_last_turn_at=T0, issues=[issue(55)]), {})[1]
        fired_hours = []
        for hour in range(1, 13):
            wakes, s = lc.evaluate(snap(now=T0 + timedelta(hours=hour), lead_last_turn_at=T0, issues=[issue(55)]), s)
            if any(w.key == "r5" for w in wakes):
                fired_hours.append(hour)
        self.assertEqual(fired_hours, [1, 3, 7, 11])
        self.assertEqual(s["r5"]["interval_s"], 14400)

    def test_r5_resets_on_activity(self):
        s = lc.evaluate(snap(now=T0, lead_last_turn_at=T0, issues=[issue(55)]), {})[1]
        w1, s = lc.evaluate(snap(now=T0 + timedelta(hours=1), lead_last_turn_at=T0, issues=[issue(55)]), s)
        self.assertEqual([w.key for w in w1], ["r5"])
        self.assertEqual(s["r5"]["interval_s"], 7200)
        act = msg("agent:backend", "agent:lead", T0 + timedelta(hours=1, minutes=10), body="backend -> lead: DONE")
        _, s = lc.evaluate(snap(now=T0 + timedelta(hours=1, minutes=20),
                                lead_last_turn_at=T0 + timedelta(hours=1, minutes=11),
                                issues=[issue(55)], messages=[act]), s)
        self.assertEqual(s["r5"]["interval_s"], 3600)

    def test_r5_no_open_issues_no_wake(self):
        s = lc.evaluate(snap(now=T0, lead_last_turn_at=T0, issues=[]), {})[1]
        wakes, _ = lc.evaluate(snap(now=T0 + timedelta(hours=10), lead_last_turn_at=T0, issues=[]), s)
        self.assertEqual(wakes, [])

    def test_gate_messages_are_ignored(self):
        s = lc.evaluate(snap(now=T0, lead_last_turn_at=T0, issues=[issue(55)]), {})[1]
        _, s = lc.evaluate(snap(now=T0 + timedelta(hours=1), lead_last_turn_at=T0, issues=[issue(55)]), s)
        self.assertEqual(s["r5"]["interval_s"], 7200)
        gate_msg = msg("agent:backend", "agent:lead", T0 + timedelta(hours=1, minutes=5),
                       body=lc.GATE_PREFIX + "synthetic")
        req = msg("agent:lead", "agent:backend", T0 + timedelta(minutes=50))
        wakes, s = lc.evaluate(snap(now=T0 + timedelta(hours=1, minutes=25), lead_last_turn_at=T0,
                                    issues=[issue(55)], messages=[req, gate_msg]), s)
        # gate_msg neither ended the R3 streak nor reset R5's interval
        self.assertEqual([w.key for w in wakes], ["r3:backend"])
        self.assertEqual(s["r5"]["interval_s"], 7200)

    def test_heartbeat_messages_are_not_activity(self):
        s = lc.evaluate(snap(now=T0, lead_last_turn_at=T0, issues=[issue(55)]), {})[1]
        _, s = lc.evaluate(snap(now=T0 + timedelta(hours=1), lead_last_turn_at=T0, issues=[issue(55)]), s)
        hb = msg("user:admin@example.com", "agent:lead", T0 + timedelta(hours=1, minutes=15),
                 body="HEARTBEAT (every 15 min): 1) run scion list...")
        _, s = lc.evaluate(snap(now=T0 + timedelta(hours=1, minutes=20), lead_last_turn_at=T0,
                                issues=[issue(55)], messages=[hb]), s)
        self.assertEqual(s["r5"]["interval_s"], 7200)

    def test_no_wake_while_lead_working_or_recent_turn(self):
        s = lc.evaluate(snap(issues=[issue(55)]), {})[1]
        i56 = issue(56, created_at=T0 + timedelta(minutes=2))
        w_busy, s_busy = lc.evaluate(snap(now=T0 + timedelta(minutes=15), lead_idle=False,
                                          issues=[issue(55), i56]), s)
        self.assertEqual(w_busy, [])
        # And #56 was NOT swallowed while lead was busy: once idle, R1 still fires
        w_after, _ = lc.evaluate(snap(now=T0 + timedelta(minutes=20), lead_idle=True,
                                      issues=[issue(55), i56]), s_busy)
        self.assertEqual([w.key for w in w_after], ["r1:56"])
        w_recent, _ = lc.evaluate(snap(now=T0 + timedelta(minutes=4), lead_last_turn_at=T0,
                                       issues=[issue(55), i56]), s)
        self.assertEqual(w_recent, [])

    def test_issues_unreadable_only_r5_runs(self):
        s = lc.evaluate(snap(now=T0, lead_last_turn_at=T0, issues=[issue(55), issue(56)]), {})[1]
        unreadable = lc.Snapshot(now=T0 + timedelta(hours=1, minutes=5), lead_idle=None, lead_last_turn_at=T0,
                                 issues=None, messages=[], activity={})
        wakes, _ = lc.evaluate(unreadable, s)
        self.assertEqual([w.key for w in wakes], ["r5"])
        self.assertEqual(wakes[0].reason, "periodic check: 2 open issue(s), no lead turn for 65 min")


if __name__ == "__main__":
    unittest.main()
