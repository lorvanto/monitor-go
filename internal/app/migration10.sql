ALTER TABLE usage_settings ADD COLUMN notice_version TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_settings ADD COLUMN consent_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_settings ADD COLUMN activated_at INTEGER NOT NULL DEFAULT 0;
INSERT INTO schema_migrations(version) VALUES(10);
