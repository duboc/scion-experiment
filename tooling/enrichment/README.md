# Enrich catalog products with Vertex AI

This directory contains a batch enrichment tool that generates product images and descriptions from BigQuery catalog rows using Vertex AI Gemini models, Cloud Storage, and Firestore.
This document follows the Google Developer documentation style and the ASD-STE100 Simplified Technical English standard.

> [!IMPORTANT]
> Do not commit real project IDs, bucket names, or service account emails to version control.
> Replace placeholders such as `PROJECT_ID` and `BUCKET_NAME` in your local `.env` file.

## Overview

The enrichment pipeline (`main.py`) reads product records from BigQuery and runs a fault-tolerant batch workflow:

1. **Image generation (`imagen_client.py`):** Generates product images with `gemini-3.1-flash-image` via the Google Gen AI SDK.
2. **Description generation (`gemini_client.py`):** Generates detailed product descriptions with `gemini-3.8-flash` (`GENAI_LOCATION=global`).
3. **Asset upload:** Uploads generated images to a Cloud Storage bucket.
4. **State checkpointing (`firestore_client.py`):** Records per-product status in Firestore and retries failed items up to 3 times.
5. **Report consolidation (`consolidate_results.py`):** Merges BigQuery source rows with Firestore status documents into a consolidated CSV report in Cloud Storage.

## Before you begin

1. Enable the Vertex AI, BigQuery, Firestore, Cloud Storage, and Cloud Run APIs in your Google Cloud project.
2. Install Python 3.11 or later and `pip`.
3. Grant the following IAM roles to the runtime service account:
   - `roles/bigquery.dataViewer`
   - `roles/datastore.user` (Firestore User)
   - `roles/storage.objectCreator`
   - `roles/aiplatform.user`

You can run `./setup_permissions.sh` to grant these roles to your service account.

## Configure the tool

1. Open a terminal in `tooling/enrichment`:

   ```bash
   cd tooling/enrichment
   ```

2. Copy `.env.example` to `.env`:

   ```bash
   cp .env.example .env
   ```

3. Set the required variables in `.env`:

   | Variable | Example placeholder | Description |
   |---|---|---|
   | `project_id` | `PROJECT_ID` | Google Cloud project ID. |
   | `bq_dataset` | `psearch_dataset` | Source BigQuery dataset ID. |
   | `bq_table` | `products_raw` | Source BigQuery table name. |
   | `psearch_img_bucket` | `BUCKET_NAME` | Cloud Storage bucket for generated images and CSV reports. |
   | `firestore_collection` | `product_enrichment` | Firestore collection that stores per-product job state. |
   | `firestore_database` | `(default)` | Firestore database ID. |
   | `service_account` | `SA_NAME@PROJECT_ID.iam.gserviceaccount.com` | Service account used by the Cloud Run Job. |

4. Install the Python dependencies:

   ```bash
   python3 -m pip install -r requirements.txt
   ```

## Run the enrichment workflow

### Run locally

1. Execute the batch enrichment script:

   ```bash
   python3 main.py
   ```

2. Generate and upload the consolidated CSV status report:

   ```bash
   python3 consolidate_results.py
   ```

### Deploy as a Cloud Run Job

Run `deploy.sh` to build the container image and deploy the Cloud Run Job (configured with 2 CPU cores, 2 GiB RAM, 1-hour timeout, and 3 retries):

```bash
chmod +x deploy.sh
./deploy.sh
```

## Troubleshoot common issues

| Symptom | Cause | Solution |
|---|---|---|
| Vertex AI returns `429 RESOURCE_EXHAUSTED` | Batch concurrency exceeds model quota. | Re-run `python3 main.py`; Firestore state skips completed items and retries failed items up to 3 times. |
| Product status is `permanently_failed` | A product failed 3 times during image or text generation. | Inspect the error field on the Firestore document for that `product_id` and delete or reset the document to retry. |
| `404 Not Found` on Gemini model calls | Vertex AI region does not serve Gemini 3.x models. | Set `GENAI_LOCATION=global` in the environment. |

## What's next

- Export enriched products and 1024-dimension embeddings to Cloud Spanner using [`../../src/iac/bigquery_scripts/transform_embed_export_to_spanner.sql`](../../src/iac/bigquery_scripts/transform_embed_export_to_spanner.sql).
- Backfill missing `embedding_v2` vectors in Cloud Spanner with `cmd/backfill-embeddings` (see [`../../README.md`](../../README.md)).
