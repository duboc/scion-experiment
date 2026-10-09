CREATE TABLE products (
  product_id STRING(1024) NOT NULL,
  product_data JSON,
  title STRING(1024),
  description STRING(MAX),
  embedding VECTOR(768),
  timestamp TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (product_id);

CREATE VECTOR INDEX products_embedding_idx
ON products(embedding)
OPTIONS (distance_type = 'COSINE');

CREATE SEARCH INDEX products_text_idx
ON products(title, description);
