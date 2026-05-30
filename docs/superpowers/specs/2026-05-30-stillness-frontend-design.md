# LockedIn frontend — "Stillness" design

**Status:** Approved, implementing
**Date:** 2026-05-30
**Scope:** Full frontend surface (CSS + templates). Backend (handlers, routes, `PageData`, SQL) is OUT of scope and must not change.

---

## 1. Concept

The interface should feel like *a quiet room you sit down to read in*, and should make the
anti-addictive thesis **visible**: the feed ends, nothing nags, nothing counts at you. Calm comes
from content, spacing, and typography — not from hiding navigation.

## 2. Design system (defined once, used everywhere)

- **Palette:** stone-sage `#eef0ea` (bg) · ink `#2a2f29` · soft ink `#535b53` · muted `#8b9389` ·
  hairline `#dadfd4` · surface `#f6f7f2` · **dusty-clay accent `#b5734a`** (hover `#c98a5f`).
  Atmosphere via layered radial gradients, not a flat fill.
- **Type:** Instrument Serif (display / headlines / wordmark) + Instrument Sans (UI / body),
  loaded from Google Fonts in `layout.html`.
- **Tokens:** palette, type scale, spacing, radius, one calm transition — all as CSS custom
  properties in `:root`. Everything references these.
- **Motion:** gentle staggered fade-up on content load; full `prefers-reduced-motion` fallback.

## 3. App shell — quiet top bar

Wordmark left · `Feed` / `Subscriptions` right · active page underlined in clay · clay
initial-avatar + `Sign out`. **No badges, no unread counts.**

**Implementation:** the nav is `{{define "appnav"}}` inside `layout.html`. Because `layout.html`
is parsed alongside every page, authenticated pages call `{{template "appnav" .}}` — shared, no
duplication, and `main.go`'s `ParseFiles` lines and handlers stay untouched.

- Active state derived from existing `PageData.Title`: Feed active when `.Title == "Home"`,
  Subscriptions active when `.Title == "Subscriptions"`. No new fields.
- Avatar initial: `{{slice .Email 0 1}}` uppercased in CSS (`text-transform: uppercase`).

## 4. Screens

| Screen | File | Status |
|---|---|---|
| Login = front door | `login.html` | Fully wired. Wordmark, tagline, calm pitch, sign-in form; existing `check-email` / `verified` messages restyled. |
| Signup | `signup.html` | Fully wired. Matching auth treatment + password-hint styling. |
| Feed (live) | `home.html` | Polished **empty state** in the shell. Uses only `Title`+`Email` — does not break the build. |
| Feed (reference) | `web/templates/feed_reference.html` | NOT parsed by Go. Full items feed (day dividers, items, "you're all caught up") + wiring note. For future use. |
| Subscriptions | `subscriptions.html` | Fully wired. Restyled add-form, list, per-row fetch status, `added`/`invalid`/`removed` messages, empty state. |
| Account / change-password | — | **Skipped** (no route/handler yet). |

## 5. Backaffected boundary

Do NOT touch: `cmd/api/main.go` (handlers, routes, `PageData`, `ParseFiles`), `internal/**`,
`sql/**`. Verify after with `go build ./cmd/api/`.

## 6. Wiring the real feed later (no help needed from me at build time)

When the fetcher + items query exist, wire `home.html` to the reference feed:

1. Add to `PageData`:
   ```go
   Days []FeedDay   // grouped, newest first
   ```
   with
   ```go
   type FeedItem struct {
       SourceTitle string
       Title       string
       URL         string
       Dek         string    // optional summary; may be ""
       Ago         string    // e.g. "2 hours ago" / "Thu"
   }
   type FeedDay struct {
       Label string       // "Today", "Yesterday", "Wednesday · 28 May"
       Items []FeedItem
   }
   ```
2. In `handlerHome`, query the user's items (items JOIN user_subscriptions WHERE user_id, ORDER BY
   published_at DESC), group into `[]FeedDay`, set `PageData.Days`.
3. Replace `home.html`'s body with the markup from `feed_reference.html` (it already expects
   `.Days` / `.Items`). The empty state stays as the `{{if not .Days}}` branch.

Grouping by day is done in Go (not the template) because `html/template` has no clean
date-bucketing primitive.

## 7. Files

- **Rewrite:** `web/static/style.css`, `web/templates/{layout,home,login,signup,subscriptions}.html`
- **New:** `web/templates/feed_reference.html`
- `mockups/` are throwaway references; add to `.gitignore`.
