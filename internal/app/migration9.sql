CREATE TABLE usage_settings(
 id INTEGER PRIMARY KEY CHECK(id=1),
 enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)),
 console_url TEXT NOT NULL DEFAULT '',
 instance_id TEXT NOT NULL DEFAULT '',
 credential TEXT NOT NULL DEFAULT '',
 registered INTEGER NOT NULL DEFAULT 0 CHECK(registered IN (0,1)),
 last_report_at INTEGER NOT NULL DEFAULT 0,
 next_report_at INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',
 account_login TEXT NOT NULL DEFAULT '',
 failures INTEGER NOT NULL DEFAULT 0 CHECK(failures>=0)
);
INSERT INTO usage_settings(id) VALUES(1);
CREATE TABLE usage_activity(day TEXT PRIMARY KEY CHECK(length(day)=10));
INSERT INTO schema_migrations(version) VALUES(9);
