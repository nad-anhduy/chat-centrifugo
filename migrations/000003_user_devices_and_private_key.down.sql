DROP TABLE IF EXISTS user_device_changed;
DROP TABLE IF EXISTS user_devices;

ALTER TABLE users DROP COLUMN IF EXISTS private_key;
