ALTER TABLE shop_transactions DROP INDEX idx_shop_tx_instance, DROP COLUMN instance_id;
DROP TABLE IF EXISTS inventory_items;
DROP TABLE IF EXISTS inventory_instances;
