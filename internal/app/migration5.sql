CREATE INDEX jobs_queue ON jobs(id) WHERE state='queued';
INSERT INTO schema_migrations(version) VALUES(5);
