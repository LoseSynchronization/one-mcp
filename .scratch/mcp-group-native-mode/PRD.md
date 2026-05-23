---
Status: ready-for-agent
---

# MCP Group Native Mode

## Problem Statement

Currently, MCP Service Groups expose tools through a two-step wrapped process: clients must first call `search_tools` to discover a service's tool list, then call `execute_tool` to invoke a specific tool. This design forces AI models to perform an extra discovery step before they can understand what capabilities are available. The LLM never sees the actual tool schemas upfront, preventing MCP clients from leveraging their native tool-use capabilities (which rely on `tools/list` returning the full tool catalog). This makes Group MCP endpoints less capable than direct service endpoints and creates friction for AI-powered clients like Cursor.

## Solution

Add a new **Native Mode** (`mode: "native"`) to MCP Service Groups. In this mode, the group MCP server automatically exposes all tools from all constituent services directly via the standard `tools/list` MCP endpoint, with tool names prefixed by their service name for disambiguation. The existing two-step Wrapped Mode (`mode: "wrapped"`) remains available for backward compatibility.

Tool discovery is handled asynchronously: when a service is installed or updated, the system triggers a background fetch of its tool list and caches it. The group simply reads from this cache when building its tool list.

## User Stories

1. As a user configuring an MCP group, I want to choose between "native" and "wrapped" mode when creating a group, so that I can decide how tools are exposed to MCP clients.
2. As a user editing an existing group, I want to change its mode, so that I can adjust the exposure strategy without recreating the group.
3. As a user, I want to see which mode a group is currently using in the group list, so that I can quickly understand how it will behave.
4. As an MCP client connecting to a native-mode group, I want `tools/list` to return all tools from all services in the group, so that I know what the group can do without additional discovery steps.
5. As an MCP client, I want each tool name to include a service prefix (e.g., `fetch.get_page`), so that I can distinguish tools from different services.
6. As an MCP client calling a tool in native mode, I want `tools/call` to route to the correct underlying service and tool, so that the tool invocation works transparently.
7. As a user, I want tool arguments in native mode to be passed through without modification, so that the tool behaves exactly as it would when called directly.
8. As a system administrator, I want the tool delimiter character to be configurable system-wide, so that I can avoid conflicts with tool names that contain the delimiter.
9. As a developer, I want service names validated to not contain the delimiter character, so that tool name parsing is always unambiguous.
10. As a user installing a new MCP service, I want its tools to be automatically fetched in the background and cached, so that groups using native mode can immediately see the new tools.
11. As a user, I want the tool cache to be periodically refreshed, so that tool lists stay up-to-date without manual intervention.
12. As a user enabling/disabling a service that belongs to a group in native mode, I want the tool list to reflect the change, so that only enabled services' tools are exposed.
13. As a client of a disabled group in native mode, I want to receive an appropriate error, so that I know the group is unavailable rather than getting an empty tool list.
14. As a developer, I want the native and wrapped mode implementations to be in separate files, so that changes to one mode don't risk breaking the other.
15. As a maintainer, I want the `callServiceTool` function to be shared between native and wrapped modes, so that RPD checks, stats recording, and logging are consistent.

## Implementation Decisions

### 1. Group Mode Field

`MCPServiceGroup` model gains a `mode` string field (not null, default `"wrapped"` for backward compatibility). Valid values: `"native"`, `"wrapped"`.

### 2. Tool Naming Convention

Tools in native mode are exposed as `{serviceName}{delimiter}{toolName}`. The delimiter is determined by a system-wide option `GroupNativeToolDelimiter` (default: `"."`). The tool name is parsed by splitting at the **first** occurrence of the delimiter from the left.

### 3. Service Name Validation

Service names must not contain the configured delimiter character. Validation is enforced at service create/update time.

### 4. Code Organization

Three files replace the single `group_mcp_server.go`:

- **`group_mcp_server.go`** — Public structs (`groupMCPHandlerEntry`), caches (`groupMCPHandlers`), dispatch logic (`buildGroupMCPHandler` / `buildGroupMCPServer`), the shared `callServiceTool` function, and helper functions (`toolErrorResult`, `toolResultFromStructured`, `extractContent`).
- **`group_mcp_server_wrapped.go`** — `addGroupTools`, `addGroupResources`, `searchGroupTools`, `execGroupTool`, argument parsing helpers (`parseGroupSearchArgs`, `parseExecuteArgs`, `parseArgumentsValue`, `extractRemainingAsArguments`).
- **`group_mcp_server_native.go`** — Native mode server builder, tool prefixing logic, `tools/list` and `tools/call` handlers.

### 5. Shared callServiceTool Function

```go
// Prototype — encodes the shared execution contract
func callServiceTool(ctx, svc, toolName, args) -> (result, error) {
    // 1. RPD check if userID > 0 && svc.RPDLimit > 0
    // 2. GetOrCreateSharedMcpInstanceWithKey
    // 3. CallTool with configurable timeout
    // 4. Record RequestStat on success
    // 5. Save MCPLog (info/error)
    // 6. Return result
}
```

Both `executeGroupTool` (wrapped) and the native `tools/call` handler call this same function.

### 6. Tool Caching Strategy

- **MCP 服务安装/更新时** → 后台 goroutine 调用 `ListTools` 并写入 `ToolsCacheManager`
- **`ToolsCacheManager`** → 保持 10 分钟 TTL，新增 `StartPeriodicRefresh()` 后台定时刷新
- **Native mode `tools/list`** → 从 `ToolsCacheManager` 读取，缓存命中直接返回，缓存未命中返回空列表（不阻塞）

### 7. API Changes

**新增字段：**
- `POST /api/groups` → 接受可选 `mode` 字段
- `PUT /api/groups/:id` → 接受可选 `mode` 字段
- `GET /api/groups` → 返回 `mode` 字段

**新增系统配置：**
- `GroupNativeToolDelimiter` — 字符串，默认 `"."`

### 8. Frontend Changes

- Group 创建/编辑弹窗添加"Mode"下拉选择器（Native / Wrapped）
- Group 卡片显示当前模式标签
- 创建时 mode 不填则默认 wrapped

## Testing Decisions

**测试原则：** 只测外部行为，不测实现细节。遵循项目中已有测试风格。

**需要测试的模块：**

| 模块 | 测试方式 | 现有参考 |
|------|---------|---------|
| Model 校验（mode 字段默认值、服务名分隔符校验） | 纯单元测试 | `market_test.go` |
| Native mode `tools/list`（返回已加前缀的工具列表） | Handler 集成测试，用 `ToolsCacheManager` mock 工具数据 | `group_test.go:TestGroupMCPHandlerToolsList` |
| Native mode `tools/call`（正确解析前缀并路由） | Handler 集成测试，mock MCP client | `group_test.go:TestGroupMCPHandlerSearchToolsSuccess` |
| `callServiceTool` 共享函数 | 单元测试，mock MCP client | `service_test.go` |
| 后台异步工具刷新机制 | 集成测试 | `service_test.go:TestSharedMcpInstanceWithRealMCPServer` |
| Wrapped 模式回归测试 | 已有测试继续通过 | `group_test.go` 全部 |

## Out of Scope

- Native 模式下 Prompts 的支持（当前无此需求）
- 工具列表的 websocket 实时推送
- 跨 group 共享工具缓存
- 工具版本管理和冲突自动解决（多个服务提供同名工具时用前缀区分即可）
- 前端测试（项目当前无前端测试规范）

## Further Notes

- Native 模式不注册 Resources（`addGroupResources` 会跳过），因为 `tools/list` 已提供完整工具信息
- 服务名不允许包含分隔符，在 handler 层校验，错误信息包含当前分隔符字符提示
- 迁移路径：现有 group 的 `mode` 字段默认为 `"wrapped"`，不影响已有客户端
