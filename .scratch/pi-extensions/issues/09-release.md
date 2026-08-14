# 09 — 统一发布与安装验收

**What to build:** 把四个已完成插件作为一个版本一致的 Organon Pi 产品发布，确保受支持平台可直接安装且文档和 CI 描述真实行为。

**Blocked by:** 02 — Web 插件; 04 — Src 媒体读取与 read 接管; 05 — Src 结构化 mutation; 06 — Src 原子批量 edit 与 edit 接管; 08 — OG guarded mutation

Status: ready-for-agent

- [ ] Tag version 是 GoReleaser、四个主包和十二个 native packages 的唯一版本来源
- [ ] Darwin ARM64、Linux x64、Linux ARM64 均以 CGO disabled 交叉构建，每个 native package 只包含对应插件 binary
- [ ] Native packages 在依赖它们的 main package 之前发布，main package 使用 exact-version optional dependencies
- [ ] Runtime 不下载 GitHub latest、不编译 Go、不回退 PATH；unsupported platform 提供 actionable startup error
- [ ] 无网络 pack/install smoke tests 验证所有十六个 package manifests、平台选择、binary execution 和 Pi extension discovery
- [ ] PR CI 运行完整 Go gates 与 pnpm format/typecheck/test/build，并机械检查非 host artifacts
- [ ] Tag workflow 发布版本一致的 GitHub artifacts 和 npm packages，并可通过 dry run 验证顺序和版本
- [ ] 文档覆盖四插件独立安装、支持平台、Src read/edit takeover、symbol ID 规则和 Pi 用户移除重复 MCP 配置
- [ ] Web/project/og/skill MCP 文档保留，TypeScript/Defuddle Web fetch 明确留给后续 spec

