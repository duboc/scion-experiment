# Design: export every agent's transcript to BigQuery

- **Status:** Draft for PO review (2026-11-12). Nothing is built or installed yet.
- **Scope:** the ten agents of the `psearch` and `psearch-eval` factories on the self-hosted Scion Hub, plus the Hub's
  message table. Destination: dataset `${BQ_DATASET}` in `${SANDBOX_PROJECT}`.
- **Tracks:** issue #64 (skills from trajectories), which needs the full conversations as raw material.

## 1. Proposal and why

**Proposal:** a cron job on the Hub VM reads each agent's transcript files where the harness writes them and appends
every record, unchanged, to one BigQuery table. A second table gets the Hub's agent-to-agent messages.

The telemetry that already flows to Cloud Logging (and, since 2026-11-11, to the BigQuery table `scion_agents` through
a log sink) is not enough for this:

- Its events carry metadata only. The message text is masked before it leaves the container.
- The six Antigravity agents have sent only status events since mid-October. The cause is still open.
- A sink exports new entries only, and the log bucket keeps 30 days, so October would be lost.

The conversations themselves exist only inside the agent containers, in four formats.

## 2. What was measured (Hub VM, 2026-11-11)

| Harness | Agents | Where the conversation lives | Size seen |
|---|---|---|---|
| Claude Code | `lead`, `deployer` | one JSONL file per session | 36 MB and 8 MB |
| Antigravity | `frontend`, 5 eval agents | one JSONL transcript per conversation, plus a SQLite file | up to 69 MB per agent |
| OpenCode | `backend` | one SQLite database | 937 messages, 4,408 parts |
| Hermes | `reviewer` | one SQLite database | 17 sessions, 1,929 messages |

- The Hub message table holds 1,164 messages since 2026-10-02.
- The VM has `python3`, `sqlite3`, `bq` and `gcloud`; the containers have `python3`.
- The Hub VM service account has no BigQuery role in `${SANDBOX_PROJECT}` today.
- The eval agents already wrote a turn shipper (`ship-harness-turns.js` on the eval branch) that writes summarized
  turns to Spanner. It has parsers for Claude Code and Antigravity only, it is started by a harness hook that no
  longer fires, and it stores derived fields (such as a cost from a hard-coded price table), not the raw records.

Not checked yet: the layout of the Antigravity SQLite files, and whether an Antigravity transcript file is only ever
appended to or can rewrite earlier lines.

## 3. Goals and non-goals

- **Goals:** every transcript record of every agent lands in BigQuery, unchanged, within 10 minutes; the history since
  October is loaded once; a rerun never duplicates rows; a reader can rebuild any session in order.
- **Non-goals (v1):** parsing records into turns, tool calls or costs (views can do that later on top of the raw
  rows); masking (PO decision, §7); replacing the log sink or the Spanner turn table; fixing the Antigravity hooks.
- **Anti-goals:** changing a running agent's files; slowing the agents down; exporting stored logins.

## 4. Design

### 4.1 Tables

`agent_transcript_events`, partitioned by day on `event_ts` and clustered by project and agent. One row per source
record:

| Column | Meaning |
|---|---|
| `project_slug`, `agent_name`, `harness` | who wrote it |
| `source_path`, `source_kind` | file or table it came from |
| `session_id` | the harness's own session or conversation id |
| `seq` | line number in the file, or row id in the table |
| `event_ts` | the record's own timestamp, when it has one |
| `event_type`, `role` | lifted from the record when present, else NULL |
| `raw` | the record itself, as JSON, unchanged |
| `source_updated_at` | last-update time, for records that can change |
| `ingested_at`, `exporter_version` | bookkeeping |

`hub_messages`: sender, recipient, type, state, text and creation time of each row of the Hub message table.

A view `agent_transcript_events_latest` keeps one row per (`agent_name`, `source_path`, `seq`), the newest. Readers use
the view.

### 4.2 Reading the sources

The exporter runs on the VM as the Hub OS user and reads through `docker exec`, read-only, as the lead cost
controller already does.

- **JSONL (Claude Code, Antigravity):** read from the saved byte offset to the last complete line. A file that
  shrank, or whose first bytes changed, is read again from the start; the view absorbs the repeats.
- **SQLite (OpenCode, Hermes):** open the database read-only inside the container and select rows newer than the saved
  cursor (row id for Hermes messages, last-update time for OpenCode messages and parts). Only the conversation tables
  are read. The OpenCode tables that hold accounts and stored logins are never read.
- **Hub messages:** select rows newer than the saved creation time from the Hub database, read-only.

### 4.3 Loading without duplicates

Each tick writes one newline-delimited JSON file per source and appends it with a load job. The job id is derived from
the source and the cursor range, so a tick that crashed after loading and before saving its cursor repeats the same job
id, and BigQuery refuses the repeat. This is the design intent; it has to be proven in the tests of §6.

The cursor file is saved atomically after each successful load, with mode 0600, next to the controller's state file.

### 4.4 Schedule, backfill and failure handling

- Cron every 5 minutes, with a lock file so ticks never overlap and a kill-switch file, like the cost controller.
- Each run logs one JSON line: rows read and loaded per source, duration, failures.
- A source that cannot be read is skipped for that tick and counted. Three failed ticks in a row for the same source
  write one ERROR entry to the existing alert log, which emails the PO.
- **Backfill:** the first run starts every cursor at zero, in batches, so October comes in through the same path. It
  runs once by hand, in dry-run first (counts only, nothing loaded).

### 4.5 Access

- The Hub VM service account gets write access on `${BQ_DATASET}` only, and permission to run jobs in
  `${SANDBOX_PROJECT}` (approved by the PO on 2026-11-11).
- Anyone who can read the dataset can read every agent's conversations. Today that is the project's owners, editors
  and viewers, plus the deploy service account.

## 5. Alternatives considered

| Option | Advantage | Why not |
|---|---|---|
| Log sink only | No code, already running | No message text, almost nothing from Antigravity, no history |
| Each agent exports its own transcript | No operator access needed | Depends on agents following a prompt; every agent would need write access |
| Extend the agents' Spanner shipper | Exists, two parsers written | Hook-driven and currently not firing; stores derived fields; lives on the eval branch |
| Container stdout logs | Native to Docker | The harness runs inside tmux; reading them timed out twice and their content was not confirmed |

## 6. Verification

- **Counts:** for every source, rows in BigQuery equal lines in the file or rows in the table, checked by a script that
  prints both.
- **Idempotence:** running the exporter twice with no new activity loads zero rows; a simulated crash between load and
  cursor save leaves the counts unchanged.
- **Round trip:** one known session per harness is rebuilt from BigQuery and compared, record by record, with the
  source file.
- **Known events:** the lead's wake of 2026-11-11 11:50Z (drill issue #55) is found by a query on agent and time.
- **Freshness:** a query reports, per agent, the gap between the newest record and now.
- **Tests:** unit tests with fixture files for the four readers, the cursor logic and the job-id rule (no Hub needed).

## 7. PO decisions so far

2026-11-11:

1. Load the records as they are, without masking.
2. Load the full history since October, then incremental.
3. Write access for the VM service account on the dataset only, plus permission to run jobs.

2026-11-12 (the PO said to proceed on the recommended options):

4. Antigravity: export the JSONL transcripts only. The per-conversation SQLite files stay out of v1.
5. No derived views in this change. All parsing is left to the study in #64.

6. The eval agents may read this dataset. They run as the Hub VM service account, so the grant of §4.5 covers them
   and the exporter needs no service account of its own.

## 8. Open question for the PO

**Retention:** keep the exported records in BigQuery forever, or have BigQuery delete them after a set number of
days? Until decided, no expiry is set.
