---
Status: ready-for-agent
---

## What to build

Add mode selection to the Group create/edit dialog in the frontend. Show the current mode as a badge on group cards. Pass the `mode` field in create/update API calls. The export skill zip functionality should account for mode (native mode groups may export differently from wrapped mode groups).

## Acceptance criteria

- [ ] Group create dialog has a "Mode" dropdown (Native / Wrapped, default Wrapped)
- [ ] Group edit dialog shows current mode and allows changing it
- [ ] Group cards display current mode as a badge/tag
- [ ] API calls (`create`, `update`) include the `mode` field
- [ ] UI is consistent with existing shadcn/ui components

## Blocked by

- Issue #01 (model/config/validation)
