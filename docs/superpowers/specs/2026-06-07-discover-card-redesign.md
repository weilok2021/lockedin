# Discover Card Redesign — Anchored Grid + Casual Copy

**Date:** 2026-06-07
**Status:** Approved (chosen from browser mockups, layout A + copy variant B)
**Scope:** Visual-only. No new routes, handlers, or behavior changes.

## Context

The discover card on `/catalog` (shipped with Task 2.7) read as "messy". Diagnosis:

1. **No shared grid line** — the form was vertically centered against the copy block, so the field's edges aligned with nothing.
2. **Orphaned chips** — the try-chips are suggestions *for the input*, but sat under the copy, a card-width away from the field.
3. **Dead space** — `.discover-try { width: 100% }` forced a full-width row whose content only filled the left third.
4. **Redundant placeholder** — `golang, stocks, selfimprovement…` repeated the three chips verbatim.

## Decision

### Layout: anchored grid

Two-column grid. Copy block anchors the left, vertically centered against the right stack. Field + button on the right's first row; try-chips on its second row, **left-aligned to the field's edge**.

```
┌── rainbow crown ───────────────────────────────────────────┐
│                                                            │
│  Want more community?    ┌ r/ type any community… ┐ (Discover)
│  Follow any topic        └─────────────────────────┘       │
│  in seconds.             TRY (r/golang) (r/investing) (r/self…)
│                                                            │
└────────────────────────────────────────────────────────────┘
```

### Copy

| Element     | Old                                              | New                           |
|-------------|--------------------------------------------------|-------------------------------|
| Title       | Not on the shelf?                                | Want more community?          |
| Subtitle    | Summon any community straight into your lineup.  | Follow any topic in seconds.  |
| Placeholder | golang, stocks, selfimprovement…                 | type any community…           |

Rationale: direct/casual over whimsical metaphor; subtitle avoids repeating "community" and "topic" matches the form field name. Placeholder no longer duplicates the chips.

### Unchanged

Rainbow crown, glass card, pill field with violet `r/` prefix, blue focus ring, gradient Discover button, TRY label + three chips (zero-JS mini-forms), message banners.

## Implementation

### `web/static/style.css` (`.discover` block, ~lines 983–1099)

- `.discover`: replace `display: flex; flex-wrap: wrap; align-items: center; gap: 1rem 2rem` with:

  ```css
  display: grid;
  grid-template-columns: auto 1fr;
  grid-template-areas:
      "copy form"
      "copy try";
  column-gap: 2.2rem;
  row-gap: 0.65rem;
  align-items: center;
  ```

- Assign areas: `.discover-copy { grid-area: copy; }`, `.discover-form { grid-area: form; }`, `.discover-try { grid-area: try; }`.
  No HTML restructuring needed — `grid-template-areas` places the existing three flat children.
- `.discover-try`: **remove** `width: 100%` (the grid area handles placement now).
- `.discover-sub`: add `max-width: 17ch` so the subtitle wraps after "topic" ("Follow any topic / in seconds."), keeping the copy column compact. Eyeball and adjust; an explicit `<br>` in the template is an acceptable fallback.
- Mobile (`@media (max-width: 640px)`): collapse to a single column —

  ```css
  .discover {
      grid-template-columns: 1fr;
      grid-template-areas: "copy" "form" "try";
  }
  ```

  Keep the existing `.discover-form { flex-direction: column }` + full-width button rules.

### `web/templates/catalog.html` (discover section, lines 20–40)

Text-only edits: title (line 22), subtitle (line 23), placeholder (line 29). Structure stays as-is.

## Verification

1. `go run ./cmd/api` from project root, log in, open `/catalog`.
2. Desktop: chips share the field's left edge; copy block is vertically centered; no dead space below the form.
3. Resize to ≤640px: card stacks copy → field → button → chips, button full width.
4. Focus the input: blue ring still appears on the pill.
5. Click a TRY chip: still POSTs `/discover` and follows the community.

## Design artifacts

Mockup HTML (Bento Pop-faithful) from the brainstorm session: `.superpowers/brainstorm/136790-1780839411/content/` (gitignored, served via the visual companion).
