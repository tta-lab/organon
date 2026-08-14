# 05 — Src 结构化 mutation

**What to build:** 为 Src 插件加入现有 symbol-aware mutation，使 agent 能用准确返回的 ID 完成代码和 Markdown 的结构化写入，而无需复述旧文本。

**Blocked by:** 03 — Src 文本与 symbol 读取

Status: ready-for-agent

- [ ] Src 工具提供 `replace`、`insert`、`delete`、`comment` 的闭合 action schemas
- [ ] Replace/delete 使用准确 symbol ID；insert 使用准确 ID 和 before/after position；multiline content 从 Extension 传入 CLI stdin
- [ ] Code symbols 与 Markdown sections 保留既有 replace/insert/delete 行为；Markdown comment 继续明确拒绝
- [ ] Comment 支持读取既有 doc comment 和写入新 doc comment，并保持现有语言支持及错误语义
- [ ] 所有 mutation CLI 命令提供纯 JSON 成功结果和 aggregate diff 信息，诊断写 stderr
- [ ] 每次 mutation 在 resolved absolute target 的 Pi file mutation queue 中覆盖完整 child-process read-modify-write 窗口
- [ ] 失败不会产生未声明的部分写入，成功后返回足够信息让 agent 重新获取可能变化的 symbol IDs

