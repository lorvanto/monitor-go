ALTER TABLE monitors ADD COLUMN keyword TEXT NOT NULL DEFAULT '' CHECK(length(CAST(keyword AS BLOB))<=1024);
ALTER TABLE monitors ADD COLUMN keyword_invert INTEGER NOT NULL DEFAULT 0 CHECK(keyword_invert IN (0,1));
ALTER TABLE monitors ADD COLUMN latency_warn_ms INTEGER NOT NULL DEFAULT 0 CHECK(latency_warn_ms BETWEEN 0 AND 30000);
INSERT INTO schema_migrations(version) VALUES(7);
