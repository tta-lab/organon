# 06 — Src 原子批量 edit 与 edit 接管

**What to build:** 让 Src 支持与 Pi builtin edit 相同的单文件多替换合同，并在验证完整覆盖后成为唯一 active edit 工具。

**Blocked by:** 04 — Src 媒体读取与 read 接管

Status: ready-for-agent

- [ ] Src `edit` action 使用 `path` 和非空 `edits[]`，每项包含 multiline-capable `oldText` 与 `newText`
- [ ] Public CLI batch mode 从 stdin 读取 JSON edits envelope，并将 JSON 结果单独写 stdout；既有 human BEFORE/AFTER 模式保持可用
- [ ] 所有 oldText 都对原始文件匹配且必须唯一；overlapping、nested、missing、duplicate、empty 和 no-op edits 被拒绝
- [ ] 任意验证失败都保持文件原样；成功时所有替换一次写入并保留 BOM 和 line endings；Pi batch edit 路径移除现有 srcop 的 100KB 限制，不比被取代的 builtin edit 更早拒绝普通文本文件
- [ ] Go editing core 实现真正 batch operation，不循环调用 incremental single-edit path
- [ ] 成功结果包含一个 aggregate diff/patch 和 first changed line
- [ ] Prompt 要求同文件多个 disjoint exact edits 使用一次 Src edit，且说明 exact text 已知时不需要 symbols
- [ ] Src Extension 扩展 ticket 04 建立的同一套 provenance/lifecycle policy 来停用 builtin edit，并在 session shutdown 只恢复本实例实际接管的 builtins，不新增会互相覆盖 active-tool state 的第二套策略

