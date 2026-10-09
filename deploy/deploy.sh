#!/usr/bin/env bash
# Deploys the Incident Status Dashboard to Cloud Run from source (Cloud Build + Artifact Registry).
#
# Usage: PROJECT_ID=my-proj REGION=southamerica-east1 [SERVICE=incident-dashboard] [PUBLIC=true] deploy/deploy.sh
#
# Required IAM for the deploying identity: roles/run.admin, roles/cloudbuild.builds.editor,
# roles/artifactregistry.admin (first deploy creates the repo), roles/storage.admin (source upload
# bucket), roles/iam.serviceAccountUser,
# and roles/serviceusage.serviceUsageAdmin if APIs still need enabling.
set -euo pipefail

: "${PROJECT_ID:?set PROJECT_ID}"
REGION="${REGION:-southamerica-east1}"
SERVICE="${SERVICE:-incident-dashboard}"
PUBLIC="${PUBLIC:-false}"

cd "$(dirname "$0")/.."

gcloud services enable run.googleapis.com cloudbuild.googleapis.com artifactregistry.googleapis.com \
  --project "$PROJECT_ID"

auth_flag="--no-allow-unauthenticated"
[[ "$PUBLIC" == "true" ]] && auth_flag="--allow-unauthenticated"

# State is in-memory: pin to exactly one instance so every request sees the same data,
# and keep it warm so incidents are not wiped by scale-to-zero.
gcloud run deploy "$SERVICE" \
  --project "$PROJECT_ID" \
  --region "$REGION" \
  --source . \
  --min-instances 1 \
  --max-instances 1 \
  --cpu 1 --memory 256Mi \
  --concurrency 80 \
  --timeout 30s \
  "$auth_flag"

url="$(gcloud run services describe "$SERVICE" --project "$PROJECT_ID" --region "$REGION" --format 'value(status.url)')"
echo "Deployed: $url"
echo "Smoke:    curl -fsS $url/healthz   (add -H \"Authorization: Bearer \$(gcloud auth print-identity-token)\" if not public)"
