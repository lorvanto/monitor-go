# 项目文档索引

更新日期：2026-10-15。当前开发基线为 `0.4.0`，SQLite 结构版本为 `6`，尚不代表已发布 GitHub Release。项目代码与文档采用 Apache-2.0。当前能力、启动和边界以 [完整中文指南](README.md) 为准，其他文档按下列类型阅读。

## 当前使用与协作

| 文档 | 用途 |
|---|---|
| [英文界面截图](../../README.md#screenshots) | 监控、主机、检测、告警、文件、权限与编码工具；使用独立的合成演示数据 |
| [完整中文指南](README.md) | 当前功能、安装、Agent、通知、文件、升级与备份 |
| [Docker 部署](DOCKER.md) | 源码镜像、Compose、持久卷、初始化、HTTPS、备份恢复与升级 |
| [项目协作说明](COLLABORATION.md) | 仓库范围、开发约束、验证与国际化维护；与根目录英文 AGENTS.md 同步 |
| [多语言文档入口](../README.md) | 简体、英文、日文、韩文、繁体当前指南 |
| [React 前端迁移与验证](REACT_MIGRATION.md) | 当前 React 技术栈、迁移后复查与修复、测试和部署 |
| [国际化与文档维护](INTERNATIONALIZATION.md) | 五语目录、语言协商、验证与新增语言流程 |
| [GitHub 发布与协作](GITHUB.md) | lorvanto 账号下的发布准备、贡献、安全反馈与待办 |
| [应用分发与编码工具](APPLICATION_DISTRIBUTION.md) | 当前应用密钥、机器 API、安装包与编码工具 |
| [繁體中文使用指南](../zh-TW/README.md) | 目前功能、部署與運維 |
| [繁體中文國際化維護](../zh-TW/INTERNATIONALIZATION.md) | 翻譯約定與驗證方式 |
| [繁體中文 GitHub 指南](../zh-TW/GITHUB.md) | 發佈、貢獻、安全與授權說明 |

根目录遵循 GitHub 的英文文件惯例：[贡献](../../CONTRIBUTING.md)、[支持](../../SUPPORT.md)、[安全](../../SECURITY.md)、[变更记录](../../CHANGELOG.md)、[路线图](../../ROADMAP.md)、[许可证](../../LICENSE)、[第三方声明](../../THIRD_PARTY_NOTICES.md)。中文说明集中在本目录。

## 设计与待实施建议

| 文档 | 阅读边界 |
|---|---|
| [总体设计方案](ARCHITECTURE.md) | 第一节为已确认边界；建议选型、完整模块与目录可能尚未实施 |
| [数据模型与权限设计](DATA_MODEL_AND_PERMISSIONS.md) | 完整产品的数据与权限规划，现行结构以仓库 SQL 迁移为准 |
| [接口与运行机制](API_AND_RUNTIME.md) | 当前接口说明与标明为建议的后续协议；现行路由以服务端注册为准 |
| [界面与交互规划](UI_DESIGN.md) | 已实现视觉规范与后续交互规划 |
| [实施计划与验收标准](IMPLEMENTATION_PLAN.md) | 长期阶段和验收目标，不能按整阶段视为已完成 |
| [后续改进任务书](IMPROVEMENT_PLAN.md) | 基于 0.4.0 代码审查整理的缺陷、功能与性能任务及验收标准，按状态表跟踪逐项实施与验证，其余为待实施建议 |

## 历史材料

| 文档 | 阅读边界 |
|---|---|
| [历史验证记录](VALIDATION_HISTORY.md) | 开发期间特定快照的测试与限制，不替代新版本发布验证 |

维护文档时区分“已实现”“待验证”“建议方案”。历史材料不随每次发布改写为新的验收结论；发布结果写入对应版本的变更与 Release 说明。
