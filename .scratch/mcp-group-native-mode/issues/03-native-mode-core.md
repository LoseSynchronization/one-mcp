---
Status: ready-for-agent
---

## What to build

Implement the native mode MCP server. When `mode: "native"`, the group exposes all tools from all constituent services directly via `tools/list` with service name prefixing. `tools/call` parses the prefixed name and routes to the correct service via `callServiceTool`. Resources are skipped in native mode.

Tool name format: `{serviceName}{delimiter}{toolName}`, where delimiter defaults to `"."`. Splitting is done at the **first** delimiter occurrence from the left.

### Routing logic (prototype)

```
on tools/call(name: "fetch.get_page", arguments: {...}):
  1. serviceName, rawToolName = splitAtFirstDelimiter(name, delimiter)
  2. svc = group.GetServiceByName(serviceName)
  3. return callServiceTool(ctx, svc, rawToolName, arguments)
```

## Acceptance criteria

- [ ] `mode: "native"` group exposes `tools/list` with all tools prefixed by service name
- [ ] Each tool's `inputSchema` matches the original tool schema unchanged
- [ ] `tools/call` correctly splits prefixed name and routes to the correct service
- [ ] `tools/call` passes arguments through without modification
- [ ] RPD check, stats recording, and logging work via `callServiceTool`
- [ ] Disabled group returns appropriate error
- [ ] Resources are NOT registered in native mode
- [ ] Unit tests pass: mock cache for `tools/list`, mock MCP client for `tools/call`

## Blocked by

- Issue #02 (refactor wrapped + extract shared)
