ALTER TABLE products ADD COLUMN attributes JSON;
CREATE TABLE product_attributes (
  product_id STRING(1024) NOT NULL,
  attr_name STRING(64) NOT NULL,
  attr_value STRING(256) NOT NULL,
  attr_value_lower STRING(256) AS (LOWER(attr_value)) STORED,
  tenant_id STRING(64) NOT NULL DEFAULT ('default')
) PRIMARY KEY (product_id, attr_name, attr_value),
  INTERLEAVE IN PARENT products ON DELETE CASCADE;
CREATE INDEX product_attributes_by_tenant_name_val
ON product_attributes(tenant_id, attr_name, attr_value_lower)
