# 应用分发与编码工具

> 文档类型：当前开发基线的应用接口与能力说明。当前使用方式以 [使用指南](README.md) 为准，公开发布流程见 [GitHub 发布指南](GITHUB.md)。

本文描述 v0.4 的实际实现；对应 `internal/app/applications.go`、`releases.go`、`codes.go` 与 `migration4.sql`。资源属于本套实例，创建者不构成数据隔离。

## 应用与密钥

浏览器 API 需要正常 Cookie 会话与写入 CSRF。`GET /applications` 对应用读取/管理/密钥管理及安装包读取/管理/下载成员开放应用目录；修改仅允许 `application:manage`。应用标识唯一、软归档后仍保留。密钥管理需要独立的 `application:keys`，授权该敏感权限即允许签发所有注册的机器 API 范围。

所有路径以 `/api/v1` 为前缀：

| 方法与路径 | 请求或结果 |
|---|---|
| `POST /applications` | `{slug,name,description,enabled}` → `{id}` |
| `PATCH /applications/{id}` | 相同字段加 `version`，slug 不可变 |
| `DELETE /applications/{id}` | `{version}`；有未归档安装包则拒绝；软归档并撤销密钥 |
| `GET /applications/{id}/keys` | `{items,scopes}`；最近 100 条元数据，不返回原密钥 |
| `POST /applications/{id}/keys` | `{name,days,scopes,rotateId?,version?}` → `{id,token,prefix,expiresAt}`；轮换时带旧密钥 ID 与版本 |
| `DELETE /application-keys/{id}` | `{version}`，立即撤销后续访问 |

密钥有效期 1–365 天；应用最多 10 个有效密钥，实例最多 200 个有效应用。停用不延长密钥有效期；重新启用后未过期且未撤销的密钥恢复使用。一次性明文以 `ymk_` 开头，含 32 字节密码学随机数的十六进制表示，只保存 SHA-256 摘要、前缀；不支持检索原文。审计不记录密钥正文，前端也不写入本地持久缓存。

## 机器 API

仅接受 `Authorization: Bearer <密钥>`，不接受管理会话代替或 URL 查询参数传密钥。该凭据不能用于管理 API、Agent 注册或指标上报。各路径均以 `/api/v1/integrations` 为前缀。

| GET 路径 | 范围 | 数据 |
|---|---|---|
| `/me` | 任意有效密钥 | 密钥/应用编号、名称、范围、到期时间 |
| `/hosts` | `host:read` | 实例主机状态、系统资料与当前指标，不含 Agent 凭据 |
| `/monitors` | `monitor:read` | 全部有效目标的 ID、名称、类型、目标、分组、启用、状态与检查时间；不含用户名、加密凭据或响应详情 |
| `/alerts` | `alert:read` | 复用告警列表，最多 200 条事件与每条 100 条时间线 |
| `/releases` | `release:read` | 仅所属应用的已发布安装包，支持 `page`、`q`；忽略客户端 applicationId |
| `/releases/{id}/download` | `release:download` | 仅所属应用已发布且应用启用的版本；本地文件流或外部地址 JSON |

```sh
# 将完整密钥在自己的服务端环境中设置为 MONITOR_GO_API_KEY。
curl 'https://monitor.example.com/api/v1/integrations/me' \
  -H "Authorization: Bearer $MONITOR_GO_API_KEY"
```

JSON 成功返回 `data` 和 `request_id`，错误返回 `error.message`。失效/停用返回 401，范围不足 403，版本不可下载或跨应用下载返回 404，频率限制返回 429。限流按密钥 + 实际远端 IP，5 分钟 300 次；目前不信任代理传入的来源 IP 头，在同一代理下可能共用额度。最后使用时间最多每分钟写一次，不是逐请求流水。若 API 将来需要高频消费，应先容量压测并设计相应分页与速率策略。

## 安装包

列表 `GET /releases?applicationId=&state=&q=&page=1` 返回 `{items,total,page,pageSize}`，每页 30 条。维护使用 `release:manage`；下载单独使用 `release:download`。仅有读取权限的列表不返回外部 downloadUrl，维护或下载成员可读该地址。

创建/编辑字段为 `applicationId,name,versionName,platform,arch,source,fileId,downloadUrl,notes,state`，编辑另传 `version`。

- platform：`windows,macos,linux,android,ios,web,other`；arch：`amd64,arm64,universal,other`。
- source 为 `file` 时只有 fileId；source 为 `url` 时只有 downloadUrl，要求 HTTPS、无用户名密码。外部地址不由服务端访问。
- state 为 `draft,published,withdrawn`。首次发布后文件来源、应用、版本号、平台、架构固定，不可退回草稿；可以修改说明/名称、下架或再次发布。
- 有效/归档记录共用版本身份唯一约束。归档 `DELETE /releases/{id}` 传 `{version}`，是软归档，当前不提供恢复界面。
- 安装文件复用文件管理；新增/更换本地关联要有 `file:download`，上传另外需 `file:upload`。被任何未归档安装包引用的文件或父目录禁止移入回收站。归档安装包不删除磁盘文件。
- 本地文件成功 GET（含 Range）计数，HEAD、304、无效 Range、失败下载不计数；计数不代表设备/用户/安装数量，也不是审计计费账本。SHA-256 在上传时计算，下载返回 `X-Checksum-SHA256`。
- 浏览器外链请求 302 跳转并计数；机器 API 返回 `{source:"url",url,message}`，不重定向、不计数，避免客户端自动转发 Bearer。接入方须使用无应用密钥的独立请求访问外部 URL。
- 上传按流处理，单文件可设 1–1024 MB，默认仍为 32 MB；需同时调整反向代理请求大小/超时。当前未实现分片、上传续传或客户端自动升级。安装包只存储与分发，不执行安装程序。

## 二维码与条码

`POST /tools/codes` 需要 `tool:generate`，请求示例：

```json
{"kind":"qr","content":"团队协作","width":400,"height":160,"level":"M","label":true}
```

`kind` 可为 `qr,code128,code39,ean13,ean8`；返回 `{kind,content,width,height,png,svg}`，两个图片字段是裸 base64。QR 高度等于宽度；`height` 和 `label` 对条码生效，level 对二维码生效。EAN 返回含校验位的完整 content。

二维码最多 1200 字节 UTF-8，可选 L/M/Q/H；条码文本最多 80 个字符，Code 128 限可打印 ASCII，Code 39 限大写字母、数字、空格与 `- . $ / + %`。EAN 输入 12/7 位会补齐校验位，13/8 位则校验。尺寸不足返回 422 和建议宽度；图形使用整数缩放、至少 2 像素模块、QR 4 模块静区或条码 10 模块静区，最大宽度 2048、高度 512。SVG 只输出固定几何元素，无脚本、外链或用户文本标记。

不保留正文历史，成功生成不写入正文审计。频率限制为用户 + 来源 IP 每 5 分钟 120 次，并行上限 2。生成图片不是 GS1 商品号码分配，也不保证特定打印机或扫描设备效果，正式使用需要试扫。

依赖锁定：[boombuler/barcode v1.1.0](https://github.com/boombuler/barcode) 负责生成；[gozxing v0.1.1](https://github.com/makiuchi-d/gozxing) 仅用于测试解码回读；`golang.org/x/image v0.47.0` 的基础字体用于条码下方文字。运行无外部服务依赖。

## 验证记录

集成测试使用真实临时 SQLite 与 HTTP，覆盖密钥摘要保存、鉴权命名空间隔离、范围不足、轮换、撤销、过期、应用停用、跨应用安装包访问、已发布身份锁定、文件/目录引用保护、Range/HEAD/错误下载计数与外链不转发密钥。v3 → v4 使用已有菜单编号冲突的真实数据库迁移，验证自定义菜单、旧用户、权限最小授予及重启幂等。

中文、含标记字符的二维码和四类条码由独立解码库回读，校验 PNG 的精确内容；SVG 解析检查元素白名单。全量 Go 竞态测试、go vet、前端类型检查/构建及漏洞扫描通过。正式目录不含验收账号、应用或文件；本地 HTTP 和浏览器验收使用 `tmp/apps-tools-qa/` 隔离数据。

浏览器验收覆盖安装包说明保存与重读、二维码 PNG/SVG 真实导出、EAN 校验位错误/补齐与修改内容后禁止导出旧图。390px 手机布局无页面横向溢出；重启后浏览器下载的 ZIP 样本与数据库 SHA-256 一致。正式 5080 已升级至 schema 4 并核对保留数量，测试数据保持独立。
