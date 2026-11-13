"""Unit tests for docs/scion/scripts/trajectory-export.py (stdlib unittest; no Hub, Docker or network needed).

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
from datetime import datetime, timezone

SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "trajectory-export.py"
_spec = importlib.util.spec_from_file_location("trajectory_export", SCRIPT)
te = importlib.util.module_from_spec(_spec)
sys.modules[_spec.name] = te
_spec.loader.exec_module(te)

CLAUDE = te.CLAUDE_DIR + "/-workspace/abc.jsonl"
AGY_FULL = te.AGY_DIR + "/conv1/.system_generated/logs/transcript_full.jsonl"
AGY_SHORT = te.AGY_DIR + "/conv1/.system_generated/logs/transcript.jsonl"
AGY_ONLY_SHORT = te.AGY_DIR + "/conv2/.system_generated/logs/transcript.jsonl"

CLAUDE_LINES = [
    json.dumps({"type": "user", "timestamp": "2026-11-09T12:00:00.000Z", "message": {"role": "user"}}),
    "{truncated",
    json.dumps({"type": "assistant", "timestamp": "2026-11-09T12:00:05.000Z", "message": {"role": "assistant"}}),
]
AGY_LINE = json.dumps({"type": "USER_INPUT", "source": "USER_EXPLICIT", "created_at": "2026-11-09T13:00:00Z"})


class PureTest(unittest.TestCase):
    def test_classify(self):
        self.assertEqual(te.classify(CLAUDE)["harness"], "claude")
        self.assertEqual(te.classify(CLAUDE)["session_id"], "abc")
        info = te.classify(AGY_FULL)
        self.assertEqual((info["harness"], info["session_id"], info["kind"]),
                         ("antigravity", "conv1", "antigravity_transcript_full"))
        self.assertIsNone(te.classify(te.AGY_DIR + "/conv1/scratch/notes.jsonl"))
        self.assertIsNone(te.classify("/home/scion/other.jsonl"))

    def test_select_sources_prefers_full_antigravity_transcript(self):
        got = te.select_sources([(10, AGY_SHORT), (20, AGY_FULL), (5, AGY_ONLY_SHORT), (7, CLAUDE),
                                 (1, "/home/scion/x.jsonl")])
        self.assertEqual(sorted(s["path"] for s in got), sorted([AGY_FULL, AGY_ONLY_SHORT, CLAUDE]))

    def test_split_complete_leaves_partial_last_line(self):
        self.assertEqual(te.split_complete(b"a\nb\nc"), ([b"a", b"b"], 4))
        self.assertEqual(te.split_complete(b"no newline"), ([], 0))

    def test_plan_read_resumes_or_restarts(self):
        entry = {"offset": 100, "lines": 3, "head": te.head_hash(b"abc"), "head_len": 3}
        self.assertEqual(te.plan_read(entry, 150, b"abc"), (100, 3))
        self.assertEqual(te.plan_read(entry, 150, b"abcdef"), (100, 3))   # a short file that only grew
        self.assertEqual(te.plan_read(entry, 50, b"abc"), (0, 0))         # file shrank
        self.assertEqual(te.plan_read(entry, 150, b"xbcdef"), (0, 0))     # start of the file changed
        self.assertEqual(te.plan_read(None, 150, b"abc"), (0, 0))

    def test_make_rows_keeps_record_and_counts_invalid(self):
        meta = {"harness": "claude", "project_slug": "p", "agent_name": "lead", "path": CLAUDE,
                "kind": "claude_jsonl", "session_id": "abc"}
        rows, invalid = te.make_rows(meta, [line.encode() for line in CLAUDE_LINES] + [b""], 10, "2026-11-09T12:10:00Z")
        self.assertEqual(invalid, 1)
        self.assertEqual([r["seq"] for r in rows], [11, 13])
        self.assertEqual((rows[1]["event_type"], rows[1]["role"], rows[1]["event_ts"]),
                         ("assistant", "assistant", "2026-11-09T12:00:05.000Z"))
        self.assertEqual(rows[0]["raw"], json.loads(CLAUDE_LINES[0]))  # an object, not text

    def test_event_fields_antigravity_and_bad_timestamp(self):
        self.assertEqual(te.event_fields("antigravity", json.loads(AGY_LINE)),
                         ("2026-11-09T13:00:00Z", "USER_INPUT", "USER_EXPLICIT"))
        self.assertEqual(te.event_fields("claude", {"timestamp": "yesterday", "type": "user"}), (None, "user", None))
        self.assertEqual(te.event_fields("claude", ["a", "list"]), (None, None, None))

    def test_db_row_to_event_opencode_and_hermes(self):
        meta = {"project_slug": "p", "agent_name": "backend"}
        oc = te.DB_SOURCES[1]
        ev = te.db_row_to_event(meta, oc, {"_rowid": 7, "id": "prt_1", "session_id": "ses_1",
                                           "time_created": 1791568679799, "time_updated": 1791568679803,
                                           "data": json.dumps({"type": "text", "text": "hi"})}, "2026-11-09T14:00:00Z")
        self.assertEqual((ev["harness"], ev["seq"], ev["session_id"], ev["event_type"]), ("opencode", 7, "ses_1", "text"))
        self.assertEqual(ev["raw"]["data"], {"type": "text", "text": "hi"})
        self.assertTrue(ev["event_ts"].startswith("2026-10-09T"))
        hm = te.DB_SOURCES[2]
        ev = te.db_row_to_event(meta, hm, {"_rowid": 3, "id": 3, "session_id": "s", "role": "assistant",
                                           "content": "FAIL", "tool_calls": "not json", "timestamp": 1791568653.4},
                                "2026-11-09T14:00:00Z")
        self.assertEqual((ev["harness"], ev["role"], ev["event_type"], ev["raw"]["tool_calls"]),
                         ("hermes", "assistant", "messages", "not json"))

    def test_job_id_is_deterministic(self):
        self.assertEqual(te.job_id("c:/p", 0, 10), te.job_id("c:/p", 0, 10))
        self.assertNotEqual(te.job_id("c:/p", 0, 10), te.job_id("c:/p", 10, 20))
        self.assertRegex(te.job_id("c:/p", 0, 10), r"^[A-Za-z0-9_]+$")


class FakeRunner:
    """Stands in for docker and bq: serves files per container and records every load."""

    def __init__(self, files):
        self.files = files            # {container: {path: bytes}}
        self.loads = []               # (table, job id, rows)
        self.jobs = set()

    def __call__(self, argv, timeout):
        if argv[:2] == ["docker", "ps"]:
            return 0, "\n".join(list(self.files) + ["unrelated"]).encode()
        if argv[0] == "bq":
            jid = next(a.split("=", 1)[1] for a in argv if a.startswith("--job_id="))
            if jid in self.jobs:
                return 1, b"BigQuery error in load operation: Already Exists: Job x"
            self.jobs.add(jid)
            with open(argv[-2], encoding="utf-8") as f:
                self.loads.append((argv[-3], jid, [json.loads(line) for line in f]))
            return 0, b""
        container, cmd = argv[4], argv[5]
        files = self.files[container]
        if cmd == "find":
            hits = [f"{len(data)}\t{path}" for path, data in files.items() if path.startswith(argv[6] + "/")]
            return (0, "\n".join(hits).encode()) if hits else (1, b"")
        if cmd == "head":
            return 0, files[argv[-1]][:int(argv[7])]
        if cmd == "dd":
            opts = dict(a.split("=", 1) for a in argv[6:] if "=" in a)
            start = int(opts["skip"])
            return 0, files[opts["if"]][start:start + int(opts["count"])]
        return 1, b""


def make_hub_db(path):
    conn = sqlite3.connect(path)
    conn.executescript("""
        CREATE TABLE projects (id TEXT, slug TEXT);
        CREATE TABLE messages (id TEXT, project_id TEXT, sender TEXT, recipient TEXT, msg TEXT, created TEXT);
        INSERT INTO projects VALUES ('p1', 'psearch'), ('p9', 'other');
        INSERT INTO messages VALUES ('m1', 'p1', 'agent:lead', 'agent:backend', 'do #55', '2026-11-09 10:00:00');
        INSERT INTO messages VALUES ('m2', 'p9', 'agent:x', 'agent:y', 'elsewhere', '2026-11-09 10:01:00');
        INSERT INTO messages VALUES ('m3', 'p1', 'agent:backend', 'agent:lead', 'DONE', '2026-11-09 10:05:00');
    """)
    conn.commit()
    conn.close()


class RunOnceTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.tmp.name, "hub.db")
        make_hub_db(self.db)
        self.claude = ("\n".join(CLAUDE_LINES) + "\n").encode()
        self.fake = FakeRunner({
            "psearch--lead": {CLAUDE: self.claude},
            "psearch--frontend": {AGY_FULL: (AGY_LINE + "\n").encode(), AGY_SHORT: (AGY_LINE + "\n").encode()},
        })
        self.cfg = {
            "db_path": self.db, "cursor_path": os.path.join(self.tmp.name, "cursor.json"),
            "projects": ["psearch"], "bq_project": "my-proj", "dataset": "ds",
            "events_table": "agent_transcript_events", "messages_table": "hub_messages",
        }
        self.deps = te.Deps(run=self.fake, now=lambda: datetime(2026, 11, 9, 14, 0, tzinfo=timezone.utc))

    def tearDown(self):
        self.tmp.cleanup()

    def test_dry_run_counts_and_changes_nothing(self):
        out = te.run_once(self.deps, self.cfg, apply=False)
        self.assertEqual(out["result"], "ok")
        self.assertEqual(out["agents"]["psearch--lead"]["records"], 2)
        self.assertEqual(out["agents"]["psearch--lead"]["invalid_lines"], 1)
        self.assertEqual(out["agents"]["psearch--frontend"]["files"], 1)
        self.assertEqual(out["hub_messages"], 2)
        self.assertEqual(self.fake.loads, [])
        self.assertFalse(os.path.exists(self.cfg["cursor_path"]))

    def test_apply_loads_once_then_only_new_lines(self):
        out = te.run_once(self.deps, self.cfg, apply=True)
        self.assertEqual(out["loaded_rows"], 3)
        tables = sorted(t for t, _, _ in self.fake.loads)
        self.assertEqual(tables, ["ds.agent_transcript_events", "ds.agent_transcript_events", "ds.hub_messages"])
        self.assertEqual(oct(os.stat(self.cfg["cursor_path"]).st_mode & 0o777), "0o600")

        again = te.run_once(self.deps, self.cfg, apply=True)
        self.assertEqual((again["records"], again["hub_messages"]), (0, 0))
        self.assertEqual(len(self.fake.loads), 3)

        extra = json.dumps({"type": "user", "timestamp": "2026-11-09T12:01:00.000Z"}) + "\n"
        self.fake.files["psearch--lead"][CLAUDE] = self.claude + extra.encode() + b'{"type": "partial'
        more = te.run_once(self.deps, self.cfg, apply=True)
        self.assertEqual(more["loaded_rows"], 1)
        self.assertEqual(self.fake.loads[-1][2][0]["seq"], 4)

    def test_repeated_job_id_is_not_an_error(self):
        te.run_once(self.deps, self.cfg, apply=True)
        os.unlink(self.cfg["cursor_path"])  # crash after the load, before the cursor was saved
        out = te.run_once(self.deps, self.cfg, apply=True)
        self.assertEqual(out["failures"], [])
        self.assertEqual(len(self.fake.loads), 3)

    def test_rewritten_file_is_read_again_from_the_start(self):
        te.run_once(self.deps, self.cfg, apply=True)
        self.fake.files["psearch--lead"][CLAUDE] = (json.dumps({"type": "summary"}) + "\n").encode()
        out = te.run_once(self.deps, self.cfg, apply=True)
        self.assertEqual(out["agents"]["psearch--lead"]["records"], 1)
        self.assertEqual(self.fake.loads[-1][2][0]["seq"], 1)

    def test_unreadable_hub_db_is_reported(self):
        self.cfg["db_path"] = os.path.join(self.tmp.name, "missing.db")
        out = te.run_once(self.deps, self.cfg, apply=False)
        self.assertEqual(out["result"], "partial")
        self.assertEqual(len(out["failures"]), 1)


if __name__ == "__main__":
    unittest.main()
