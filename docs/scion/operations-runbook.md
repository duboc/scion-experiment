# Operations runbook

How to keep a running factory healthy. The PO's job is intake (issues), decisions (IAM, cost, scope) and acceptance
(closing issues); everything below is for when the factory needs a hand.

All commands run from `docs/scion` with a filled-in `config/factory.env`. `<project>` is a Scion project slug
(`BUILD_PROJECT` or `EVAL_PROJECT`).

## Daily check (5 minutes)

```bash
scripts/run-on-hub.sh scripts/factory-status.sh --since 240
```

Read it in this order:

1. **Messages to users** (the PO inbox): decisions waiting for you. Answer with `scion-admin.py message` or on the issue.
2. **Schedules:** `errors=0` and `last_run` within the last 15 minutes.
3. **Messages in the last N minutes:** handoffs flowing (`DONE`, verdicts, deploy reports). Long silence is the first
   sign of a stall.
4. **Agents:** `idle_min` and `activity` are hints, not facts (L18). An agent is fine if GitHub or its messages show
   progress.
5. **Workspaces:** uncommitted files are normal while an issue is in flight; a growing pile with no commits for hours
   is not.

Then skim the open issues on GitHub: every in-flight issue should have a comment from the last hour or two.

---

Each runbook below has: **Trigger**, **Diagnose**, **Act**, **Check**, **Escalate**.

## The factory is quiet

- **Trigger:** no commits, issue comments or agent messages for more than an hour while issues are open.
- **Diagnose:** `factory-status.sh --since 180 --panes 30`. Look for who is waiting on whom: the last messages show the
  last handoff; the panes show whether an agent is mid-task, at a prompt, or showing an error.
- **Act:**
  - Someone waits for a message that was never sent: see [A handoff was lost](#a-handoff-was-lost).
  - The lead is at a prompt and ignored heartbeats: see [A lead doesn't react to heartbeats](#a-lead-doesnt-react-to-heartbeats).
  - A harness error is on screen: see the harness-specific runbooks below.
- **Check:** a new message or issue comment within 15 minutes.
- **Escalate:** a stall that repeats with the same cause is a protocol bug: fix the template or the heartbeat text, and
  add it to [lessons-learned.md](lessons-learned.md).

## A handoff was lost

- **Trigger:** an agent says it is waiting for X from Y, and Y's terminal shows it produced X (a verdict, a deploy
  result) but there is no matching message (L4).
- **Act:** forward it yourself, with the evidence, to the agent that waits:
  ```bash
  scripts/run-on-hub.sh scripts/scion-admin.py message --project <project> --agent lead \
    --text "PO -> lead: reviewer's verdict for #N, which never reached you: PASS. Evidence: <files, commands, results>."
  ```
  Then remind Y that only `scion message` reaches others and that it must see `Message delivered`.
- **Check:** the waiting agent acts on it within minutes.

## An agent shows STALLED

- **Trigger:** `activity=stalled` in the Hub or in `factory-status.sh`.
- **Diagnose:** usually it ended a turn without `sciontool status` (L5), and is fine. Hermes never reports activity, so
  the reviewer always looks stalled (L18). Check its pane and its last message.
- **Act:** nothing, if its work is moving. If it is really stuck at a prompt with work pending, message it with
  `--interrupt` (cancels what the harness is doing first) or `--wake` (resumes a suspended agent):
  ```bash
  scripts/run-on-hub.sh scripts/scion-admin.py message --project <project> --agent <agent> --interrupt \
    --text "PO -> <agent>: status check. Post your progress on the issue, then end your turn with sciontool status."
  ```
- **Check:** its next turn ends with a status (`blocked`, `task_completed` or `ask_user`).

## A lead doesn't react to heartbeats

- **Trigger:** `schedule list` shows recent successful runs, but the lead's pane and the messages show no reaction.
- **Diagnose:** a successful fire doesn't guarantee delivery (L11). Look at the lead's pane: is it mid-task (the
  heartbeat queues behind it, which is fine), at a prompt, or showing an error?
- **Act:** send the heartbeat text by hand with `--interrupt`. If schedules show errors, check that the schedule was
  created by the admin (`created_by`) and that the agent name is right; recreate it with `schedule create`.
- **Check:** the lead's next turn runs the heartbeat steps.

## Antigravity tool calls fail

- **Trigger:** Antigravity agents report "hook timed out" or abort every tool call; nothing gets written (L6).
- **Diagnose:** `scripts/run-on-hub.sh scripts/patch-antigravity-hooks.py` (check mode). After a Scion upgrade the
  provisioner patch may be gone (L20).
- **Act:** `scripts/run-on-hub.sh scripts/patch-antigravity-hooks.py --apply`. Running agents pick up the
  `hooks.json` change on their next tool call; the provisioner change applies at the next agent start.
- **Check:** the check mode exits 0, and the agent's next tool call succeeds (watch its pane).

## OpenCode gets 404s from the model

- **Trigger:** the OpenCode agent's pane shows 404 errors or a model other than Gemini 2.5 Pro (L3).
- **Act:** make sure the agent uses the pinned launcher from
  [`templates/backend/home/.local/bin/opencode-launch.sh`](templates/backend/home/.local/bin/opencode-launch.sh), then
  restart the harness in place (this discards the current OpenCode session, not the workspace):
  ```bash
  # on the VM, as the Hub OS user
  docker exec -u scion <project>--backend tmux respawn-pane -k -t scion:0.0 "sh /home/scion/.local/bin/opencode-launch.sh"
  ```
  Then re-brief the agent: its conversation starts empty.
- **Check:** the pane footer shows "Gemini 2.5 Pro" and the first reply arrives without errors.

## A deploy failed or needs a rollback

- **Trigger:** the deployer reports a failed check, or the live service misbehaves after a deploy.
- **Act:** route traffic back to the last good revision (the deployer records it before deploying):
  ```bash
  gcloud run revisions list --service "$SERVING_SERVICE" --project "$SANDBOX_PROJECT" --region "$SANDBOX_REGION" --limit 5
  gcloud run services update-traffic "$SERVING_SERVICE" --project "$SANDBOX_PROJECT" --region "$SANDBOX_REGION" \
    --to-revisions <last-good-revision>=100
  ```
  A 403 from Vertex AI or Spanner means the runtime SA is missing a role; grant exactly that role to `RUNTIME_SA`,
  never switch the service to the default compute SA (L7).
- **Check:** `/api/health` returns 200 and the issue's acceptance checks pass on the old revision; the deployer posts
  the rollback on the issue.

## The dashboard shows wrong or empty data

- **Trigger:** empty panels, numbers that don't move, or values you can't trace to a source (L9).
- **Diagnose:**
  ```bash
  url=$(gcloud run services describe "$DASHBOARD_SERVICE" --project "$SANDBOX_PROJECT" --region "$SANDBOX_REGION" --format='value(status.url)')
  curl -s "$url/api/agents" | head -c 600; echo
  curl -s "$url/api/backlog" | head -c 600; echo     # HTML here means the route doesn't exist
  ```
  Agents come from Spanner `factory_agent_status` (is the sync running? see the next runbook), the backlog from the
  GitHub API (is `GITHUB_TOKEN <- secret github-token` on the service?), metrics from `eval_runs`.
- **Act:** give `eval-lead` the evidence and the data-source rule; don't accept "fixed" without a `curl` that shows
  real data.
- **Check:** the same `curl` returns data that matches the source (e.g. agent count equals `factory-status.sh`).

## The agent-status sync stopped

- **Trigger:** `factory_agent_status.updated_at` older than a few minutes, or the dashboard's agent list is stale.
- **Diagnose:** on the VM, `tail ~/.scion/factory-sync.log`. Quota or permission errors point to
  `BILLING_PROJECT` and the VM SA's roles (L12); "no agents found" to `FACTORY_PROJECTS`.
- **Act:** fix `factory.env`, then `scripts/install-sync-cron.sh` again (idempotent).
- **Check:** a `synced N agents` line every minute.

## Web terminal button does nothing

- **Trigger:** the Terminal button on an agent page doesn't open a working terminal (L2).
- **Act:** `scripts/run-on-hub.sh scripts/scion-admin.py experiments --set web.terminal_workspace=false`; reload the
  page. Close other Hub tabs if it still hangs.

## After a Scion upgrade

The Hub follows the nightly channel and updates itself. After an update:

1. `scripts/run-on-hub.sh scripts/patch-antigravity-hooks.py` (re-apply with `--apply` if needed; L20).
2. `scripts/run-on-hub.sh scripts/scion-admin.py experiments`: overrides still in place.
3. `scripts/run-on-hub.sh scripts/factory-status.sh`: agents running, schedules firing, sync log moving.

## Change the heartbeat text or cadence

```bash
scripts/run-on-hub.sh scripts/scion-admin.py schedule update --project <project> --name <project>-lead-heartbeat \
  --text "$(scripts/render-templates.py --file config/heartbeat-message.txt)"
scripts/run-on-hub.sh scripts/scion-admin.py schedule update --project <project> --name <project>-lead-heartbeat \
  --cron "*/30 * * * *"
```

Fifteen minutes balances reaction time against cost: each heartbeat is at least one model turn per lead.

## Pause and resume the factory

- **Pause:** `schedule pause` for both heartbeats, then tell each lead: "PO -> lead: pause. Finish or park the current
  step, comment the state on each in-flight issue, then stay idle." Agents keep their containers.
- **Resume:** `schedule resume`, then send the GO message again (setup-runbook step 11).

## Give an agent a decision or a new rule

Messages are the fastest channel; template changes only apply at the next start.

```bash
scripts/run-on-hub.sh scripts/scion-admin.py message --project <project> --agent <agent> \
  --text "PO -> <agent>: <decision>. From now on: <rule>."
```

Make the same change in the template so it survives restarts.

## Switch an agent's harness (keeping its identity and work)

Scion's `reincarnate` stops an agent and starts a new generation with the same ID and slug, re-resolving its
configuration from the current template, with a handoff note as the first task.

1. Change `default_harness_config` in the role's template, render it, and push it (make sure the template has its own
   files; L16).
2. Get a handoff: ask the agent to write one, using the format from `scion reincarnate --handoff-template`, to a file
   in `/workspace`, and to commit nothing else.
3. Preview, then migrate (from a CLI context linked to the project):
   ```bash
   scion reincarnate <agent> --dry-run                      # shows the template, image, harness and env changes
   scion reincarnate <agent> --handoff-file <handoff.md>
   ```
   The agent can also migrate itself from inside its container (`scion reincarnate --handoff-file <file>`).
4. **Check:** `factory-status.sh` shows the new harness for the agent, and its first message confirms it read the
   handoff.

The harness switch follows the template's default only if the agent was created without an explicit harness config.

## Restart a stuck agent

Prefer, in order: a message with `--interrupt`; `--wake` for a suspended agent; restarting the harness pane (see the
OpenCode runbook); `scion stop <agent>` then `scion resume <agent>`. Each step loses more context, which is why work
state must live on GitHub. Don't delete and recreate an agent unless the PO decides so: it gets a new ID, and schedules
or subscriptions that point at it must be checked.

## Rotate the GitHub token

1. Create the new token on GitHub.
2. Update the Hub secret `GITHUB_TOKEN` (web UI, Settings, Secrets).
3. `read -rs GITHUB_TOKEN && export GITHUB_TOKEN && UPDATE_SECRET=1 scripts/setup-sandbox.sh && unset GITHUB_TOKEN`
   (adds a new version of `github-token`; the dashboard reads `latest` on its next revision or cold start).
4. Revoke the old token.

## Tear down

1. Pause the heartbeats, stop the agents, remove the sync line from the VM crontab (`crontab -e` as the Hub OS user).
2. Back up `~/.scion/projects/<project>` before deleting a Hub-managed project without a git remote: deleting the
   project deletes its folder.
3. Delete the sandbox project (everything the factories created lives there).
4. Revoke the GitHub token.
