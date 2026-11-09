# Lead Cost Controller Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A cron-driven script on the Scion Hub VM that keeps the Claude agents' context under 200k tokens and wakes
the `psearch` lead only when a deterministic rule finds work, so idle heartbeats stop costing Opus tokens.

**Architecture:** One stdlib-only Python file, `docs/scion/scripts/lead-controller.py`, split into a pure core
(parsing, ceiling decision, wake rules; unit-tested with fixtures) and thin adapters (`hub.db`, `gh` and tmux through
`docker exec`, Hub API, Cloud Logging) that are injected so tests run offline. It copies the patterns of
`factory-watchdog.py` (admin JWT, `api()`, `log_alert()`, atomic state file, `--install-cron`) without modifying it.

**Tech Stack:** Python 3 standard library only (the Hub's `/usr/bin/python3`), `unittest`, SQLite (read-only), Docker
CLI, tmux, GitHub CLI inside `psearch--lead`, `gcloud logging write`.

**Spec:** [docs/scion/specs/2026-11-06-lead-cost-controller-design.md](../scion/specs/2026-11-06-lead-cost-controller-design.md)

## Global Constraints

- Keep Claude Opus; no model, template or system-prompt changes; do not edit `factory-watchdog.py`.
- One self-contained file (`run-on-hub.sh` streams a single file to the VM); stdlib only; `from __future__ import annotations`; no import-time side effects (no env reads, no file I/O at module level).
- No real project IDs, project numbers or service-account emails in tracked files (repo guard `TestTrackedSourceContainsNoForbiddenIDPattern`); read `HUB_PROJECT_ID`, `GITHUB_REPO`, `ADMIN_EMAIL`/`PO_EMAIL` from the environment or `~/.scion/lead-controller.env`.
- Containers `psearch--lead` and `psearch--deployer`; tmux target `scion:agent`; transcripts = newest `/home/scion/.claude/projects/-workspace/*.jsonl` in the container.
- Ceiling `200_000` tokens; compaction cooldown 30 min; no compaction within 120 s of the next scheduled heartbeat.
- Idle = Hub `agents.activity` ≠ `working` AND pane shows no `esc to interrupt` AND last transcript event older than 90 s.
- Never wake the lead while it is working or within 5 min after its last turn.
- Wake prefix exactly `GATE (automated, not the PO): `; every rule ignores messages whose body starts with it.
- Rules: R1 new issue (first run records only); R2 issue activity newer than lead's last turn + 10 min, cooldown 30 min per issue; R3 team = `backend`, `frontend`, `reviewer`, `deployer`, threshold 30 min, cooldown 30 min per agent, max 2 wakes per streak; R5 ≥ 1 open issue and no lead turn for T, T = 3600 s doubling to 14400 s after no-op wakes, reset to 3600 s on activity. No PO-reminder rule.
- Fail-open for wakes (unknown lead state counts as idle for R5), fail-closed for `/compact`; 3 consecutive runs with an unreadable signal → one ERROR entry in `scion-factory-alerts`.
- Files on the VM: `~/.scion/lead-controller.py`, `~/.scion/lead-controller.env` (0600), `~/.scion/lead-controller-state.json`, `~/.scion/lead-controller.log`, kill switch `~/.scion/lead-controller.disabled`.
- Heartbeat schedule after 24 h of gate `--apply`: `0 */4 * * *`.

## Review Focus

1. **Hub timestamps in Go format** (`2026-11-06 18:00:18.665550575 +0000 UTC m=+1979146.354969246`): must parse to the right UTC instant (nanoseconds truncated, monotonic suffix ignored), never crash. → test in Task 1.
2. **Lead starts a turn between the idle check and the keystrokes:** `/compact` must not be typed into a busy pane. → test in Task 2 (re-check pane right before sending).
3. **`gh` fails or prints non-JSON** (rate limit, token expiry): R1/R2 skip, R5 still runs on the last known open-issue count, failure counted toward the alert. → tests in Tasks 3 and 4.
4. **Missing or corrupt state file** (first install, disk issue): behaves like a first run, records issues, sends no wake storm. → test in Task 4.
5. **The controller's own messages** are sent with the PO's identity: they must not count as activity, replies or requests. → test in Task 4.

---

### Task 1: Parsing and idle detection

**Files:**
- Create: `docs/scion/scripts/lead-controller.py`
- Create: `docs/scion/scripts/tests/test_lead_controller.py`

**Interfaces:**
- Produces:
  - `parse_hub_time(value: str | None) -> datetime | None` — aware UTC datetime for Go time strings and ISO-8601 (`...Z`); `None` for empty or unparsable input.
  - `@dataclass TranscriptState: last_event_at: datetime | None; last_assistant_at: datetime | None; last_context_tokens: int | None; last_compact_at: datetime | None`
  - `parse_transcript(lines: list[str]) -> TranscriptState` — context = `input_tokens + cache_creation_input_tokens + cache_read_input_tokens` of the last `assistant` event; `last_compact_at` from the last `system` event with `subtype == "compact_boundary"`; invalid JSON lines skipped.
  - `pane_state(text: str) -> str` — `"busy"` if `esc to interrupt` appears (case-insensitive), else `"idle"` if a line starts with `❯`, else `"unknown"`.
  - `is_idle(activity: str | None, pane: str, last_event_at: datetime | None, now: datetime) -> bool | None` — `None` when the pane is `unknown` or there is no transcript.

The test file loads the hyphenated script with `importlib.util.spec_from_file_location("lead_controller", <scripts dir>/lead-controller.py)`.

- [ ] **Step 1: Write the failing tests**

```python
def test_parse_hub_time_go_format(self):
    self.assertEqual(lc.parse_hub_time("2026-11-06 18:00:18.665550575 +0000 UTC m=+1979146.354969246"),
                     datetime(2026, 11, 6, 18, 0, 18, 665550, tzinfo=timezone.utc))
def test_parse_hub_time_iso_z(self):
    self.assertEqual(lc.parse_hub_time("2026-11-06T21:54:32.525Z"),
                     datetime(2026, 11, 6, 21, 54, 32, 525000, tzinfo=timezone.utc))
def test_parse_hub_time_garbage(self):
    self.assertIsNone(lc.parse_hub_time("not a time"))
def test_parse_transcript_last_context_and_compact(self):
    # fixture: assistant usage {input_tokens: 2, cache_creation_input_tokens: 12745, cache_read_input_tokens: 0}
    # at 22:00:34Z, preceded by a compact_boundary at 21:54:32Z and one invalid line
    st = lc.parse_transcript(FIXTURE_LINES)
    self.assertEqual(st.last_context_tokens, 12747)
    self.assertEqual(st.last_compact_at, datetime(2026, 11, 6, 21, 54, 32, 525000, tzinfo=timezone.utc))
def test_pane_state(self):
    self.assertEqual(lc.pane_state("✻ Thinking… (esc to interrupt)"), "busy")
    self.assertEqual(lc.pane_state(PANE_AFTER_STEP0), "idle")   # real tail captured on 2026-11-06 (has a "❯ " line)
    self.assertEqual(lc.pane_state(""), "unknown")
def test_is_idle_requires_all_three(self):
    now = datetime(2026, 11, 9, 12, 0, tzinfo=timezone.utc)
    old, recent = now - timedelta(seconds=120), now - timedelta(seconds=60)
    self.assertTrue(lc.is_idle("completed", "idle", old, now))
    self.assertFalse(lc.is_idle("working", "idle", old, now))
    self.assertFalse(lc.is_idle("completed", "busy", old, now))
    self.assertFalse(lc.is_idle("completed", "idle", recent, now))
    self.assertIsNone(lc.is_idle("completed", "unknown", old, now))
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: FAIL / ERROR (`lead-controller.py` missing or functions undefined).

- [ ] **Step 3: Implement the four functions and the dataclass in `lead-controller.py`**

`parse_hub_time`: drop a trailing ` m=...` part, map ` +0000 UTC` / `Z` to `+00:00`, truncate fractional seconds to 6 digits, then `datetime.fromisoformat`.

- [ ] **Step 4: Run the tests and see them pass**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: all Task 1 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/scion/scripts/lead-controller.py docs/scion/scripts/tests/test_lead_controller.py
git commit -m "feat(scion): lead-controller transcript, pane and idle parsing"
```

### Task 2: Context ceiling

**Files:**
- Modify: `docs/scion/scripts/lead-controller.py`
- Modify: `docs/scion/scripts/tests/test_lead_controller.py`

**Interfaces:**
- Consumes: `TranscriptState`, `parse_transcript`, `pane_state` (Task 1).
- Produces:
  - Constants `CEILING_TOKENS = 200_000`, `COMPACT_COOLDOWN_S = 1800`, `HEARTBEAT_GUARD_S = 120`, `TMUX_TARGET = "scion:agent"`.
  - `COMPACT_INSTRUCTIONS: dict[str, str]` with exactly these texts (each sent as `/compact <text>`):
    - `"lead"`: `Keep only what the lead needs to continue: open or in-flight GitHub issues and their current step, who you are waiting on and since when, standing PO rules and PO decisions not yet recorded on GitHub, the commit-slot holder, and pending escalations. Drop everything else: GitHub issues, commits and knowledge-plane/ are the record.`
    - `"deployer"`: `Keep only what the deployer needs to continue: an in-progress deploy or migration and its current step, the last deployed revision and rollback target, and pending live checks. Drop everything else: DEPLOYMENT.md and the GitHub issues are the record.`
  - `Runner = Callable[[list[str], int], tuple[int, str]]` (argv, timeout → returncode, stdout).
  - `should_compact(st: TranscriptState, idle: bool | None, now: datetime, last_compact_at: datetime | None, next_heartbeat_at: datetime | None) -> tuple[bool, str]` — `last_compact_at` is the later of the state file value and `st.last_compact_at`.
  - `compact(run: Runner, container: str, role: str, since: datetime, sleep: Callable[[float], None] = time.sleep) -> bool` — re-reads the pane and aborts if it is not `idle`; sends `docker exec -u scion <container> tmux send-keys -t scion:agent -l "/compact <text>"`, sleeps 2 s, sends `... send-keys -t scion:agent Enter`; then polls the transcript every 15 s for up to 420 s for a `compact_boundary` newer than `since`.

- [ ] **Step 1: Write the failing tests**

```python
def test_should_compact_above_ceiling_when_idle(self):
    ok, _ = lc.should_compact(st(ctx=200_001), True, NOW, None, NOW + timedelta(minutes=30))
    self.assertTrue(ok)
def test_no_compact_at_or_below_ceiling(self):
    self.assertFalse(lc.should_compact(st(ctx=200_000), True, NOW, None, None)[0])
def test_no_compact_when_busy_or_unknown(self):
    self.assertFalse(lc.should_compact(st(ctx=500_000), False, NOW, None, None)[0])
    self.assertFalse(lc.should_compact(st(ctx=500_000), None, NOW, None, None)[0])
def test_no_compact_within_cooldown(self):
    self.assertFalse(lc.should_compact(st(ctx=500_000), True, NOW, NOW - timedelta(minutes=20), None)[0])
def test_no_compact_near_heartbeat(self):
    self.assertFalse(lc.should_compact(st(ctx=500_000), True, NOW, None, NOW + timedelta(seconds=90))[0])
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
```

`FakeRunner` (in the test file) records each argv as a list and answers `tmux capture-pane` with `pane`, `ls -t ...jsonl` with a path, and `tail` with fixture lines that include a newer `compact_boundary` once a send happened (when `boundary_after_send=True`).

- [ ] **Step 2: Run the tests and see them fail**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: Task 2 tests FAIL (`should_compact` / `compact` undefined).

- [ ] **Step 3: Implement the constants, `should_compact` and `compact`**

- [ ] **Step 4: Run the tests and see them pass**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/scion/scripts/lead-controller.py docs/scion/scripts/tests/test_lead_controller.py
git commit -m "feat(scion): lead-controller context ceiling with guarded /compact"
```

### Task 3: Hub and GitHub readers

**Files:**
- Modify: `docs/scion/scripts/lead-controller.py`
- Modify: `docs/scion/scripts/tests/test_lead_controller.py`

**Interfaces:**
- Consumes: `parse_hub_time`, `Runner` (Tasks 1–2).
- Produces:
  - `class SignalError(Exception)` — raised by every reader when its source cannot be read.
  - `@dataclass Message: sender: str; recipient: str; body: str; created_at: datetime`
  - `@dataclass Issue: number: int; title: str; created_at: datetime; updated_at: datetime; last_comment_at: datetime | None`
  - `@dataclass HubSnapshot: project_id: str; activity: dict[str, str]; messages: list[Message]`
  - `read_hub(db_path: str, project_slug: str, since: datetime) -> HubSnapshot` — opens `file:<db>?mode=ro`; `activity` from non-deleted agents of the project; `messages` of the project with `created >= since`, oldest first.
  - `parse_issues(gh_json: str) -> list[Issue]` — `last_comment_at` = max `createdAt` of `comments`; raises `SignalError` on invalid JSON.
  - `read_issues(run: Runner, repo: str, label: str = "psearch") -> list[Issue]` — `docker exec -u scion psearch--lead gh issue list -R <repo> --state open --label <label> --json number,title,createdAt,updatedAt,comments --limit 50`; non-zero exit → `SignalError`.

- [ ] **Step 1: Write the failing tests**

```python
def test_read_hub_from_fixture_db(self):
    # temp sqlite: projects(id, slug); agents(project_id, name, activity, deleted_at);
    # messages(project_id, sender, recipient, msg, created) with Go-format timestamps; one row in another project
    snap = lc.read_hub(self.db, "psearch", since=datetime(2026, 11, 9, tzinfo=timezone.utc))
    self.assertEqual(snap.activity["backend"], "completed")
    self.assertEqual([m.sender for m in snap.messages], ["agent:lead", "agent:backend"])
def test_read_hub_missing_db_raises_signal_error(self):
    with self.assertRaises(lc.SignalError):
        lc.read_hub("/nonexistent/hub.db", "psearch", since=NOW)
def test_parse_issues_last_comment(self):
    issues = lc.parse_issues(GH_JSON_TWO_ISSUES)   # #55 with comments at 10:00Z and 11:30Z; #56 without comments
    self.assertEqual(issues[0].last_comment_at, datetime(2026, 11, 9, 11, 30, tzinfo=timezone.utc))
    self.assertIsNone(issues[1].last_comment_at)
def test_parse_issues_invalid_json_raises(self):
    with self.assertRaises(lc.SignalError):
        lc.parse_issues("HTTP 403: API rate limit exceeded")
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: Task 3 tests FAIL.

- [ ] **Step 3: Implement the dataclasses, `SignalError`, `read_hub`, `parse_issues`, `read_issues`**

- [ ] **Step 4: Run the tests and see them pass**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/scion/scripts/lead-controller.py docs/scion/scripts/tests/test_lead_controller.py
git commit -m "feat(scion): lead-controller hub.db and GitHub readers"
```

### Task 4: Wake rules engine

**Files:**
- Modify: `docs/scion/scripts/lead-controller.py`
- Modify: `docs/scion/scripts/tests/test_lead_controller.py`

**Interfaces:**
- Consumes: `Message`, `Issue` (Task 3).
- Produces:
  - `GATE_PREFIX = "GATE (automated, not the PO): "`, `TEAM = ("backend", "frontend", "reviewer", "deployer")`.
  - `@dataclass Snapshot: now: datetime; lead_idle: bool | None; lead_last_turn_at: datetime | None; issues: list[Issue] | None; messages: list[Message]; activity: dict[str, str]` — `issues is None` means `gh` was unreadable.
  - `@dataclass Wake: rule: str; key: str; reason: str` — keys `r1:<n>`, `r2:<n>`, `r3:<agent>`, `r5`.
  - `evaluate(snap: Snapshot, state: dict) -> tuple[list[Wake], dict]` — pure; returns a new state dict. State keys: `initialized: bool`, `known_issues: list[int]`, `open_issue_count: int`, `last_wake: dict[str, str]` (ISO), `r3: dict[str, {"streak_start": str, "wakes": int}]`, `r5: {"interval_s": int, "last_wake_at": str | None}`.
  - Reason texts: R1 `new issue #<n> "<title>" is open and has no plan from you yet`; R2 `issue #<n> has activity since your last turn (latest at <HH:MM>Z)`; R3 `<agent> has not replied for <m> min to your message at <HH:MM>Z`; R5 `periodic check: <k> open issue(s), no lead turn for <m> min`.
  - R5 no-op detection: a wake is a no-op when no non-gate message from `agent:lead` and no issue comment is newer than `r5.last_wake_at` by the next evaluation after the lead's next turn; a no-op doubles `interval_s` (max 14400); any newer non-gate message, issue update or lead message resets it to 3600.
  - Messages whose body starts with `GATE_PREFIX` or with `HEARTBEAT` (the schedule's text) never count as a request, a reply or activity.

- [ ] **Step 1: Write the failing tests**

```python
def test_first_run_records_issues_without_waking(self):
    wakes, state = lc.evaluate(snap(issues=[issue(55)]), {})
    self.assertEqual(wakes, []); self.assertEqual(state["known_issues"], [55])
def test_missing_or_corrupt_state_behaves_as_first_run(self):
    self.assertEqual(lc.evaluate(snap(issues=[issue(55)]), {"garbage": 1})[0], [])
def test_r1_new_issue_wakes_once(self):
    s1 = lc.evaluate(snap(issues=[issue(55)]), {})[1]
    wakes, s2 = lc.evaluate(snap(issues=[issue(55), issue(56)]), s1)
    self.assertEqual([w.key for w in wakes], ["r1:56"])
    self.assertEqual(lc.evaluate(snap(issues=[issue(55), issue(56)]), s2)[0], [])
def test_r2_after_grace_wakes_then_cooldown(self):
    # lead last turn 12:00Z; comment on #55 at 12:11Z; evaluations at 12:15Z (wake), 12:25Z (none), 12:46Z (wake)
def test_r2_within_grace_does_not_wake(self):
    # comment at 12:09Z, evaluation at 12:15Z -> no wake
def test_r3_unanswered_request_wakes_after_30_min(self):
    # agent:lead -> agent:backend at 12:00Z, no reply, backend "completed", now 12:31Z -> key "r3:backend"
def test_r3_not_when_agent_working(self):
    # same, backend "working" -> no wake
def test_r3_stops_after_two_wakes_followups_do_not_reset(self):
    # wakes at 12:31Z and 13:02Z; a lead->backend follow-up at 12:40Z; evaluation at 13:33Z -> no wake
def test_r3_reply_ends_streak(self):
    # agent:backend -> agent:lead at 12:20Z -> no wake at 12:31Z, r3 state for backend cleared
def test_r5_backoff_1h_2h_4h_cap(self):
    # one open issue, nothing else happens: wakes at +1h, +3h, +7h, +11h (intervals 3600, 7200, 14400, 14400)
def test_r5_resets_on_activity(self):
    # after a no-op wake (interval 7200), a non-gate agent message arrives -> interval back to 3600
def test_r5_no_open_issues_no_wake(self):
    # issues=[] for 10 h -> no wake
def test_gate_messages_are_ignored(self):
    # a user -> agent:lead message starting with GATE_PREFIX: no reply for R3, no activity for R5
def test_heartbeat_messages_are_not_activity(self):
    # a schedule message to the lead starting with "HEARTBEAT" does not reset r5.interval_s
def test_no_wake_while_lead_working_or_recent_turn(self):
    # lead_idle False -> []; lead_last_turn_at = now - 4 min -> []
def test_issues_unreadable_only_r5_runs(self):
    # state with open_issue_count 2; snap(issues=None) -> no r1/r2 wakes; r5 still fires when T elapsed
```

Each commented test is written with the exact timestamps above and asserts the list of wake keys (and the relevant
state field where named).

- [ ] **Step 2: Run the tests and see them fail**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: Task 4 tests FAIL.

- [ ] **Step 3: Implement `Snapshot`, `Wake`, the constants and `evaluate`**

Apply the global guard first (no wakes when `lead_idle is False` or the last turn is < 300 s old; `lead_idle is None` counts as idle), then R1, R2, R3, R5 in that order; one `Wake` per key per evaluation.

- [ ] **Step 4: Run the tests and see them pass**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/scion/scripts/lead-controller.py docs/scion/scripts/tests/test_lead_controller.py
git commit -m "feat(scion): lead-controller wake rules R1-R3 and R5 with backoff"
```

### Task 5: Effects — wake message, logs and blind-signal alerts

**Files:**
- Modify: `docs/scion/scripts/lead-controller.py`
- Modify: `docs/scion/scripts/tests/test_lead_controller.py`

**Interfaces:**
- Consumes: `Wake`, `GATE_PREFIX` (Task 4).
- Produces:
  - `format_wake(wakes: list[Wake]) -> str` — `GATE_PREFIX + "; ".join(reasons) + ". Follow your wake-up checklist."`
  - `ApiCall = Callable[[str, str, dict | None], tuple[int, dict]]` (same shape as the watchdog's `api()`).
  - `send_wake(api: ApiCall, project_id: str, text: str) -> bool` — `POST /api/v1/projects/<project_id>/agents/lead/message` with `{"message": text}`; `True` on 200/204.
  - `record_signal_health(state: dict, failures: list[str]) -> tuple[dict, list[str]]` — per-signal consecutive counters in `state["signal_failures"]`; returns the signals whose counter just reached 3 (alert once per outage); a successful read resets that signal's counter.
  - `log_run(path: str, entry: dict) -> None` — appends one JSON line.
  - `log_alert(event_type: str, payload: dict, severity: str, project_id: str) -> None` — same `gcloud logging write scion-factory-alerts ... --payload-type=json` call as the watchdog; no-op without `project_id`.

- [ ] **Step 1: Write the failing tests**

```python
def test_format_wake_prefix_and_reasons(self):
    text = lc.format_wake([lc.Wake("r1", "r1:56", 'new issue #56 "x" is open and has no plan from you yet')])
    self.assertTrue(text.startswith("GATE (automated, not the PO): new issue #56"))
    self.assertTrue(text.endswith(". Follow your wake-up checklist."))
def test_send_wake_posts_to_lead_endpoint(self):
    calls = []
    ok = lc.send_wake(lambda m, p, b: (calls.append((m, p, b)) or (200, {})), "pid-1", "GATE ...")
    self.assertTrue(ok)
    self.assertEqual(calls, [("POST", "/api/v1/projects/pid-1/agents/lead/message", {"message": "GATE ..."})])
def test_signal_alert_fires_on_third_consecutive_failure_only(self):
    s, a1 = lc.record_signal_health({}, ["gh"]); s, a2 = lc.record_signal_health(s, ["gh"])
    s, a3 = lc.record_signal_health(s, ["gh"]); s, a4 = lc.record_signal_health(s, ["gh"])
    self.assertEqual((a1, a2, a3, a4), ([], [], ["gh"], []))
def test_signal_counter_resets_on_success(self):
    s, _ = lc.record_signal_health({}, ["gh"]); s, _ = lc.record_signal_health(s, [])
    self.assertEqual(s["signal_failures"].get("gh", 0), 0)
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: Task 5 tests FAIL.

- [ ] **Step 3: Implement the five functions**

- [ ] **Step 4: Run the tests and see them pass**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/scion/scripts/lead-controller.py docs/scion/scripts/tests/test_lead_controller.py
git commit -m "feat(scion): lead-controller wake sender, run log and blind-signal alerts"
```

### Task 6: CLI, run loop, kill switch and cron install

**Files:**
- Modify: `docs/scion/scripts/lead-controller.py`
- Modify: `docs/scion/scripts/tests/test_lead_controller.py`

**Interfaces:**
- Consumes: everything above.
- Produces:
  - `@dataclass Deps: run: Runner; api: ApiCall; now: Callable[[], datetime]; sleep: Callable[[float], None]` (production values built in `main`).
  - `run_once(deps: Deps, cfg: dict, apply_compact: bool, apply_wake: bool) -> dict` — returns the log entry; `cfg` keys `home`, `db_path`, `state_path`, `log_path`, `repo`, `project_slug`, `hub_project_id`. Order: kill switch → readers (each `SignalError` → failure name: `hub`, `gh`, `lead_transcript`, `lead_pane`, `deployer_transcript`, `deployer_pane`; a pane in state `unknown` or a transcript without any assistant event also counts as a failure of that signal) → ceiling for `lead` and `deployer` → `evaluate` → effects (only when the matching `--apply-*` flag is set; otherwise log `would_compact` / `would_wake`; every applied compaction or wake also calls `log_alert("LEAD_CONTROLLER_COMPACT" | "LEAD_CONTROLLER_WAKE", payload, "INFO", hub_project_id)`) → `record_signal_health` + `log_alert(..., "ERROR", ...)` for new alerts → atomic state save (temp file + `os.replace`, mode 0600) → `log_run`.
  - `next_heartbeat_at` comes from `GET /api/v1/projects/<pid>/schedules` (`nextRunAt` of `psearch-lead-heartbeat`); unreadable → `None` and no compaction in that run (fail-closed).
  - `install_cron(flags: list[str], cfg: dict) -> str` — copies the script to `~/.scion/lead-controller.py` (0700), writes `~/.scion/lead-controller.env` (0600: `HUB_PROJECT_ID`, `GITHUB_REPO`, `ADMIN_EMAIL`, `PATH`), replaces any crontab line containing `lead-controller.py` with `*/5 * * * * /usr/bin/python3 <home>/lead-controller.py <flags> >> <home>/lead-controller.cron.log 2>&1`; returns the cron line.
  - `main(argv: list[str] | None = None) -> int` — flags `--apply-compact`, `--apply-wake`, `--install-cron`; loads `~/.scion/lead-controller.env` without overriding existing env.

- [ ] **Step 1: Write the failing tests**

```python
def test_kill_switch_exits_without_reading(self):
    open(os.path.join(self.home, "lead-controller.disabled"), "w").close()
    entry = lc.run_once(self.deps, self.cfg, True, True)
    self.assertEqual(entry["result"], "disabled"); self.assertEqual(self.fake.calls, [])
def test_dry_run_never_types_or_messages(self):
    # lead transcript at 250k idle + new issue in a non-first run
    entry = lc.run_once(self.deps, self.cfg, False, False)
    self.assertIn("lead", entry["would_compact"]); self.assertTrue(entry["would_wake"])
    self.assertFalse(any("send-keys" in c for c in self.fake.calls)); self.assertEqual(self.api_posts, [])
def test_apply_wake_sends_one_message_for_multiple_wakes(self):
    # R1 and R3 both fire -> exactly one POST whose body contains both reasons
def test_unreadable_schedule_blocks_compaction(self):
    # schedules endpoint returns 500 -> no send-keys even with --apply-compact
def test_state_saved_atomically_with_0600(self):
    lc.run_once(self.deps, self.cfg, False, False)
    self.assertEqual(oct(os.stat(self.cfg["state_path"]).st_mode & 0o777), "0o600")
def test_install_cron_replaces_existing_line(self):
    # fake crontab with an old lead-controller line + the watchdog line -> one new lead-controller line, watchdog kept
```

- [ ] **Step 2: Run the tests and see them fail**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: Task 6 tests FAIL.

- [ ] **Step 3: Implement `Deps`, `run_once`, `install_cron`, `main`**

Copy `mint_admin_jwt`/`api` from `factory-watchdog.py` for the production `ApiCall`; production `Runner` wraps `subprocess.run`.

- [ ] **Step 4: Run the tests and see them pass**

Run: `python3 -m unittest discover -s docs/scion/scripts/tests -v`
Expected: all tests PASS.

- [ ] **Step 5: Run the repo's forbidden-ID guard on the new files**

Run: `cd src/psearch/serving && FORBIDDEN_ID_PATTERN='<pattern built from config/factory.env HUB_PROJECT|SANDBOX_PROJECT|PO_EMAIL>' go test ./internal/config/ -run TestTrackedSourceContainsNoForbiddenIDPattern -v` (after `git add` of the new files)
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add docs/scion/scripts/lead-controller.py docs/scion/scripts/tests/test_lead_controller.py
git commit -m "feat(scion): lead-controller CLI, kill switch and cron install"
```

### Task 7: Ship to the Hub — ceiling live, gate in dry-run

**Files:**
- Modify: `docs/scion/operations-runbook.md` (new section "Lead cost controller": purpose, flags, files, kill switch, rollback, how to read the log)
- Modify: `docs/scion/scripts/README.md` (one entry for `lead-controller.py`)
- Modify: `docs/scion/README.md` (one row in the Status table)

- [ ] **Step 1: Check the Hub's Python and run one dry-run pass**

Run: `docs/scion/scripts/run-on-hub.sh docs/scion/scripts/lead-controller.py`
Expected: a JSON entry with `failures: []`, the lead's and deployer's context sizes, `would_compact` / `would_wake` lists; no keystrokes, no messages.

- [ ] **Step 2: Install the cron with the ceiling applied and the gate in dry-run**

Run: `docs/scion/scripts/run-on-hub.sh docs/scion/scripts/lead-controller.py --install-cron --apply-compact`
Expected: the printed cron line ends with `--apply-compact >> .../lead-controller.cron.log 2>&1`; `crontab -l` on the Hub still shows the watchdog, sync and backup lines.

- [ ] **Step 3: Verify the ceiling acts**

Expected within 30 min of the lead passing 200k: a `COMPACT` entry in `~/.scion/lead-controller.log`, a matching INFO entry in `scion-factory-alerts`, and a `compact_boundary` with `trigger: manual` in the lead's transcript; the next heartbeat's first call writes < 30k tokens. The log gets one new line every 5 min.

- [ ] **Step 4: Write the runbook, scripts README and Status entries, then commit**

```bash
git add docs/scion/operations-runbook.md docs/scion/scripts/README.md docs/scion/README.md
git commit -m "docs(scion): operate the lead cost controller"
```

### Task 8: Enable the gate and move the heartbeat to every 4 h

- [ ] **Step 1: Compare 24 h of dry-run decisions with what happened**

For each `would_wake` in the log, find the lead's next prompt in its transcript; for each Hub message to the lead or new issue, check a matching decision or a reason it was not needed. Expected: no missed wake; false wakes listed in the issue or PR description.

- [ ] **Step 2: Turn the gate on**

Run: `docs/scion/scripts/run-on-hub.sh docs/scion/scripts/lead-controller.py --install-cron --apply-compact --apply-wake`
Expected: the cron line now has both flags.

- [ ] **Step 3: Live drills**

(1) Open a test issue labeled `psearch` without messaging the lead → `WAKE r1:<n>` in the log and a plan comment from the lead within 10 min (then close the test issue). (2) `touch ~/.scion/lead-controller.disabled` → next run logs `disabled`; remove the file. The spec's drills for R3 (unanswered request) and R5 (worker stuck in `working`) are covered by the Task 4 tests and the Step 1 comparison; the rollback commands are the ones written in the runbook in Task 7.

- [ ] **Step 4: After 24 h with the gate on, switch the heartbeat**

Run: `docs/scion/scripts/run-on-hub.sh docs/scion/scripts/scion-admin.py schedule update --project psearch --name psearch-lead-heartbeat --cron "0 */4 * * *"`
Expected: `schedule list` shows `cron=0 */4 * * *` and `status=active`; the watchdog log still says `Check 2 (Heartbeat guard): OK`.

- [ ] **Step 5: Verify the cost on the next idle day**

Query Cloud Monitoring for the lead (`gen_ai.api.calls` with `model=claude-opus-5-5`, `scion.usage.tokens` with `token_type=cache_write`) for a 24 h window with no open issues.
Expected: ≤ 20 Opus calls and < 1M cache-write tokens (baseline before step 0: ~200 calls and 27–87M).

- [ ] **Step 6: Record the result**

Add the before/after numbers and the dry-run comparison to `docs/scion/lessons-learned.md` as a new lesson, then commit:

```bash
git add docs/scion/lessons-learned.md
git commit -m "docs(scion): lesson on idle heartbeat cost and the lead cost controller"
```
