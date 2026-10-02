-- Apply before initialization/upgrade; this role never belongs to a service.
CREATE ROLE IF NOT EXISTS 'configra_migrate';
GRANT SELECT, INSERT, CREATE, ALTER, REFERENCES ON `configra`.* TO 'configra_migrate';
