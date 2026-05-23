---
Status: ready-for-agent
---

## What to build

Refactor existing `group_mcp_server.go` into three files: public dispatch logic stays, wrapped mode logic moves to a new file, and a `callServiceTool` shared function is extracted from `executeGroupTool`.

No behavioral changes — existing tests must continue to pass.

### callServiceTool prototype (extracted from executeGroupTool)

```
callServiceTool(ctx, svc, toolName, args) -> (result, error):
  1. RPD check if userID > 0 && svc.RPDLimit > 0
  2. GetOrCreateSharedMcpInstanceWithKey
  3. CallTool with configurable timeout
  4. Record RequestStat on success
  5. Save MCPLog (info / error)
  6. Return result
```

## Acceptance criteria

- [ ] Wrapped mode logic (`addGroupTools`, `addGroupResources`, `searchGroupTools`, `executeGroupTool`, argument parsing helpers) is in its own file
- [ ] `callServiceTool` is extracted as a shared function callable from both modes
- [ ] `buildGroupMCPServer` dispatches to wrapped builder when `mode: "wrapped"` (default)
- [ ] All existing `group_test.go` tests pass without modification
- [ ] No behavioral changes to wrapped mode

## Blocked by

- Issue #01 (model/config/validation)
