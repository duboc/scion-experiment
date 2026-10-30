CREATE VECTOR INDEX products_embedding_v2_idx
ON products(embedding_v2)
STORING (is_visible, is_available, stock, tenant_id, locale, category, brand, price)
WHERE embedding_v2 IS NOT NULL
OPTIONS (distance_type = 'COSINE', tree_depth = 2, num_leaves = 100)
