# 07 — OG 只读插件路径

**What to build:** 建立可独立安装的 OG 插件及其 daemon adapter，让 Pi 无需 MCP 即可安全查看认证、PR 和 CI 状态。

**Blocked by:** 01 — Project 插件与 pnpm 基础

Status: ready-for-agent

- [ ] OG 插件只注册 `og` 工具，并先提供 auth_status、pr_find、pr_get、pr_checks、pr_log、pr_failures actions
- [ ] Project alias、PR ID、state、tail 的字段、默认值、范围和错误语义与现有 MCP 合同一致
- [ ] OG CLI 支持 exact project selection 和纯 JSON output，不接受 caller-selected path、root、URI 或 credential inputs
- [ ] PR ID omitted 时保留 current-branch workflow，positive PR ID 时保留 branch-free remote operation
- [ ] 所有调用继续通过 OG daemon；Extension 不读取、传递或记录 forge secret
- [ ] Line、auth 和 PR structured results 与 MCP 共享 domain behavior，并通过 fake daemon caller 和 CLI integration 验证
- [ ] OG 主包及三个目标平台 native packages 可 build/pack

