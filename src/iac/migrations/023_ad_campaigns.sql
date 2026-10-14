CREATE TABLE ad_campaigns (
  campaign_id STRING(64) NOT NULL,
  tenant_id STRING(64) NOT NULL,
  advertiser STRING(128),
  product_id STRING(64) NOT NULL,
  cpc_micros INT64 NOT NULL,
  budget_micros INT64 NOT NULL,
  spent_micros INT64 NOT NULL DEFAULT (0),
  status STRING(16) NOT NULL,
  start_at TIMESTAMP,
  end_at TIMESTAMP,
  updated_at TIMESTAMP OPTIONS (allow_commit_timestamp = true)
) PRIMARY KEY (campaign_id);
CREATE INDEX ad_campaigns_by_tenant_product ON ad_campaigns(tenant_id, product_id) STORING (cpc_micros, budget_micros, spent_micros, status, start_at, end_at);
CREATE TABLE ad_clicks (
  campaign_id STRING(64) NOT NULL,
  click_id STRING(64) NOT NULL,
  product_id STRING(64),
  cost_micros INT64,
  clicked_at TIMESTAMP OPTIONS (allow_commit_timestamp = true)
) PRIMARY KEY (campaign_id, click_id),
  INTERLEAVE IN PARENT ad_campaigns ON DELETE CASCADE
