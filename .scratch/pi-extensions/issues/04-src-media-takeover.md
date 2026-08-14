# 04 — Src 媒体读取与 read 接管

**What to build:** 补齐 Src 对 Pi 已知媒体读取能力的覆盖，然后让 Src 成为 Pi 唯一 active read 工具，并删除不再符合本地路径模型的 Src MCP。

**Blocked by:** 03 — Src 文本与 symbol 读取

Status: ready-for-agent

- [ ] Src read 通过内容签名识别非动画 PNG、JPEG、GIF、WebP 和 BMP，而不是把媒体解码为 UTF-8
- [ ] CLI JSON 用明确 media kind、MIME type 和 base64 data 表达图片，Extension 转换为 Pi image content block 和简洁说明
- [ ] 图片在进入模型前具备与 Pi read 等价的方向和尺寸安全；非视觉模型得到清晰提示
- [ ] Animated PNG 和其他 unsupported binary 明确失败，不返回乱码
- [ ] Src Extension 按 source provenance 仅停用 active 的 Pi builtin read，并保留所有无关 active tools
- [ ] Session 生命周期只恢复该 Extension 实例实际接管的 builtin read，不覆盖用户其他工具选择
- [ ] Project-scoped Src MCP、命令、专属测试和无剩余调用者的 project-scoped source code 被删除，不增加替代 MCP
- [ ] 文档不再宣传 Src MCP，并说明 Src 已接管 Pi read

