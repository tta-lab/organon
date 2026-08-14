# 01 — Project 插件与 pnpm 基础

**What to build:** 建立可复用但不单独发布的薄 Pi subprocess adapter，并交付可独立安装的 Project 插件，让 Pi 无需 MCP 即可列出和读取 registered project。

**Blocked by:** None — can start immediately

Status: ready-for-agent

- [ ] pnpm workspace 可以安装、构建、类型检查和测试四插件代码库，Pi 核心包按 peer dependency 规则使用；共享 TypeScript 实现被打入各主包，不产生第五个 runtime package
- [ ] Project 插件只注册 `project` 工具，并以闭合 action union 提供 `list` 与 `get`
- [ ] `list` 保留 include-archived 默认值，`get` 保留 exact alias 验证，结果使用既有五字段 project record
- [ ] Project CLI 的 JSON stdout、stderr 诊断、错误退出和 registry reload-on-call 行为通过 CLI 集成测试
- [ ] 插件通过固定 argv 调用包内 binary，支持取消、JSON 解析和简洁错误转换，不使用 shell、MCP、PATH fallback 或常驻进程
- [ ] Project 主包及 Darwin ARM64、Linux x64、Linux ARM64 native packages 可在无网络测试中完成 build/pack 和平台解析

