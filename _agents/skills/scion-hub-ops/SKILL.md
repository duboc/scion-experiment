---
name: scion-hub-ops
description: >-
  Operates the self-hosted Scion Hub of this repository: reads agent state,
  logs and transcripts on the Hub VM, runs the repo's admin scripts there,
  messages agents as the PO, and avoids the known traps of this setup. Use
  when asked to inspect, diagnose or change the build or eval factories, the
  lead cost controller, heartbeats, agent templates, or the eval dashboard.
  Don't use for product code under src/.
---

# Operating the Scion Hub

Read [operations-runbook.md](../../../docs/scion/operations-runbook.md) first.
This file only adds what the runbook does not say.

## Setup (once per session)

1. Tools needed on PATH: `gcloud`, `bq`, `gh`, `python3`, `go`.
2. `docs/scion/config/factory.env` is git-ignored and missing in a clean
   clone. Ask the user for the values, write them to a private file outside
   the repository (mode 600), and `export FACTORY_ENV=<that file>`.
3. Repo scripts run on the VM with
   `docs/scion/scripts/run-on-hub.sh docs/scion/scripts/<script> [args]`.
4. For ad-hoc reads, write the commands to a script file and stream it:
   `gcloud compute ssh "$HUB_OS_USER@$HUB_VM" --project "$HUB_PROJECT" --zone "$HUB_ZONE" --tunnel-through-iap --command 'sudo -u scion bash -s' < probe.sh`.
   Inline `--command` strings with nested quotes break.
5. The first SSH from a new machine creates a key and adds it to the
   project's SSH metadata. Tell the user when that happens.

## Rules of engagement

- Read first. Open `hub.db` with `?mode=ro`; capture tmux panes, never type
  into them.
- Anything that changes a running agent, a cron, a schedule, a template or
  an access grant needs a specific yes from the user. A general "go ahead"
  after a list of pending decisions does not cover it.
- Commits stay local until the user says to push. No session trailers.
- Never put real project IDs, hostnames or emails in tracked files. Use the
  `${...}` placeholders from `factory.env.example`.
- Before calling work done, run
  `python3 -m unittest discover -s docs/scion/scripts/tests -q` and the
  forbidden-ID guard in `src/psearch/serving/internal/config`.

## Traps seen on this Hub

- **Cron timing:** `lead-controller.py --install-cron` takes effect on the
  next 5-minute tick. Check the clock before installing a behavior change,
  and say so when asking for approval.
- **Issue labels wake leads:** an issue labeled with a project slug wakes
  that project's lead through the cost controller. Use no factory label for
  operator-only work.
- **Eval factory has no heartbeat:** nothing wakes `eval-lead` except a
  message. After opening an eval issue, message it.
- **Templates:** a template change only applies when an agent is created.
  Send the new rule to running agents by message as well.
- **Hook file:** `.agents/hooks.json` is one file per project workspace,
  shared by every agent of that project. It cannot be changed for one agent.
- **tmux targets:** Claude Code agents use `scion:agent`; the other
  harnesses use `scion:0.0`.
- **Restarting a harness pane** (`tmux respawn-pane -k`) drops the agent's
  conversation and replays its original start prompt.
- **`docker logs`** on agent containers did not return within 100 s. Read
  the harness's own files instead.
- **Transcripts:** Claude Code keeps JSONL under `~/.claude/projects/`;
  Antigravity under `~/.gemini/antigravity-cli/brain/*/.system_generated/logs/`;
  OpenCode and Hermes keep SQLite databases.
- **Telemetry:** the events exported to the log carry no message text, and
  the BigQuery table fed by the log sink repeats rows. Deduplicate before
  counting.
- **Agent reports:** numbers and file references in issue reports written by
  agents were wrong more than once. Check the diff or the raw source before
  repeating a claim.

## Checking that a change took effect

- Controller: one JSON line per tick in `~/.scion/lead-controller.log`;
  `result`, `duration_s`, `compacted`, `compact_failed`, `woke`, `failures`.
- Message to an agent: the admin script prints `status=delivered`; then
  confirm in the agent's pane or on the issue.
- Dashboard deploy: the latest ready revision changed and `/api/health`
  answers 200.
