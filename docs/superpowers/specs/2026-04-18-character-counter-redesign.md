# Character Counter Redesign

**Date:** 2026-04-18

## Overview

Replace the current single `0 characters` counter with a per-platform inline pill row that shows remaining characters for each selected platform, color-coded by proximity to the limit.

## Platform Limits

| Platform | Limit |
|---|---|
| Mastodon | 500 characters |
| Bluesky | 300 characters |

## UI Layout

The `.char-count` div becomes a single right-aligned flex row containing one pill per **selected/checked** platform. If no platforms are checked, the row is hidden.

Each pill displays: `[emoji  N]` where N is characters remaining (limit minus typed length).

Example with both selected and 253 typed:
```
[🦣 247]  [🦋 47]
```

Platform emojis:
- Mastodon: 🦣
- Bluesky: 🦋

## Color States

| Condition | Color |
|---|---|
| Remaining > 20% of limit | Muted (default) |
| Remaining ≤ 20% of limit | Orange |
| Remaining < 0 (over limit) | Red |

For Mastodon (limit 500): orange kicks in at ≤ 100 remaining.  
For Bluesky (limit 300): orange kicks in at ≤ 60 remaining.

## Behavior

- Pills update live on textarea `input` event
- Pills also update when any account checkbox `change` fires (platforms may be checked/unchecked mid-compose)
- `loadAccounts()` already renders checkboxes with `data-provider` values; the counter reads those
- Over-limit: pill turns red, shows negative number (e.g., `-23`). Submit is not blocked — user decides.
- If no accounts are checked, the `.char-count` row is hidden (`display: none`)

## Implementation Scope

Changes confined to:
- `frontend/js/app.js` — rewrite `setupCharacterCounter()`, update the reset after submit
- `frontend/index.html` — the `#char-count` span is replaced; the wrapping `.char-count` div stays
- `frontend/css/style.css` — add `.char-pill`, `.char-pill--warning`, `.char-pill--over` classes

No backend changes needed. Platform limits are hardcoded on the frontend keyed by provider name string.
