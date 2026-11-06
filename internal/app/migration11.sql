-- Remove the obsolete Console account snapshot; installation identity,
-- credentials, activation consent and local monitoring data remain intact.
ALTER TABLE usage_settings DROP COLUMN account_login;
INSERT INTO schema_migrations(version) VALUES(11);
