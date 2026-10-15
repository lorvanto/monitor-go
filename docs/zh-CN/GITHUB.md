# GitHub 发布与协作指南

[English](../en-US/RELEASING.md) · **简体中文** · [日本語](../ja-JP/GITHUB.md) · [한국어](../ko-KR/GITHUB.md) · [繁體中文](../zh-TW/GITHUB.md) · [Documentation](../README.md)

## 已确定与待完成事项

公开源码仓库为 [lorvanto/monitor-go](https://github.com/lorvanto/monitor-go)。当前显示名称为 Monitor Go，应用版本为 `0.4.0-dev`，SQLite 结构版本为 `7`。这两个版本号表示开发基线，不代表已发布 GitHub Release。项目代码与文档采用 [Apache-2.0](../../LICENSE)，第三方依赖保留原许可证。

当前 Go module 与内部导入已统一为 `github.com/lorvanto/monitor-go`。源码推送与版本发布分开管理；当前不包含 GitHub Release 标签或下载资产。

## 文档分工

| 文件 | 用途 |
|---|---|
| [README](../../README.md) | 面向首次访问者的英文项目入口 |
| [多语言文档](../README.md) | 五语当前使用指南与维护入口 |
| [贡献指南](../../CONTRIBUTING.md) | 开发环境、改动边界、测试、PR 与贡献许可 |
| [项目协作说明](COLLABORATION.md) · [English](../../AGENTS.md) | 仓库范围、开发约束、验证要求与国际化维护 |
| [安全政策](../../SECURITY.md) | 私密漏洞反馈方式和部署边界 |
| [支持说明](../../SUPPORT.md) | 问题反馈信息与常见故障排查 |
| [变更记录](../../CHANGELOG.md) | 当前 Unreleased 变更；实际发布后再添加日期与版本条目 |
| [路线图](../../ROADMAP.md) | 候选后续工作，不承诺交付日期 |
| [第三方声明](../../THIRD_PARTY_NOTICES.md) | 锁定版本、用途、许可证和原始许可文本 |
| [发布流程](../en-US/RELEASING.md) | 建仓、版本、验证、打包和发布检查 |
| [PR 模板](../../.github/pull_request_template.md) | 问题、最终行为、实际验证与兼容性 |
| [Release 模板](../../.github/RELEASE_TEMPLATE.md) | 发布说明、下载、校验和、升级与限制 |

Issue 模板在 `.github/ISSUE_TEMPLATE/`，分别用于缺陷、功能建议和文档翻译。模板不预设尚未创建的标签或团队，也不会自动创建 CI、分支保护或 GitHub Release。

## 参与开发

需要 Go 1.26.9 或更新的兼容版本、Node.js 22.12+ / 24.x、npm 与 Make。示例使用 POSIX shell；Windows 开发者可使用 WSL。

```sh
make build
./bin/monitor -data tmp/development -listen 127.0.0.1:5093
# 在第二个终端运行：
MONITOR_GO_API_TARGET=http://127.0.0.1:5093 npm --prefix web run dev
```

从本机 `tmp/development/setup-token` 读取初始化码，浏览器访问 `http://127.0.0.1:5188`。Go 会嵌入前端资源，运行 Go 构建或测试前应先生成 `web/dist`。后端修改后需要重新构建并重启。开发和验收使用隔离 SQLite 文件，不使用正式数据。

SQLite 是唯一业务存储；资源由本套安装共享，创建人字段只作审计。菜单隐藏不能代替服务端授权。新写接口需检查权限，数据库变更需验证真实文件上的迁移、约束和事务。依赖锁文件必须保留，新增依赖记录用途、版本与许可证。

```sh
make test
npm --prefix web run format:check
```

Go 代码使用 `gofmt`，Go 实现另运行 `go test ./...`。API 变更启动隔离服务做真实 HTTP 检查，PR 中写明端口；界面变更验证桌面、窄屏和相关权限。纯文档改动运行 `npm --prefix web run check:docs` 即可，该检查只需 Node.js。检查未执行时明确说明，不能写成通过。

提交 PR 时说明问题、最终行为、实际验证、兼容性和迁移影响。新增文案同步五语目录，变量、机器字段和用户内容不能随显示语言改变。可见界面变更附脱敏截图，用户可感知变更写入 Unreleased。重大行为调整先讨论范围。贡献按 Apache-2.0 的贡献条款处理，保留第三方署名；当前没有额外 CLA 或 DCO 流程。交流应尊重他人，围绕问题和证据讨论。

## 问题与安全反馈

普通问题使用仓库 Issue，附版本或提交、系统架构、安装方式、浏览器与界面语言、复现步骤、预期与实际结果。不要提交数据库、完整日志、Cookie、API 密钥、主密钥、接入码或 Agent 配置。

仓库已开启私密漏洞报告，可通过 [Report a vulnerability](https://github.com/lorvanto/monitor-go/security/advisories/new) 或 **Security → Advisories → Report a vulnerability** 提交安全问题。若入口不存在，只公开询问私密联系方式，不公开漏洞细节或利用步骤。`SECURITY.md` 不会自动开启该功能，当前也未配置独立安全邮箱、聊天群或响应时限。具体以 [安全政策](../../SECURITY.md) 为准。

## 首次公开源码前

- [x] 五语当前指南、贡献、安全、支持、路线图和变更记录已整理。
- [x] Apache-2.0、NOTICE、直接依赖许可证清单与许可文本已加入。
- [x] 缺陷／功能／翻译 Issue 模板、PR 与 Release 说明模板已加入。
- [x] 确定公开仓库 `lorvanto/monitor-go`，简介与 topics 描述当前监控范围。
- [x] 审查实际暂存文件与内容；`.gitignore` 不能清理已经跟踪或存到其他路径的数据。
- [x] 从暂存源码的独立导出目录完成构建和测试，确认无旧目录、软链接或本机运行数据依赖。
- [x] module 路径和内部导入已同步到正式仓库地址。
- [x] Issues 和私密漏洞报告已开启并验证。
- [ ] 按实际需要配置分支保护和 CI；目前尚未配置。

```sh
git status --short --untracked-files=all
git ls-files
git diff --cached --stat
git diff --cached --check
npm --prefix web run check:docs
```

还应在本机审查实际暂存内容，检查密钥、私人地址、机器路径和真实截图。示例使用假数据与 example 域名。不要上传 `data/`、`tmp/`、`backups/`、上传文件或凭据。上述命令不能代替完整的敏感信息检查。

公开源码仓库已经创建。后续同步前审查待推送内容，确认 remote 指向 `https://github.com/lorvanto/monitor-go.git`，再推送获准的分支。仅在实际创建其他仓库时选择建仓步骤；已有本地内容时，避免自动生成另一份 README 或许可证造成无关历史。版本标签、发布资产和下载链接仍需在实际发布并验证后添加。

## 发布下载包前

先确定要承诺的操作系统与架构，完成 `make test`、格式检查和 `make build`，记录工具链与提交。对候选构建重新执行依赖与可达漏洞检查，不能沿用历史“零漏洞”结论。交叉编译成功不代表目标系统运行通过；目标机需要验证安装、初始化、登录、权限、探测、Agent 重连、文件、通知和停止流程。

升级与回退使用测试数据库演练。停机完整备份数据库及伴随文件、`master.key` 与原上传文件；旧二进制不能直接读取 schema 7，回退依赖整套升级前备份。确认服务端与 Agent 的兼容组合，明确未验证平台。

选定发布版本后，同步 `web/package.json`、锁文件根包版本、`internal/app/server.go` / `internal/app/settings.go` 返回的版本、变更记录和使用指南。当前没有 `-version` 参数或链接器版本注入机制，不使用不存在的参数。数据库版本单独管理。

发布归档包含对应平台二进制、启动说明、项目 LICENSE／NOTICE 与该产物的完整第三方许可资料。当前第三方清单覆盖直接依赖，发布二进制前还需按实际链接和打包内容核对间接依赖。生成真实归档的 SHA-256，记录源码提交、工具链和已测平台。`make build` 不自动生成归档、校验和、签名或容器镜像。

先建立草稿 Release，按模板填写变更、兼容性、下载资产、校验和、验证结果与限制，核对后再公开。已提供 [Docker 源码构建与 Compose](DOCKER.md)，尚未发布预构建仓库镜像、签名服务或自动发布流水线；发布镜像前另行核实标签、许可证与目标平台。

## 文档维护

修改命令、支持范围或协作规则时，同步相关五语指南。法律文本保留官方英文原文，其他语言说明不改写许可条款。历史设计与验证资料仅记录当时范围；新发布必须以当前候选代码和实际验证为准。

容器变更需通过 `docker compose config --quiet` 和 `node scripts/test-docker.mjs`。持久化、主机指标边界和本地构建方式见 [Docker 部署](DOCKER.md)。向镜像仓库发布仍需单独执行并核实发布任务。
