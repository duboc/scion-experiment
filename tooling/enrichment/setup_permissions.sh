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
SERVICE_ACCOUNT=${service_account}

# Create service account if it doesn't exist
echo "Creating service account if it doesn't exist..."
gcloud iam service-accounts create product-enrichment \
    --display-name="Product Enrichment Service Account" \
    --project=$PROJECT_ID || true

# Required roles:
# 1. BigQuery Data Viewer - for reading product data
# 2. Cloud Run Invoker - for running the Cloud Run job
# 3. Firestore User - for state management
# 4. Storage Object Creator - for uploading images and exports
# 5. Vertex AI User - for using Imagen and Gemini APIs

echo "Granting necessary IAM roles..."

# BigQuery Data Viewer
gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:${SERVICE_ACCOUNT}" \
    --role="roles/bigquery.dataViewer"

# Cloud Run Invoker
gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:${SERVICE_ACCOUNT}" \
    --role="roles/run.invoker"

# Firestore User
gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:${SERVICE_ACCOUNT}" \
    --role="roles/datastore.user"

# Storage Object Creator
gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:${SERVICE_ACCOUNT}" \
    --role="roles/storage.objectCreator"

# Vertex AI User
gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:${SERVICE_ACCOUNT}" \
    --role="roles/aiplatform.user"

# BigQuery Job User
gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:${SERVICE_ACCOUNT}" \
    --role="roles/bigquery.jobUser"

echo "IAM permissions setup completed!"

# Verify permissions
echo "Verifying IAM permissions..."
gcloud projects get-iam-policy $PROJECT_ID \
    --flatten="bindings[].members" \
    --format='table(bindings.role)' \
    --filter="bindings.members:${SERVICE_ACCOUNT}"
