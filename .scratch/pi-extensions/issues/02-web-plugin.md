# 02 — Web 插件

**What to build:** 交付可独立安装的 Web 插件，让 Pi 通过一个原生工具使用现有 Web 搜索、页面读取、文档和 Sourcegraph 能力，同时保持 Go Web domain behavior。

**Blocked by:** 01 — Project 插件与 pnpm 基础

Status: ready-for-agent

- [ ] Web 插件只注册 `web` 工具，并提供 `search`、`fetch`、`docs_resolve`、`docs_fetch`、`sgraph` 的闭合 action schemas
- [ ] 所有字段、默认值、验证和结构化结果与现有 MCP 合同一致
- [ ] Web CLI 为五种操作提供纯 JSON stdout，诊断留在 stderr，并继续调用共享 Web service
- [ ] Extension abort 会取消 CLI 和下游请求；backend selection、配置、缓存、timeout 和错误语义保持不变
- [ ] 大文本结果遵守 Pi 的行数/字节截断合同并提供可操作的 continuation 信息
- [ ] Web 主包及三个目标平台 native packages 可 build/pack，并有每个 action family 的 extension-to-binary 行为验证
- [ ] 本 ticket 不引入 TypeScript/Defuddle fetch 实现

