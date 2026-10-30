CREATE TABLE search_impressions (
  impression_id STRING(64) NOT NULL,
  tenant_id STRING(64) NOT NULL,
  query_raw STRING(512),
  query_norm STRING(512),
  experiment_id STRING(64),
  retrieval_mode STRING(32),
  total_found INT64 NOT NULL,
  latency_ms FLOAT64,
  result_ids ARRAY<STRING(1024)>,
  created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true)
) PRIMARY KEY (tenant_id, impression_id),
  ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY));
CREATE INDEX search_impressions_by_tenant_time
ON search_impressions(tenant_id, created_at DESC)
STORING (query_norm, experiment_id, total_found, latency_ms);
CREATE TABLE search_conversions (
  tenant_id STRING(64) NOT NULL,
  event_id STRING(64) NOT NULL,
  impression_id STRING(64) NOT NULL,
  session_id STRING(64),
  event_type STRING(24) NOT NULL,
  product_id STRING(1024) NOT NULL,
  position INT64 NOT NULL,
  experiment_id STRING(64),
  revenue_micros INT64 NOT NULL DEFAULT (0),
  created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true)
) PRIMARY KEY (tenant_id, event_id),
  ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY));
CREATE INDEX search_conversions_by_impression
ON search_conversions(tenant_id, impression_id, event_type)
STORING (product_id, position, revenue_micros)
