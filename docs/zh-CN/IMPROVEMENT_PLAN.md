# 后续改进任务书

整理日期：2026-10-15。基线为应用版本 `0.4.0`、SQLite 结构版本 `4`，当时 `go test ./...` 全部通过。

本文已按用户要求开始逐项实施，**完成情况以状态表和对应验证记录为准**；尚未完成的条目仍是待实施建议。每完成一项，在下方状态表中更新状态，并按项目规范同步 README、CHANGELOG 和相关指南。文中行号对应整理时的代码，实施前请重新定位。

## 执行规则

1. 开始前完整阅读根目录 [AGENTS.md](../../AGENTS.md) 和 [CONTRIBUTING.md](../../CONTRIBUTING.md)，其中的约束优先于本文。
2. 一个任务对应一个独立分支和提交，不要把多个任务混在一次改动里。
3. **结构版本串行分配**：需要数据库迁移的任务（T1、T5、T6 等）按实际合入顺序依次使用 `migration5.sql`、`migration6.sql`……，不要在并行分支里抢占同一个版本号。每次升级都要：
   - 在 `internal/app/store.go` 中嵌入新迁移、追加升级分支，并把最终版本检查改为新版本；
   - 在当前指南中更新结构版本说明（用 `grep -rn "schema" README.md CHANGELOG.md SECURITY.md docs/` 查找），`docs/zh-CN/VALIDATION_HISTORY.md` 属于历史记录，不要改写；
   - 在 CHANGELOG 的 Unreleased 升级说明中写明：升级前停服并完整备份数据目录，旧版本程序无法读取新结构。
4. 新增可见文案必须同时更新 `locales/` 下五种语言，保留插值参数，并运行 `npm --prefix web run check:i18n`。
5. 新增写接口必须有服务端权限检查；菜单可见性不能代替授权。
6. 不引入新的外部基础设施依赖。确需新增 Go 或 npm 依赖时，记录版本和用途，并更新 `THIRD_PARTY_NOTICES.md`。
7. 不要在没有实际运行验证的情况下宣称完成。

### 通用验证命令

```sh
gofmt -l ./cmd ./internal
go vet ./...
go test -race ./...
npm --prefix web run typecheck
npm --prefix web test
npm --prefix web run build
npm --prefix web run format:check
```

- 涉及数据库：用真实 SQLite 临时文件测试迁移、约束和事务，不能只用内存库。
- 涉及接口：在隔离端口启动本项目服务（例如 `./bin/monitor -data tmp/verify-data -listen 127.0.0.1:5091`），发送真实 HTTP 请求，并记录实际端口。
- 涉及关键交互：在真实浏览器中验证。
- 涉及文档：运行 `npm --prefix web run check:docs`。

## 状态总览

| 编号 | 任务 | 类别 | 优先级 | 需要迁移 | 状态 |
|---|---|---|---|---|---|
| T1 | 调度器任务队列索引 | 缺陷 / 性能 | P0 | 是 | 已完成 |
| T2 | 登录限流识别真实客户端 | 缺陷 / 安全 | P0 | 否 | 已完成 |
| T3 | 检测目标完整编辑与归档 | 功能 | P1 | 否 | 已完成 |
| T4 | 钉钉 / 飞书 / 企业微信消息格式 | 功能 | P1 | 否 | 待实施 |
| T5 | 主机 CPU / 内存 / 磁盘阈值可配置 | 功能 | P1 | 是 | 待实施 |
| T6 | HTTP 检测增强 | 功能 | P2 | 是 | 待实施 |
| T7 | 前端按页加载与接口查询合并 | 性能 | P2 | 否 | 待实施 |
| T8 | 静态资源缓存与压缩 | 性能 | P2 | 否 | 待实施 |
| T9 | 在线备份命令 | 功能 / 运维 | P2 | 否 | 待实施 |
| T10 | 统一版本号与 CI 工作流 | 工程 | P2 | 否 | 待实施 |
| T11 | Agent 与主机采集测试 | 工程 | P3 | 否 | 待实施 |
| T12 | 历史数据汇总与长期可用率 | 功能 | P3 | 是 | 待实施 |
| T13 | 数据库只读连接池 | 性能 | P3 | 否 | 待评估 |

建议顺序：T1 → T2 → T3 → T4 → T5 → T7 → T8 → T10 → T9 → T6 → T11 → T12 → T13。

---

## T1 调度器任务队列索引

### 问题与证据

`internal/app/scheduler.go:48` 每个调度周期（1 秒）至少执行一次：

```sql
SELECT id FROM jobs WHERE state='queued' ORDER BY id LIMIT 1
```

现有部分索引 `jobs_active ON jobs(monitor_id) WHERE state IN ('queued','running')` 无法用于这条查询，执行计划为 `SCAN jobs`（全表扫描）。

实测环境：项目使用的 `modernc.org/sqlite` v1.60.1（SQLite 3.53.4），真实文件库，WAL 模式，200 万条已完成任务。

| 方案 | 执行计划 | 单次耗时 |
|---|---|---|
| 现状 | `SCAN jobs` | 143ms |
| 改写为 `state IN ('queued','running') AND state='queued'` | `SCAN jobs` | 158ms（无效） |
| 新增部分索引 `jobs(id) WHERE state='queued'` | `SCAN jobs USING COVERING INDEX jobs_queue` | 22µs |

数据库只有一个连接（`internal/app/store.go:72`，`SetMaxOpenConns(1)`），扫描期间所有 HTTP 请求和 Agent 上报都要排队。按上限计算（500 个检测、最短 15 秒周期、已完成任务保留 7 天，约 2000 万行），线性估算单次扫描会超过 1 秒。

### 实施要点

1. 新增迁移文件，内容：
   ```sql
   CREATE INDEX jobs_queue ON jobs(id) WHERE state='queued';
   INSERT INTO schema_migrations(version) VALUES(5);
   ```
2. 按“执行规则”第 3 条接入 `store.go` 并更新文档。可顺带把 `store.go` 中重复的逐版本迁移代码整理为循环，但行为必须保持一致：每个版本一个事务，失败回滚。
3. 可选：已完成任务的保留期从 7 天缩短为 1 天（`scheduler.go` 中 `DELETE FROM jobs` 那一行），因为检测结果已经记录在 `check_results`。如果修改，要在文档中写明保留策略。

### 验收标准

- 新增 Go 测试：在临时目录用真实文件创建 v4 数据库，打开后自动升级到 v5，`schema_migrations` 最大版本为 5，`jobs_queue` 索引存在。
- 新增 Go 测试：对上述查询执行 `EXPLAIN QUERY PLAN`，断言计划中包含 `jobs_queue`。
- 现有调度、手动检测和取消任务的测试全部通过。
- 当前指南中的结构版本说明已更新为 5。

### 实施记录（2026-10-15）

- 分支：`codex/t1-scheduler-queue-index`。新增结构版本 5 和部分索引；已完成任务仍保留 7 天。
- 真实 SQLite 文件验证 v4 → v5、历史保留、FIFO、迁移失败回滚及重试；实际查询计划：`SCAN jobs USING COVERING INDEX jobs_queue`。
- `go test ./...`、`go test -race ./...`、`go vet ./...`、Go 构建及格式检查通过。前端类型检查、111 项 React 测试、7 项脚本测试、生产构建及格式检查通过。五语文档和国际化检查通过。
- `docker compose config --quiet` 与 `MONITOR_IMAGE=monitor-go:t1-verify MONITOR_PLATFORM=linux/arm64 node scripts/test-docker.mjs` 通过；临时端口 `61425` 验证初始化、结构版本 5、会话与上传持久化、容器重建、冷备份恢复。验证使用一次性数据卷。


---

## T2 登录限流识别真实客户端

### 问题（读代码得出，尚未复现）

- `internal/app/server.go:329` 的 `allowedRequest` 按 `r.RemoteAddr` 计数。文档推荐多人访问时放在可信 HTTPS 反向代理之后，这时所有用户的 `RemoteAddr` 都是代理地址，共用一个计数。
- `internal/app/auth.go:126` 在校验密码**之前**调用 `allowedLogin`，登录成功也消耗次数。
- 结果：放在反向代理后，5 分钟内整个团队合计只能发起 10 次登录请求，第 11 次起全部返回 429。
- 另外，`attempts` 表超过 10000 项时会拒绝所有限流接口的所有请求，包括登录、Agent 注册和机器接口。

先写一个测试复现上述行为，再修复。

### 实施要点

1. 新增启动参数 `-trusted-proxy`，值为逗号分隔的 CIDR 列表，默认为空（不信任任何代理，行为与现在一致）。解析结果放入 `app.Config`，非法值启动失败并给出明确错误。
2. 新增 `clientIP(r)`：只有当 `RemoteAddr` 落在可信网段内时，才读取 `X-Forwarded-For`，并从右往左取第一个不在可信网段内的地址；否则使用 `RemoteAddr`。所有调用 `allowedRequest` 的地方（登录、初始化、Agent 注册、编码工具、应用密钥）统一改用它。
3. 登录限流调整：
   - 只统计**失败**的登录；成功登录不消耗次数，也不重置其他来源的计数。
   - 同时按“客户端 IP”和“用户名”两个维度计数（例如同一 IP 每 5 分钟 30 次失败，同一用户名每 5 分钟 10 次失败），具体阈值在代码中集中定义。
   - 返回给客户端的错误信息不能泄露用户名是否存在。
   - 初始化接口 `setup` 保持对所有尝试计数，防止暴力猜测初始化码。
4. 表满时的处理：不再拒绝全部请求。可以优先清理已过期项；仍然满时只拒绝新建计数项，已有项照常判断。
5. 新增启动参数属于部署方式变更，需同步：五种语言的当前指南、`docs/*/DOCKER.md`、`compose.yaml`（如通过环境变量传入），并在 CHANGELOG 中说明。

### 验收标准

- 单元测试覆盖：
  - 来自可信代理的请求使用 `X-Forwarded-For` 中的真实地址；
  - 来自不可信地址的伪造 `X-Forwarded-For` 被忽略；
  - 连续 20 次成功登录不会被限流；
  - 某个用户名失败次数达到上限后，其他用户仍可正常登录；
  - 表满时已有计数项仍按规则判断。
- 更新 `internal/app/integration_test.go` 中现有的限流测试，以匹配新行为。
- 在隔离端口启动服务，用 `curl` 带不同的 `X-Forwarded-For` 验证真实 HTTP 行为，并记录端口。

---

### T2 实施记录（2026-10-15）

- 分支：`codex/t2-trusted-proxy-login`。先用测试复现成功登录耗尽次数和计数表满误伤，再实现 CIDR 信任边界、失败双维限流与并发预约。初始化单独计数，已有限流接口统一采用客户端识别。
- 完整 Go 测试、`go vet ./...`、构建、重点竞态测试及格式检查通过。覆盖 IPv4/IPv6、代理链、不可信或畸形转发头、20 次成功登录、不同用户、过期、容量和并发预约。
- 隔离服务经真实 `curl` 验证：端口 `63541`（可信回环代理）及 `63655`（无可信代理），验证成功登录、30 次失败上限与转发头边界。均已停止并清理临时数据。
- 五语部署/容器指南、README、CHANGELOG 已同步；文档检查通过。Compose 配置与 Linux arm64 Docker 一次性卷安装、重建和冷备份恢复验证通过（端口 `63455`）。

---

## T3 检测目标完整编辑与归档

### 现状

- `internal/app/monitors.go:170` 附近的 `updateMonitor` 只接受 `{version, enabled}`，无法修改名称、地址、周期、超时、阈值、凭据等。
- `DELETE /api/v1/monitors/{id}` 软归档接口已存在，但界面没有入口。
- 文档“当前边界”已列出这两项缺失。

### 实施要点

**后端**

1. 新增 `PUT /api/v1/monitors/{id}`，权限 `monitor:update`。请求体为 `monitorInput` 的全部字段，加上 `version` 和 `clearPassword`。复用 `monitorInput.validate()`。
2. 现有 `PATCH /api/v1/monitors/{id}`（启停）保持兼容，不改变请求格式。
3. 凭据规则：
   - `password` 为空且 `clearPassword=false`：保留原加密凭据；
   - `password` 非空：重新加密并替换；
   - `clearPassword=true`：清空凭据；
   - 类型改为不需要凭据的类型（HTTP、TCP、SSL）时，同时清空用户名和凭据。
4. 在同一个事务中完成：乐观锁校验（版本不符返回 409）、更新字段、`version+1`、`failures=0`、`state='unknown'`、`next_check=0`，取消该检测已排队的任务，写入审计记录（动作“编辑检测”）。已删除的检测返回 409 或 404，与现有接口保持一致。
5. 保留原有检测历史和未恢复的告警，由下一次检测结果决定告警恢复或继续。

**前端**（`web/src/views/MonitorsView.tsx`）

1. 复用新增检测的表单作为编辑模式。密码输入框提示“留空保持不变”，另外提供“清除已保存凭据”选项。
2. 增加归档按钮，使用现有 `Modal` 二次确认后调用 `DELETE`。
3. 收到 409 时提示内容已被他人修改，并刷新数据，不丢失用户正在编辑的输入。
4. 没有 `monitor:update` 权限时隐藏编辑和归档入口（服务端仍要校验）。
5. 新增文案同步五种语言。

### 验收标准

- Go 测试：
  - 编辑成功后字段、版本号、审计记录正确；
  - 版本冲突返回 409；
  - 三种凭据规则各有测试；
  - 只读角色调用返回 403；
  - 编辑已归档的检测被拒绝；
  - 编辑会取消已排队的任务。
- React 测试：编辑表单预填、留空保持密码、归档确认、409 冲突提示。
- 在真实浏览器中完成一次“编辑地址 → 立即检测 → 归档”的完整流程。
- 更新文档“当前边界”中关于检测编辑和归档的说明（五种语言的当前指南）。

---

### T3 实施记录（2026-10-15）

- 分支：`codex/t3-monitor-edit-archive`。增加 `PUT /api/v1/monitors/{id}` 和编辑/归档界面；凭据保留、替换、清除及类型切换清除均有实际 SQLite 集成测试。版本冲突、只读角色、排队任务取消、历史保留、审计及已归档目标拒绝通过验证。
- 完整 Go 测试、重点竞态测试、vet、构建、格式检查通过；前端类型检查、115 项 React 测试及 7 项脚本测试、国际化、文档检查和生产构建通过。五语当前指南、README、路线图和 CHANGELOG 已更新。
- 隔离服务端口 `65217` 完成真实浏览器“编辑地址 → 立即检测 → 恢复正常 → 确认归档”；实际 SQLite 验证版本 1 → 2 → 3、归档停用及历史结果保留，无浏览器警告/错误。临时服务已停止，原预览实例未改动。

---

## T4 钉钉 / 飞书 / 企业微信消息格式

### 现状

只有 SMTP 和通用 JSON Webhook（`internal/app/notifications.go` 中的 `sendWebhook`）。`notification_channels.kind` 有 `CHECK(kind IN ('webhook','smtp'))` 约束，渠道配置以 JSON 保存在 `channelConfig` 中。

### 实施要点

1. **不改表结构**：在 `channelConfig` 中新增 `format` 字段，取值 `generic`（默认，兼容现有数据）、`dingtalk`、`feishu`、`wecom`。仍使用 `kind='webhook'`。
2. 各平台的请求格式和签名（以平台官方文档为准，实施时核对）：
   - 钉钉自定义机器人：`msgtype=markdown`。配置了加签密钥时，在 URL 上追加 `timestamp`（毫秒）和 `sign`（对 `timestamp + "\n" + secret` 做 HMAC-SHA256，Base64 后再 URL 编码）。
   - 飞书自定义机器人：`msg_type=text` 或卡片。配置了签名校验时，在请求体中加入 `timestamp`（秒）和 `sign`（以 `timestamp + "\n" + secret` 为密钥，对空串做 HMAC-SHA256 后 Base64）。
   - 企业微信群机器人：`msgtype=markdown`，密钥在 URL 的 `key` 参数中，无额外签名。
3. **成功判定**：这三个平台通常返回 HTTP 200，真实结果在响应体里（钉钉、企业微信看 `errcode == 0`，飞书看 `code == 0`）。必须解析响应体判断成败；失败时进入现有的重试流程（15 秒、1 分钟、5 分钟）。
4. 消息内容包含：事件类型（故障 / 恢复 / 测试）、标题、资源、级别、描述、时间、投递编号。告警标题和描述保留原文，不做翻译；事件类型等固定标签可按实例语言处理，但不能用显示文字作为判断依据。
5. 继续使用 `safeDial` 并且不跟随重定向，保持现有的 SSRF 防护。
6. 前端渠道表单增加“消息格式”选择。选择专有格式时，隐藏通用 Webhook 签名说明，改为显示对应平台的密钥说明。
7. 文案同步五种语言，并更新指南中的通知章节和“当前边界”。

### 验收标准

- Go 测试：用 `httptest` 模拟三个平台：
  - 校验请求体结构和签名计算（用固定时间戳和密钥比对预期值）；
  - 响应 `errcode != 0` 时判为失败并进入重试；
  - `generic` 格式的行为与改动前完全一致。
- 现有渠道数据无需迁移即可正常读取和发送。
- 有条件时用真实机器人完成一次测试发送，并在验证记录中注明平台；没有条件时明确写“未做真实平台验证”。

---

## T5 主机 CPU / 内存 / 磁盘阈值可配置

### 现状

- 主机告警只有两类：远程主机离线（超过 90 秒未上报）、磁盘使用率 ≥ 85%。
- 磁盘阈值写死在 `internal/app/agents.go:244`（远程主机）和 `internal/app/hosts.go:61` 附近（本机采集）两处。
- 没有 CPU 和内存告警。

### 实施要点

1. 迁移：在 `system_settings` 增加全局阈值列，例如：
   - `host_cpu_percent`（默认 90）
   - `host_memory_percent`（默认 90）
   - `host_disk_percent`（默认 85，保持现有行为）
   - `host_alert_minutes`（持续时长，默认 5）

   每列加 `CHECK` 约束限制取值范围；值为 0 表示关闭该项告警。
2. 远程上报和本机采集共用一个判定函数，消除两处重复的写死阈值。
3. CPU 和内存按持续时长判定：最近 `host_alert_minutes` 分钟内的 `host_samples` 全部高于阈值才告警；样本不足时不告警。恢复条件为最新样本低于阈值。告警指纹分别为 `host:%d:cpu` 和 `host:%d:memory`。
4. 磁盘告警沿用 `host:%d:disk` 指纹，阈值改为读取配置。
5. 设置页增加阈值配置，权限 `system:settings`，带乐观锁和审计记录。
6. 可选的第二阶段：按主机覆盖全局阈值（需要另一次迁移，单独提交）。

### 验收标准

- 迁移测试使用真实 SQLite 文件。
- 判定逻辑的单元测试：持续超阈值、中途回落、样本不足、阈值为 0（关闭）、恢复。
- 修改设置后，下一次上报即按新阈值判定。
- 文档更新告警说明和“当前边界”。

---

## T6 HTTP 检测增强

### 现状

`internal/app/probe.go` 的 HTTP 检测只发 GET 请求、只比较状态码，响应体只读前 4096 字节且不做判断。“警告”状态目前只由 SSL 证书即将到期产生。

### 实施要点（建议拆成两次提交）

**第一阶段**（一次迁移）：

- `monitors` 新增列：
  - `keyword`：响应体需包含的文本，空表示不检查；
  - `keyword_invert`：为真时改为“不应包含”；
  - `latency_warn_ms`：响应超过该毫秒数时记为“警告”，0 表示关闭。
- 检查关键字时，响应体最多读取 1 MiB，超出部分不参与匹配，并在详情中注明。
- 状态判定顺序：连接失败或状态码不符为“失败”；关键字不符为“失败”；超过延迟阈值为“警告”。
- 详情中不要回显完整响应体，避免泄露敏感内容。

**第二阶段**（可选）：

- 自定义请求方法（仅 GET 和 HEAD）和请求头。请求头可能含令牌，需要使用 `Store.encrypt` 加密保存，接口只返回是否已设置。

两个阶段的表单都要同步支持 T3 的编辑功能。

### 验收标准

- 用 `httptest` 覆盖：关键字命中与不命中、反向匹配、响应体超过 1 MiB、延迟超阈值记为警告、状态码优先于关键字判定。
- 迁移测试使用真实文件；旧检测的新列默认值不改变原有行为。

---

## T7 前端按页加载与接口查询合并

### 现状

- `web/src/App.tsx` 中每 10 秒调用一次 `refreshAll()`（页面隐藏时跳过）。`web/src/stores/app.ts:301` 的 `loadAll` 每次都请求：`/me`、`/hosts`、`/monitors`、`/alerts`、`/roles`、`/users`、`/permissions`、`/me/navigation`，与当前在哪个页面无关。
- `/monitors`（`internal/app/monitors.go:47`）对每个检测执行 2 次查询：一次统计 24 小时可用率，一次取最近 32 条结果。500 个检测就是 1001 次查询。
- `/alerts`（`internal/app/alerts.go:62`）对每条告警单独查询处理记录，最多 1 + 200 次查询。
- 用到这些全局数据的页面：`MonitorsView`、`OverviewView`、`AlertsView`、`HostView`、`RolesView`。实施前还要检查 `App.tsx` 和导航组件中是否有依赖它们的徽标或计数。

### 实施要点

**后端**（保持响应结构不变）

1. `monitors()`：用一次 `GROUP BY monitor_id` 查询得到所有检测的可用率；用窗口函数 `ROW_NUMBER() OVER (PARTITION BY monitor_id ORDER BY at DESC, id DESC)` 一次取出每个检测最近 32 条结果。
2. `listAlerts()`：用一次带窗口函数的查询取回本页告警各自最近 100 条处理记录；或新增 `GET /api/v1/alerts/{id}/activity`（权限 `alert:read`），展开时再加载。若选择后者，前端也要同步修改。
3. 用 `EXPLAIN QUERY PLAN` 确认新查询能用上 `check_results_time` 等索引。

**前端**

1. 定时刷新只加载当前页面需要的数据，再加上全局必须的 `/me` 和导航。
2. 角色、用户、权限列表只在进入相关页面或数据变更后加载，不参与 10 秒轮询。
3. 保留现有的会话版本保护、写入后强制刷新、乱序响应丢弃等逻辑和对应的回归测试。

### 验收标准

- Go 测试比较改动前后 `/monitors` 和 `/alerts` 的响应内容，结果一致。
- 用 500 个检测、每个检测 24 小时结果的真实 SQLite 测试库，对比改动前后接口耗时，把数据写进 PR 描述。
- React 测试：停留在某个页面时，轮询不请求无关接口；切换页面后能加载对应数据。
- 现有 16 个回归测试全部通过。

---

## T8 静态资源缓存与压缩

### 现状

`internal/app/server.go:301` 直接用 `http.FileServer(http.FS(s.cfg.Assets))` 提供嵌入的前端资源：

- `embed.FS` 的文件修改时间为零，所以不返回 `Last-Modified`，也没有 `ETag` 或 `Cache-Control`；
- 没有压缩；
- 每次请求都新建一个 `FileServer`。

当前构建产物：主 JS 约 349KB，每个语言包约 90–100KB，CSS 约 57KB。每次打开页面都要完整重新下载。

### 实施要点

1. 只使用标准库，不新增依赖。启动时遍历嵌入资源，为每个文件计算 SHA-256 作为强 `ETag`；对文本类资源（js、css、html、svg、json）预先在内存中生成 gzip 版本。
2. 响应头：
   - `/assets/*`（文件名带哈希）：`Cache-Control: public, max-age=31536000, immutable`；
   - `index.html` 和其他不带哈希的文件：`Cache-Control: no-cache`，依靠 `ETag` 协商；
   - 请求头 `Accept-Encoding` 包含 gzip 时返回压缩版本，并设置 `Content-Encoding: gzip` 和 `Vary: Accept-Encoding`；
   - 支持 `If-None-Match`，命中时返回 304。
3. 保留现有的安全响应头和“非 GET/HEAD 返回 405”的行为。`FileServer` 只创建一次。

### 验收标准

- Go 测试：哈希资源的缓存头、`index.html` 的 `no-cache`、gzip 协商、304 响应、405 响应。
- 启动服务后用 `curl -I` 和 `curl --compressed` 实际验证，记录端口和压缩前后的大小。
- 浏览器中刷新页面，确认带哈希的资源来自缓存。

---

## T9 在线备份命令

### 现状

文档要求停服后完整复制数据目录。运行中的服务会持有 `instance.lock`，所以现有的 `OpenStore` 不能被第二个进程打开。

### 实施要点

1. 在 `cmd/monitor` 增加子命令：`monitor backup -data <数据目录> -out <输出目录>`。输出目录必须不存在，创建时权限为 0700。
2. 备份进程**不获取** `instance.lock`、**不执行迁移**。以只读方式打开 `monitor.db`，用 `VACUUM INTO` 生成一致快照。只读连接能否执行 `VACUUM INTO`，需要在 v1.60.1 驱动上实际验证；不行时使用普通连接，但不能写入原库。
3. 复制 `master.key`（权限 0600）和 `uploads/objects/`。不要复制 `setup-token`、`instance.lock`、`uploads/staging/` 和 `uploads/cache/`。
4. **处理竞态**：快照之后、复制完成之前，服务端可能上传新文件，或回收已永久删除的文件。复制完成后，逐一检查快照中所有未回收文件的对象是否都已复制：
   - 缺失时重试复制；
   - 仍缺失时在输出中列出对象并以非零状态退出，不能静默生成不完整的备份。
5. 在输出目录写入清单：时间、应用版本、结构版本、文件数量、各对象的 SHA-256。
6. 恢复流程写进文档：停服 → 替换整个数据目录 → 用不低于备份结构版本的程序启动。本任务不做在线恢复。
7. 更新五种语言的当前指南和 `docs/*/DOCKER.md` 的备份章节，包括 Docker 中的执行方式（`docker compose exec`）。

### 验收标准

- Go 测试：
  - 服务运行（持有锁）时执行备份成功；
  - 用备份目录调用 `OpenStore` 能正常启动，数据与原库一致；
  - 输出目录已存在时拒绝执行；
  - 模拟对象缺失时以非零状态退出。
- 做一次真实的恢复演练：启动服务 → 写入检测和文件 → 运行中备份 → 停服 → 用备份目录启动 → 核对数据。把步骤和结果写进 PR。

---

## T10 统一版本号与 CI 工作流

### 现状

- 版本号分散在多处且不一致：
  - `internal/app/server.go:325` 和 `internal/app/settings.go:95`：`0.4.0`
  - `internal/app/probe.go:79` 的检测请求 User-Agent：`MonitorGo/0.1`
  - `internal/app/notifications.go:191` 的 Webhook User-Agent：`MonitorGo/0.2.0`
  - `web/package.json`：`0.4.0`
- `.github/` 只有 Issue、PR 和 Release 模板，没有 CI 工作流。

### 实施要点

1. 新增 `internal/buildinfo` 包，内含 `var Version = "0.4.0-dev"`，构建时用 `-ldflags "-X github.com/lorvanto/monitor-go/internal/buildinfo.Version=..."` 覆盖。所有版本号和 User-Agent（`MonitorGo/<版本>`）都改为读取这个变量。Agent 上报的 `agent_version` 也使用它。
2. `Makefile` 和 `Dockerfile` 传入版本：优先使用 `git describe --tags --always --dirty`，没有 Git 信息时回退到默认值。
3. 新增 `.github/workflows/ci.yml`，在 push 和 pull request 时运行：
   - Go 版本取自 `go.mod`（`setup-go` 的 `go-version-file`），Node.js 22；
   - 执行 `make test`、`npm --prefix web run format:check`，以及 `gofmt -l` 结果为空的检查；
   - 第三方 Action 固定到提交 SHA，并在注释中写明对应版本。
4. 不要在文档中写 CI 已通过或任何平台已验证，除非真的在远端运行过。本地准备工作流文件不代表已发布或已运行。

### 验收标准

- `make build` 后，`./bin/monitor` 的 `/api/v1/status` 和系统信息页显示注入的版本号。
- 检测请求和 Webhook 请求的 User-Agent 一致，有测试断言。
- 工作流 YAML 语法正确；本地按工作流的同样步骤执行一遍并全部通过。

---

## T11 Agent 与主机采集测试

### 现状

`go test` 输出显示 `internal/agent` 和 `internal/hoststats` 没有测试文件。Agent 的注册、重连退避（`internal/agent/client.go` 约 242 行）、凭据保存和禁止重定向等逻辑没有自动化保护。

### 实施要点

1. 用 `httptest` 模拟服务端，覆盖：
   - 首次注册并保存凭据；
   - 正常上报；
   - 服务端返回 401 或 5xx 时的退避间隔（失败次数 × 5 秒，最长 60 秒）；
   - 拒绝重定向；
   - 私有 CA 参数。
2. 把采集逻辑中的时间、外部调用抽象为可注入的依赖，以便测试；不改变对外行为。
3. `hoststats` 可以先针对磁盘分区过滤、最多 16 个分区的上限和错误收集写单元测试。

### 验收标准

- `go test -race ./internal/agent ./internal/hoststats` 通过，并覆盖上述场景。
- 测试不依赖真实网络和固定端口。

---

## T12 历史数据汇总与长期可用率

### 现状

`internal/app/scheduler.go:159-160` 把 `host_samples` 和 `check_results` 的保留期写死为 7 天，接口只支持 1、24、168 小时范围，无法提供 30 天或 90 天的可用率。`audit_days` 已经是可配置的，可以参考它的做法。

### 实施要点

1. 迁移：
   - 新增 `check_hourly`（`monitor_id`、`hour`、`total`、`good`、`latency_avg`、`latency_max`），主键为 `(monitor_id, hour)`；
   - 主机指标可以按需新增 `host_hourly`；
   - `system_settings` 增加原始数据保留天数和汇总数据保留天数，加 `CHECK` 约束。
2. 在小时维护任务中，把已结束的整点数据汇总进去（`INSERT ... ON CONFLICT DO UPDATE`，可重复执行），然后再按配置清理原始数据。
3. 接口增加 30 天和 90 天的范围，读取汇总表；总览和检测页面显示长期可用率。
4. 文档写明保留策略和磁盘占用估算方法。

### 验收标准

- 汇总结果与直接统计原始数据一致（测试中构造跨小时边界的数据）。
- 重复执行汇总不会重复计数。
- 清理原始数据不影响已汇总的结果。

---

## T13 数据库只读连接池（先评估）

### 背景

`store.go` 使用单连接，所有读请求都要排在写事务后面。SQLite 的 WAL 模式支持一写多读，可以拆成单写连接加只读连接池（只读连接设置 `PRAGMA query_only=1`）。

### 实施要点

1. **先完成 T1 和 T7 再评估。** 用压测数据（并发浏览器轮询加检测写入）证明单连接仍是瓶颈，再决定是否实施。
2. 如果实施：
   - 写操作只走 `Store.write` 和写连接；
   - 事务回调内部不能使用只读连接读数据（否则看不到同一事务中尚未提交的写入），逐一检查所有 `s.Store.DB` 调用点；
   - 迁移、`seed` 和备份都使用写连接。
3. 不满足条件就把本任务状态改为“不实施”，并在 PR 中附上评估数据。

### 验收标准

- 提供改动前后的并发压测数据（工具、参数、结果）。
- `go test -race ./...` 通过；新增测试证明写事务中的读取能看到同一事务的写入。
