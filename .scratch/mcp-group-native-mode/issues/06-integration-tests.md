---
Status: ready-for-agent
---

## What to build

Write end-to-end integration tests for native mode using the same testing infrastructure as existing `group_test.go` (temporary SQLite DB, Gin test context, `ToolsCacheManager` mock data). Cover the full flow: create group in native mode → initialize MCP session → `tools/list` → `tools/call`. Also run wrapped mode regression tests to confirm no breakage.

## Acceptance criteria

- [ ] Test: create group with `mode: "native"` → `tools/list` returns prefixed tool names
- [ ] Test: `tools/call` with prefixed tool name routes to correct service
- [ ] Test: `tools/call` with invalid prefix returns appropriate error
- [ ] Test: `tools/call` with invalid tool name returns appropriate error
- [ ] Test: disabled group returns error in native mode
- [ ] Test: disabled service within group is excluded from tool list
- [ ] Test: delimiter configuration affects tool name format
- [ ] All existing wrapped mode tests continue to pass

## Blocked by

- Issue #03 (native mode core)
- Issue #04 (async tool refresh)
