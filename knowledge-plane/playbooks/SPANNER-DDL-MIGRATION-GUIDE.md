# Playbook: Cloud Spanner DDL Migrations & SQL Validation (`psearch`)

- **GCP Project:** `riojucu-sandbox` (`249096448072`)
- **Spanner Instance:** `psearch-instance` (Enterprise Edition, `100 PU`, `regional-us-central1`)
- **Spanner Database:** `psearch-db`
- **Cloud Run Service Account:** `psearch-serving@riojucu-sandbox.iam.gserviceaccount.com`
- **Deployer Service Account:** `agent-deployer@riojucu-sandbox.iam.gserviceaccount.com`

## 1. Applying DDL & DML Migrations (`@deployer`)

1. **DDL updates (`src/iac/migrations/0XX_*.sql`):**
   ```bash
   gcloud spanner databases ddl update psearch-db \
     --instance=psearch-instance \
     --project=riojucu-sandbox \
     --ddl-file=/workspace/src/iac/migrations/<migration>.sql
   ```
   *(If a migration contains dependent DDL statements such as adding generated `TOKENLIST` columns and then altering a `SEARCH INDEX`, apply the table `ALTER`/`CREATE` first, then the `SEARCH INDEX` statement).*
2. **DML backfills / seeds (`src/iac/migrations/0XXb_*.dml.sql`):**
   ```bash
   gcloud spanner databases execute-sql psearch-db \
     --instance=psearch-instance \
     --project=riojucu-sandbox \
     --sql="$(grep -v '^--' /workspace/src/iac/migrations/<seed>.dml.sql)"
   ```
3. **Pre-Deploy Live SQL Gate (`cmd/sqlcheck`):**
   Before deploying a new `psearch-serving` revision to Cloud Run, always run the read-only SQL gate against `psearch-db`:
   ```bash
   cd /workspace/src/psearch/serving && go run ./cmd/sqlcheck
   ```

## 2. Hard-Won Cloud Spanner SQL & DDL Rules in `psearch`

1. **No Nested Aggregations in `ORDER BY` (Issue #1/#3):**
   - Spanner rejects `ORDER BY IF(IFNULL(ANY_VALUE(is_available), TRUE), 0, 1)` when `is_available` is also a `SELECT` alias of `ANY_VALUE(is_available)` (`Aggregations of aggregations are not allowed`).
   - Always wrap the `GROUP BY product_id` RRF union in an inner subquery and apply `ORDER BY` in the outer `SELECT` over plain column aliases (`buildHybridSQLWith` in `spanner_service.go`).
2. **`TOKENIZE_NGRAMS` Requires a Plain Column (Issue #9/#21):**
   - Spanner rejects `TOKENIZE_NGRAMS(REGEXP_REPLACE(NORMALIZE(col, NFD), ...))` (`regexp_replace is not supported as a tokenization argument`).
   - Store the accent-folded string in a plain `STRING` column (`suggestion_folded`) and define `TOKENIZE_NGRAMS(suggestion_folded, ngram_size_min=>2, ngram_size_max=>4) HIDDEN` on that column.
3. **`SEARCH_NGRAMS` Cannot Be `OR`-ed with Non-Indexed Predicates in `WHERE` (Issue #9/#21):**
   - Spanner rejects `WHERE SEARCH_NGRAMS(suggestion_ngrams, @q) OR STARTS_WITH(suggestion_folded, @q)` (`SEARCH_NGRAMS is not supported in this query`).
   - Keep `SEARCH_NGRAMS(suggestion_ngrams, @q)` alone in `WHERE` (for inputs of $\ge 2$ runes) and place `IF(STARTS_WITH(suggestion_folded, @q), 0, 1)` in `ORDER BY`.
4. **`INTERLEAVE IN PARENT products` Primary Key Rule (Issue #8/#22):**
   - Because `products` has `PRIMARY KEY (product_id)`, any child table interleaved in `products` (`skus`, `product_prices`) MUST start its primary key with `product_id` (e.g., `PRIMARY KEY (product_id, price_table_id), INTERLEAVE IN PARENT products ON DELETE CASCADE`).
