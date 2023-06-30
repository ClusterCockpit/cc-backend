CREATE TABLE IF NOT EXISTS configuration_new (
username varchar(255),
confkey  varchar(255),
value    varchar(255),
PRIMARY KEY (username, confkey),
FOREIGN KEY (username) REFERENCES user (username) ON DELETE CASCADE ON UPDATE NO ACTION);

INSERT INTO configuration_new SELECT * FROM configuration;
DROP TABLE configuration;
ALTER TABLE configuration_new RENAME TO configuration;
