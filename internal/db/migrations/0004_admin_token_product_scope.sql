-- 0 means all products; a foreign key would reject the sentinel.
ALTER TABLE admin_tokens DROP CONSTRAINT IF EXISTS admin_tokens_product_id_fkey;
