#!/usr/bin/env bash
set -euo pipefail

PROJECT_ID="riojucu-sandbox"
REGION="us-central1"
SERVICE_NAME="psearch-eval-dashboard"
IMAGE_TAG="gcr.io/${PROJECT_ID}/${SERVICE_NAME}:latest"

echo "=== 1. Building Container Image with Cloud Build ==="
gcloud builds submit \
  --project="${PROJECT_ID}" \
  --tag="${IMAGE_TAG}" \
  /workspace/eval-dashboard

echo "=== 2. Deploying to Google Cloud Run ==="
gcloud run deploy "${SERVICE_NAME}" \
  --project="${PROJECT_ID}" \
  --region="${REGION}" \
  --image="${IMAGE_TAG}" \
  --platform="managed" \
  --allow-unauthenticated \
  --set-env-vars="GCP_PROJECT=${PROJECT_ID},PSEARCH_SERVING_URL=https://psearch-serving-mhjw244bgq-uc.a.run.app" \
  --update-secrets="GITHUB_TOKEN=github-token:latest"

echo "=== 3. Obtaining Service URL ==="
SERVICE_URL=$(gcloud run services describe "${SERVICE_NAME}" \
  --project="${PROJECT_ID}" \
  --region="${REGION}" \
  --format='value(status.url)')

echo "Dashboard live at: ${SERVICE_URL}"
