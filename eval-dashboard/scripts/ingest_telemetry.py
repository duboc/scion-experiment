#!/usr/bin/env python3
"""
Telemetry Ingestion Worker for psearch multi-agent factory.
Ingests live agent state from Cloud Spanner table factory_agent_status
in psearch-instance/psearch-db into public/telemetry.json.
Note: current_turns and model_calls are NULL in Spanner (rendered as 'n/a').
"""

import os
import sys
import json
import subprocess
from datetime import datetime, timezone

PROJECT_ID = os.getenv("GCP_PROJECT", "riojucu-sandbox")
INSTANCE_ID = os.getenv("SPANNER_INSTANCE", "psearch-instance")
DATABASE_ID = os.getenv("SPANNER_DATABASE", "psearch-db")
OUTPUT_PATH = os.path.join(os.path.dirname(os.path.dirname(__file__)), "public", "telemetry.json")

def fetch_agents_from_spanner():
    sql = "SELECT project_slug, agent_name, agent_id, template, harness, phase, activity, parent_agent, current_turns, model_calls, last_summary, updated_at, last_activity_at FROM factory_agent_status ORDER BY project_slug, agent_name"
    cmd = [
        "gcloud", "spanner", "databases", "execute-sql", DATABASE_ID,
        f"--instance={INSTANCE_ID}",
        f"--project={PROJECT_ID}",
        f"--billing-project={PROJECT_ID}",
        "--format=json",
        f"--sql={sql}"
    ]
    try:
        res = subprocess.run(cmd, capture_output=True, text=True, check=True)
        data = json.loads(res.stdout)
        fields = [f["name"] for f in data.get("metadata", {}).get("rowType", {}).get("fields", [])]
        agents = []
        for raw_row in data.get("rows", []):
            item = dict(zip(fields, raw_row))
            agents.append({
                "project": item.get("project_slug"),
                "name": item.get("agent_name"),
                "agent_id": item.get("agent_id"),
                "template": item.get("template"),
                "role": item.get("template") or item.get("agent_name"),
                "harness": item.get("harness"),
                "phase": item.get("phase"),
                "activity": item.get("activity"),
                "parent_agent": item.get("parent_agent"),
                "current_turns": "n/a" if item.get("current_turns") is None else item.get("current_turns"),
                "model_calls": "n/a" if item.get("model_calls") is None else item.get("model_calls"),
                "active_issue": item.get("last_summary") or "Standby",
                "last_summary": item.get("last_summary"),
                "updated_at": item.get("updated_at"),
                "last_activity_at": item.get("last_activity_at")
            })
        return agents
    except Exception as e:
        print(f"[ERROR] Failed to fetch agent telemetry from Spanner: {e}", file=sys.stderr)
        return []

def sync_telemetry():
    now_iso = datetime.now(timezone.utc).isoformat()
    agents = fetch_agents_from_spanner()
    if not agents:
        print("[WARN] No agent records retrieved from Spanner; preserving existing snapshot if available.")
        return False

    projects = sorted(list(set(a["project"] for a in agents if a.get("project"))))

    payload = {
        "timestamp": now_iso,
        "source": "spanner:factory_agent_status",
        "database": f"{PROJECT_ID}/{INSTANCE_ID}/{DATABASE_ID}",
        "projects": projects,
        "agents": agents
    }

    os.makedirs(os.path.dirname(OUTPUT_PATH), exist_ok=True)
    with open(OUTPUT_PATH, "w", encoding="utf-8") as f:
        json.dump(payload, f, indent=2)

    print(f"[SUCCESS] Ingested {len(agents)} agents across projects {projects} from Cloud Spanner into {OUTPUT_PATH}")
    return True

if __name__ == "__main__":
    success = sync_telemetry()
    sys.exit(0 if success else 1)
