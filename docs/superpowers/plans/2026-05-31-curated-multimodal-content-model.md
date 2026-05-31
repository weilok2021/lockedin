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

**Goal:** The schema supports all three modalities, and the fetcher stores card fields (title, link, excerpt, thumbnail) from a real curated catalog.

### Task 1.1 — Schema migration: add the multi-modal columns

**Files:** Create `sql/schema/000008_multimodal_columns.sql` (goose format).

- [ ] Write the `+goose Up`: on `feeds`, add `source_type text NOT NULL DEFAULT 'article' CHECK (source_type IN ('article','youtube','podcast'))`, plus nullable `category text` and `description text`. On `items`, add nullable `image_url text` (the card thumbnail). (Column rationale: spec §4. `items.content` is reused as the card excerpt — no new column. The `media_url`/`media_type` playback fields are deferred — not in this migration.)
- [ ] Write the matching `+goose Down` (drop the four columns).
- [ ] Run `goose -dir sql/schema postgres "$DB_URL" up`.
- [ ] Verify with `psql "$DB_URL" -c "\d feeds"` and `\d items` — confirm the columns and the CHECK constraint exist.

**Gotcha:** `ADD COLUMN … NOT NULL` needs the `DEFAULT 'article'` so existing rows backfill (they'll be cleared in Task 1.3 anyway). The CHECK is what makes `source_type` a safe discriminator — a typo'd value is rejected at write time.

### Task 1.2 — Regenerate sqlc and confirm the structs

**Files:** none by hand — `internal/database/*` is generated.

- [ ] Run `sqlc generate`.
- [ ] Confirm `database.Feed` now has `SourceType string`, `Category sql.NullString`, `Description sql.NullString`; and `database.Item` has `ImageUrl sql.NullString`.
- [ ] Run `go build ./...` — should still compile (existing `InsertItemParams` is unchanged; new columns are nullable and not yet referenced).

**Gotcha:** if gopls shows stale errors, trust `go build` + `go vet` over the editor — they re-index lazily (you hit this before).

### Task 1.3 — Seed a verified article catalog

**Files:** Create `sql/seed/catalog.sql`. Claude provides the SQL once feeds are verified.

- [ ] **Sanity-check each candidate feed** — `curl -s <feed_url> | grep -oE "<item>|<entry>|<title>|media:thumbnail" | head` to confirm it has items and (ideally) a thumbnail field. Full body is NOT required now — cards need only title + link + excerpt + thumbnail. Strong starting candidates (match your interests): `https://simonwillison.net/atom/everything/` (LLM/AI/Claude-heavy), `https://jvns.ca/atom.xml` (Julia Evans), `https://danluu.com/atom.xml`. Curate for quality + a usable thumbnail.
- [ ] Clear the old topic rows: `psql "$DB_URL" -c "TRUNCATE feeds CASCADE;"` (dev only — cascades to items + user_subscriptions).
- [ ] Claude writes `catalog.sql`: `INSERT … ON CONFLICT (feed_url) DO UPDATE` rows with `feed_url`, `title`, `site_url`, `source_type='article'`, `category`, `description`. Run it via `psql "$DB_URL" -f sql/seed/catalog.sql`.
- [ ] Verify: `psql "$DB_URL" -c "SELECT title, source_type, category FROM feeds;"`.

**Gotcha:** `TRUNCATE … CASCADE` wipes items and everyone's subscriptions — fine in dev, never in prod.

### Task 1.4 — Fetch and confirm the card fields

**Files:** `cmd/fetcher/main.go` — extend the item mapping to populate the excerpt (strip tags from `item.Description`) and `image_url` (spec §7 extraction order). *(The one bit of fetcher code Phase 1 touches — you implement, I review.)*

- [ ] `go run ./cmd/fetcher`.
- [ ] Verify the card fields: `psql "$DB_URL" -c "SELECT count(*) AS total, count(image_url) AS with_thumb, count(content) AS with_excerpt FROM items;"`.
- [ ] Spot-check one: `psql "$DB_URL" -t -c "SELECT left(content,160), image_url FROM items ORDER BY fetched_at DESC LIMIT 1;"` — excerpt is short plain text (no HTML tags); `image_url` is an http(s) URL or empty.

**Done when:** items carry title, url, a clean excerpt, and (where the feed provides one) a thumbnail.

---

## Phase 2 — Follow & read (the payoff)

**Goal:** A user browses the catalog, follows individual sources, and gets an appealing chronological feed of preview cards that link out to the source.

### Task 2.1 — `ListCatalogForUser` query

**Files:** Modify `sql/queries/feeds.sql` (Claude provides; shape in spec §6).

- [ ] Add `ListCatalogForUser :many` (the `feeds LEFT JOIN user_subscriptions … is_following` query). `sqlc generate`.
- [ ] Confirm the generated row type has `IsFollowing bool` plus the feed display columns.

### Task 2.2 — Simplify the follow handler; retire the topic provider

**Files:** Modify `cmd/api/main.go` (`handlerCreateSubscription`). Delete `internal/feeds/topic.go` + `internal/feeds/topic_test.go`.

- [ ] **Contract (you implement):** `POST /subscriptions` reads a `feed_id` form value (hidden input from the catalog), parses it to `uuid.UUID`, gets the user from context, calls `CreateUserSubscription(user.ID, feedID)` with `custom_title` left NULL, redirects to `/catalog?msg=followed`. No `FeedURLForTopic`, no gofeed call.
- [ ] Delete the topic provider files once nothing references them; `go build ./...` confirms.

**Gotchas:** invalid/missing `feed_id` → redirect with an error msg (3xx, not 500 — your earlier lesson). `CreateUserSubscription` is already idempotent (`ON CONFLICT DO NOTHING`), so a double-follow is harmless.

### Task 2.3 — Catalog page

**Files:** Modify `cmd/api/main.go` (new `handlerBrowseCatalog` + route, register template). Claude writes `web/templates/catalog.html` + CSS.

- [ ] **Contract (you implement):** `GET /catalog` (auth-wrapped) → get user from context → `ListCatalogForUser(user.ID)` → render the catalog template with the rows.
- [ ] Register `GET /catalog` and the `catalog` template in `main()` (remember: an unregistered template = nil-map panic — your earlier bug).
- [ ] Claude writes the template: per row show title, description, category, a `source_type` badge, and a **Follow** form (`POST /subscriptions` + hidden `feed_id`) or **Following ✓ / Unfollow** (`POST /subscriptions/{feed_id}/delete`) based on `is_following`.

**Done when:** `/catalog` lists every curated source and you can follow/unfollow, with the button reflecting state.

### Task 2.4 — `ListItemsForUser` query

**Files:** Modify `sql/queries/items.sql` (Claude provides; shape in spec §8).

- [ ] Add `ListItemsForUser :many` (items INNER JOIN subscriptions INNER JOIN feeds, newest-first, `LIMIT/OFFSET`). `sqlc generate`.
- [ ] Confirm the row type carries `SourceTitle`, `SourceType`, `ImageUrl`, and `content` (the excerpt).

### Task 2.5 — The card reading page

**Files:** Modify `cmd/api/main.go` (`handlerHome` / `GET /`). Claude writes/updates `web/templates/home.html` + CSS. **No `bluemonday`** — excerpts are plain text.

- [ ] **Contract (you implement):** `GET /` when logged in → `ListItemsForUser(user.ID, limit, offset)` → pass the rows to the template. Before passing, **validate each `image_url`**: blank it unless it starts with `http://`/`https://`, so a bad scheme never reaches `<img src>`. No HTML sanitization needed — the excerpt is already plain text from the fetcher.
- [ ] Claude writes `home.html`: the card list from spec §8 — each item is an `<a class="card card-{{.SourceType}}" href="{{.Url}}" target="_blank" rel="noopener noreferrer">` with thumbnail (if present), source+date, title, excerpt. `source_type` drives card styling (play overlay for youtube, badge for podcast).

**Gotchas (security):** the excerpt renders as an auto-escaped string — do NOT wrap it in `template.HTML`. Validate the `image_url` scheme before emitting `<img>`. Outbound links use `rel="noopener noreferrer"`.

**Done when:** logged in, `/` shows an appealing chronological feed of preview cards; clicking one opens the source in a new tab. **This is the milestone payoff — a working curated card feed.**

---

## Phase 3 — Finish the fetcher (rest of original M6)

**Goal:** The catalog fetches on a schedule, concurrently, politely. (Independent of modality.)

- [ ] **3.1 Run-once → ticker loop.** Wrap the `ListFeeds`-and-loop body in a `Run(ctx)` method: fetch once on start, then a `time.Ticker` (30 min prod / 1 min dev via config). `select` on `ticker.C` and `ctx.Done()`. (Spec §7; original plan M6 has the reasoning.)
- [ ] **3.2 Concurrency.** Fetch feeds in parallel with `golang.org/x/sync/errgroup` + `SetLimit(10)`; per-feed errors are logged and isolated (one bad feed never aborts the tick).
- [ ] **3.3 Conditional GET.** Send `If-None-Match`/`If-Modified-Since` from stored `etag`/`last_modified`; on `304`, just update `last_fetched_at`. Add the "update feed fetch state" query (write back `last_fetched_at`, `last_fetch_status`, `last_fetch_error`, `etag`, `last_modified`).

**Note:** detail these tasks further when you reach them — signatures depend on how Phase 1–2 shake out.

---

## Phase 4 — Add modalities (additive)

**Goal:** YouTube then podcast, each = a thumbnail/excerpt extraction tweak + card styling + seed rows. No rework of Phase 1–3 — everything still links out.

- [ ] **4.1 Generalize the `itemParams(feed, item)` mapper** (spec §7): move the article mapping into a `switch feed.SourceType` so per-kind thumbnail/excerpt extraction slots in.
- [ ] **4.2 YouTube.** Mapper `case "youtube"`: prefer `media:thumbnail` for `image_url`, excerpt from `media:description`; the card links out to `item.Link` (the watch URL). Card styling: a play-triangle over the 16:9 thumbnail. Seed channels (`youtube.com/feeds/videos.xml?channel_id=…`). Verify cards show a thumbnail + play overlay and open YouTube.
- [ ] **4.3 Podcast.** Mapper `case "podcast"`: `image_url` from `itunes:image`/feed artwork, excerpt from show notes; the card links out to the episode page (`item.Link`). Card styling: a "Listen" badge. Seed shows (Apple search API → feed URLs). Verify cards show artwork + badge.
- [ ] **4.4 (optional) `link` kind** for Reddit/news: a title+source+date card (no guaranteed thumbnail) that links out.

**Note:** detail when reached.

---

## Testing (per spec §9, woven through the phases)

- [ ] **Mapper unit tests** (`topic_test.go` style): guid-fallback, excerpt tag-stripping, thumbnail field-preference per `source_type`, nullable handling.
- [ ] **Excerpt/image safety test** (Phase 1.4 / 2.5): the excerpt has no markup after mapping; an `image_url` with a non-`http(s)` scheme is rejected/blanked.
- [ ] **Query tests** (DB-backed): `ListCatalogForUser` `is_following` correctness; `ListItemsForUser` user-scoping + ordering.
- [ ] **Handler tests** (`httptest`): follow creates a subscription, unfollow deletes it, per-user isolation (user A's feed never shows user B's items).

---

## Self-review notes (coverage vs spec)

- Spec §2 thesis correction → reflected in memory + spec banner (no code task).
- Spec §4 schema (source_type, category, description, image_url) → Task 1.1. §5 catalog/seed → 1.3. §6 browse/follow → 2.1–2.3. §7 fetcher excerpt/thumbnail mapping → 1.4 (generalized in 4.1; Phase 3 loop/concurrency). §8 card rendering → 2.4–2.5. §9 testing → Testing section. §10 sequence → Phase order. §11 out-of-scope (in-app playback, OG-unfurl, scraping) → not planned (correct).
