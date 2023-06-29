ALTER TABLE configuration ADD COLUMN value_new TEXT;
INSERT INTO configuration (value_new) SELECT value FROM configuration;
ALTER TABLE configuration DROP COLUMN  value;
ALTER TABLE configuration RENAME COLUMN value_new TO value;
