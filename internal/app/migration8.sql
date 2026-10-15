ALTER TABLE check_results ADD COLUMN rolled_up INTEGER NOT NULL DEFAULT 0 CHECK(rolled_up IN (0,1));
CREATE INDEX check_results_pending_rollup ON check_results(at,monitor_id) WHERE rolled_up=0;
CREATE TABLE check_hourly(
 monitor_id INTEGER NOT NULL REFERENCES monitors(id),
 hour INTEGER NOT NULL CHECK(hour%3600000=0),
 total INTEGER NOT NULL CHECK(total>0),
 good INTEGER NOT NULL CHECK(good BETWEEN 0 AND total),
 latency_avg REAL NOT NULL CHECK(latency_avg>=0),
 latency_max REAL NOT NULL CHECK(latency_max>=0),
 PRIMARY KEY(monitor_id,hour)
);
CREATE INDEX check_hourly_time ON check_hourly(hour);
ALTER TABLE system_settings ADD COLUMN raw_retention_days INTEGER NOT NULL DEFAULT 7 CHECK(raw_retention_days BETWEEN 1 AND 30);
ALTER TABLE system_settings ADD COLUMN summary_retention_days INTEGER NOT NULL DEFAULT 90 CHECK(summary_retention_days BETWEEN 90 AND 730);
INSERT INTO schema_migrations(version) VALUES(8);
