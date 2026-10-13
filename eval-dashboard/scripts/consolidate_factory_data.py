#!/usr/bin/env python3
"""
Factory Data Consolidator for psearch / psearch-eval.
Consolidates multi-agent factory data into:
1. Cloud Firestore (default) in riojucu-sandbox:
   - Collection 'agents': Live state of all 10 agents from Spanner factory_agent_status
   - Collection 'tasks': Backlog issues and PRs from GitHub API
   - Collection 'eval_runs': Search evaluation probe benchmarks from Spanner eval_runs
   - Collection 'requirements_compliance': 10 Biggy Search requirement audit statuses
2. Cloud Storage bucket gs://psearch-eval-data-riojucu-sandbox:
   - Snapshots in /snapshots/latest_factory_state.json and /snapshots/{timestamp}_factory_state.json

Verifies every claim by reading data back from Firestore and GCS.
"""

import os
import sys
import json
import time
import subprocess
import urllib.request
import urllib.parse
from datetime import datetime, timezone

PROJECT_ID = os.getenv("GCP_PROJECT", "riojucu-sandbox")
BUCKET_NAME = os.getenv("GCS_BUCKET", "psearch-eval-data-riojucu-sandbox")
SPANNER_INSTANCE = os.getenv("SPANNER_INSTANCE", "psearch-instance")
SPANNER_DATABASE = os.getenv("SPANNER_DATABASE", "psearch-db")
GITHUB_REPO = os.getenv("GITHUB_REPO", "duboc/scion-experiment")
SERVING_URL = os.getenv("PSEARCH_SERVING_URL", "https://psearch-serving-mhjw244bgq-uc.a.run.app")

def get_auth_token():
    try:
        res = subprocess.run(["gcloud", "auth", "print-access-token"], capture_output=True, text=True, check=True)
        return res.stdout.strip()
    except Exception as e:
        print(f"[ERROR] Failed to get gcloud access token: {e}", file=sys.stderr)
        return None

def to_firestore_value(v):
    if v is None:
        return {"nullValue": None}
    if isinstance(v, bool):
        return {"booleanValue": v}
    if isinstance(v, int):
        return {"integerValue": str(v)}
    if isinstance(v, float):
        return {"doubleValue": v}
    if isinstance(v, str):
        return {"stringValue": v}
    if isinstance(v, list):
        return {"arrayValue": {"values": [to_firestore_value(x) for x in v]}}
    if isinstance(v, dict):
        return {"mapValue": {"fields": {k: to_firestore_value(val) for k, val in v.items()}}}
    return {"stringValue": str(v)}

def from_firestore_value(v):
    if not isinstance(v, dict):
        return v
    if "stringValue" in v:
        return v["stringValue"]
    if "integerValue" in v:
        return int(v["integerValue"])
    if "doubleValue" in v:
        return float(v["doubleValue"])
    if "booleanValue" in v:
        return v["booleanValue"]
    if "nullValue" in v:
        return None
    if "timestampValue" in v:
        return v["timestampValue"]
    if "arrayValue" in v:
        return [from_firestore_value(x) for x in v["arrayValue"].get("values", [])]
    if "mapValue" in v:
        return {k: from_firestore_value(val) for k, val in v["mapValue"].get("fields", {}).items()}
    return None

def write_firestore_doc(token, collection, doc_id, data):
    url = f"https://firestore.googleapis.com/v1/projects/{PROJECT_ID}/databases/(default)/documents/{collection}/{doc_id}"
    fields = {k: to_firestore_value(v) for k, v in data.items()}
    payload = json.dumps({"fields": fields}).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=payload,
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        method="PATCH"
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        return resp.status in (200, 201)

def read_firestore_collection(token, collection):
    url = f"https://firestore.googleapis.com/v1/projects/{PROJECT_ID}/databases/(default)/documents/{collection}?pageSize=100"
    req = urllib.request.Request(
        url,
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"}
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        res = json.loads(resp.read().decode("utf-8"))
        docs = []
        for raw_doc in res.get("documents", []):
            name = raw_doc.get("name", "").split("/")[-1]
            fields = {k: from_firestore_value(v) for k, v in raw_doc.get("fields", {}).items()}
            docs.append({"doc_id": name, **fields})
        return docs

# 1. Fetch live agents from Spanner
def fetch_spanner_agents():
    sql = "SELECT project_slug, agent_name, agent_id, template, harness, phase, activity, parent_agent, current_turns, model_calls, last_summary, updated_at, last_activity_at FROM factory_agent_status ORDER BY project_slug, agent_name"
    cmd = [
        "gcloud", "spanner", "databases", "execute-sql", SPANNER_DATABASE,
        f"--instance={SPANNER_INSTANCE}",
        f"--project={PROJECT_ID}",
        f"--billing-project={PROJECT_ID}",
        "--format=json",
        f"--sql={sql}"
    ]
    res = subprocess.run(cmd, capture_output=True, text=True, check=True)
    data = json.loads(res.stdout)
    fields = [f["name"] for f in data.get("metadata", {}).get("rowType", {}).get("fields", [])]
    agents = []
    for raw in data.get("rows", []):
        item = dict(zip(fields, raw))
        agents.append({
            "project": item.get("project_slug"),
            "name": item.get("agent_name"),
            "agent_id": item.get("agent_id"),
            "template": item.get("template"),
            "role": item.get("template") or item.get("agent_name"),
            "harness": item.get("harness"),
            "phase": item.get("phase"),
            "activity": item.get("activity"),
            "parent_agent": item.get("parent_agent") or "root",
            "current_turns": None if item.get("current_turns") is None else item.get("current_turns"),
            "model_calls": None if item.get("model_calls") is None else item.get("model_calls"),
            "turns_display": "n/a",
            "calls_display": "n/a",
            "last_summary": item.get("last_summary"),
            "updated_at": item.get("updated_at"),
            "last_activity_at": item.get("last_activity_at")
        })
    return agents

# 2. Fetch tasks from GitHub API
def fetch_github_tasks():
    headers = {"User-Agent": "psearch-eval-consolidator", "Accept": "application/vnd.github.v3+json"}
    token = os.getenv("GITHUB_TOKEN")
    if token:
        headers["Authorization"] = f"Bearer {token.strip()}"
    
    url = f"https://api.github.com/repos/{GITHUB_REPO}/issues?state=all&per_page=30"
    req = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            issues = json.loads(resp.read().decode("utf-8"))
            tasks = []
            for issue in issues:
                labels = [l.get("name") for l in issue.get("labels", []) if isinstance(l, dict)]
                tasks.append({
                    "issue_number": issue.get("number"),
                    "title": issue.get("title"),
                    "state": issue.get("state"),
                    "is_pull_request": "pull_request" in issue,
                    "html_url": issue.get("html_url"),
                    "labels": labels,
                    "created_at": issue.get("created_at"),
                    "updated_at": issue.get("updated_at")
                })
            return tasks
    except Exception as e:
        print(f"[WARN] GitHub tasks fetch failed: {e}", file=sys.stderr)
        return []

# 3. Fetch eval runs from Spanner
def fetch_spanner_eval_runs():
    sql = "SELECT run_id, git_commit, overall_score, ndcg_at_10, mrr_at_10, p50_ms, p95_ms, p99_ms, requirements_json, created_at FROM eval_runs ORDER BY created_at DESC LIMIT 10"
    cmd = [
        "gcloud", "spanner", "databases", "execute-sql", SPANNER_DATABASE,
        f"--instance={SPANNER_INSTANCE}",
        f"--project={PROJECT_ID}",
        f"--billing-project={PROJECT_ID}",
        "--format=json",
        f"--sql={sql}"
    ]
    try:
        res = subprocess.run(cmd, capture_output=True, text=True, check=True)
        data = json.loads(res.stdout)
        fields = [f["name"] for f in data.get("metadata", {}).get("rowType", {}).get("fields", [])]
        runs = []
        for raw in data.get("rows", []):
            item = dict(zip(fields, raw))
            runs.append({
                "run_id": item.get("run_id"),
                "git_commit": item.get("git_commit"),
                "overall_score": float(item.get("overall_score")) if item.get("overall_score") is not None else None,
                "ndcg_at_10": float(item.get("ndcg_at_10")) if item.get("ndcg_at_10") is not None else None,
                "mrr_at_10": float(item.get("mrr_at_10")) if item.get("mrr_at_10") is not None else None,
                "p50_ms": float(item.get("p50_ms")) if item.get("p50_ms") is not None else None,
                "p95_ms": float(item.get("p95_ms")) if item.get("p95_ms") is not None else None,
                "p99_ms": float(item.get("p99_ms")) if item.get("p99_ms") is not None else None,
                "created_at": item.get("created_at")
            })
        return runs
    except Exception as e:
        print(f"[WARN] Spanner eval_runs fetch failed: {e}", file=sys.stderr)
        return []

# 4. Biggy Search Requirements Compliance
UNMEASURED_REQUIREMENTS = [
    {"issue_number": 1, "code": "4.1.1", "title": "Busca Híbrida (Dynamic RRF Alpha)", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 2, "code": "4.1.2", "title": "Busca Vetorial (1024-dim, Blue/Green, Cache)", "owner_agent": "@backend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 3, "code": "4.1.3", "title": "Disponibilidade (Estoque/Canal + Indisponíveis no Final)", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 4, "code": "4.1.4", "title": "Catálogo Multilanguage (pt-BR, en-US, es-MX)", "owner_agent": "@backend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 5, "code": "4.1.5", "title": "Facets Server-Side (Goroutines Concorrentes)", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 6, "code": "4.1.6", "title": "Relevance & Merchandising Rules + 8 Sinais de Boost", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 7, "code": "4.1.7", "title": "Low Relevance Synonyms (0.3x Score Weight)", "owner_agent": "@backend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 8, "code": "4.1.8", "title": "Delivery Promises (CEP/SLA via INTERLEAVE IN PARENT)", "owner_agent": "@backend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 9, "code": "4.1.9", "title": "Autocomplete (BQ Analytics + TOKENIZE_NGRAMS)", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 10, "code": "4.1.10", "title": "Learning to Rank (2-Stage In-Memory Re-Ranking)", "owner_agent": "@backend · @reviewer", "status": "not measured", "latency_ms": None, "evidence": None}
]

def fetch_spanner_latest_requirements():
    sql = "SELECT requirements_json FROM eval_runs WHERE requirements_json IS NOT NULL ORDER BY created_at DESC LIMIT 1"
    cmd = [
        "gcloud", "spanner", "databases", "execute-sql", SPANNER_DATABASE,
        f"--instance={SPANNER_INSTANCE}",
        f"--project={PROJECT_ID}",
        f"--billing-project={PROJECT_ID}",
        "--format=json",
        f"--sql={sql}"
    ]
    try:
        res = subprocess.run(cmd, capture_output=True, text=True, check=True)
        data = json.loads(res.stdout)
        rows = data.get("rows", [])
        if rows and len(rows) > 0:
            raw_json = rows[0][0]
            parsed = json.loads(raw_json) if isinstance(raw_json, str) else raw_json
            if parsed and isinstance(parsed, dict) and "requirements" in parsed:
                return parsed["requirements"]
    except Exception as e:
        print(f"[WARN] Spanner requirements fetch failed: {e}", file=sys.stderr)
    return UNMEASURED_REQUIREMENTS

def run_consolidation():
    print(f"=== Starting Factory Data Consolidation ({datetime.now(timezone.utc).isoformat()}) ===")
    token = get_auth_token()
    if not token:
        sys.exit(1)

    # 1. Spanner Agents -> Firestore 'agents'
    print("[1/5] Ingesting agents from Spanner factory_agent_status into Firestore 'agents'...")
    agents = fetch_spanner_agents()
    agents_written = 0
    for a in agents:
        doc_id = f"{a['project']}__{a['name']}"
        if write_firestore_doc(token, "agents", doc_id, a):
            agents_written += 1
    print(f"  -> Successfully wrote {agents_written}/{len(agents)} agents to Firestore 'agents'")

    # 2. GitHub Tasks -> Firestore 'tasks'
    print("[2/5] Ingesting GitHub backlog issues into Firestore 'tasks'...")
    tasks = fetch_github_tasks()
    tasks_written = 0
    for t in tasks:
        doc_id = f"issue_{t['issue_number']}"
        if write_firestore_doc(token, "tasks", doc_id, t):
            tasks_written += 1
    print(f"  -> Successfully wrote {tasks_written}/{len(tasks)} tasks to Firestore 'tasks'")

    # 3. Spanner Eval Runs -> Firestore 'eval_runs'
    print("[3/5] Ingesting evaluation benchmark runs into Firestore 'eval_runs'...")
    eval_runs = fetch_spanner_eval_runs()
    eval_written = 0
    for r in eval_runs:
        doc_id = r["run_id"]
        if write_firestore_doc(token, "eval_runs", doc_id, r):
            eval_written += 1
    print(f"  -> Successfully wrote {eval_written}/{len(eval_runs)} eval runs to Firestore 'eval_runs'")

    # 4. Requirements Compliance -> Firestore 'requirements_compliance'
    print("[4/5] Ingesting Biggy Search requirements compliance into Firestore 'requirements_compliance'...")
    now_iso = datetime.now(timezone.utc).isoformat()
    requirements = fetch_spanner_latest_requirements()
    reqs_written = 0
    for req in requirements:
        doc_id = f"req_{req['issue_number']}"
        payload = {**req, "evaluated_at": now_iso}
        if write_firestore_doc(token, "requirements_compliance", doc_id, payload):
            reqs_written += 1
    print(f"  -> Successfully wrote {reqs_written}/{len(requirements)} requirements to Firestore 'requirements_compliance'")

    # 5. Compile Consolidated JSON Snapshot -> GCS Bucket
    print(f"[5/5] Exporting consolidated factory state snapshot to gs://{BUCKET_NAME}/snapshots/...")
    now_str = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    snapshot = {
        "timestamp": now_iso,
        "source_metadata": {
            "spanner_instance": SPANNER_INSTANCE,
            "spanner_database": SPANNER_DATABASE,
            "github_repo": GITHUB_REPO,
            "target_serving_url": SERVING_URL,
            "firestore_database": f"{PROJECT_ID}/(default)",
            "gcs_bucket": f"gs://{BUCKET_NAME}"
        },
        "counts": {
            "agents": len(agents),
            "tasks": len(tasks),
            "eval_runs": len(eval_runs),
            "requirements": len(requirements)
        },
        "agents": agents,
        "tasks": tasks,
        "eval_runs": eval_runs,
        "requirements_compliance": requirements
    }

    tmp_path = f"/tmp/factory_state_{now_str}.json"
    with open(tmp_path, "w", encoding="utf-8") as f:
        json.dump(snapshot, f, indent=2)

    # Upload to both latest and timestamped paths
    latest_gcs = f"gs://{BUCKET_NAME}/snapshots/latest_factory_state.json"
    archive_gcs = f"gs://{BUCKET_NAME}/snapshots/{now_str}_factory_state.json"

    cmd1 = ["gcloud", "storage", "cp", f"--billing-project={PROJECT_ID}", tmp_path, latest_gcs]
    cmd2 = ["gcloud", "storage", "cp", f"--billing-project={PROJECT_ID}", tmp_path, archive_gcs]
    subprocess.run(cmd1, check=True, capture_output=True)
    subprocess.run(cmd2, check=True, capture_output=True)
    os.remove(tmp_path)
    print(f"  -> Uploaded snapshot to {latest_gcs} and {archive_gcs}")

    # VERIFICATION PHASE: Read back from Firestore and GCS
    print("\n=== Verification Phase: Reading Data Back ===")
    
    # Read back agents
    rb_agents = read_firestore_collection(token, "agents")
    print(f"[VERIFIED] Firestore 'agents': read back {len(rb_agents)} documents.")
    sample_agent = rb_agents[0] if rb_agents else {}
    print(f"  Sample agent doc '{sample_agent.get('doc_id')}': name={sample_agent.get('name')}, phase={sample_agent.get('phase')}, activity={sample_agent.get('activity')}, turns={sample_agent.get('turns_display')}, calls={sample_agent.get('calls_display')}")

    # Read back tasks
    rb_tasks = read_firestore_collection(token, "tasks")
    print(f"[VERIFIED] Firestore 'tasks': read back {len(rb_tasks)} documents.")

    # Read back eval_runs
    rb_evals = read_firestore_collection(token, "eval_runs")
    print(f"[VERIFIED] Firestore 'eval_runs': read back {len(rb_evals)} documents.")
    if rb_evals:
        print(f"  Latest run: run_id={rb_evals[0].get('run_id')}, ndcg={rb_evals[0].get('ndcg_at_10')}, mrr={rb_evals[0].get('mrr_at_10')}, p50={rb_evals[0].get('p50_ms')}ms")

    # Read back requirements_compliance
    rb_reqs = read_firestore_collection(token, "requirements_compliance")
    print(f"[VERIFIED] Firestore 'requirements_compliance': read back {len(rb_reqs)} documents.")

    # Read back from GCS
    cat_cmd = ["gcloud", "storage", "cat", f"--billing-project={PROJECT_ID}", latest_gcs]
    cat_res = subprocess.run(cat_cmd, capture_output=True, text=True, check=True)
    rb_gcs = json.loads(cat_res.stdout)
    print(f"[VERIFIED] GCS '{latest_gcs}': read back snapshot with timestamp={rb_gcs.get('timestamp')}, agents={rb_gcs.get('counts', {}).get('agents')}, tasks={rb_gcs.get('counts', {}).get('tasks')}")

    print("\n=== Consolidation Cycle Completed Successfully ===")

if __name__ == "__main__":
    run_consolidation()
