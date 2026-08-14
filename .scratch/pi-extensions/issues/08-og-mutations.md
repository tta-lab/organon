# 08 — OG guarded mutation

**What to build:** 完成 OG 插件的 clone、Git network 和 PR mutation，使 Pi 获得完整现有 OG MCP 能力但仍受 daemon policy 保护。

**Blocked by:** 07 — OG 只读插件路径

Status: ready-for-agent

- [ ] OG 工具增加 clone、push、pull、pr_create、pr_modify、pr_comment actions 及完整 action-specific validation
- [ ] Clone 严格要求 registered project 或 HTTP(S) URL 两种 selector mode，并保留 alias/reference 语义
- [ ] PR body/comment multiline content 经 stdin 进入 CLI，JSON stdout 不混入诊断或 secret
- [ ] Push force 默认 false，force 使用 force-with-lease 且 default branch 始终拒绝
- [ ] Pull、clone 和 PR mutations 保留 archived-project、canonical remote、provider 和 branch policy
- [ ] PR modify 至少变更 title/body 之一，空 body 可明确清除；comment body 必须 nonblank
- [ ] 所有操作继续经 daemon credential boundary，禁止 path、token 和 token-env escape hatches
- [ ] CLI、MCP 与 Extension 对等行为通过 fake daemon 和 targeted integration tests 验证

