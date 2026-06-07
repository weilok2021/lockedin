# LockedIn — v1 design (consolidated)

**Status:** Living design doc, kept in sync with the implementation
**Date:** 2026-06-07
**Consolidates:** `2026-05-16-low-noise-aggregator-design.md` (original v1 design) + `2026-05-31-curated-multimodal-content-model-design.md` (content-model revision). Those two files are removed; git history preserves them. Section numbering follows the 05-16 backbone, so the implementation plans' `§` references still resolve (see Appendix A).
**Companion docs:** implementation plans in `docs/superpowers/plans/`; frontend visual system in `2026-06-06-bento-pop-frontend-design.md` (replaces Stillness); pagination math in `docs/pagination-walkthrough.md`.

**The product in one line:** a curated, multi-platform feed of **appealing preview cards that link out**. The app curates trusted sources (blogs/Substacks, YouTube channels, podcasts); users follow individual sources; the reader shows a chronological feed of cards (thumbnail + title + source + short summary) that open the **source platform** to read/watch/listen. Nothing is republished in-app — LockedIn is a curated *front door*, which keeps it clear of content-ownership/copyright and free-tier limits.

| Revision | What changed |
|---|---|
| 2026-05-16 | Original design: topic→Google-News feeds, anti-engagement framing, ticker fetcher, new-item email digest. |
| 2026-05-31 | Content-model pivot: curated multi-modal catalog + source-as-unit follow + link-out cards; thesis corrected (villain is junk, not engagement); ticker dropped for user-triggered fetch; email re-scoped to a random resurfacing digest. |
| 2026-06-07 | Consolidation. Also reflects shipped reality: numbered pagination on the reading feed, `fetched_at` feed ordering, Bento Pop frontend. |

---

## 1. Background & thesis

### 1.1 Background

Modern content platforms (algorithmic social feeds, YouTube home, X timelines) are engineered for engagement-at-any-cost. The user wants to stay current on trusted sources — engineering blogs, trusted YouTubers, specific Substacks — without subjecting themselves to the platforms' attention-hijacking surfaces.

This project is a **curated content aggregator**: the app curates quality sources, users follow the ones they care about, and a low-friction surface (web feed + email digest) delivers *only* what they chose.

The project also serves as an applied capstone for the user's Boot.dev backend curriculum. Technology choices favor exercise of those skills (Go, Postgres, Docker) over absolute minimum implementation cost.

### 1.2 The content-model pivot (why v1 changed shape)

The original v1 mapped a free-text **topic** to a single **Google News search feed**. Building the fetcher exposed a fundamental flaw: that model cannot deliver readable content.

What we verified against live feeds (2026-05-31):

| Source | Full content in a free feed? | Topic → source (app-owned)? |
|---|---|---|
| Google News (original v1) | ❌ headline + redirect link only | ✅ any topic |
| Dev.to tag feed | 🟡 inconsistent — descriptions ran 76 / 3454 / 14137 chars | ✅ (topic must be a real tag) |
| Medium tag feed | ❌ `<description>` previews only, **no `<content:encoded>`** | ✅ |
| X / Twitter | ❌ no free feed at all (Nitter dead, API paid) — fails at *discovery* | ❌ |
| YouTube channel RSS | ✅ title + link + `media:thumbnail` | 🟡 channel_id needed; *search* needs Data API key |
| Reddit subreddit RSS | ✅ title + link (verified) | ✅ |
| Podcast RSS (via Apple search) | ✅ title + link + artwork + `<enclosure>` | ✅ free Apple search API, no key |

**Two independent gates emerged.** (1) **Discovery** — does the platform publish a machine-readable feed of new items? This is the real gate: X fails here (no free feed), while Substack/YouTube/Reddit/podcasts pass. (2) **Content depth** — does that feed carry the full body, or just title+link? *Full text, for an arbitrary topic, for free* is near-impossible (indexers only point; paywalls truncate).

**The resolution:** don't fight the content-depth gate. **Curate** quality sources (clearing the discovery gate is enough), and instead of republishing bodies in-app — which is hard, and raises ownership/copyright/free-tier problems — **link out**: present an appealing preview card and let the user read/watch/listen on the origin platform.

### 1.3 Thesis

The original spec framed the product as *anti-engagement / anti-addictive*. That framing was wrong and is corrected here:

- **North star:** quality, curated content — material that is good for the user's brain/productivity (e.g. Claude Code best practices, life advice from trusted creators).
- **The villain is junk and uncurated noise — not engagement.** Engagement mechanics (gamification, streaks, notifications, discovery, "excitement") are a neutral tool. Pointed at quality content they are *desirable* — the Boot.dev model: deliberate gamification that drives people to do something valuable.
- **Curation is the quality filter** and the product's core value. "Low-noise" means *curate which sources get in*, not *limit how much content the user gets*. For a source the user deliberately chose, rich content *is* signal — reducing content richness in the name of being "anti-addictive" is a category error.
- One guardrail: aim mechanics at the user's genuine goals; avoid manufacturing compulsion detached from real value (e.g. streak-guilt).

## 2. Goals & core decisions

### 2.1 Goals (v1)

1. **Multi-user web application** with password-based authentication and email verification.
2. **Curated catalog + source-as-unit follow** — the app owns a curated catalog of trusted sources; users browse it and follow individual sources. Users never handle feed URLs (or free-text topics).
3. **On-demand content fetcher** — fetching is user-triggered (a Refresh action / run-once CLI), pulling new items from curated feeds into a durable archive.
4. **Personalized web UI** — server-rendered HTML showing the user's items as link-out preview cards, reverse chronological, with numbered pagination.
5. **Resurfacing email digest** — a scheduled email that re-surfaces a few random items from the user's archive (a calm "you might've missed these" nudge). Reads the DB; never fetches.
6. **Public landing page** for unauthenticated visitors.

### 2.2 Core decisions

1. **Curated catalog, not free-text search.** The *app* owns a curated catalog of trusted sources. Users never provide URLs.
2. **Source-as-unit subscription.** Users browse the catalog and follow *individual sources/creators* (like channel subscriptions), not topic bundles. Reuses the `user_subscriptions(user_id, feed_id)` join as-is.
3. **Read on the source — link-out preview cards.** The reader never republishes article bodies, audio, or video. Each item is a card (thumbnail + title + source + short summary) that links to the origin platform. Simplest path; sidesteps content-ownership/copyright and free-tier limits.
4. **Multi-modal by design, one modality at a time in build.** A single discriminator (`source_type`: `article`, `youtube`, `podcast`) drives **card styling** and thumbnail extraction. v1 *implements* `article` first; youtube/podcast are additive.
5. **One pipeline.** Blogs, YouTube channels, and podcasts are all RSS, so one `ListFeeds → parse → insert` fetcher serves all of them. The only per-kind difference is *thumbnail/summary extraction* (fetch) and *card styling* (web).

## 3. Non-goals (v1)

Explicitly out of scope, with notes on where each could plug in later:

- **In-app reading/playback** — sanitized article bodies, `<audio>` players, `<iframe>` embeds. v1 links out; revisit only if in-app consumption proves worth the ownership/free-tier cost (`media_url`/`media_type` columns deferred with it).
- **OG-unfurl for thumbnails** — fetching each link's `<meta og:image>` for prettier cards. v1 uses feed-provided thumbnails only.
- **Content extraction / scraping** of article bodies — out (and moot under link-out). Scraping JS-rendered pages, Twitter/X, LinkedIn stays out: fragile and platform-hostile.
- **YouTube topic *search*** (Data API key + quota) — v1 curates channels.
- **LLM summarization** — additive (`items` columns); v2.
- **Relevance scoring** — additive (new table); v2.
- **Read-state tracking ("mark as read")** — additive (new table); the resurfacing digest deliberately doesn't need it (§6.2).
- **Native mobile apps** — out.
- **Catalog admin UI, per-source rename, gamification/discovery surfaces** — legitimate future directions under the corrected thesis (§1.3); not v1.
- **AWS-specific services (RDS, ECS, SES, S3)** — deferred to v2 polish; v1 is a single VPS with Docker Compose (§11).
- **JWT-based auth** — server-side sessions are the correct tool for a single-backend web app.

## 4. Architecture

Go binaries sharing one Postgres database; SMTP via a transactional email provider.

```
┌─────────────────────────────────────────────────────────────────┐
│                         Single host                             │
│                                                                 │
│   ┌────────────┐                       ┌────────────────────┐   │
│   │   api      │   ──────────────►     │                    │   │
│   │  (Go)      │   reads/writes        │     Postgres       │   │
│   │            │                       │                    │   │
│   │  - HTTP    │                       │  - users           │   │
│   │  - auth    │                       │  - sessions        │   │
│   │  - HTML    │                       │  - email_tokens    │   │
│   │  - refresh │                       │  - feeds (catalog) │   │
│   └────────────┘                       │  - items (archive) │   │
│         ▲                              │  - user_subs       │   │
│         │ browser                      │  - item_notifs     │   │
│   ┌─────┴──────┐                       └────────────────────┘   │
│   │   User     │                          ▲              ▲      │
│   └────────────┘                          │              │      │
│                                 ┌─────────┴────┐  ┌──────┴────┐ │
│                                 │   fetcher    │  │  notify   │ │
│                                 │ (Go, run-    │  │ (Go, cron;│ │
│                                 │  once CLI)   │  │  planned) │ │
│                                 │ - RSS pull   │  │ - digest  │ │
│                                 │ - dedupe     │  │   email   │ │
│                                 └──────────────┘  └─────┬─────┘ │
│                                                         ▼       │
│                                                  ┌────────────┐ │
│                                                  │ SMTP       │ │
│                                                  │ provider   │ │
│                                                  └────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### 4.1 Pattern: "app + worker, shared database"

The most common backend shape in industry, between strict monolith and microservices: independently-runnable binaries sharing a database. It teaches web-tier vs worker-tier separation — transferable across most production backends.

**Fetch is user-triggered, not a background loop** (decision, mid-build): for a personal, manually-refreshed app, a `time.Ticker` loop earns nothing. The fetch core lives in a shared package (`internal/fetcher`, extraction planned — Phase 3.2) so both the `cmd/fetcher` CLI and the api's `POST /refresh` handler run the same code. The only job that genuinely needs a schedule is the email digest — a separate cron'd binary that *reads* the DB (§6.2); it does not fetch.

### 4.2 Component responsibilities

| Component | Responsibilities |
|---|---|
| `api` | Serve HTTP, authentication (signup, login, verify, logout; reset/change designed in §6.3), render server-side HTML, catalog browse + follow/unfollow, paginated reading feed, on-demand refresh (planned) |
| `fetcher` | Run-once CLI: pull curated RSS feeds, map items to cards (summary + thumbnail), dedupe, insert |
| `notify` *(planned, Phase 5)* | Cron'd run-once binary: random resurfacing digest per user via SMTP |
| Postgres | Persistent store and source of truth; the items table is a durable archive (fetcher only inserts) |
| SMTP provider | Deliver verification emails, password-reset emails, digest emails |

All binaries live in one repository and share `internal/` packages (`internal/config`, `internal/database` from sqlc, `internal/fetcher` planned).

### 4.3 v2 expansion path

When v2 features arrive (LLM summarization, relevance scoring), a third-party process (e.g. Python summarizer) reads from Postgres and writes back enriched data without schema migration of existing rows. The data model is designed to accommodate this without rework (§12).

## 5. Data model

Seven Postgres tables. Migrations are goose (`sql/schema/`), query code is sqlc (`sql/queries/` → `internal/database/`), driver is `database/sql` + `lib/pq`.

### 5.1 Authentication tables

```sql
users (
  id                  uuid PRIMARY KEY,
  email               text UNIQUE NOT NULL,
  hashed_password     text NOT NULL,           -- bcrypt
  email_verified_at   timestamptz,             -- NULL until verified
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
)

email_tokens (
  token         text PRIMARY KEY,
  user_id       uuid REFERENCES users(id) NOT NULL,
  purpose       text NOT NULL,                 -- 'verify' | 'password_reset'
  expires_at    timestamptz NOT NULL,
  used_at       timestamptz
)

sessions (
  token         text PRIMARY KEY,
  user_id       uuid REFERENCES users(id) NOT NULL,
  expires_at    timestamptz NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now()
)
```

- `email_tokens` is one table reused for both email verification and password reset; `purpose` disambiguates.
- `sessions` are server-side; no JWT. The cookie carries only the random session token.
- Only `users` carries `updated_at` — it is the only table whose row state mutates without a more specific timestamp serving the same purpose.

### 5.2 Content tables (globally shared)

```sql
feeds (                                        -- the catalog: every row is a curated source
  id                  uuid PRIMARY KEY,
  feed_url            text UNIQUE NOT NULL,
  title               text,                    -- from feed <title>
  site_url            text,                    -- from feed <link>
  source_type         text NOT NULL DEFAULT 'article'
                      CHECK (source_type IN ('article','youtube','podcast')),
  category            text,                    -- browse grouping ("AI", "Engineering"); nullable
  description         text,                    -- short blurb for the catalog card; nullable
  etag                text,                    -- HTTP If-None-Match   (optional polish)
  last_modified       text,                    -- HTTP If-Modified-Since (optional polish)
  last_fetched_at     timestamptz,
  last_fetch_status   text,                    -- 'ok' | 'http_error' | 'parse_error' | 'timeout'
  last_fetch_error    text,
  created_at          timestamptz NOT NULL DEFAULT now()
)

items (                                        -- the archive: card data, never full bodies
  id            uuid PRIMARY KEY,
  feed_id       uuid REFERENCES feeds(id) NOT NULL,
  guid          text NOT NULL,                 -- publisher's stable identifier
  url           text NOT NULL,                 -- the outbound link — the whole point
  title         text NOT NULL,
  summary       text,                          -- short plain-text blurb (tags stripped at fetch)
  image_url     text,                          -- preview thumbnail (feed-provided); nullable
  author        text,
  published_at  timestamptz,
  fetched_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (feed_id, guid)
)
```

- **Globally shared, not per-user:** if two users follow the same blog, the fetcher pulls once and inserts once — how real aggregators (Feedly, Inoreader) work. `user_subscriptions` (§5.3) provides the per-user view.
- **The on-feeds vs on-items rule:** same for every item in a feed → on `feeds` (`source_type`); different per item → on `items` (`image_url`, `summary`). `source_type` tells the code how to *style the card*.
- **`items` is a durable archive.** RSS is a sliding window (~20–50 recent entries); the fetcher only inserts (`ON CONFLICT DO NOTHING`), never deletes, so the archive outgrows any single feed snapshot. The reading feed paginates over it (§6.4).
- `feeds.last_fetched_at` serves as that table's "updated_at" semantically.
- *(Deferred:* `media_url`/`media_type` for in-app playback — the link-out model doesn't need them.)*

### 5.3 Per-user tables

```sql
user_subscriptions (
  user_id          uuid REFERENCES users(id) NOT NULL,
  feed_id          uuid REFERENCES feeds(id) NOT NULL,
  custom_title     text,                       -- optional rename; NULL = use feeds.title
  subscribed_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, feed_id)
)

item_notifications (
  user_id       uuid REFERENCES users(id) NOT NULL,
  item_id       uuid REFERENCES items(id) NOT NULL,
  notified_at   timestamptz NOT NULL,
  PRIMARY KEY (user_id, item_id)
)
```

- `user_subscriptions` is the join that makes shared content + per-user views work; it *is* the follow.
- `item_notifications` was designed as the new-item-email receipt/queue. The digest redesign (§6.2) deliberately doesn't dedupe, so the table is **currently unused** — retained in the schema in case a "notify on new item" email ever returns.

### 5.4 The catalog & seeding

The catalog **is** the `feeds` table — users can only follow app-curated sources, so every `feeds` row is a catalog entry. No separate table.

**Seeding (v1):** a re-runnable seed SQL file, `sql/seed/catalog.sql`, run via psql, using `INSERT … ON CONFLICT (feed_url) DO UPDATE`. Idempotent, version-controlled, keeps curated *data* out of schema *migrations*. (Graduate to a `cmd/seed` Go program if curation gets fiddly.)

**Source verification is easy under link-out:** cards need only title + link + summary + thumbnail — not a full body — so almost any RSS feed qualifies. Curate for *quality and a usable thumbnail*, not for `content:encoded`. Once seeded, the fetcher finds sources automatically (`ListFeeds` returns every feed).

### 5.5 Explicitly absent from v1

- **No `read_state`** — the digest picks items at random (§6.2) and the web feed shows pages of the archive; neither needs unread tracking. Additive if a real need emerges.
- **No notification queue table** — the digest reads live data; nothing to queue.
- **No `summaries` or `scores`** — additive v2 changes requiring no migration.

## 6. Data flow

### 6.1 Fetch flow (user-triggered)

A fetch run — whether from the `cmd/fetcher` CLI or the api's Refresh (planned, Phase 3.2) — does:

```
Fetch run:
  1. ListFeeds (every catalog source)
  2. For each feed:
       a. GET + parse RSS/Atom with gofeed (10s timeout per feed)
       b. Map each entry to card fields (itemParams, below)
       c. INSERT INTO items (...) ON CONFLICT (feed_id, guid) DO NOTHING
       d. On error (timeout, HTTP, parse): log and continue to the next feed
  3. Done — the reading feed reflects new items on next page load.
```

**The per-item mapper** (factored as `itemParams(feed, item)` when modalities arrive — Phase 4):

- **Common to all kinds:** `title`, `url` (`item.Link`), `guid` (fall back to `item.Link` when the feed omits guid, so empty guids don't collide), `published_at` (guard the nil pointer), `author` (guard the empty slice).
- **`summary`:** take `item.Description` (or `item.Content` when Description is empty), strip tags **before** unescaping entities (so `&lt;dl&gt;` survives as text), collapse whitespace, truncate rune-safely (~200 chars).
- **`image_url` — try in order, else NULL:** `item.Image.URL` (gofeed normalizes many feeds here) → `media:thumbnail` / `media:content` (YouTube, many blogs) → `itunes:image` / feed artwork (podcasts) → first `<img>` in the body (blogs) → NULL (card renders a placeholder/badge). `source_type` mainly informs which thumbnail field to prefer; the rest of the mapping is uniform.

**Design properties (kept from the original loop):**

- **DB-layer dedupe** via `UNIQUE(feed_id, guid)` + `ON CONFLICT DO NOTHING` — atomic, no race.
- **Per-feed failure isolation** — one broken feed never aborts the run; status is per-feed (§8.1).
- **Idempotent** — kill and re-run loses nothing, duplicates nothing.
- **No mid-run retries** — a flaky feed is simply retried on the next refresh.

**Deferred (were in the original design):** the 30-minute `time.Ticker` loop and `errgroup` fan-out — revisit only if autonomous background fetching returns or refreshing many feeds gets slow. **Optional polish:** conditional GETs (`If-None-Match`/`If-Modified-Since`, handle `304`) + fetch-state writeback (`last_fetched_at`/`last_fetch_status`), which also upgrades the subscriptions page's "pending" badge to "ok" (Phase 3.3).

### 6.2 Email: the resurfacing digest (planned, Phase 5)

A scheduled email that **re-surfaces a few items already in the user's archive** — a calm "you might've missed these" nudge, plus a "sources you might like" footer (catalog rows the user doesn't follow yet; no relevance scoring). It reads the DB; it never fetches.

**Deliberately dead simple:** no read-state, no `last_visited`, no `item_notifications` dedup. Items are picked **at random** — occasionally re-showing one is fine for a reminder:

```sql
-- ListRandomItemsForUser: samples the WHOLE archive server-side.
-- (Reusing the paginated ListItemsForUser would only sample the newest page;
--  shuffling in Go would transfer the whole archive. ORDER BY random() is fine at this scale.)
SELECT … FROM items i
JOIN user_subscriptions u ON u.feed_id = i.feed_id
WHERE u.user_id = $1
ORDER BY random() LIMIT 5;
```

**Plumbing:** a `cmd/notify` run-once binary (loop users with subscriptions → query → render HTML + plain-text alternative → send), scheduled with OS cron (e.g. `0 8 * * 1`, Monday 8am). No `time.Ticker`.

> This **supersedes the original "new items" notification path** (per-tick LEFT-JOIN against `item_notifications`, email on non-empty, receipt rows on send). That design's guarantee — at most one email per cycle, no double-notification — came from the receipt table; the digest model doesn't need it because resurfacing is intentionally repeatable. SMTP failure handling stays the same shape: log it, send nothing, the next scheduled run simply tries again.

### 6.3 Authentication flows

Signup, verification, login, and logout are implemented; password reset/change are designed and pending.

**Signup:**
```
POST /signup { email, password }
  → Validate password meets minimum requirements
  → INSERT users (email, hashed_password = bcrypt(password), email_verified_at = NULL)
  → INSERT email_tokens (token, user_id, purpose='verify', expires_at = now + 24h)
  → Send verification email
  → 200 "Check your email"
```

**Email verification:**
```
GET /verify?token=<t>
  → SELECT email_tokens WHERE token = $1 AND purpose='verify'
  → Validate: row exists, not expired, used_at IS NULL
  → UPDATE email_tokens SET used_at = now()
  → UPDATE users SET email_verified_at = now()
  → 302 → /login
```

**Login:**
```
POST /login { email, password }
  → SELECT users WHERE email = $1
  → Verify bcrypt(password, hashed_password)
  → Require email_verified_at IS NOT NULL
  → INSERT sessions (token, user_id, expires_at = now + 30d)
  → Set-Cookie: session=<token>; HttpOnly; Secure; SameSite=Lax
  → 302 → /
```

**Forgot password (request)** *(designed; not yet implemented)*:
```
POST /forgot { email }
  → If user exists: INSERT email_tokens (token, user_id, purpose='password_reset', expires_at = now + 15min)
                    Send reset email
  → Always return identical response: "If that email exists, we sent a link"
```

**Forgot password (consume)** *(designed; not yet implemented)*:
```
GET  /reset?token=<t>   → render form for new password (after validating token)
POST /reset { token, new_password }
  → Validate token (matches above rules)
  → UPDATE users SET hashed_password = bcrypt(new_password), updated_at = now()
  → UPDATE email_tokens SET used_at = now()
  → DELETE FROM sessions WHERE user_id = <user>  -- invalidate all sessions
  → 302 → /login
```

**Change password (logged in)** *(designed; not yet implemented)*:
```
POST /settings/password { current_password, new_password }
  → Verify session
  → Verify bcrypt(current_password, hashed_password)
  → UPDATE users SET hashed_password = bcrypt(new_password), updated_at = now()
  → DELETE FROM sessions WHERE user_id = <user> AND token != <current_session>
  → 200 OK
```

**Logout:**
```
POST /logout
  → DELETE FROM sessions WHERE token = <current>
  → Clear cookie
  → 302 → /
```

### 6.4 Browse, follow & read

**Two browse surfaces, both queries over `feeds`, differing only by join:**

| Page | Route | Shows | Join |
|---|---|---|---|
| Catalog | `GET /catalog` | *all* curated sources + follow state | `feeds LEFT JOIN user_subscriptions` |
| Your sources | `GET /subscriptions` | only what the user follows | `feeds INNER JOIN user_subscriptions` |

The catalog page's `is_following` (the LEFT-JOIN null-check) drives whether each row renders **Follow** or **Following ✓ / Unfollow**.

**Follow routes:**
- `POST /subscriptions/{feed_id}` — follow: insert one `user_subscriptions` row. No provider, no network call at subscribe-time (the feed already exists in the catalog).
- `POST /subscriptions/{feed_id}/delete` — unfollow. The `feeds` row stays — other users may follow it.
- `custom_title` is optional/NULL — the source has its own `title`; the column is retained for a future "rename a source" feature.

**Reading flow:**
```
GET /
  → logged out: public landing page
  → logged in:  one page of preview cards + numbered pager
```

A single chronological list of **preview cards** across all followed sources; clicking a card opens the source platform (`target="_blank" rel="noopener noreferrer"`).

**Pagination is count-then-arithmetic** (no probe rows; full walkthrough in `docs/pagination-walkthrough.md`):

```sql
-- CountItemsForUser: one integer; drives page count + "X–Y of Z" labels
SELECT COUNT(*) FROM user_subscriptions u
JOIN items i ON i.feed_id = u.feed_id
WHERE u.user_id = $1;

-- ListItemsForUser: exactly one page of card rows
SELECT i.id, i.title, i.url, i.summary, i.image_url, i.published_at,
       f.title AS source_title, f.source_type
FROM user_subscriptions u
JOIN feeds f ON f.id = u.feed_id
JOIN items i ON i.feed_id = f.id
WHERE u.user_id = $1
ORDER BY i.fetched_at DESC          -- arrival order: stable, never NULL
LIMIT $2 OFFSET $3;                 -- LIMIT = pageSize, OFFSET = (page-1)*pageSize
```

- Ordering is **`fetched_at DESC`** (not `published_at`): it's NOT NULL and append-only, so pages are stable; feeds with missing/garbled dates can't scramble the order.
- The handler derives `totalPages` (ceil), the `X–Y of Z` range, prev/next, and clickable page numbers by arithmetic; out-of-range `?page` clamps to the last real page.
- **Zero JavaScript** — every pager control is an `<a href="/?page=N">`.

**Card markup, styled by `source_type`** (illustrative; the visual system is the Bento Pop spec):

```
{{range .Items}}
  <a class="card card-{{.SourceType}}" href="{{.Url}}" target="_blank" rel="noopener noreferrer">
    {{if .ImageUrl.Valid}}<img src="{{.ImageUrl.String}}" alt="" loading="lazy">{{end}}
    <div class="card-body">
      <span class="card-src">{{.SourceTitle}} · {{.PublishedAt}}</span>
      <h2 class="card-title">{{.Title}}</h2>
      {{if .Summary.Valid}}<p class="card-dek">{{.Summary.String}}</p>{{end}}
    </div>
  </a>
{{end}}
```

`source_type` adds affordances via CSS/markup: a play-triangle over `youtube` thumbnails, a "Listen" badge on `podcast`, plain image on `article`.

## 7. Security

Required, not optional:

1. **Password hashing** with bcrypt (cost ≥10). Never sha256/md5 or any unsalted/fast hash.
2. **Email enumeration resistance** — login responses do not distinguish "no such email" from "wrong password"; forgot-password always returns the same response.
3. **HTTP-only, Secure, SameSite=Lax cookies** for the session token. Immune to XSS-based theft.
4. **Server-side sessions, no JWT.** Logout is instantaneous (delete row).
5. **Session invalidation on password change** — both reset completion and logged-in change delete the user's other sessions.
6. **Token entropy:** all random tokens (sessions, email_tokens) are at least 32 bytes from `crypto/rand`.
7. **Authorization checks on every per-user route.** Every query against `user_subscriptions` is scoped by `user_id` from the session.

Card rendering is much simpler to secure than full bodies would have been:

8. **Summary is plain text.** Tags are stripped at fetch time and `html/template` auto-escapes the string. No raw HTML rendered → no sanitizer dependency, no `template.HTML`, no XSS surface from bodies.
9. **Validate `image_url` is `http(s)`** before emitting `<img src>` (reject `javascript:`/`data:` etc.); if invalid/NULL, render the placeholder.
10. **Outbound links** use `target="_blank" rel="noopener noreferrer"`.

Optional for v1 (flagged as a known gap if skipped): rate-limiting login attempts (simple `(email, attempt_at)` table; lock 15 min after 5 failures in 15 min). Without it, login is brute-forceable.

## 8. Error handling

Principle: **fail loudly per-feed, gracefully overall.**

### 8.1 Feed-level failures

| Failure | Behavior |
|---|---|
| HTTP 4xx/5xx | Record `last_fetch_status='http_error'`, error message. Continue to next feed. |
| Timeout (10s) | Record `last_fetch_status='timeout'`. Continue. |
| DNS / connection refused | Record `last_fetch_status='http_error'`. Continue. |
| Body returned but not valid RSS/Atom | Record `last_fetch_status='parse_error'`. Continue. |
| Valid feed, zero items | Treat as success (`last_fetch_status='ok'`). |

Each feed is processed in its own error boundary; one bad feed never aborts the run. Status is surfaced on `/subscriptions` so the user can see which sources are broken. *(Status writeback is the Phase 3.3 polish; until then the page shows "pending".)*

### 8.2 Idempotency

The fetch run is fully idempotent: killing and restarting loses nothing and duplicates nothing — items dedupe at the DB layer (`UNIQUE(feed_id, guid)` + `ON CONFLICT DO NOTHING`). The digest's worst case (dying mid-send) is a duplicate *reminder* email — by design tolerable (§6.2).

### 8.3 SMTP failures

| Email type | Failure handling |
|---|---|
| Verification, password reset | Surface failure to the user immediately (500 with clear message); they resubmit the form. |
| Digest | Log and skip; the next scheduled run simply tries again. |

### 8.4 Database failures

- `database/sql` + `lib/pq`; the stdlib pool handles transient drops.
- DB fully unavailable → `api` returns 503; `fetcher`/`notify` log and exit (they're run-once; the next invocation retries).
- No exotic retry logic for v1.

### 8.5 Explicitly out of scope for v1

Circuit breakers; dead-letter queues; exponential backoff (the next refresh/cron run already retries); graceful-shutdown finesse (idempotency makes `Ctrl+C` safe).

## 9. Operational hygiene

Minimal but real:

- **Structured logging** with `log/slog`; log lines carry context (feed_id, user_id, request_id).
- **`/healthz` endpoint** on `api`: 200 if the DB is reachable.
- **Per-run fetch summary log line:** "run complete: N feeds checked, M new items, K errors."
- **No metrics/tracing/dashboards in v1.** Sufficient at this scale.

## 10. Testing strategy

Aim for **"tests where bugs would be silent, expensive, or embarrassing"** — not coverage maximization.

### 10.1 Tier 1 (must-have)

| Test | Type | Why |
|---|---|---|
| Item dedupe | Integration (real Postgres) | Verifies `ON CONFLICT DO NOTHING` |
| Fetch-run restart idempotency | Integration | Kill mid-run → no duplicates, no losses |
| Per-user data isolation | Integration | User A cannot read user B's subscriptions/items (security) |
| Password hashing roundtrip | Unit | Catastrophic regression guard |
| Email enumeration resistance | Integration | Identical responses for valid/invalid emails on login + forgot |
| Session middleware | Integration | Valid cookie → user; missing/expired cookie → redirect to /login |
| RSS parsing edge cases | Unit | Missing guid (fall back to link), missing pubDate, empty authors, malformed XML |
| Item mapper (`itemParams`) | Unit | Summary tag-stripping (strip-then-unescape order), thumbnail preference per `source_type`, nullable handling |
| Summary/`image_url` safety | Unit | Summary contains no markup after mapping; non-`http(s)` `image_url` is rejected/blanked |
| Catalog + feed queries | Integration | `is_following` correctness; `ListItemsForUser` scoping + ordering; `CountItemsForUser` drives correct page math |
| Follow/unfollow handlers | `httptest` | Follow creates the row, unfollow deletes it, both scoped to the session user |

### 10.2 Tier 2 (nice-to-have)

HTTP handler responses (status codes, redirects) via `httptest`; digest email rendering (golden-file); conditional-GET behavior (if/when added).

### 10.3 Tier 3 (skip)

Trivial CRUD wrappers, getters/setters, pure glue.

### 10.4 Tooling & isolation

- `testing` stdlib + `stretchr/testify/require`; table-driven tests for multi-case scenarios.
- Real Postgres for integration tests (`testcontainers-go` or a dedicated local `lockedin_test` DB); `net/http/httptest` for handlers.
- **Isolation pattern:** wrap each integration test in a transaction with `defer tx.Rollback()`. sqlc's generated `Queries` accept the `DBTX` interface, which both `*sql.DB` and `*sql.Tx` satisfy — so tests pass a `*sql.Tx` and rollback wipes test data automatically.

### 10.5 Out of scope for v1

End-to-end browser tests (Playwright/Selenium); load tests; mutation/property-based testing, fuzzing.

## 11. Deployment

v1 ships as a deployed, always-on service. Two layers: **architectural decisions** (locked in; affect code structure) and **operational decisions** (deferred research; do not affect code).

### 11.1 Fixed architectural decisions

- **Containerization:** all v1 code ships as Docker images — one Dockerfile per Go binary, multi-stage build to a minimal final image (Alpine or distroless), statically compiled (`CGO_ENABLED=0`).
- **Orchestration:** Docker Compose on a single host: `api`, `postgres` (+ cron-invoked `fetcher`/`notify` containers or host cron). A named volume backs Postgres data.
- **Configuration:** all environment-specific values via env vars (`DB_URL`, SMTP creds, `BASE_URL`, …) loaded from `.env` (godotenv locally; referenced by `docker-compose.yml` in deploy). No secrets in code or repo.
- **Reverse proxy + TLS:** Caddy in front of `api` with automatic Let's Encrypt issuance/renewal.
- **Workflow:** SSH in, `git pull`, `docker compose up -d --build`. No CI/CD, no zero-downtime choreography — manual is fine at this scale.

These are independent of which cloud/VPS provider is chosen — they describe how the app is *packaged and run*, not where.

### 11.2 Deferred research items (decide before launch)

**Hosting** (ranked by cost): Oracle Cloud Free Tier (free forever; approval unpredictable) → Hetzner CAX11/CX22 (€3–5/mo, best paid value) → DigitalOcean/Linode/Vultr (~$5–6/mo) → AWS/GCP/Azure (defer to v2 if at all).

**Email service:** Resend (3k emails/mo free, recommended start) → AWS SES (cheapest, needs domain verification + production approval) → Postmark/SendGrid/Mailgun.

**Domain:** $10–15/yr (`.xyz`/`.app`/`.dev` via Cloudflare/Namecheap/Porkbun); free workarounds for testing: `sslip.io`, `duckdns.org`.

Switching any of these later is configuration, never code: Go cross-compiles to ARM trivially, Docker abstracts the host, SMTP is provider-uniform.

## 12. Future expansion

Each is additive — none requires migrating existing v1 data:

| Future feature | Required change |
|---|---|
| LLM summary per item | Nullable `summary_text`/`summary_generated_at` on `items`; summarizer service fills `WHERE summary_text IS NULL` |
| Relevance scoring | New `relevance_scores (user_id, item_id, score, generated_at)` table; UI LEFT-JOINs and orders by score when present |
| Read state | New `read_state (user_id, item_id, read_at)` table |
| In-app playback / sanitized bodies | `media_url`/`media_type` columns + sanitizer; only if worth the ownership/free-tier cost |
| OG-unfurl thumbnails | Fetch `<meta og:image>` per link for prettier cards |
| Reddit/news `link` source_type | Title+link cards, no thumbnail guaranteed |
| Topic-search "Discover" | Search within the curated catalog / map topics onto curated sources |
| Per-feed fetch schedule | `fetch_interval_minutes` on `feeds`; fetcher selects by "due" predicate |
| AWS managed services (RDS, ECS, SES, S3) | Operational migration only; no code change |

**Tracked but unscheduled:** post-password-change notification email; auth-event audit log; per-source rename (schema already supports via `custom_title`); OPML import/export; 2FA; account deletion/data export; login rate-limiting (§7).

---

## Appendix A — section map for older references

The implementation plans reference spec sections by number. Mapping from the two source docs:

| Old reference | Now |
|---|---|
| 05-16 §1 thesis · §2 goals · §3 non-goals | §1 · §2.1 · §3 |
| 05-16 §4 architecture · §5 data model · §6.1 fetcher · §6.2 notifications · §6.3 auth · §6.4 reading | §4 · §5 · §6.1 · §6.2 (now the digest) · §6.3 · §6.4 |
| 05-16 §7 security · §8 errors · §9 ops · §10 testing · §11 deploy · §12/§14 future | §7 · §8 · §9 · §10 · §11 · §12 |
| 05-16 §13 implementation budget | dropped — progress lives in the plans |
| 05-31 §1 why · §2 thesis · §3 decisions | §1.2 · §1.3 · §2.2 |
| 05-31 §4 schema · §5 catalog/seed · §6 browse/follow · §7 fetcher · §8 cards/reading · §9 testing · §10 sequence · §11 out-of-scope | §5.2 · §5.4 · §6.4 · §6.1 · §6.4 · §10 · plans · §3/§12 |
