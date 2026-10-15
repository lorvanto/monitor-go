ALTER TABLE system_settings ADD COLUMN host_cpu_percent INTEGER NOT NULL DEFAULT 90 CHECK(host_cpu_percent BETWEEN 0 AND 100);
ALTER TABLE system_settings ADD COLUMN host_memory_percent INTEGER NOT NULL DEFAULT 90 CHECK(host_memory_percent BETWEEN 0 AND 100);
ALTER TABLE system_settings ADD COLUMN host_disk_percent INTEGER NOT NULL DEFAULT 85 CHECK(host_disk_percent BETWEEN 0 AND 100);
ALTER TABLE system_settings ADD COLUMN host_alert_minutes INTEGER NOT NULL DEFAULT 5 CHECK(host_alert_minutes BETWEEN 1 AND 60);
INSERT INTO schema_migrations(version) VALUES(6);
