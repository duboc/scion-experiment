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

# Create a service account for the ingestion task
resource "google_service_account" "ingestion_service_account" {
  account_id   = "ingestion-service-account"
  display_name = "Ingestion Service Account"
  description  = "Service account for ingestion tasks"
  project      = var.project_id
}

# Create a service account for the embeddings function
resource "google_service_account" "embeddings_service_account" {
  account_id   = "embeddings-service-account"
  display_name = "Embeddings Service Account"
  description  = "Service account for embeddings generation function"
  project      = var.project_id
}

# Grant Roles to Service Account
resource "google_project_iam_member" "grant_sa_function_roles" {
  for_each = toset([
    # Existing roles
    "roles/bigquery.user",
    "roles/storage.objectAdmin",
    "roles/run.invoker",
    "roles/cloudbuild.builds.builder",
    "roles/cloudfunctions.developer",
    "roles/iam.serviceAccountUser",
    "roles/run.developer",
    "roles/artifactregistry.reader",
    "roles/serviceusage.serviceUsageConsumer",
    "roles/logging.logWriter",
    "roles/cloudbuild.serviceAgent",
    "roles/datastore.user",

    # Added Vector Search specific roles
    "roles/aiplatform.user",
    "roles/aiplatform.serviceAgent",
    "roles/datastore.owner",

    # Add Spanner roles
    "roles/spanner.databaseUser",
    "roles/spanner.databaseReader",
    "roles/spanner.databaseAdmin"
  ])

  role    = each.key
  project = var.project_id
  member  = "serviceAccount:${google_service_account.ingestion_service_account.email}"
}

# Grant Roles to Embeddings Service Account
resource "google_project_iam_member" "grant_embeddings_sa_roles" {
  for_each = toset([
    "roles/aiplatform.user",
    "roles/aiplatform.serviceAgent",
    "roles/storage.objectViewer",
    "roles/cloudfunctions.invoker",
    "roles/run.invoker",
    "roles/logging.logWriter",
    "roles/serviceusage.serviceUsageConsumer"
  ])

  role    = each.key
  project = var.project_id
  member  = "serviceAccount:${google_service_account.embeddings_service_account.email}"
}

resource "google_project_iam_member" "compute_default_sa_permissions" {
  for_each = toset([
    "roles/logging.logWriter",
    "roles/cloudfunctions.developer",
    "roles/iam.serviceAccountUser",
    "roles/artifactregistry.writer",
    "roles/storage.objectViewer"
  ])

  project = var.project_id
  role    = each.key
  member  = "serviceAccount:${var.project_number}-compute@developer.gserviceaccount.com"
}
