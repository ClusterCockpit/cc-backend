ALTER TABLE tag ADD COLUMN insert_ts TEXT DEFAULT NULL /* replace me */;
ALTER TABLE jobtag ADD COLUMN insert_ts TEXT DEFAULT NULL /* replace me */;
UPDATE tag SET insert_ts = CURRENT_TIMESTAMP;
UPDATE jobtag SET insert_ts = CURRENT_TIMESTAMP;
PRAGMA writable_schema = on;

UPDATE sqlite_master
SET sql = replace(sql, 'DEFAULT NULL /* replace me */',
                       'DEFAULT CURRENT_TIMESTAMP')
WHERE type = 'table'
  AND name = 'tag';
UPDATE sqlite_master
SET sql = replace(sql, 'DEFAULT NULL /* replace me */',
                       'DEFAULT CURRENT_TIMESTAMP')
WHERE type = 'table'
  AND name = 'jobtag';

PRAGMA writable_schema = off;
