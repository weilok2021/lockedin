# LockedIn frontend — "Bento Pop" redesign

**Status:** Approved, ready for implementation planning
**Date:** 2026-06-06
**Replaces:** `2026-05-30-stillness-frontend-design.md` (visual system only; its template-architecture decisions — `appnav` partial, `.Title`-derived active state — carry forward).
**Scope:** Full frontend surface (CSS + templates). Backend (handlers, routes, `PageData`, SQL) is OUT of scope and must not change. Zero JavaScript stays zero.

---

## 1. Concept

**Bento Pop**: a light, layered magazine grid that treats the user's curated picks like a
lineup worth getting excited about. The old "Stillness" calm came from the retracted
anti-engagement thesis; the corrected thesis (quality curation, engagement welcome) gets a
matching look — vivid modality colors, real depth, celebratory voice. Gamified in *look and
copy*, not mechanics: no new features, no new data.

Three expressions of layering: elevation shadows on white cards, glassy blurred chips
(nav/pager), and fixed gradient blobs behind the page.

## 2. Design system (defined once in `:root`, used everywhere)

- **Type:** Sora (display: wordmark, titles, headings — tight letter-spacing) +
  Hanken Grotesk (body/UI). Google Fonts links in `layout.html` replace the Instrument pair.
- **Palette:** porcelain `#EEF2F7` (bg) · navy ink `#16203A` · slate `#5A6B8C` (sub-text) ·
  white cards. Accents: blue `#2D5BFF` (primary) · coral `#FF5470` · mint `#00C2A8` ·
  violet `#7C5CFF`. **Light mode only.**
- **Modality gradients** (the signature): article `mint → blue` · youtube `coral → #FF9472` ·
  podcast `violet → coral`. Used on card thumbnails/placeholder bands, catalog bands,
  avatar, CTA accents.
- **Texture:** two fixed radial-gradient blobs (blue top-right, coral bottom-left) on the
  page background — atmosphere without dark mode.
- **Tokens:** colors, gradients, shadow (`0 14px 30px rgba(22,32,58,.12)`), radius (`18px`),
  glass chip recipe (`rgba(255,255,255,.72)` + `backdrop-filter: blur`), one transition.
- **Motion:** staggered fade-up on cards at load (CSS `animation-delay` by `nth-child`),
  hover lift on cards (`translateY(-3px)`), full `prefers-reduced-motion` fallback.

## 3. App shell

Wordmark `LockedIn.` (gradient dot) left · glassy chip nav `Feed / Catalog / Subscriptions`
(active = ink-filled chip) · gradient initial-avatar + ghost `Sign out`. Architecture
unchanged from Stillness: `{{define "appnav"}}` in `layout.html`, active state derived from
`.Title` (`Home` / `Catalog` / `Subscriptions`), avatar `{{slice .Email 0 1}}`.

## 4. Screens

| Screen | File | Treatment |
|---|---|---|
| Feed | `home.html` | Bento grid, 3 cols → 1 on mobile. **Hero = first card of the page** (2×2 span, larger thumb, shows summary) — pure CSS `:first-child`, same template loop. Hero is feed-only (chronology justifies the hierarchy; catalog/subscriptions stay uniform). Compact (non-hero) cards show source + title only — the dek is CSS-hidden for grid density. |
| Feed cards | `home.html` | `card card-{{.SourceType}}`: gradient thumb area with modality badge — badge text is the raw `source_type` uppercased by CSS (`ARTICLE` / `YOUTUBE` / `PODCAST`, no mapping logic); youtube gets a play disc, podcast a 🎧 glyph, article 📝. `{{if}}` no `image_url` → "noimg" variant: 6px gradient accent bar + glyph beside the source line (cards never go flat). Thumb `<img>` keeps `loading="lazy"`; outbound links keep `target="_blank" rel="noopener noreferrer"`. |
| Pager | `home.html` | Same `/?page=N` anchors restyled: glassy pill steps (`← Newer` / `Older →`), round number chips, current = ink-filled. Status line ("Showing X–Y of Z · Page N of M") moves below the controls. |
| Feed end state | `home.html` | Last page (`{{if not .Pagination.HasNextPage}}`): celebratory card — rainbow top strip, 🏁, "That's the whole shelf." |
| Feed empty state | `layout.html` (`feedempty`) | **Rewritten.** The topic-chip forms POST to the retired `POST /subscriptions` route — dead UI, removed. New: bento card with glyph trio, headline ("Your shelf is waiting"), and a gradient-button **link** to `/catalog`. Logic removal only (forms → one `<a>`). |
| Catalog | `catalog.html` | "The shelf": uniform grid of source cards — gradient band + glyph by `source_type`, name, description, category chip, **+ Follow** (ink pill) / **Following ✓** (mint tint pill). Same POST forms. Flat grid, category as chip — no grouping logic. Existing `added/removed/invalid` messages restyled. |
| Subscriptions | `subscriptions.html` | Card-rows: modality tile, name (prefers `custom_title`), category chip, fetch-status pill (`ok` mint / `error` coral / `pending` slate), ghost Unfollow. Same data and forms. |
| Login / Signup | `login.html`, `signup.html` | Floating white card on porcelain + blobs, rainbow modality strip across the card top, gradient CTA. Tagline: "Quality picks. Zero noise." Existing `check-email` / `verified` / error messages restyled. |
| Landing | `landing.html` | Hero: Sora headline with one gradient word ("Your corner of the internet, *curated*."), pitch copy, gradient CTA → signup + ghost → login, and an overlapping rotated fan of three mini modality cards (static HTML/CSS — the layering showcase). |
| Reference feed | `web/templates/feed_reference.html` | **Deleted.** Never parsed by Go; expects a `.Days` shape that doesn't exist. |

## 5. Copy voice

Curation-proud and celebratory; never sleepy, never guilt-driven.

| Old (Stillness) | New (Bento Pop) |
|---|---|
| "A few quiet things have arrived. Take your time." | "Your lineup" + meta line built from `Pagination` (e.g. "132 picks · page 1 of 17") |
| "Your reading room is quiet." | "Your shelf is waiting." |
| "That's everything… rest your eyes" | "That's the whole shelf. 🏁 New finds land after the next refresh." |
| "A hand-picked shelf of writers and shows…" | "Hand-picked sources worth your attention. Follow what feeds you." |

Greeting/meta lines use only data the templates already receive (`Pagination`, counts);
nothing implies unread tracking — there is none.

## 6. Roadmap-friendliness (styled now, wired later)

- `card-article` / `card-youtube` / `card-podcast` variants ship now — Phase 4 sources light
  up with zero CSS work.
- The greeting row reserves a right-aligned slot where Phase 3.2's **Refresh** button will
  sit; a `.btn` gradient style already fits it. No dormant markup shipped.

## 7. Boundary & verification

Do NOT touch: `cmd/api/main.go` (handlers, routes, `PageData`, `ParseFiles`), `internal/**`,
`sql/**`. No JS files. Verify with `go build ./cmd/api/` and a manual click-through of
feed pages 1/N/last, catalog follow/unfollow, subscriptions, auth flows, and the empty
state (after `/dev/reset`).

## 8. Files

- **Rewrite:** `web/static/style.css`,
  `web/templates/{layout,home,catalog,subscriptions,login,signup,landing}.html`
- **Delete:** `web/templates/feed_reference.html`
- Mockups from this session live under `.superpowers/brainstorm/` (gitignored, throwaway).
