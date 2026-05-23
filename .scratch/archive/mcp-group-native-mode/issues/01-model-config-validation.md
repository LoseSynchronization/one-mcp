---
Status: ready-for-agent
---

## What to build

Add the `mode` field to `MCPServiceGroup` model (string, default `"wrapped"`, valid values: `"native"` / `"wrapped"`). Add a system-wide configuration option `GroupNativeToolDelimiter` (default `"."`). Enforce service name validation on create/update — service names must not contain the configured delimiter character.

This is the foundation slice: everything else depends on the model having a `mode` field and the delimter being configurable.

## Acceptance criteria

- [ ] `MCPServiceGroup` model has a `mode` field defaulting to `"wrapped"`
- [ ] `POST /api/groups` accepts optional `mode` field
- [ ] `PUT /api/groups/:id` accepts optional `mode` field
- [ ] `GET /api/groups` returns `mode` in response
- [ ] `GroupNativeToolDelimiter` is added as a system option, default `"."`
- [ ] Service name validation on create/update rejects names containing the delimiter character
- [ ] Model validation tests pass
- [ ] Handler validation tests pass (existing CRUD tests still green)

## Blocked by

None - can start immediately
