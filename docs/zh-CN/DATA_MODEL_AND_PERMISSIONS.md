# 数据模型与权限设计

> 文档类型：设计方案；表与权限的完整规划不等于现行数据库结构。当前使用方式以 [使用指南](README.md) 为准，公开发布流程见 [GitHub 发布指南](GITHUB.md)。

本方案基于内置 SQLite 文件数据库，面向一套安装内的多人协作。业务资源共享，用户和角色控制操作范围。表名、字段和表拆分属于建议，实施迁移前进一步确定类型与索引。

## 当前 v4 实现

实际迁移源见 `internal/app/schema.sql`、`migration2.sql`、`migration3.sql` 和 `migration4.sql`。以下表为本轮已实现，其余完整产品表设计仍是规划。

| 表 | 本轮实际职责 |
|---|---|
| `applications` | 应用标识、名称、说明、启停、版本与软归档 |
| `api_keys` | 应用关联、摘要与前缀、范围 JSON、到期/最近使用/撤销时间、版本 |
| `app_releases` | 应用、版本号、平台、架构、文件/HTTPS 地址二选一、发布状态、下载请求计数、软归档 |
| `menus` | 自引用导航树、分组/页面/外链、内置标记、展示权限、排序、版本和软删除 |
| `files` | 统一目录与文件元数据、随机对象键、SHA-256、尺寸、创建人、版本、回收批次与永久删除墓碑 |
| `image_styles` | 名称、宽高、contain/cover、版本与软归档 |
| `system_settings` | 单行强类型实例配置：名称、上传上限、存储配额、默认目录、审计天数、版本 |
| `audit_logs` | 在原日志上增加服务端请求编号、HTTP 方法、接口路径与时间索引 |

根目录使用固定记录（id=1），避免空父级绕过目录内唯一约束；默认上传目录为 id=2。有效文件/目录名称通过 `WHERE deleted_at IS NULL AND purged_at IS NULL` 的部分唯一索引约束。磁盘对象路径与用户文件名无关，单次上传最终入库时在事务内重新检查目录及配额。回收站按 `trash_root` 恢复一个完整删除批次，物理回收完成后清空对象键，保留元数据墓碑。

v3 新增 9 项权限，v4 再新增 7 项，现共 27 项。文件资源是实例内共享，预览、上传、下载、管理、图片样式分别授权。菜单显隐与接口权限独立；审计列表及总览摘要均需 `audit:read`。升级仅给内置超级管理员新增权限，并撤销其旧会话。实际 API 返回毫秒时间戳；下文标准时间字符串等建议不覆盖当前协议。

## 设计原则

- 所有表不包含租户字段。
- 固定字段用于查询、排序和约束；JSON 只存协议差异、展示设置或结构经过校验的扩展内容。
- 用户和角色使用明确关系表；不把角色编号或权限编号拼接成字符串。
- 分组、检查记录、告警事件统一建模；主机指标和证书订单因生命周期不同单独建模。
- 状态、默认值和约束集中定义。前端通过接口获取需要展示的枚举，不自行猜测数字含义。
- 业务配置保留软删除；指标和执行记录使用可配置的过期清理。清理历史不删除监控目标。
- 事务限于一次业务一致性操作；网络探测、文件上传和通知发送不长时间占用数据库事务。

## 通用字段

业务主键建议使用 64 位整数，API 以字符串传递标识，避免浏览器整数精度问题。时间戳使用 UTC 毫秒的 64 位整数，API 返回带时区的标准时间字符串；任务的日历时区单独存 IANA 时区名称。

可编辑配置使用 `created_at`、`updated_at`、`created_by`、`updated_by` 和 `version`。软删资源使用 `deleted_at`。流水记录只保存所需字段，不复制全部通用列。

所有依赖关系在可用时建立外键。按类型引用多类资源的字段由业务服务验证类型、存在性和删除限制，写接口不得直接相信客户端传入的资源标识。

业务唯一键不靠包含可空 `deleted_at` 的复合唯一索引实现。用户名和稳定代码在软删除后仍保留，支持恢复；可重用名称使用针对有效记录的部分唯一索引。文件根目录名称约束需规范化空父级，实施时覆盖同名与恢复冲突测试。

## 账号与菜单

| 表 | 主要字段 | 约束与用途 |
|---|---|---|
| `users` | username、display_name、password_hash、status、auth_version | 用户名规范化后唯一；密码独立加盐哈希 |
| `sessions` | user_id、token_hash、expires_at、last_seen_at、revoked_at | 保存随机会话令牌摘要；支持主动注销和踢下线 |
| `roles` | code、name、description、is_builtin、status | 稳定角色代码唯一；内置角色有修改边界 |
| `user_roles` | user_id、role_id | 两字段联合唯一，支持一个用户多个角色 |
| `role_permissions` | role_id、permission_code | 两字段联合唯一，权限代码必须来自注册表 |
| `menus` | parent_id、name、page_key、external_url、icon、sort、visible、permission_code | 父子关系不能成环，页面必须已注册 |

权限注册表随代码发布，包含权限代码、中文名称、所属模块和风险级别。API 提供权限树供管理员勾选，无需维护一张人工编辑的 API 路由表。

菜单有目录、内部页面、外链三种类型。内部页面通过固定 `page_key` 关联前端组件；菜单配置不能上传或执行任意组件代码。按钮显示直接检查动作权限，按钮不是隐藏菜单。

## 文件与应用

| 表 | 主要字段 | 约束与用途 |
|---|---|---|
| `storage_backends` | name、driver、config_json、secret_ref、enabled、is_default | 首版实现本地存储；其他驱动按实际需求添加 |
| `files` | parent_id、kind、storage_id、object_key、name、mime、size、sha256、visibility、metadata_json | 目录与文件统一元数据；对象键由服务器生成 |
| `image_styles` | code、name、transform_json、enabled | 允许受限的图片处理参数，不保存可执行表达式 |
| `api_clients` | name、description、enabled、owner_user_id | 外部应用身份，与浏览器用户身份分开 |
| `api_keys` | client_id、key_prefix、secret_hash、scopes_json、expires_at、revoked_at | 明文只在创建时展示；权限范围必须来自注册表 |
| `app_releases` | client_id、name、version、platform、file_id、download_url、download_count | file_id 与外部下载地址按规则择一，保留安装包管理能力 |
| `settings` | key、value_json、schema_version、updated_by | 固定键与校验规则；公开配置和密钥配置分开 |
| `audit_logs` | actor_type、actor_id、action、resource_type、resource_id、result、request_id、metadata_json | 记录审计摘要，敏感值不入日志 |

文件树与实际磁盘目录解耦，重命名或移动只更新元数据。真实对象读取必须先经过文件授权。用户上传的 HTML 等主动内容默认按附件下载，公开分享须有明确设置。

密钥引用指向受保护的加密资料；实现时可由独立密钥存储组件管理，不能把解密主密钥与密文写入同一数据库。文件垃圾回收由独立任务处理，数据库操作失败不得误删仍被引用的文件。

## 监控资源

| 表 | 主要字段 | 约束与用途 |
|---|---|---|
| `resource_groups` | name、description、kind、sort | 可区分主机或检测分组，共用一套分组结构 |
| `enrollment_tokens` | token_hash、expires_at、max_uses、used_count、capabilities_json、revoked_at | 安装注册凭据，原子扣减可用次数 |
| `agents` | uid、name、credential_hash、capabilities_json、version、last_seen_at、enabled | 一套 Agent 身份，可启用主机采集或服务探测 |
| `hosts` | agent_id、group_id、hostname、alias、inventory_json、last_snapshot_json、last_seen_at、enabled | agent_id 唯一；主机识别以服务端 Agent 身份为准 |
| `monitors` | group_id、name、type、endpoint、agent_id、host_id、interval_seconds、timeout_seconds、config_json、last_state、next_check_at | HTTP、TCP、MySQL、Redis、SSL 统一目标；host_id 可选，用于关联展示 |

`monitors.type` 使用明确枚举。`agent_id` 为空表示由服务端执行，非空表示分配给指定 Agent。`config_json` 按检测类型校验：HTTP 请求条件、TCP 端口、数据库连接选项、SSL 预警阈值等。凭据使用敏感资料引用。

主机在线状态基于服务端接收时间计算。Agent 报告的采样时间用于曲线，同时检查时钟偏差；机器时钟异常不能让设备永久显示在线。

## 指标和检查历史

| 表 | 主要字段 | 核心索引与用途 |
|---|---|---|
| `host_metric_samples` | host_id、agent_boot_id、sequence、sampled_at、received_at、cpu、memory、load、network、disks_json | 幂等键 host_id + boot_id + sequence；查询索引 host_id + sampled_at |
| `host_metric_rollups` | host_id、bucket_start、bucket_seconds、sample_count、metrics_json | host_id + bucket_start + bucket_seconds 唯一 |
| `check_results` | monitor_id、run_key、checked_at、received_at、state、latency_ms、error_code、detail_json | monitor_id + run_key 唯一；monitor_id + checked_at 索引 |
| `check_rollups` | monitor_id、bucket_start、bucket_seconds、success_count、failure_count、unknown_count、latency_sum、latency_max、sample_count | 按目标和时间桶唯一，可加权合并 |

主机指标一条记录包含一次主机采样；磁盘明细按稳定挂载点和文件系统标识保存，聚合时按同一标识计算，不能把不同磁盘混在一起。常用主机指标保留固定列，明细采用经过校验的结构。

平均值根据总和与样本数计算，不能平均多个平均值。图表明确显示聚合粒度。没有采样的区间显示缺失，不用 0 填充。首版可用率定义为成功次数除以成功与失败次数之和，缺失和未知单独显示，不宣传为精确时间 SLA。

## 告警和通知

| 表 | 主要字段 | 约束与用途 |
|---|---|---|
| `alert_rules` | name、scope_kind、resource_kind、resource_id、group_id、condition_json、priority、repeat_seconds、enabled | 指定明确匹配范围和优先级 |
| `alert_rule_channels` | rule_id、channel_id | 联合唯一，渠道引用可以做外键约束 |
| `alert_events` | resource_kind、resource_id、rule_id、fingerprint、active_key、state、opened_at、recovered_at、ack_by、ack_at、silenced_until | 同一条件最多一个活动事件；恢复后保留历史 |
| `notification_channels` | name、type、config_json、secret_ref、enabled | 企业微信、钉钉、飞书、邮件和通用 Webhook |
| `notification_deliveries` | event_id、channel_id、transition_key、payload_snapshot、status、attempt_count、next_attempt_at、last_error | 同时作为持久投递队列和投递记录 |
| `maintenance_windows` | name、scope_kind、resource_kind、resource_id、group_id、starts_at、ends_at、timezone、recurrence_json、enabled | 规则匹配范围与告警规则一致 |

`active_key` 在事件活动时设为稳定指纹，恢复后置空，配合唯一索引防止并发重复建活动事件；在 SQLite 文件数据库中验证空值与并发行为。通知通过 event_id + channel_id + transition_key 的唯一约束避免重复入队。

告警确认、静默和维护窗口不改写真实检测状态。事件恢复与发送恢复通知分别记录；通知失败不撤销告警事件。

## 证书和任务

| 表 | 主要字段 | 约束与用途 |
|---|---|---|
| `acme_accounts` | directory_url、email、account_url、key_ref、status | 账号密钥单独保护；同服务商与账户身份去重 |
| `certificates` | monitor_id、account_id、domains_json、not_before、not_after、certificate_ref、private_key_ref、renewal_mode、status | 与 SSL 检测可选关联，签发与线上证书状态分开 |
| `certificate_orders` | certificate_id、account_id、order_url、state、challenge_json、expires_at、last_error | 保存创建、挑战、签发过程和可恢复状态 |
| `jobs` | type、resource_ref、payload_json、idempotency_key、state、run_after、lease_until、attempts | 所有工作类型来自白名单；幂等键唯一 |
| `job_runs` | job_id、started_at、finished_at、result、error_code、summary_json | 执行记录，支持列表和失败排查 |

以上为逻辑表建议，并非已建表。基础表与历史表各有职责，后续依据查询和验证结果调整拆分，不用表数量作为重构成功标准。

## 权限规则

| 预置角色 | 默认能力 |
|---|---|
| 管理员 | 管理账号、角色、菜单、系统配置及全部业务；关键密钥操作留审计 |
| 运维 | 管理监控、处理告警、管理证书；默认不管理账号权限和应用凭据 |
| 只读 | 查看监控状态、趋势和允许展示的配置摘要；默认不下载私钥或读取凭据 |

自定义角色可以组合权限。多个角色取允许权限的并集，首版不引入复杂的显式拒绝规则。敏感权限单独设置，例如 `certificate:download_private_key`、`agent:rotate_credential`、`role:update`。

菜单是否显示不决定接口能否访问。接口统一调用授权函数，未登录返回 401，无权限返回 403。文件下载、证书下载、导出、后台触发接口也执行授权。

会话每次请求验证用户状态与 `auth_version`；短期缓存须有明确失效机制。用户授权变化、密码变化和停用操作递增版本并使旧会话失效。最后一个有效管理员的删除、停用和降权操作必须拒绝，相关检查处于同一事务。

创建人并不自动获得独占权限。任何未来的数据范围限制都需要显式规则，不能隐藏在各列表查询中。

## 并发修改和删除

PATCH 请求必须携带读取时的 `version`。更新执行 `WHERE id = ? AND version = ?`，成功后版本递增；版本冲突返回 409，页面保留用户输入以便比较。

删除角色前检查关联用户；删除分组前要求转移资源或明确处理；删除通知渠道前检查规则引用；删除 Agent 前检查绑定主机和检测目标。资源停用后同时终止后续调度，撤销凭据后立即拒绝接入。

管理操作批量执行时逐项返回结果，默认不承诺跨多个资源的全局原子性；需要全有或全无的操作必须单独定义事务语义。

## SQLite 存储规则

使用一个 `data/monitor.db` 保存业务配置、历史、任务和投递队列。SQLite 每个连接启用外键，显式设置 WAL、忙等待和事务策略。用户名规范化、字符串长度和 JSON 结构校验在业务层完成，关键不变量再使用 CHECK、外键和唯一索引约束。

配置写入、历史批量写入和通知入队经单写入通道协调，写连接数限制为一；读取连接独立受控。队列有容量与等待超时，只有事务提交成功才向 Agent 返回接收成功。配置写入与后台批次公平调度，批量大小通过测试调整。起步建议 `synchronous=FULL`，如评估其他模式，必须说明断电持久性差异。

维护单套版本化迁移，升级只在持有数据目录锁后执行。迁移失败保留诊断并阻止服务以半升级状态接收业务写入。每次更新在真实 SQLite 文件上验证新安装、旧版本升级、约束、锁冲突和异常恢复。

监测数据库大小、WAL 大小、写队列长度、写入延迟和磁盘空间，执行有节制的检查点与历史清理。运行时需要保留数据库可能生成的 WAL 与共享内存伴随文件；在线备份使用一致性备份接口。
