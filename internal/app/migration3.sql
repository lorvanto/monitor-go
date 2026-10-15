CREATE TABLE menus(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES menus(id), name TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('group','page','link')), page_key TEXT NOT NULL DEFAULT '', external_url TEXT NOT NULL DEFAULT '', icon TEXT NOT NULL DEFAULT 'LayoutDashboard', sort INTEGER NOT NULL DEFAULT 0, visible INTEGER NOT NULL DEFAULT 1, permission TEXT NOT NULL DEFAULT '', builtin INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1, deleted_at INTEGER);
CREATE INDEX menus_parent ON menus(parent_id,sort,id);
INSERT INTO menus(id,name,kind,icon,sort,builtin) VALUES(1,'观测与运维','group','Activity',10,1),(2,'文件与资源','group','FolderOpen',20,1),(3,'系统管理','group','Settings2',30,1);
INSERT INTO menus(id,parent_id,name,kind,page_key,icon,sort,builtin) VALUES
(10,1,'监控总览','page','overview','LayoutDashboard',10,1),
(11,1,'主机详情','page','host','Server',20,1),
(12,1,'服务检测','page','monitors','Radar',30,1),
(13,1,'告警中心','page','alerts','Bell',40,1),
(14,2,'文件管理','page','files','FolderOpen',10,1),
(15,3,'角色与权限','page','roles','ShieldCheck',10,1),
(16,3,'菜单管理','page','menus','ListTree',20,1),
(17,3,'操作审计','page','audit','ScrollText',30,1),
(18,3,'系统设置','page','settings','Settings2',40,1);
CREATE TABLE files(id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES files(id), kind TEXT NOT NULL CHECK(kind IN ('folder','file')), name TEXT NOT NULL, object_key TEXT NOT NULL DEFAULT '', mime TEXT NOT NULL DEFAULT '', size INTEGER NOT NULL DEFAULT 0, sha256 TEXT NOT NULL DEFAULT '', width INTEGER NOT NULL DEFAULT 0, height INTEGER NOT NULL DEFAULT 0, cover_id INTEGER REFERENCES files(id), created_by INTEGER REFERENCES users(id), created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, version INTEGER NOT NULL DEFAULT 1, deleted_at INTEGER, trash_root INTEGER REFERENCES files(id), purged_at INTEGER);
CREATE UNIQUE INDEX file_names ON files(parent_id,name COLLATE NOCASE) WHERE deleted_at IS NULL AND purged_at IS NULL;
CREATE UNIQUE INDEX file_objects ON files(object_key) WHERE object_key <> '';
CREATE INDEX files_parent ON files(parent_id,deleted_at);
INSERT INTO files(id,parent_id,kind,name,created_at,updated_at) VALUES(1,NULL,'folder','全部文件',0,0),(2,1,'folder','未分类',0,0);
CREATE TABLE image_styles(id INTEGER PRIMARY KEY, name TEXT NOT NULL, width INTEGER NOT NULL, height INTEGER NOT NULL, fit TEXT NOT NULL CHECK(fit IN ('contain','cover')), version INTEGER NOT NULL DEFAULT 1, deleted_at INTEGER);
INSERT INTO image_styles(name,width,height,fit) VALUES('列表缩略图',320,240,'cover'),('文档插图',1200,900,'contain');
CREATE TABLE system_settings(id INTEGER PRIMARY KEY CHECK(id=1), instance_name TEXT NOT NULL DEFAULT '我的监控中心', upload_limit_mb INTEGER NOT NULL DEFAULT 32, storage_limit_mb INTEGER NOT NULL DEFAULT 1024, default_folder INTEGER NOT NULL REFERENCES files(id) DEFAULT 2, audit_days INTEGER NOT NULL DEFAULT 90, version INTEGER NOT NULL DEFAULT 1);
INSERT INTO system_settings(id) VALUES(1);
ALTER TABLE audit_logs ADD COLUMN request_id TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN method TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN path TEXT NOT NULL DEFAULT '';
CREATE INDEX audit_time ON audit_logs(at,id);
INSERT OR IGNORE INTO role_permissions(role_id,permission) SELECT id,'menu:manage' FROM roles WHERE id=1 AND builtin=1;
INSERT OR IGNORE INTO role_permissions(role_id,permission) SELECT id,'file:read' FROM roles WHERE id=1 AND builtin=1;
INSERT OR IGNORE INTO role_permissions(role_id,permission) SELECT id,'file:upload' FROM roles WHERE id=1 AND builtin=1;
INSERT OR IGNORE INTO role_permissions(role_id,permission) SELECT id,'file:manage' FROM roles WHERE id=1 AND builtin=1;
INSERT OR IGNORE INTO role_permissions(role_id,permission) SELECT id,'file:download' FROM roles WHERE id=1 AND builtin=1;
INSERT OR IGNORE INTO role_permissions(role_id,permission) SELECT id,'file:styles' FROM roles WHERE id=1 AND builtin=1;
INSERT OR IGNORE INTO role_permissions(role_id,permission) SELECT id,'audit:read' FROM roles WHERE id=1 AND builtin=1;
INSERT OR IGNORE INTO role_permissions(role_id,permission) SELECT id,'system:read' FROM roles WHERE id=1 AND builtin=1;
INSERT OR IGNORE INTO role_permissions(role_id,permission) SELECT id,'system:settings' FROM roles WHERE id=1 AND builtin=1;
DELETE FROM sessions WHERE user_id IN (SELECT user_id FROM user_roles WHERE role_id=1);
INSERT INTO schema_migrations(version) VALUES(3);
