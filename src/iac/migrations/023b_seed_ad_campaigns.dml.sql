-- Issue #35 seed (run AFTER 023_ad_campaigns.sql). Mirrors services.DefaultAdCampaigns().
INSERT OR UPDATE INTO ad_campaigns (campaign_id, tenant_id, advertiser, product_id, cpc_micros, budget_micros, spent_micros, status, start_at, end_at, updated_at) VALUES
  ('camp-nike-pegasus', 'default', 'Nike Retail Media', 'prod-003', 400000, 5000000, 0, 'active', NULL, NULL, PENDING_COMMIT_TIMESTAMP()),
  ('camp-asics-nimbus', 'default', 'Asics Brasil', 'prod-002', 250000, 4000000, 0, 'active', NULL, NULL, PENDING_COMMIT_TIMESTAMP()),
  ('camp-apple-macbook', 'default', 'Apple Enterprise', 'prod-007', 500000, 10000000, 0, 'active', NULL, NULL, PENDING_COMMIT_TIMESTAMP()),
  ('camp-acer-predator', 'default', 'Acer Gaming', 'prod-006', 350000, 6000000, 0, 'active', NULL, NULL, PENDING_COMMIT_TIMESTAMP()),
  ('camp-bose-qc', 'default', 'Bose Audio', 'prod-008', 300000, 4500000, 0, 'active', NULL, NULL, PENDING_COMMIT_TIMESTAMP())
