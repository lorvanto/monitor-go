-- Certificate dates are structured milliseconds, not parsed diagnostic text.
-- Zero means no verified certificate has been observed for this result.
ALTER TABLE monitors ADD COLUMN certificate_expires_at INTEGER NOT NULL DEFAULT 0 CHECK(certificate_expires_at>=0);
ALTER TABLE check_results ADD COLUMN certificate_expires_at INTEGER NOT NULL DEFAULT 0 CHECK(certificate_expires_at>=0);
INSERT INTO schema_migrations(version) VALUES(12);
