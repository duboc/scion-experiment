#!/bin/bash

# 
# Copyright 2025 Google LLC
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#     https://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Exit on error
set -e

# Load environment variables
source .env

# Variables
PROJECT_ID=${project_id}
REGION="us-central1"
JOB_NAME="product-enrichment-job"
REPOSITORY="cloud-run-jobs"

# Create Artifact Registry repository if it doesn't exist
echo "Ensuring Artifact Registry repository exists..."
gcloud artifacts repositories create $REPOSITORY \
    --repository-format=docker \
    --location=$REGION \
    --project=$PROJECT_ID || true

# Build the container image using Cloud Build
echo "Building container image..."
gcloud builds submit --tag ${REGION}-docker.pkg.dev/$PROJECT_ID/$REPOSITORY/$JOB_NAME

# Create/Update the Cloud Run job
echo "Deploying Cloud Run job..."
gcloud run jobs create $JOB_NAME \
  --image ${REGION}-docker.pkg.dev/$PROJECT_ID/$REPOSITORY/$JOB_NAME \
  --region $REGION \
  --project $PROJECT_ID \
  --set-env-vars="project_id=${project_id}" \
  --set-env-vars="bq_dataset=${bq_dataset}" \
  --set-env-vars="bq_table=${bq_table}" \
  --set-env-vars="firestore_collection=${firestore_collection}" \
  --set-env-vars="psearch_img_bucket=${psearch_img_bucket}" \
  --memory=2Gi \
  --cpu=2 \
  --max-retries=3 \
  --task-timeout=3600 \
  --execute-now

echo "Deployment completed!" 
