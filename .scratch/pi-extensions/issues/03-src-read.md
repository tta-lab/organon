# 03 — Src 文本与 symbol 读取

**What to build:** 建立 cwd-native 的 Src 插件读取路径，让 Pi 能以相对或绝对路径读取普通文本，并高效地通过准确 symbol ID 导航代码和 Markdown。

**Blocked by:** 01 — Project 插件与 pnpm 基础

Status: ready-for-agent

- [ ] Src 与 project registry 完全解耦；相对路径基于当前 Pi cwd，绝对路径直接规范化使用，并处理惯常的前导 `@`
- [ ] Src 插件注册一个 `src` 工具，提供 `symbols` 与 `read` action，不要求 project alias
- [ ] Src CLI 提供正常公开的 symbols/read JSON 子命令，同时保留既有 human root invocation
- [ ] 普通文本无需 tree-sitter 支持即可读取；read 支持一索引 line offset/limit、symbol-relative pagination 和 Pi 等价截断/continuation
- [ ] Symbols 使用固定 depth 2，并返回 opaque ID、display name、kind、parent、byte/line ranges、targetability 和 attached-doc 状态；后续 symbol actions 使用同一 outline 深度
- [ ] Symbol/section read 只接受当前 symbols 输出中的准确 ID，display name 不被当作 ID
- [ ] Prompt 明确 symbols 通常更高效、symbol ID 不是 symbol name、symbol-scoped 操作必须复制准确 ID，且结构变化后需刷新 IDs；尚未交付的 edit action 不在本 ticket 的 active prompt 中出现
- [ ] Src 主包和三个目标平台 native packages 可 build/pack；此 ticket 尚不禁用内置 read

