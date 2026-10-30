-- Issue #42 seed (run AFTER 025_product_attributes.sql). Mirrors services.DefaultProductAttributes().
INSERT OR UPDATE INTO product_attributes (product_id, attr_name, attr_value, tenant_id)
SELECT p.product_id, v.attr_name, v.attr_value, IFNULL(p.tenant_id, 'default')
FROM UNNEST([
  STRUCT('prod-001' AS product_id, 'surface' AS attr_name, 'Road' AS attr_value),
  STRUCT('prod-001', 'surface', 'Track'),
  STRUCT('prod-001', 'cushioning', 'Maximum'),
  STRUCT('prod-001', 'drop', '10mm'),
  STRUCT('prod-002', 'surface', 'Road'),
  STRUCT('prod-002', 'cushioning', 'Maximum'),
  STRUCT('prod-002', 'drop', '8mm'),
  STRUCT('prod-002', 'fps', 'FPS 50'),
  STRUCT('prod-002', 'skin_type', 'Pele Oleosa'),
  STRUCT('prod-002', 'form', 'Gel-Creme'),
  STRUCT('prod-003', 'surface', 'Trail'),
  STRUCT('prod-003', 'cushioning', 'Responsive'),
  STRUCT('prod-003', 'drop', '9mm'),
  STRUCT('prod-003', 'fps', 'FPS 70'),
  STRUCT('prod-003', 'skin_type', 'Todos os Tipos'),
  STRUCT('prod-003', 'form', 'Fluido'),
  STRUCT('prod-004', 'surface', 'Casual'),
  STRUCT('prod-004', 'cushioning', 'Standard'),
  STRUCT('prod-004', 'drop', '6mm'),
  STRUCT('prod-005', 'ram', '32GB'),
  STRUCT('prod-005', 'storage', '1TB'),
  STRUCT('prod-005', 'processor', 'Intel Core Ultra 7'),
  STRUCT('prod-005', 'screen_size', '14"'),
  STRUCT('prod-006', 'ram', '32GB'),
  STRUCT('prod-006', 'storage', '1TB'),
  STRUCT('prod-006', 'processor', 'Intel Core i9'),
  STRUCT('prod-006', 'screen_size', '16"'),
  STRUCT('prod-007', 'ram', '16GB'),
  STRUCT('prod-007', 'storage', '512GB'),
  STRUCT('prod-007', 'processor', 'Apple M3'),
  STRUCT('prod-007', 'screen_size', '15"'),
  STRUCT('prod-008', 'connectivity', 'Bluetooth 5.3'),
  STRUCT('prod-008', 'battery_life', '24h'),
  STRUCT('prod-008', 'noise_cancelling', 'Active ANC'),
  STRUCT('prod-009', 'connectivity', 'Bluetooth 5.2'),
  STRUCT('prod-009', 'battery_life', '30h'),
  STRUCT('prod-009', 'noise_cancelling', 'Active ANC'),
  STRUCT('prod-010', 'ram', '16GB'),
  STRUCT('prod-010', 'storage', '512GB'),
  STRUCT('prod-010', 'connectivity', '5G'),
  STRUCT('prod-011', 'voltage', '220V'),
  STRUCT('prod-011', 'voltage', 'Bivolt'),
  STRUCT('prod-011', 'energy_rating', 'A+++'),
  STRUCT('prod-011', 'capacity', '540L'),
  STRUCT('prod-012', 'voltage', '110V'),
  STRUCT('prod-012', 'voltage', '220V'),
  STRUCT('prod-012', 'energy_rating', 'Energy Star'),
  STRUCT('prod-012', 'capacity', '600L'),
  STRUCT('prod-013', 'voltage', '110V'),
  STRUCT('prod-013', 'voltage', '220V'),
  STRUCT('prod-013', 'energy_rating', 'A+'),
  STRUCT('prod-013', 'pressure', '15 Bar'),
  STRUCT('prod-014', 'screen_size', '65"'),
  STRUCT('prod-014', 'resolution', '4K OLED'),
  STRUCT('prod-014', 'refresh_rate', '120Hz'),
  STRUCT('prod-015', 'connectivity', 'Bluetooth'),
  STRUCT('prod-015', 'connectivity', 'USB-C'),
  STRUCT('prod-015', 'switch_type', 'Tactile Mechanical')
]) AS v
JOIN products AS p ON p.product_id = v.product_id
UNION ALL
SELECT p.product_id, a.attr_name, a.attr_value, IFNULL(p.tenant_id, 'default')
FROM (
  SELECT product_id, tenant_id, ROW_NUMBER() OVER (ORDER BY product_id) AS rn
  FROM products
  WHERE LOWER(IFNULL(title, '')) LIKE '%protetor%'
    AND NOT STARTS_WITH(product_id, 'prod-0')
  LIMIT 150
) AS p
CROSS JOIN UNNEST([
  STRUCT(0 AS mod_val, 'fps' AS attr_name, 'FPS 30' AS attr_value),
  STRUCT(0, 'skin_type', 'Pele Mista'),
  STRUCT(0, 'form', 'Loção'),
  STRUCT(1, 'fps', 'FPS 50'),
  STRUCT(1, 'skin_type', 'Pele Oleosa'),
  STRUCT(1, 'form', 'Gel-Creme'),
  STRUCT(2, 'fps', 'FPS 70'),
  STRUCT(2, 'skin_type', 'Todos os Tipos'),
  STRUCT(2, 'form', 'Fluido')
]) AS a
WHERE MOD(p.rn, 3) = a.mod_val
