# Lockedin — Curated multi-modal content model (implementation plan)

**Companion spec:** `docs/superpowers/specs/2026-05-31-curated-multimodal-content-model-design.md`
**Status:** Active
**Tracked:** Lives in the repo, kept in sync with the implementation.

---

## How to read this plan

Same convention as `2026-05-16-low-noise-aggregator-implementation.md`: **mentor-style, no code bodies.** You implement the backend Go; this tells you what to build, in what order, what "done" looks like, and where to be careful. Claude authors the things already delegated — SQL migrations/queries (shapes are in the spec §4–8), test code, HTML templates, CSS — and reviews each step you write.

**Relationship to the original plan:** this supersedes the *topic→Google-News* half of **Milestone 5**, reshapes **Milestone 6 (fetcher)** to carry the schema/catalog/`source_type` work, and replaces **Milestone 7 (web UI)** with the catalog + blended-reader described here. The original plan's M0–M4, M8 (notifications), M9 (tests), M10 (deploy) still stand.

**Order matters.** Phase 1 → 2 give a working curated *article* reader. Phases 3–4 are additive (no rework). Don't start Phase 4 (other modalities) before Phase 2 proves the pipeline.

**Checkboxes** (`- [ ]`) track progress as you go.

---

## Phase 1 — Make article real

**Goal:** The schema supports all three modalities, and the fetcher stores card fields (title, link, summary, thumbnail) from a real curated catalog.

### Task 1.1 — Schema migration: add the multi-modal columns

**Files:** Create `sql/schema/000008_multimodal_columns.sql` (goose format).

- [x] Write the `+goose Up`: on `feeds`, add `source_type text NOT NULL DEFAULT 'article' CHECK (source_type IN ('article','youtube','podcast'))`, plus nullable `category text` and `description text`. On `items`, **rename `content` → `summary`** (it's the card blurb now, not a full body) and add nullable `image_url text` (the card thumbnail). (Rationale: spec §4. `media_url`/`media_type` playback fields are deferred — not in this migration.)
- [x] Write the matching `+goose Down` (reverse order): drop `items.image_url`, rename `summary` back to `content`, then drop the three `feeds` columns. *(Note: in Postgres a `RENAME COLUMN` can't be combined with `ADD`/`DROP` in one `ALTER` — make it its own statement.)*
- [x] Run `goose -dir sql/schema postgres "$DB_URL" up`.
- [x] Verify with `psql "$DB_URL" -c "\d feeds"` and `\d items` — confirm the columns and the CHECK constraint exist.

**Gotcha:** `ADD COLUMN … NOT NULL` needs the `DEFAULT 'article'` so existing rows backfill (they'll be cleared in Task 1.3 anyway). The CHECK is what makes `source_type` a safe discriminator — a typo'd value is rejected at write time.

### Task 1.2 — Regenerate sqlc and confirm the structs

**Files:** none by hand — `internal/database/*` is generated.

- [x] Run `sqlc generate`.
- [x] Confirm `database.Feed` now has `SourceType string`, `Category sql.NullString`, `Description sql.NullString`; and `database.Item` has `ImageUrl sql.NullString`.
- [x] Run `go build ./...` — should still compile (existing `InsertItemParams` is unchanged; new columns are nullable and not yet referenced).

**Gotcha:** if gopls shows stale errors, trust `go build` + `go vet` over the editor — they re-index lazily (you hit this before).

### Task 1.3 — Seed a verified article catalog

**Files:** Create `sql/seed/catalog.sql`. Claude provides the SQL once feeds are verified.

- [x] **Sanity-check each candidate feed** — `curl -s <feed_url> | grep -oE "<item>|<entry>|<title>|media:thumbnail" | head` to confirm it has items and (ideally) a thumbnail field. Full body is NOT required now — cards need only title + link + summary + thumbnail. Strong starting candidates (match your interests): `https://simonwillison.net/atom/everything/` (LLM/AI/Claude-heavy), `https://jvns.ca/atom.xml` (Julia Evans), `https://danluu.com/atom.xml`. Curate for quality + a usable thumbnail.
- [x] Clear the old topic rows: `psql "$DB_URL" -c "TRUNCATE feeds CASCADE;"` (dev only — cascades to items + user_subscriptions).
- [x] Claude writes `catalog.sql`: `INSERT … ON CONFLICT (feed_url) DO UPDATE` rows with `feed_url`, `title`, `site_url`, `source_type='article'`, `category`, `description`. Run it via `psql "$DB_URL" -f sql/seed/catalog.sql`.
- [x] Verify: `psql "$DB_URL" -c "SELECT title, source_type, category FROM feeds;"`.

**Gotcha:** `TRUNCATE … CASCADE` wipes items and everyone's subscriptions — fine in dev, never in prod.

### Task 1.4 — Fetch and confirm the card fields

**Files:** `cmd/fetcher/main.go` — extend the item mapping to populate the summary (strip tags from `item.Description`) and `image_url` (spec §7 extraction order). *(The one bit of fetcher code Phase 1 touches — you implement, I review.)*

- [x] `go run ./cmd/fetcher`.
- [x] Verify the card fields: `psql "$DB_URL" -c "SELECT count(*) AS total, count(image_url) AS with_thumb, count(summary) AS with_summary FROM items;"`.
- [x] Spot-check one: `psql "$DB_URL" -t -c "SELECT left(summary,160), image_url FROM items ORDER BY fetched_at DESC LIMIT 1;"` — summary is short plain text (no HTML tags); `image_url` is an http(s) URL or empty.

**Done when:** items carry title, url, a clean summary, and (where the feed provides one) a thumbnail.

---

## Phase 2 — Follow & read (the payoff)

**Goal:** A user browses the catalog, follows individual sources, and gets an appealing chronological feed of preview cards that link out to the source.

### Task 2.1 — Catalog queries ✅

**Files:** Modify `sql/queries/feeds.sql`.

- [x] Added `ListCatalog :many` (all feeds, ordered) **and** `ListFollowedFeedIDs :many` (the user's followed feed_ids). `sqlc generate`. *(Used a two-query + Go-map approach instead of a single `LEFT JOIN … is_following` — the handler correlates them. Chosen for readability over conciseness.)*
- [x] Confirmed `ListCatalog` returns `[]Feed` and `ListFollowedFeedIDs` returns `[]uuid.UUID`.

### Task 2.2 — Simplify the follow handler ✅

**Files:** Modify `cmd/api/main.go` (`handlerCreateSubscription`).

- [x] **Done:** `POST /subscriptions/{feed_id}` reads `feed_id` via `r.PathValue` (path param — symmetric with the delete route, not a hidden form field), parses to `uuid.UUID`, gets the user, calls `CreateUserSubscription(user.ID, feedID)` with `custom_title` NULL, redirects to `/catalog?msg=added`. No `FeedURLForTopic`, no gofeed call.
- [ ] ~~Delete the topic provider~~ — **kept on purpose.** The old topic handler is commented out and `FeedURLForTopic`/`UpsertFeed` are retained to power the planned **v1 topic-search "Discover" path** (see Task 2.7). So the provider is *not* retired.

**Gotchas:** invalid/missing `feed_id` → redirect with an error msg (3xx, not 500). `CreateUserSubscription` is idempotent (`ON CONFLICT DO NOTHING`), so a double-follow is harmless.

### Task 2.3 — Catalog page ✅

**Files:** `cmd/api/main.go` (`handlerBrowseCatalog` + route, template registration); `web/templates/catalog.html` + CSS.

- [x] `GET /catalog` (auth-wrapped) → `ListCatalog` + `ListFollowedFeedIDs` → build `[]CatalogCard{Feed, IsFollowing}` via a Go map of followed ids → render the `catalog` template.
- [x] Registered `GET /catalog` and the `catalog` template in `main()`.
- [x] Template: per row shows title, description, category, a `source_type` tag, and a **Follow** form (`POST /subscriptions/{feed_id}`) or **Following ✓** (`POST /subscriptions/{feed_id}/delete`) based on `IsFollowing`. Added a Catalog nav tab.

**Done when:** `/catalog` lists every curated source and you can follow/unfollow, with the button reflecting state. ✅

### Task 2.4 — `ListItemsForUser` query ✅

**Files:** Modify `sql/queries/items.sql` (Claude provides; shape in spec §8).

- [x] Add `ListItemsForUser :many` (items INNER JOIN subscriptions INNER JOIN feeds, newest-first, `LIMIT/OFFSET`). `sqlc generate`.
- [x] Confirm the row type carries `SourceTitle`, `SourceType`, `ImageUrl`, and `Summary`.

### Task 2.5 — The card reading page ✅

**Files:** Modify `cmd/api/main.go` (`handlerHome` / `GET /`). Claude writes/updates `web/templates/home.html` + CSS. **No `bluemonday`** — summarys are plain text.

- [x] **Contract (you implement):** `GET /` when logged in → `ListItemsForUser(user.ID, limit, offset)` → pass the rows to the template. Before passing, **validate each `image_url`**: blank it unless it starts with `http://`/`https://`, so a bad scheme never reaches `<img src>`. No HTML sanitization needed — the summary is already plain text from the fetcher.
- [x] Claude writes `home.html`: the card list from spec §8 — each item is an `<a class="card card-{{.SourceType}}" href="{{.Url}}" target="_blank" rel="noopener noreferrer">` with thumbnail (if present), source+date, title, summary. `source_type` drives card styling (play overlay for youtube, badge for podcast).

**Gotchas (security):** the summary renders as an auto-escaped string — do NOT wrap it in `template.HTML`. Validate the `image_url` scheme before emitting `<img>`. Outbound links use `rel="noopener noreferrer"`.

**Done when:** logged in, `/` shows an appealing chronological feed of preview cards; clicking one opens the source in a new tab. **This is the milestone payoff — a working curated card feed.** ✅ *(Rendered into the existing Stillness `feed-item` design — a flat chronological list, text-only. `image_url` is validated in the handler but unused in the template for now; day-dividers (`Days`) and thumbnails are deferred.)*

### Task 2.6 — Subscriptions page → source model ✅ (added scope)

Not in the original plan, but required once follows became `feed_id`-based: the old `/subscriptions` page showed the topic `custom_title` (now NULL) and a hardcoded "via Google News".

- [x] `ListUserSubscriptions` now returns `f.title`, `u.custom_title`, `f.source_type`, `f.category`, `f.last_fetch_status`. `sqlc generate`.
- [x] `subscriptions.html` shows the real source name (prefers `custom_title`, else `f.title`), a `source_type` tag, fetch status, and **Unfollow**; the topic add-form is gone; empty-state links to `/catalog`.
- Note: status shows "pending" until **Phase 3** writes `last_fetch_status` back.

### Task 2.7 — Topic-search "Discover" path (planned, v1 — added scope)

Decided mid-Phase-2: keep topic search **in v1** (not deferred to v2), alongside the curated catalog.

- [ ] Migration: add `curated boolean NOT NULL DEFAULT false` to `feeds`; the seed sets it `true`; `ListCatalog` filters `WHERE curated = true` so ad-hoc topic feeds don't pollute the browse list.
- [ ] Second handler/route (e.g. `POST /discover` with a `topic`): restore the commented topic flow — `FeedURLForTopic` → gofeed validate → `UpsertFeed` (`curated=false`) → `CreateUserSubscription`.
- [ ] Mark topic results as "Discover" (unvetted) in the UI, distinct from curated sources.

---

## Phase 3 — Manual fetch + fetch state

**Goal:** Fetching is **user-triggered**, not a background loop. A Refresh pulls new posts on demand; numbered pages read back into the already-stored archive.

**Decision (mid-Phase-2):** dropped the `time.Ticker` background loop. For a personal app, manual fetch is enough; the only thing that genuinely needs scheduling is the email digest, which a separate cron'd job handles (Phase 5) — *not* a fetch loop. See spec §7.

- [x] **3.1 Numbered pagination on the reading feed.** Server-rendered numbered pages over the stored archive — *not* a refetch (old posts are never deleted; `InsertItem` only inserts). A `CountItemsForUser` query returns the total; the handler derives page count, the `X–Y of Z` range, and prev/next purely by arithmetic; `ListItemsForUser` fetches one page via `LIMIT pageSize OFFSET (page-1)*pageSize`. No JavaScript — every control is an `<a href="/?page=N">`. Chose numbered pages over Read-More/Load-More for clearer orientation ("Page N of M", "items X–Y of Z"). Walkthrough: `docs/pagination-walkthrough.md`.
- [ ] **3.2 Refresh button.** A `POST /refresh` that runs the fetch over the user's subscribed feeds, then redirects to `/`.
- [ ] **3.3 (optional) Fetch-state writeback.** Add an "update feed fetch state" query so each fetch writes `last_fetched_at`/`last_fetch_status`/`last_fetch_error` back to the feed — this turns the subscriptions page's "pending" badge into "ok". Optionally send conditional GETs (`If-None-Match`/`If-Modified-Since` from stored `etag`/`last_modified`, handle `304`) so a refresh doesn't re-download unchanged feeds.

**Deferred (not needed for manual fetch):** the `time.Ticker` loop and `errgroup` concurrency. Revisit only if you later want autonomous background fetching, or if refreshing many feeds gets slow (then parallelize the per-feed fetch).

---

## Phase 4 — Add modalities (additive)

**Goal:** YouTube then podcast, each = a thumbnail/summary extraction tweak + card styling + seed rows. No rework of Phase 1–3 — everything still links out.

- [ ] **4.1 Generalize the `itemParams(feed, item)` mapper** (spec §7): move the article mapping into a `switch feed.SourceType` so per-kind thumbnail/summary extraction slots in.
- [ ] **4.2 YouTube.** Mapper `case "youtube"`: prefer `media:thumbnail` for `image_url`, summary from `media:description`; the card links out to `item.Link` (the watch URL). Card styling: a play-triangle over the 16:9 thumbnail. Seed channels (`youtube.com/feeds/videos.xml?channel_id=…`). Verify cards show a thumbnail + play overlay and open YouTube.
- [ ] **4.3 Podcast.** Mapper `case "podcast"`: `image_url` from `itunes:image`/feed artwork, summary from show notes; the card links out to the episode page (`item.Link`). Card styling: a "Listen" badge. Seed shows (Apple search API → feed URLs). Verify cards show artwork + badge.
- [ ] **4.4 (optional) `link` kind** for Reddit/news: a title+source+date card (no guaranteed thumbnail) that links out.

**Note:** detail when reached.

---

## Phase 5 — Resurfacing email digest

**Goal:** A scheduled email that re-surfaces a few items already in the user's feed (a calm "you might've missed these" nudge), plus a "sources you might like" suggestion. It **reads the DB; it does not fetch.**

**Design decisions (keep it dead simple):** no read-state, no `last_visited`, no `item_notifications` dedup. Items are picked **at random** — occasionally re-showing one is fine for a reminder. The real work is the email plumbing, not the selection.

- [ ] **5.1 `ListRandomItemsForUser` query.** `… JOIN user_subscriptions … WHERE u.user_id = $1 ORDER BY random() LIMIT 5`. Samples the *whole* archive server-side (reusing the paginated `ListItemsForUser` would only sample the newest page; shuffling in Go would mean transferring the whole archive). `ORDER BY random()` is fine at this scale.
- [ ] **5.2 `cmd/notify` binary.** Loop users that have subscriptions → run the query → render an email (HTML + plain-text alternative via `mime/multipart`) → send (provider/SMTP creds in config). "Sources you might like" = catalog rows the user doesn't yet follow — no relevance scoring in v1.
- [ ] **5.3 Schedule with cron** (e.g. `0 8 * * 1`, Monday 8am) running the run-once `notify` binary. No `time.Ticker`.

**Supersedes** the original spec's M8 "new-items digest + `item_notifications`": the digest now resurfaces existing items *at random* rather than tracking what's new.

---

## Testing (per spec §9, woven through the phases)

- [ ] **Mapper unit tests** (`topic_test.go` style): guid-fallback, summary tag-stripping, thumbnail field-preference per `source_type`, nullable handling.
- [ ] **Summary/image safety test** (Phase 1.4 / 2.5): the summary has no markup after mapping; an `image_url` with a non-`http(s)` scheme is rejected/blanked.
- [ ] **Query tests** (DB-backed): `ListCatalogForUser` `is_following` correctness; `ListItemsForUser` user-scoping + ordering.
- [ ] **Handler tests** (`httptest`): follow creates a subscription, unfollow deletes it, per-user isolation (user A's feed never shows user B's items).

---

## Self-review notes (coverage vs spec)

- Spec §2 thesis correction → reflected in memory + spec banner (no code task).
- Spec §4 schema (source_type, category, description, image_url) → Task 1.1. §5 catalog/seed → 1.3. §6 browse/follow → 2.1–2.3. §7 fetcher summary/thumbnail mapping → 1.4 (generalized in 4.1; Phase 3 loop/concurrency). §8 card rendering → 2.4–2.5. §9 testing → Testing section. §10 sequence → Phase order. §11 out-of-scope (in-app playback, OG-unfurl, scraping) → not planned (correct).
