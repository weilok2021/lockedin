# Lockedin — Curated, multi-modal content model (design)

**Status:** Approved design, ready for implementation planning
**Date:** 2026-05-31
**Revises:** `2026-05-16-low-noise-aggregator-design.md` — specifically §1 (thesis), §3 (non-goals), §5.2 (content tables), §6.4 (reading flow), and Milestone 7. Everything else in the original spec (auth, multi-user model, architecture, email, deployment) still stands.

**The model in one line:** a curated, multi-platform feed of **appealing preview cards that link out**. The app curates trusted sources (blogs/Substacks, YouTube channels, podcasts); users follow individual sources; the reader shows a chronological feed of cards (thumbnail + title + source + short summary) that open the **source platform** to read/watch/listen. Nothing is republished in-app — Lockedin is a curated *front door*, which keeps it clear of content-ownership/copyright and free-tier limits.

---

## 1. Why this redesign

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

## 2. Thesis correction

The original spec framed the product as *anti-engagement / anti-addictive*. That framing was wrong and is corrected here:

- **North star:** quality, curated content — material that is good for the user's brain/productivity (e.g. Claude Code best practices, life advice from trusted creators).
- **The villain is junk and uncurated noise — not engagement.** Engagement mechanics (gamification, streaks, notifications, discovery, "excitement") are a neutral tool. Pointed at quality content they are *desirable* — the Boot.dev model: deliberate gamification that drives people to do something valuable.
- **Curation is the quality filter** and the product's core value. "Low-noise" means *curate which sources get in*, not *limit how much content the user gets*.
- One guardrail: aim mechanics at the user's genuine goals; avoid manufacturing compulsion detached from real value (e.g. streak-guilt).

## 3. Core decisions

1. **Curated catalog, not free-text search.** The *app* owns a curated catalog of trusted sources. Users never provide URLs.
2. **Source-as-unit subscription.** Users browse the catalog and follow *individual sources/creators* (like channel subscriptions), not topic bundles. Reuses the existing `user_subscriptions(user_id, feed_id)` join almost as-is.
3. **Read on the source — link-out preview cards.** The reader never republishes article bodies, audio, or video. Each item is a card (thumbnail + title + source + short summary) that links to the origin platform. This is the simplest path and sidesteps content-ownership/copyright and free-tier limits.
4. **Multi-modal by design, one modality at a time in build.** A single discriminator (`source_type`: `article`, `youtube`, `podcast`) drives **card styling** and thumbnail extraction. v1 *implements* `article` first; podcast/youtube are additive.
5. **One pipeline.** Blogs, YouTube channels, and podcasts are all RSS, so the existing `ListFeeds → parse → insert` fetcher serves all of them. The only per-kind difference is *thumbnail/summary extraction* (fetch) and *card styling* (web).

## 4. Data model

The pivot is absorbed by a few **additive columns** — no table redesign.

**`feeds` (the catalog) — add:**
```
source_type  text NOT NULL CHECK (source_type IN ('article','youtube','podcast'))
                          -- the discriminator; a source has exactly one kind
category     text         -- optional: browse grouping ("AI", "Productivity"); nullable
description  text         -- optional: short blurb for the catalog card; nullable
```
Migration on existing rows: `source_type DEFAULT 'article'` (existing Google-News rows are cleared during seeding anyway).

**`items` — add (nullable):**
```
image_url    text   -- preview thumbnail for the card (feed-provided); nullable
```
`items.content` is **renamed to `summary`** — a short plain-text blurb for the card, not a full body. (A column rename in this migration, not a new column.)

**Not added (deferred):** `media_url` / `media_type` for in-app audio/video playback. The link-out model doesn't need them; add only if a future version embeds playback in-app.

**Unchanged:** `items.url` (the outbound link — the whole point), `items.guid`/`UNIQUE (feed_id, guid)` dedupe, `user_subscriptions(user_id, feed_id)`.

**The rule:** same for every item in a feed → on `feeds` (`source_type`); different per item → on `items` (`image_url`, summary). `source_type` tells the code how to *style the card*.

## 5. The catalog & seeding

The catalog **is** the `feeds` table — since users can only follow app-curated sources, every `feeds` row is a catalog entry. No separate table.

**Seeding mechanism (v1):** a re-runnable seed SQL file, `sql/seed/catalog.sql`, run via psql, using `INSERT … ON CONFLICT (feed_url) DO UPDATE`. Idempotent, version-controlled, keeps curated *data* out of schema *migrations*. (Graduate to a `cmd/seed` Go program later if curation gets fiddly.)

**Consequences of the pivot:**
- Clear the old Google-News rows (`TRUNCATE feeds CASCADE` in dev) and seed the real curated catalog.
- The topic provider (`internal/feeds/topic.go` + tests) and the topic half of `handlerCreateSubscription` retire — replaced by "follow a catalog source by `feed_id`."
- Once seeded, the fetcher finds catalog sources automatically — `ListFeeds` already returns every feed.

**Source verification is now easy:** because cards need only title + link + summary + thumbnail (not a full body), almost any RSS feed qualifies. Curate for *quality and a usable thumbnail*, not for `content:encoded`.

## 6. Browse & follow flow

Two surfaces, both queries over `feeds`, differing only by join:

| Page | Route | Shows | Join |
|---|---|---|---|
| Catalog (new) | `GET /catalog` | *all* curated sources + follow state | `feeds LEFT JOIN user_subscriptions` |
| Your sources (exists) | `GET /subscriptions` | only what the user follows | `feeds INNER JOIN user_subscriptions` |

**New query** powering the catalog page:
```sql
-- ListCatalogForUser
SELECT f.id, f.title, f.description, f.source_type, f.category, f.site_url,
       (s.user_id IS NOT NULL) AS is_following
FROM feeds f
LEFT JOIN user_subscriptions s ON s.feed_id = f.id AND s.user_id = $1
ORDER BY f.category, f.title;
```
`is_following` drives whether the template renders **Follow** or **Following ✓ / Unfollow**.

**Route changes:**
- `GET /catalog` — new handler + page (structurally a cousin of the subscriptions page).
- `POST /subscriptions` — simplified: takes a hidden `feed_id` and inserts one `user_subscriptions` row. No provider, no network call at subscribe-time (the feed already exists).
- `POST /subscriptions/{feed_id}/delete` — unfollow, unchanged.
- `custom_title` becomes optional/NULL — the source has its own `title`; the column is retained for a future "rename a source" feature.

## 7. Fetcher: the `source_type` switch

The fetch loop is unchanged. The only addition is a mapper that extracts the card fields, factored out of `fetchFeed`:

```go
func itemParams(feed database.Feed, item *gofeed.Item) database.InsertItemParams {
    // common to all kinds: title, url (item.Link), guid (fallback item.Link), published_at
    // summary  = short plain-text blurb     (strip tags from item.Description / item.Content)
    // image_url = best available thumbnail   (see below; varies by source_type)
}
```

**Thumbnail (`image_url`) extraction — try in order, else NULL:**
- `item.Image.URL` (gofeed normalizes many feeds here)
- `media:thumbnail` / `media:content` (YouTube channel RSS, many blogs)
- `itunes:image` / feed-level artwork (podcasts)
- first `<img>` in the content body (blogs)
- else NULL → the card renders a placeholder / source badge

**Summary (`summary`):** take `item.Description` (or `item.Content`), strip HTML to plain text, keep it short. `source_type` mainly informs *which thumbnail field to prefer*; the rest of the mapping is uniform across kinds.

`fetchFeed` takes the `database.Feed` row (so it has `source_type`); `ListFeeds` already returns it.

**Fetch trigger (revised).** Fetching is **user-triggered** — a manual *Refresh* button runs `fetchFeed` over the user's feeds on demand; *Read-More* is pure pagination over the stored archive. The `time.Ticker` background loop and `errgroup` concurrency are **deferred** (not needed for a personal, manually-refreshed app). The only thing that genuinely needs scheduling — the **email digest** — is a separate cron'd job that *reads* the DB and resurfaces a few random items (see the implementation plan, Phase 5); it does not fetch. Conditional GET (etag / If-Modified-Since) + fetch-state writeback remain optional polish so a refresh skips unchanged feeds and the subscriptions page shows "ok" instead of "pending".

## 8. Rendering the blended feed (M7)

A single chronological list of **preview cards** across all followed sources; clicking a card opens the source.

**New query:**
```sql
-- ListItemsForUser
SELECT i.id, i.title, i.url, i.summary, i.image_url, i.published_at,
       f.title AS source_title, f.source_type
FROM items i
INNER JOIN user_subscriptions s ON s.feed_id = i.feed_id AND s.user_id = $1
INNER JOIN feeds f ON f.id = i.feed_id
ORDER BY i.published_at DESC NULLS LAST
LIMIT $2 OFFSET $3;
```

**Card template, styled by `source_type`:**
```
{{range .Items}}
  <a class="card card-{{.SourceType}}" href="{{.Url}}" target="_blank" rel="noopener noreferrer">
    {{if .ImageUrl}}<img class="card-thumb" src="{{.ImageUrl}}" alt="" loading="lazy">{{end}}
    <div class="card-body">
      <span class="card-source">{{.SourceTitle}} · {{.PublishedAt}}</span>
      <h2 class="card-title">{{.Title}}</h2>
      <p class="card-summary">{{.Summary}}</p>
    </div>
  </a>
{{end}}
```
`source_type` adds affordances via CSS/markup: a play-triangle over `youtube` thumbnails, a "Listen" badge on `podcast`, plain image on `article`.

**Security — much simpler than rendering full bodies:**
- **Summary is plain text.** Tags are stripped at fetch time and `html/template` auto-escapes the string. No raw HTML rendered → no bluemonday, no `template.HTML`, no XSS surface from bodies.
- **Validate `image_url` is `http(s)`** before emitting `<img src>` (reject `javascript:`/`data:` etc.); if invalid/NULL, render the placeholder.
- **Outbound link** uses `target="_blank" rel="noopener noreferrer"`.

## 9. Testing

- **`itemParams` mapper** — unit tests (per the existing `topic_test.go` style): guid-fallback, summary tag-stripping, thumbnail field-preference per `source_type`, nullable handling.
- **Summary/`image_url` safety** — summary contains no markup after mapping; `image_url` with a non-`http(s)` scheme is rejected/blanked.
- **Queries** (DB-backed): `ListCatalogForUser` (`is_following` correctness), `ListItemsForUser` (user scoping, ordering).
- **Handlers** (`httptest`): follow creates a subscription, unfollow deletes it, per-user isolation (user A's feed never shows user B's items).

## 10. Build sequence

**Phase 1 — items become cards:**
1. Migration (`feeds`: `source_type`, `category`, `description`; `items`: `image_url`) → `sqlc generate`.
2. Seed SQL: clear Google-News rows; insert 2–3 curated **article** sources (curate for quality + a usable thumbnail/summary — full body not required).
3. Run fetcher manually → confirm items have title, url, summary, and (where available) `image_url`.

**Phase 2 — follow + read (the payoff):**
4. Simplify `POST /subscriptions` to `feed_id`; retire the topic provider.
5. Catalog page (`ListCatalogForUser` + `GET /catalog` + template).
6. Reading page (`ListItemsForUser` + the card template, links out) → an appealing curated feed.

**Phase 3 — finish the fetcher (rest of M6):**
7. Ticker loop (run-once → scheduled).
8. Concurrency (`errgroup`) + conditional GET + per-feed error isolation.

**Phase 4 — add modalities (additive):**
9. YouTube (prefer `media:thumbnail`, play-overlay card styling, seed channels).
10. Podcast (artwork thumbnail, "Listen" badge, seed shows via Apple search).
   - Optional later: a `link` `source_type` for Reddit/news (title+link cards, no thumbnail guaranteed).

Phases 1–2 yield a working curated card feed; everything after is additive, no rework.

## 11. Out of scope (v1) / future

- **In-app reading/playback** — sanitized article bodies, `<audio>` players, `<iframe>` video embeds. The earlier draft of this design centered on these; they are now **deferred**. v1 links out; bring them back only if in-app consumption proves worth the ownership/free-tier cost. (`media_url`/`media_type` columns are deferred with them.)
- **OG-unfurl for thumbnails** — fetching each link's `<meta og:image>` for a prettier card. v1 uses feed-provided thumbnails only; this is a later enhancement.
- **Content extraction / scraping** of article bodies — out (and moot under link-out).
- **YouTube topic *search*** (Data API key + quota) — v1 curates channels.
- **Reddit/news `link` source_type, per-source rename, catalog admin UI, gamification/discovery surfaces** — legitimate future directions under the corrected thesis; not v1.
