# Design: cut the `psearch` lead's Claude Opus cost (activity gate + context ceiling)

- **Status:** Approved by the PO on 2026-11-09 (decisions in §10). Step 0 of the rollout (§6) is done; the
  implementation plan is [docs/plans/2026-11-09-lead-cost-controller.md](../../plans/2026-11-09-lead-cost-controller.md).
- **Scope:** the `psearch` build factory on the self-hosted Scion Hub (`HUB_PROJECT`), Claude Code agents `lead` and
  `deployer`. The eval factory (`psearch-eval`, Gemini) is out of scope.
- **Hard constraints from the PO:** keep Claude Opus (`claude-opus-5-5`); keep the factory's quality.

## 1. Why: what we measured

Sources: Cloud Monitoring (`workload.googleapis.com/scion.usage.tokens`, `gen_ai.api.calls`) and the Claude Code
transcripts inside `psearch--lead`, 2026-10-25 → 2026-11-06.

- **~98% of the lead's Opus spend comes from idle heartbeats.** The `psearch-lead-heartbeat` schedule (`*/15`, 2,425
  runs) wakes the lead 96×/day. With nothing to do, each wake is 2 Opus calls and ~190 output tokens.
- **Every heartbeat rewrites the whole conversation to the prompt cache.** The cache TTL is 5 min
  (`ephemeral_5m` = 100% of writes), the heartbeat interval is 15 min, so the first call of every heartbeat is a full
  cache write (1.25× input price). Example at 21:45Z: call 1 wrote 764,348 tokens, call 2 read 764,348 tokens.
- **The context only resets near the 1M window.** One session since 2026-10-15; each idle heartbeat adds ~629 tokens
  (~60k/day). The only automatic compaction was at 964,298 tokens (2026-10-30 11:01Z, down to 4,509).
- **Totals (13 days):** lead ≈ 1,008M input-token-equivalents (eq), deployer ≈ 11M eq. Work turns were ~1–2% of the lead.
- **Validated fix of the context size (step 0, 2026-11-06):** a manual `/compact` at 21:53Z took the lead from 764,580
  to 2,326 tokens. The next heartbeat (22:00Z) wrote 12,745 + 13,059 tokens: ≈ 34k eq instead of ≈ 1.03M eq, so
  ~30× cheaper per heartbeat. That second call missing the cache was a one-off: from 2026-11-07 on, the second call of
  each heartbeat reads the cache again.
- **Regrowth after step 0, as predicted:** 18k (2026-11-07 00:00Z) → 78k → 139k → 183k (2026-11-09 17:00Z), i.e.
  ~60k tokens/day. Without a ceiling the lead goes back to ~$500/day within about two weeks.

**Pricing model used below.** Weights relative to the base input price: input 1×, 5-min cache write 1.25×, 1-h cache
write 2×, cache read 0.1×, output 5×. USD figures use a **reference** base price of $5/MTok (Opus 4.5 list price);
the real Opus 5.5 SKU price must be read in Cloud Billing. Ratios do not depend on the price.

## 2. Goals, non-goals, anti-goals

- **Goals:** idle cost close to zero; every remaining Opus turn runs on a small context; the lead notices new work
  within ≤ 15 min and an unanswered request within ≤ 45 min (as today).
- **Non-goals (v1):** changing the model; changing agent templates or system prompts; the v2 per-issue architecture
  ([factory-orchestration-v2-design.md](factory-orchestration-v2-design.md)); the eval factory.
- **Anti-goals:** a new issue or a lost handoff that goes unnoticed for hours; more lead wakes than today; any change
  that the watchdog (`factory-watchdog.py`) silently reverts.

## 3. Efficiency review: options compared

Idle-day cost model for the lead: **cost/day = wakes/day × cost per wake**. With a 5-min cache a wake costs
≈ 1.35 × context (one full write + one full read). With a 1-h cache and wakes < 60 min apart, a wake costs
≈ 0.2 × context + ~2k (two reads + small writes and output).

| Option | Idle day | Work day | Effort | Depends on / risk |
|---|---|---|---|---|
| **S0 Today** (`*/15`, 5-min cache, compaction at ~964k) | 62–98M eq (~$310–490) | 65–90M eq (~$320–440) | — | — |
| **S1** One-time `/compact` | ~3M eq on day 1, back to ~56M eq (~$280) by day 7 | same as S0 after regrowth | trivial | regrows ~60k tokens/day |
| **S2** Context ceiling at 200k only | ~14M eq (~$69) | ~15M eq (~$75) | small | none |
| **S3** S2 + `*/30` + 1-h cache | ~1.1M eq (~$6); ~6.9M eq (~$35) if Vertex ignores the 1-h TTL | ~2–8M eq (~$10–40) | config + test | 1-h TTL support on Vertex unknown; reaction 30 min |
| **S4** S2 + **activity gate** + 4-hourly safety-net heartbeat | 0.1–0.4M eq (~$0.6–2) with no open issue; ≤ ~0.7M eq (~$3.5) with open issues and no movement | ~1–3M eq (~$5–15) | script + cron + tests | wake-rule gaps (mitigated, §5.3, §5.6) |
| S5 Lead hibernates when the backlog is empty | ~0 | like S4 | medium | watchdog Check 3 restarts stopped agents and wakes suspended ones; resume uses `--continue` |
| S6 v2: fresh session per issue | ~0 | lowest | large | redesign of the factory |

Assumptions: average context over the steady-state sawtooth ≈ 107k for a 200k ceiling (15k → 200k); with the gate,
the idle context stays at 15–50k because idle wakes are rare; work-only cost scales with context (measured
2.5–5M eq/day at 500–650k).

**Conclusion.**
1. **S4 is the most efficient** and has no external dependency: it removes the wakes that cause ~98% of the spend,
   and the ceiling keeps the remaining turns small. It also checks every 5 min and wakes the lead with a reason.
2. **S2 is required in every serious option** (S3, S4): without a ceiling, each remaining turn still pays for up to
   ~964k tokens.
3. **The 1-h cache is not part of v1.** With the gate, wakes are often more than 1 h apart, so 1-h writes (2×) would
   rarely be read again. Revisit after a week of data if work-day waits show many cold writes.
4. **Heartbeat cadence stops mattering for cost** once the gate exists; the schedule becomes an independent safety net.
5. S5 fights the watchdog; S6 stays the long-term direction.

## 4. Decision

Build **S4**: a Hub-side `lead-controller` that (a) keeps the Claude agents' context under a ceiling and (b) wakes
the lead only when a deterministic rule says there is something to do. Ship it in steps (§6), ceiling first.

## 5. Design

### 5.1 Components

- **`docs/scion/scripts/lead-controller.py`** (new). Runs on the Hub VM as the Hub OS user, every 5 min by cron.
  Dry-run by default; `--apply` acts; `--install-cron` installs it like `factory-watchdog.py --install-cron`. It is a
  separate script so it does not collide with other work on `factory-watchdog.py`. It reuses the same patterns:
  read-only `hub.db`, admin JWT minted from `hub.env` for the Hub API, `log_alert` to Cloud Logging.
- **State:** `~/.scion/lead-controller-state.json` (known issues, last wake per rule/agent, R3 streaks, R5 backoff,
  last compaction per agent, consecutive signal failures).
- **Log:** one JSON line per run in `~/.scion/lead-controller.log` (signals read, decisions, would-wake in dry-run);
  WAKE and COMPACT events also go to Cloud Logging (`scion-factory-alerts`, severity INFO).
- **Kill switch:** if `~/.scion/lead-controller.disabled` exists, the script exits at once.

### 5.2 Signals (all read without an LLM)

| Signal | Source |
|---|---|
| Agent idle | Hub `agents.activity` ≠ `working` (values seen: `working`, `completed`, `stalled`), no `esc to interrupt` in the tmux pane, last transcript event > 90 s old |
| Last context size | `usage` of the last assistant event in the newest `~/.claude/projects/-workspace/*.jsonl` of the container |
| Lead's last turn end | timestamp of the last assistant event in the lead's transcript |
| Open issues + activity | `docker exec -u scion psearch--lead gh issue list -R <repo> --state open --label psearch --json number,title,createdAt,updatedAt,comments` (uses the token already injected in the lead; no new IAM) |
| Requests and replies | Hub `messages` (`sender`/`recipient` = `agent:<name>` or `user:<email>`, `created`) |

The tmux target is `scion:agent` (window 0, the same pane as `scion:0.0`); it was used for the step-0 `/compact`.
Messages sent by the controller go out with the PO's identity (admin JWT), so every rule ignores messages whose
body starts with `GATE (automated, not the PO):`.

### 5.3 Wake rules (v1)

The controller never wakes the lead while it is working or within 5 min after its last turn (it is active, and
delivered messages already wake it). Each wake is one Hub API message to `lead` that starts with
`GATE (automated, not the PO):`, states the reason and ends with "follow your wake-up checklist".

| Rule | Condition | Cooldown / stop |
|---|---|---|
| **R1 New issue** | an open `psearch` issue not in the known-issues state (the first run only records, never wakes) | none |
| **R2 Unattended activity** | an open issue has a comment or update newer than the lead's last turn end + 10 min grace | 30 min per issue |
| **R3 Unanswered request** | X (backend, frontend, reviewer, deployer) has not sent anything to the lead since the first lead → X message after X's last reply; X is not working; that message is older than 30 min; ≥ 1 open issue | 30 min per X; at most 2 wakes per streak (follow-up pings do not start a new streak); a reply from X ends the streak |
| **R5 Safety wake** | ≥ 1 open issue and no lead turn for T | T starts at 1 h and doubles after each wake in which the lead sent nothing (no message, no issue comment), up to 4 h; any new activity resets T to 1 h |

R5 is the catch-all for stalls the other rules cannot see: the lead believes it sent a handoff but never ran
`scion message`; a worker shows `working` forever; R3 has used its two wakes. "≥ 1 open issue" is enough as a
condition because the backoff bounds the cost while issues only wait for PO acceptance.

No PO-reminder rule (the earlier R4) in v1: the PO accepts by closing issues and often answers on GitHub, where
authorship cannot be detected (all agents post with the PO's token), so a reminder rule would wake the lead and email
the PO (watchdog Check 4 alerts on every lead → PO message) every 2 h while the factory is idle. Reminders happen when
the lead is woken by R5 or the schedule. Agent crashes are not a rule either: watchdog Check 3 already restarts the
agent and messages the lead.

### 5.4 Context ceiling (lead and deployer)

When an agent is idle, its last context is above **200k tokens** (≈ 20% of the 1M window), its last compaction is
more than 30 min old, and no scheduled heartbeat is due within 2 min, the controller types `/compact <instructions>`
into its tmux pane and checks for a new `compact_boundary` in the transcript. `scion message` cannot do this: Scion
wraps message bodies in a JSON envelope, so slash commands never run. `scion stop`/`resume` is not a reset either:
the Claude harness config sets `resume_flag: "--continue"`.

- **Lead instructions** (as used in step 0): keep open or in-flight issues and their current step, who it waits on and
  since when, standing PO rules and decisions not yet on GitHub, the commit-slot holder, pending escalations; drop the
  rest.
- **Deployer instructions:** keep an in-progress deploy or migration and its step, the last deployed revision and
  rollback target, pending live checks; drop the rest (`DEPLOYMENT.md` and the issues are the record).
- `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` (supported by the installed Claude Code 2.1.287) is an optional backstop at a
  higher value (e.g. 40) at the next agent restart; it is not needed for v1.

### 5.5 Heartbeat schedule (independent safety net)

After the gate has run with `--apply` for 24 h: `scion-admin.py schedule update --project psearch --name
psearch-lead-heartbeat --cron "0 */4 * * *"`. Every 4 h (not daily) so that a dead or blind controller degrades to
"the lead looks every 4 h" without depending on the controller. The schedule stays `active`, so watchdog Check 2
(which checks status, not cron) does not revert it.

### 5.6 Safety, blind signals and rollback

- **Fail-open for wakes, fail-closed for typing.** If a signal cannot be read (transcript missing, unknown pane text,
  `gh` or `hub.db` error), the controller treats the lead as idle for R5 and never sends `/compact`.
- **Alerting without new infrastructure.** After 3 consecutive runs with an unreadable signal, the controller writes
  an ERROR entry to `scion-factory-alerts`; the existing alert policy ("Log match: scion-factory-alerts
  severity >= ERROR") emails the PO. Each run also checks its own assumptions (transcript path, pane prompt marker,
  `compact_boundary` fields) so that a Claude Code update that changes them raises the same alert.
- Missed work is bounded by: PO and agent messages still wake the lead at once; R1–R3 and R5; the 4-hourly schedule.
- **Rollback (minutes):** create the kill-switch file and set the heartbeat cron back to `*/15 * * * *`.

### 5.7 Dependencies on the rest of the harness

- Watchdog Check 2 must keep checking status only; a cron guard there would undo §5.5.
- The installed watchdog on the Hub (`~/.scion/factory-watchdog.py`) is not byte-identical to the repo copy: read the
  installed one before relying on its behavior.
- The Claude harness config on the Hub is unchanged since 2026-10-14 (`resume_flag: "--continue"`, launch in tmux
  session `scion`, window `agent`); the image ships Claude Code 2.1.287.
- No commits under `docs/scion/` since 2026-10-29.

## 6. Rollout

0. **Done 2026-11-06 21:53Z:** one-time manual `/compact` of the lead (approved by the PO); result in §1.
1. Ship the controller with the ceiling in `--apply` and the gate in dry-run (logs "would wake") for 24 h.
2. Compare the dry-run log with what actually happened (lead prompts and turns in the transcript, Hub messages):
   no missed wake; false wakes counted.
3. Enable the gate (`--apply`), then switch the heartbeat to every 4 h (§5.5).
4. After one week: compare cost and quality metrics (§7); decide on phase 2 (the controller pings silent workers
   itself before waking the lead) and on a 1-h cache test.

## 7. Verification

- **Cost (Cloud Monitoring, same queries as the diagnosis):** lead Opus calls on an idle day ≤ 20 (today ~200);
  lead `cache_write` on an idle day < 1M tokens (today 27–87M); weekly check of the Opus 5.5 SKU in Cloud Billing.
- **Quality drills:** (1) open a test issue labeled `psearch` without messaging the lead → plan comment within
  10 min; (2) a lead request to an idle agent that never replies → gate wake within 35 min; (3) a worker kept in
  `working` with no progress → R5 wake within T; (4) kill switch on → the 4-hourly schedule still wakes the lead;
  (5) rollback → `*/15` heartbeats resume.
- **Controller health:** one log line every 5 min; every WAKE or COMPACT has a matching Cloud Logging entry; an
  unreadable signal for 3 runs produces a PO email.
- **Tests:** unit tests for the rule engine (R1–R3, R5 backoff, message filtering) and the ceiling decision with
  fixture data (no Hub needed).

## 8. Left to the builder

Code structure, JSON log schema, test framework, cron installation details (mirror the watchdog), transcript and
`gh` parsing details, exact wake-message wording within the rules above, the dry-run comparison script.

## 9. Calls made by the author (for PO review)

- Ceiling at 200k tokens; tunable.
- Safety-net schedule every 4 h instead of daily, so it does not depend on the controller.
- No PO-reminder rule in v1 (reminders ride on R5 and the schedule; avoids an email to the PO every 2 h).
- R3 stops after 2 wakes per streak; follow-up pings do not reset it.
- Fail-open for wakes, fail-closed for `/compact`; ERROR alerts through the existing `scion-factory-alerts` policy.
- Separate script instead of a new watchdog check; GitHub read through the lead container's `gh` (no new IAM).
- No 1-h cache, no template or system-prompt changes in v1.
- Controller-driven `/compact` with role instructions instead of relying on `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`.

## 10. PO decisions (2026-11-09)

1. **R5 latency:** 1 h with backoff to 4 h (the recommended option, ~0.7M eq/day, ~$3.5/day while issues are open
   and nothing moves). Rejected: every 30 min fixed (~$16/day); no R5 (~$2/day).
2. **Other harness work:** the PO confirmed the work can proceed; no conflicting change is expected in watchdog
   Check 2, the Claude harness launch/resume, or the Claude Code version.
