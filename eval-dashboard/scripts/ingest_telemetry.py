#!/usr/bin/env python3
"""
Telemetry Ingestion Worker for psearch multi-agent factory.
Ingests agent state, turn counts, and task completeness into Firestore and GCS.
"""

import os
import sys
import json
import time
from datetime import datetime, timezone

try:
    from google.cloud import firestore, storage
    HAS_GCP_SDK = True
except ImportError:
    HAS_GCP_SDK = False

PROJECT_ID = os.getenv("GCP_PROJECT", "riojucu-sandbox")
BUCKET_NAME = os.getenv("GCS_BUCKET", "psearch-eval-data-riojucu-sandbox")

DEFAULT_AGENTS = [
    {"name": "lead", "role": "Factory Tech Lead & Backlog Orchestrator", "harness": "claude", "activity": "active", "turns": 14, "model_calls": 28, "issue": "#1..#3 (Wave 1)"},
    {"name": "backend", "role": "Go + Cloud Spanner + BigQuery Engineer", "harness": "opencode", "activity": "active", "turns": 18, "model_calls": 36, "issue": "#1 Hybrid RRF Alpha"},
    {"name": "frontend", "role": "Storefront & Search UX Engineer", "harness": "antigravity", "activity": "active", "turns": 12, "model_calls": 24, "issue": "#1 Alpha Controls"},
    {"name": "reviewer", "role": "QA, TDD & Spanner Query Verifier", "harness": "hermes", "activity": "idle", "turns": 6, "model_calls": 12, "issue": "#1 Verification"},
    {"name": "deployer", "role": "Cloud Run & Spanner Release Engineer", "harness": "claude", "activity": "idle", "turns": 4, "model_calls": 8, "issue": "psearch-serving"},
]

def ingest_to_firestore(agents):
    if not HAS_GCP_SDK:
        print(f"[WARN] google-cloud libraries not installed; skipping live Firestore sync.")
        return False
    try:
        db = firestore.Client(project=PROJECT_ID)
        now = datetime.now(timezone.utc)
        for a in agents:
            doc_ref = db.collection("agents").document(a["name"])
            doc_ref.set({
                "agent_id": a["name"],
                "role": a["role"],
                "harness": a["harness"],
                "activity": a["activity"],
                "turns_consumed": a["turns"],
                "model_calls": a["model_calls"],
                "active_issue": a["issue"],
                "last_heartbeat": now
            }, merge=True)
        print(f"[SUCCESS] Ingested {len(agents)} agents to Firestore in {PROJECT_ID}")
        return True
    except Exception as e:
        print(f"[ERROR] Firestore ingestion failed: {e}", file=sys.stderr)
        return False

def ingest_to_gcs(agents):
    if not HAS_GCP_SDK:
        return False
    try:
        client = storage.Client(project=PROJECT_ID)
        bucket = client.bucket(BUCKET_NAME)
        timestamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        blob = bucket.blob(f"agent-telemetry/{timestamp}_agents.json")
        blob.upload_from_string(json.dumps(agents, indent=2), content_type="application/json")
        print(f"[SUCCESS] Uploaded snapshot to gs://{BUCKET_NAME}/{blob.name}")
        return True
    except Exception as e:
        print(f"[WARN] GCS ingestion failed: {e}", file=sys.stderr)
        return False

def main():
    print(f"Starting Scion Multi-Agent Telemetry Ingestion -> Project: {PROJECT_ID}")
    fs_ok = ingest_to_firestore(DEFAULT_AGENTS)
    gcs_ok = ingest_to_gcs(DEFAULT_AGENTS)
    print(f"Ingestion cycle completed. Firestore: {fs_ok}, GCS: {gcs_ok}")

if __name__ == "__main__":
    main()
