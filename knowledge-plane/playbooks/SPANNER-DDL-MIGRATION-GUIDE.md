# Playbook: Cloud Spanner DDL Migrations in `riojucu-sandbox`

- **GCP Project:** `riojucu-sandbox`
- **Spanner Instance:** `psearch-instance` (Enterprise Edition, 100 PU, `regional-us-central1`)
- **Spanner Database:** `psearch-db`
- **Command to apply DDL (run by `@deployer`):**
  ```bash
  gcloud spanner databases ddl update psearch-db \
    --instance=psearch-instance \
    --project=riojucu-sandbox \
    --ddl-file=/workspace/src/iac/migrations/<migration>.sql
  ```
