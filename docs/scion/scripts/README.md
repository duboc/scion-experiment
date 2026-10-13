# Scripts

Every script reads `../config/factory.env` (copy it from `factory.env.example`; it is git-ignored). Override the path
with `FACTORY_ENV=/path/to/factory.env`. None of them prints or stores a secret: the Hub session secret is read on the
VM at run time, and the GitHub token is only ever piped through stdin.

| Script | Runs on | Changes things? | What it does |
|---|---|---|---|
| [`run-on-hub.sh`](run-on-hub.sh) | your machine | no (transport) | Ships a local script to the Hub VM base64-encoded and runs it as the Hub OS user (`.py` with python3, anything else with bash), forwarding the factory variables. Avoids the nested-quoting trap of `gcloud compute ssh --command` (L1). |
| [`factory-status.sh`](factory-status.sh) | Hub VM | no | One-screen health check: agents with phase/activity/idle minutes, schedules, recent messages, the PO inbox, failed dispatches, workspace git state, containers, optional terminal tails (`--panes N`). |
| [`scion-admin.py`](scion-admin.py) | Hub VM | yes | PO/admin actions through the Hub API with a short-lived admin token: `projects`, `message`, `schedule list/create/update/pause/resume`, `experiments`. |
| [`sync-factory-agents.py`](sync-factory-agents.py) | Hub VM (cron) | Spanner rows | Upserts live agent state from the Hub DB into Spanner `factory_agent_status` for the eval dashboard. `--dry-run` prints the SQL. |
| [`install-sync-cron.sh`](install-sync-cron.sh) | your machine | Hub VM crontab | Installs/updates the sync script, its `sync.env` and the every-minute cron line; runs one sync. `--dry-run` prints the remote script. |
| [`patch-antigravity-hooks.py`](patch-antigravity-hooks.py) | Hub VM | with `--apply` | Checks (default) or applies the telemetry-free Antigravity hook command in the harness provisioner and in every project's `.agents/hooks.json` (L6). Re-check after Scion updates. |
| [`setup-sandbox.sh`](setup-sandbox.sh) | your machine | GCP | Idempotent sandbox setup: APIs, deployer and runtime service accounts, IAM, Spanner (+ factory tables), BigQuery, Firestore, bucket, `github-token` secret. `DRY_RUN=1` prints the plan. |
| [`render-templates.py`](render-templates.py) | your machine | writes `build/` | Renders `../templates` with your values into `build/templates/<role>/`; `--file` renders one file to stdout (used for the heartbeat text). |

## Common commands

```bash
cd docs/scion
cp config/factory.env.example config/factory.env && $EDITOR config/factory.env

scripts/run-on-hub.sh scripts/factory-status.sh --since 120            # what is the factory doing?
scripts/run-on-hub.sh scripts/factory-status.sh --panes 20             # ...including each agent's terminal
scripts/run-on-hub.sh scripts/scion-admin.py message --project psearch --agent lead \
  --text "PO -> lead: <decision or instruction>"
scripts/run-on-hub.sh scripts/scion-admin.py schedule list --project psearch
scripts/run-on-hub.sh scripts/scion-admin.py schedule update --project psearch --name psearch-lead-heartbeat \
  --text "$(scripts/render-templates.py --file config/heartbeat-message.txt)"
scripts/run-on-hub.sh scripts/patch-antigravity-hooks.py                # check; add --apply to fix
DRY_RUN=1 scripts/setup-sandbox.sh --with-spanner --with-bigquery
```

## Conventions

- Bash scripts use `set -euo pipefail`; Python scripts use only the standard library and call external commands with
  argument lists (no shell).
- Scripts that change things are idempotent and have a check or dry-run mode. Run that first.
- Scripts that run on the VM open the Hub DB read-only (`?mode=ro`). Writes go through the Hub API, never into the DB:
  hand-edited rows are how two templates ended up sharing one template's files (L16).
