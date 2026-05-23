---
Status: ready-for-agent
---

## What to build

When a service is installed or updated, trigger a background goroutine that connects to the service, calls `ListTools`, and writes the result into `ToolsCacheManager`. Add a `StartPeriodicRefresh()` method to `ToolsCacheManager` that periodically refreshes all cached tool entries.

When native mode builds its tool list and the cache is empty, return an empty list gracefully rather than blocking.

## Acceptance criteria

- [ ] Installing a service triggers async `ListTools` → cache write
- [ ] Updating a service triggers async `ListTools` → cache write (if tools changed)
- [ ] `ToolsCacheManager.StartPeriodicRefresh()` refreshes expired entries on a configurable interval
- [ ] Native mode `tools/list` returns empty list (not error) when cache is empty
- [ ] Tools cache is populated correctly for all three service types (stdio, SSE, StreamableHTTP)

## Blocked by

None - can start immediately
