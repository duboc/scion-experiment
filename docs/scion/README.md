# Scion software factory playbook

How to set up and run a **software factory on [Scion](https://github.com/GoogleCloudPlatform/scion)**: a team of
LLM agents that works through a GitHub backlog on its own (plan, implement, review, deploy, verify) while a human
product owner (PO) only releases work, makes decisions and accepts results. Next to it, a second team measures the
factory and the product it ships.

Everything here comes from running two such factories on one self-hosted Scion Hub in October 2026 (Scion nightly
`20261002`): `psearch`, which builds a hybrid search service from 11 GitHub issues, and `psearch-eval`, which watches
it. The scripts and templates are parameterized, so you can build another factory from them.

> [!IMPORTANT]
> No secrets or real project IDs live in this folder. Your values go in `config/factory.env`, which is git-ignored.
> Scripts read the GitHub token and the Hub session secret at run time and never print or store them.

## Map

| Document | Read it when |
|---|---|
| [architecture.md](architecture.md) | you want the big picture: components, identities, data flows, trade-offs |
| [setup-runbook.md](setup-runbook.md) | you are building a new factory (step by step, with a check after each step) |
| [operations-runbook.md](operations-runbook.md) | a factory is running and something looks wrong, or you need a routine task |
| [agent-protocol.md](agent-protocol.md) | you want to understand or change the rules the agents follow |
| [lessons-learned.md](lessons-learned.md) | before you change anything: 25 failures, their causes and fixes |
| [templates/](templates/README.md) | role prompts and settings for the ten agents, plus the shared protocol |
| [scripts/](scripts/README.md) | the tooling: status, admin actions, sandbox setup, sync, patches |
| [config/](config/) | `factory.env.example`, the heartbeat text, the Hub settings excerpt, the Spanner DDL |
| [specs/factory-orchestration-v2-design.md](specs/factory-orchestration-v2-design.md) | the next design (draft): one worker and PR per issue, status labels, watchdogs |

## Quick start

1. Copy `config/factory.env.example` to `config/factory.env` and fill it in.
2. Set up the Hub VM and the Scion server behind IAP (setup-runbook step 2).
3. Disable the `web.terminal_workspace` experiment.
4. `DRY_RUN=1 scripts/setup-sandbox.sh --with-spanner --with-bigquery`, then run it for real with `GITHUB_TOKEN`
   exported.
5. Create the GitHub labels, set up branch protection rulesets, and store the token as the Hub secret `GITHUB_TOKEN` with injection mode `Always`.
6. Create the two Hub projects, make their folders checkouts of the repo, register the deployer identity.
7. `scripts/run-on-hub.sh scripts/patch-antigravity-hooks.py --apply` (before any Antigravity agent starts).
8. `scripts/push-templates.py --project <slug> --roles <roles> --apply` for each project, then start the ten agents.
9. Create the two heartbeat schedules.
10. `scripts/install-sync-cron.sh`, install backups via `scripts/run-on-hub.sh scripts/backup-hub.sh --install-cron`, and install watchdog via `scripts/run-on-hub.sh scripts/factory-watchdog.py --install-cron --apply`.
11. `scripts/setup-alerts-and-budgets.sh` to configure Cloud Monitoring email alerts and GCP billing budgets.
12. Send each lead the GO message.
13. From then on: `scripts/run-on-hub.sh scripts/factory-status.sh` once a day, and answer the PO inbox (or receive real-time alerts via Cloud Monitoring email).

Details and checks for every step: [setup-runbook.md](setup-runbook.md).

## What keeps a factory running

Most of the work was not building the factory but keeping it from stopping. The rules that made the difference:

1. **Something always wakes the leads**: an admin-owned schedule messages each lead every 15 minutes.
2. **Watchdog daemon enforces recovery**: `factory-watchdog.py` runs every 5 minutes to restore heartbeats, wake suspended agents, and auto-restart failed workers with a circuit breaker.
3. **Push alerts reach the PO**: escalations and watchdog alerts trigger Cloud Monitoring email notifications, and GCP billing budgets ($150/mo) warn on spend thresholds.
4. **Controlled updates prevent drift**: `server.maintenance.update_policy: notify` keeps Scion from unannounced restarts that wipe provisioner fixes.
5. **Only delivered messages count**: agents check for `Message delivered` (or `Message sent to user:` for the PO); printed text reaches nobody (one lost verdict stopped the factory for about 60 hours).
6. **Every turn ends with a status** (`sciontool status blocked | task_completed | ask_user`), so idle and dead agents can be told apart.
7. **GitHub is the memory**: plans, evidence and decisions go on the issues, so agents can be restarted or replaced.
8. **Evidence over claims**: no invented numbers; verify every "it exists" against the source.
9. **Narrow escalation**: the PO decides IAM, cost and scope; leads decide the rest and keep going.
10. **Grant identities up front**: an IAM question waiting on the PO stalled a rollout for 10 hours.

## Status

| Part | State |
|---|---|
| Shared-workspace factory (this protocol, these templates) | running; templates here are the improved version of what the agents run |
| Heartbeats, agent-status sync, Antigravity hook patch, sandbox setup | running and verified |
| Watchdog daemon (`factory-watchdog.py`), backup cron (`backup-hub.sh`) | installed on Hub VM cron, running and verified |
| Alerts & budgets (`setup-alerts-and-budgets.sh`) | Cloud Monitoring email alert policy and $150/mo billing budgets active |
| Hub maintenance policy | set to `notify` in settings.yaml, verified across restart |
| `factory-status.sh`, `scion-admin.py` (read commands), `sync-factory-agents.py --dry-run`, `patch-antigravity-hooks.py` (check) | run against the live Hub |
| `setup-sandbox.sh`, `install-sync-cron.sh`, `setup-alerts-and-budgets.sh` | verified against the live environment |
| `push-templates.py` | its procedure fixed the five eval templates on the live Hub (2026-10-13; it reports them up to date); dry run against the build templates |
| Reincarnate (harness switch with the same agent ID) | refused by Scion for these projects (L21); recreate the agent instead |
| v2 design (per-issue workers, `factoryctl`, watchdogs) | draft, not implemented |
