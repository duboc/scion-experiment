#!/usr/bin/env python3
"""Sync live agent state from the Scion Hub into Spanner for the eval dashboard. Runs ON the Hub VM.

Reads the Hub's SQLite DB read-only and upserts one row per live agent of the factory projects into
`factory_agent_status` (PK project_slug, agent_name; DDL in config/spanner-factory-ddl.sql). Rows that were not
refreshed for PRUNE_MINUTES (deleted agents) are removed. install-sync-cron.sh runs it every minute from the
Hub OS user's crontab.

Configuration (environment):
  SANDBOX_PROJECT, SPANNER_INSTANCE, SPANNER_DATABASE   where factory_agent_status lives
  FACTORY_PROJECTS     comma-separated Scion project slugs (default: BUILD_PROJECT,EVAL_PROJECT)
  BILLING_PROJECT      quota/billing project for the Spanner API calls (default: SANDBOX_PROJECT). The VM's
                       service account defaults to the Hub project for quota, which fails without this flag
                       (lessons-learned.md L12). The VM SA needs roles/serviceusage.serviceUsageConsumer here.
  PRUNE_MINUTES        default 10
  SCION_HOME           default ~/.scion

Turn and model-call counters are written as NULL when the Hub reports 0: the Hub doesn't populate them today,
so "unknown" is the honest value, not zero (lessons-learned.md L9).

  python3 sync-factory-agents.py            # upsert + prune
  python3 sync-factory-agents.py --dry-run  # print the SQL, write nothing
"""
import argparse
import datetime
import os
import re
import sqlite3
import subprocess
import sys


def load_sync_env():
    env_paths = [
        os.path.join(os.path.expanduser(os.environ.get("SCION_HOME", "~/.scion")), "sync.env"),
        os.path.expanduser("~/factory-sync/sync.env"),
    ]
    for p in env_paths:
        if os.path.exists(p):
            env_vars = {}
            with open(p) as f:
                for line in f:
                    line = line.strip()
                    if line and not line.startswith("#") and "=" in line:
                        k, v = line.split("=", 1)
                        env_vars[k.strip()] = v.strip().strip("'\"")
            if "PATH" in env_vars:
                os.environ["PATH"] = env_vars["PATH"]
            for k, v in env_vars.items():
                if k not in os.environ:
                    os.environ[k] = v
            return


def env(name, default=None):
    value = os.environ.get(name, default)
    if not value:
        sys.exit(f"{name} is not set")
    return value


def lit(value, max_len=None):
    """GoogleSQL string literal, or NULL."""
    if value is None:
        return "NULL"
    s = str(value)
    if max_len and len(s) > max_len:
        s = s[: max_len - 1] + "…"
    s = s.replace("\\", "\\\\").replace("'", "\\'").replace("\n", "\\n").replace("\r", "\\r")
    return f"'{s}'"


def ts(value):
    """Go/SQLite timestamp text (e.g. '2026-10-12 23:45:52.44 +0000 UTC') -> GoogleSQL TIMESTAMP literal, or NULL."""
    if not value:
        return "NULL"
    m = re.match(r"(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2}:\d{2})", str(value))
    return f"TIMESTAMP '{m.group(1)}T{m.group(2)}Z'" if m else "NULL"


def counter(value):
    return str(int(value)) if value else "NULL"


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def run_spanner(spanner_cmd, sql):
    try:
        res = subprocess.run(spanner_cmd + [f"--sql={sql}"], capture_output=True, text=True, timeout=120)
        if res.returncode != 0:
            return "", res.stderr.strip()[:500]
        return res.stdout, ""
    except (FileNotFoundError, subprocess.TimeoutExpired) as e:
        return "", str(e)


def main():
    load_sync_env()
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--dry-run", action="store_true", help="print the SQL instead of running it")
    args = parser.parse_args()

    projects = os.environ.get("FACTORY_PROJECTS") or ",".join(
        p for p in (os.environ.get("BUILD_PROJECT"), os.environ.get("EVAL_PROJECT")) if p)
    projects = [p.strip() for p in projects.split(",") if p.strip()]
    if not projects:
        sys.exit("set FACTORY_PROJECTS (or BUILD_PROJECT/EVAL_PROJECT)")
    sandbox = env("SANDBOX_PROJECT")
    spanner = ["gcloud", "spanner", "databases", "execute-sql", env("SPANNER_DATABASE"),
               f"--instance={env('SPANNER_INSTANCE')}", f"--project={sandbox}",
               f"--billing-project={os.environ.get('BILLING_PROJECT') or sandbox}", "--quiet"]
    prune_minutes = int(os.environ.get("PRUNE_MINUTES", "10"))
    hub_db = os.path.join(os.path.expanduser(os.environ.get("SCION_HOME", "~/.scion")), "hub.db")

    db = sqlite3.connect(f"file:{hub_db}?mode=ro", uri=True)
    db.row_factory = sqlite3.Row
    names = {r["id"]: r["name"] for r in db.execute("SELECT id, name FROM agents")}
    rows = db.execute(
        f"""SELECT p.slug AS project_slug, a.name, a.id, a.template, a.phase, a.activity, a.created_by,
                   a.current_turns, a.current_model_calls, a.task_summary, a.message, a.last_activity_event,
                   COALESCE(json_extract(a.applied_config, '$.harnessConfig'), a.harness_config) AS harness
            FROM agents a JOIN projects p ON p.id = a.project_id
            WHERE a.deleted_at IS NULL AND p.slug IN ({",".join("?" * len(projects))})
            ORDER BY p.slug, a.name""",
        projects,
    ).fetchall()
    if not rows:
        print(f"{now()} no agents found in {projects}; nothing to sync")
        return 0

    values = []
    for r in rows:
        parent = names.get(r["created_by"])  # None when a user (not an agent) created it
        values.append("(" + ", ".join([
            lit(r["project_slug"]), lit(r["name"]), lit(r["id"]), lit(r["template"]), lit(r["harness"]),
            lit(r["phase"]), lit(r["activity"]), lit(parent), counter(r["current_turns"]),
            counter(r["current_model_calls"]), lit(r["task_summary"] or r["message"], 500),
            ts(r["last_activity_event"]), "CURRENT_TIMESTAMP()",
        ]) + ")")
    statements = [
        "INSERT OR UPDATE INTO factory_agent_status (project_slug, agent_name, agent_id, template, harness, phase, "
        "activity, parent_agent, current_turns, model_calls, last_summary, last_activity_at, updated_at) VALUES "
        + ", ".join(values),
        "DELETE FROM factory_agent_status "
        f"WHERE updated_at < TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL {prune_minutes} MINUTE)",
    ]
    if args.dry_run:
        print("\n".join(" ".join(spanner) + f" --sql=<<{sql}>>" for sql in statements))
        return 0
    for sql in statements:
        stdout, err = run_spanner(spanner, sql)
        if err:
            print(f"{now()} ERROR: {err}", file=sys.stderr)
            return 1
    print(f"{now()} synced {len(values)} agents")
    return 0


if __name__ == "__main__":
    sys.exit(main())
