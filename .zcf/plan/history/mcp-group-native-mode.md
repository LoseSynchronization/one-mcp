# 执行计划：MCP Group Native Mode

## 上下文
- 目标：为 MCP Service Group 添加 Native 模式，直接通过 tools/list 暴露所有服务工具
- 完整 PRD 请参考 `.scratch/mcp-group-native-mode/PRD.md`（在主仓库根目录）
- 共 6 个 issue，按依赖顺序实施

## 实施顺序
1. Issue 01 — 模型字段 + 配置 + 服务名校验 ← 当前
2. Issue 04 — 后台异步工具刷新（无依赖）
3. Issue 02 — 重构 + 提取 `callServiceTool`
4. Issue 03 — Native 模式核心（tools/list + tools/call）
5. Issue 05 — 前端模式选择器 UI
6. Issue 06 — 集成测试
