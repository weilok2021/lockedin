# Lockedin — v1 implementation plan (mentor-style)

**Companion spec:** `docs/superpowers/specs/2026-05-16-low-noise-aggregator-design.md`
**Status:** Active
**Local-only:** This document is gitignored. Do not commit or push.

---

## How to read this plan

This is a **mentor-style plan**, not an agent-execution script. You implement the code; this document tells you what to build, in what order, what "done" looks like, and where to be careful. Each milestone is a self-contained chunk that produces something you can actually run and verify.

**No code in this plan.** Library and tool recommendations, decision points, gotchas, and verification criteria are here. The code is yours to write.

**Order matters.** Each milestone assumes the previous ones are complete and working. Don't skip ahead — earlier milestones build the foundation later ones depend on.

**Pace yourself.** Estimates assume focused weekend work. Don't compress them artificially; this is a learning project.

## Milestone roadmap

| # | Milestone | Estimated time |
|---|---|---|
| 0 | Cross-cutting setup | 0.5 weekend |
| 1 | Project skeleton | 0.5 weekend |
| 2 | Schema + migrations | 0.5 weekend |
| 3 | DB access layer | 0.5-1 weekend |
| 4 | Auth surface | 1-1.5 weekends |
| 5 | Sources management | 0.5 weekend |
| 6 | Fetcher core | 1 weekend |
| 7 | Web UI for reading | 1 weekend |
| 8 | Notification path | 0.5 weekend |
| 9 | Test pass | 0.5-1 weekend |
| 10 | Containerize + deploy | 1 weekend |

**Total: 7-9.5 focused weekend-days.** This is honest. The spec budget (4-6 weekends) assumes full weekend days, not partial evenings.

---

## Milestone 0: Cross-cutting setup

**Goal:** Have the tools and habits in place before touching the project.

**Done when:**
- Go 1.22+ installed (`go version` shows ≥1.22).
- Docker Desktop or Docker Engine running.
- A code editor with Go support (VS Code + `gopls` extension is fine).
- A local terminal you're comfortable in.
- You've skimmed three resources (next item).

**Read before starting:**
1. **Mat Ryer — "How I write HTTP services after 13 years"** (search the title). 30 minutes. This will shape Milestone 4 more than any other reading.
2. **`golang-standards/project-layout` README disclaimer at the top.** 5 minutes. Read so you don't take that repo as gospel — it's a menu, not a recipe.
3. **The spec.** Re-read §4 (architecture) and §5 (data model) so they're fresh.

**A note on dev workflow:**
- Run Postgres in Docker locally (`docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=dev postgres:16-alpine`).
- Use a `.envrc` or `.env` file for local config; never commit it.
- Add `.env` and `.envrc` to `.gitignore` (already covered by the existing `.gitignore`).

---

## Milestone 1: Project skeleton

**Goal:** A buildable Go project structure with both binaries compiling and an empty connection to Postgres.

**Done when:**
- `go build ./...` succeeds.
- `go run ./cmd/api` starts an HTTP server on a configurable port and serves `GET /health` returning 200.
- `go run ./cmd/fetcher` starts, prints "fetcher tick" once, and exits (the cron loop comes later).
- A `Makefile` or shell aliases for the commands you run most.

**Spec reference:** §4 Architecture, §11.1 (containerization decisions inform structure)

**Directory layout (recommended):**

```
lockedin/
├── cmd/
│   ├── api/main.go
│   └── fetcher/main.go
├── internal/             # private packages (Go convention: not importable outside this module)
│   ├── db/               # database access (Milestone 3)
│   ├── auth/             # auth helpers (Milestone 4)
│   ├── fetcher/          # RSS pulling logic (Milestone 6)
│   ├── notify/           # notification rendering + send (Milestone 8)
│   └── web/              # HTTP handlers + templates (Milestones 4-7)
├── sql/
│   ├── schema/           # migration files, one per logical change (Milestone 2)
│   └── queries/          # sqlc input — SQL queries with -- name: annotations (Milestone 3)
├── web/templates/        # HTML templates (Milestone 7)
├── go.mod
├── go.sum
├── Makefile
└── .gitignore
```

**Why `internal/` not `pkg/`?** Go's `internal/` is a language-enforced convention — code in `internal/foo` can only be imported by code in this module. `pkg/` is just a directory name with no enforcement. For a single-module project, `internal/` is the right choice. Don't cargo-cult `pkg/` from other projects.

**Library/tooling decisions to make:**
- **HTTP router:** Use `github.com/go-chi/chi/v5`. Reasons: stdlib-compatible (`http.Handler`), great middleware chaining, URL params, sub-routers. Don't use stdlib mux — it's improved in 1.22 but chi is still cleaner. Don't use gin/echo — they invent their own context and request types, which fights stdlib.
- **Config:** Just `os.Getenv` + a small `config.Load()` function that reads env vars. Don't pull in viper or cobra for this.
- **Logger:** stdlib `log/slog` (Go 1.21+). Use the JSON handler in production, the text handler in dev.

**Implementation considerations:**
- Each binary's `main.go` should be tiny — set up dependencies, hand off to a `Run` function that returns an error. This makes testing easier later.
- Set up graceful shutdown from the start (`signal.NotifyContext` for SIGINT/SIGTERM, cancel context, wait for handlers/loops to drain).
- `cmd/api/main.go` should NOT import from `cmd/fetcher` or vice versa. Both depend only on `internal/*`.

**What to test:** Not yet. No business logic exists.

---

## Milestone 2: Schema + migrations

**Goal:** All 7 tables created via versioned, reversible migrations.

**Layout:** Migration files live in `sql/schema/`. This sits alongside `sql/queries/` (used in Milestone 3 by sqlc), so all SQL lives under one top-level `sql/` directory — easy to find, easy for sqlc to point at, separates schema-evolution SQL from runtime query SQL.

**Done when:**
- 7 sets of up/down `.sql` files in `sql/schema/` cover every table from §5.
- `make migrate-up` applies them to your local Postgres.
- `make migrate-down` rolls them back cleanly.
- Re-applying `migrate-up` after `migrate-down` works.

**Spec reference:** §5 Data model

**Library/tooling:**
- **`golang-migrate/migrate`** with the CLI. Most popular Go migration tool, uses plain SQL files (no Go DSL), works as a CLI for dev and as a library if you want to embed migrations in the binary later. Point it at `sql/schema/` with `-path sql/schema`.
- Alternative: `pressly/goose` (also good, supports Go migrations too).

**File naming:** golang-migrate expects files like `000001_create_users.up.sql` / `000001_create_users.down.sql`. The numeric prefix is the version — they apply in order. Use 6 digits with leading zeros for clean sorting.

**Implementation considerations:**
- **UUIDs:** Use Postgres-side generation. Either `gen_random_uuid()` (requires Postgres 13+, no extension needed in 16+) or install `pgcrypto`. Don't generate UUIDs in Go unless you have a reason.
- **Timestamps:** Always `timestamptz`, never `timestamp`. The latter is timezone-naive and a source of bugs.
- **Strings:** Use `text`, not `varchar(N)`. Postgres handles unbounded text efficiently and the length constraint usually ends up being wrong.
- **Foreign keys:** Add `ON DELETE CASCADE` where it makes sense. Deleting a user should cascade-delete their sessions, subscriptions, notifications, email tokens. Items and feeds should NOT cascade — they're shared.
- **Indexes:** Postgres auto-indexes primary keys. Add indexes manually on:
  - `users(email)` — already implied by UNIQUE
  - `items(feed_id, published_at DESC)` — for the "list items for a user" query
  - `item_notifications(user_id) WHERE notified_at IS NULL` — partial index for "what needs to be emailed?" (advanced; skip for v1 if you want).
- **Migration ordering matters.** `users` before anything that references it, etc.

**Common trap:** People put the schema in a single `001_init.sql` migration. Don't. One migration per logical change. Even if "logical change" for v1 means one per table, it's good practice and matches how you'll add features in v2.

**What to test:** Indirect — Milestone 3 tests will fail if the schema is wrong.

---

## Milestone 3: DB access layer

**Goal:** Typed Go functions for every database operation the app will need.

**Layout:** sqlc reads queries from `sql/queries/` and the schema from `sql/schema/` (same files Milestone 2 created). Generated Go lands in `internal/db/` (configurable via `sqlc.yaml` at repo root).

**Done when:**
- `sqlc generate` produces code in `internal/db/` with no errors.
- `internal/db` package compiles.
- Functions exist for: create user, get user by email, set email verified, change password, insert email token, consume email token, create session, get session, delete session, list user sessions, upsert feed, insert item with ON CONFLICT, update feed fetch status, create subscription, delete subscription, list subscriptions for user, list items for user, list un-notified items for user, insert item_notifications batch.
- Basic integration tests pass against a real local Postgres.

**Spec reference:** §5 Data model, §6 Data flow (queries to design against)

**Library/tooling:**
- **`sqlc`** — you write SQL in `sql/queries/*.sql` with `-- name: FuncName :one|:many|:exec` annotations; sqlc generates type-safe Go in `internal/db/`. Industry standard for new Go projects; SQL skills carry to any language.
- **`sqlc.yaml`** at repo root configures it. Minimal shape:
  ```yaml
  version: "2"
  sql:
    - engine: postgresql
      queries: sql/queries
      schema: sql/schema
      gen:
        go:
          package: db
          out: internal/db
          sql_package: database/sql
  ```
- **`github.com/lib/pq`** — the underlying Postgres driver. Stable, in maintenance mode (no new features), well-understood. Side-effect import (`_ "github.com/lib/pq"`) registers the `"postgres"` driver name with `database/sql`. This is the same stack used in the boot.dev backend curriculum, so you're consolidating learnings rather than picking up a new library.

**Why `database/sql` + `lib/pq` instead of `pgx`:** This is a learning project. The stdlib `database/sql` interface is what you already know, transfers to every Go SQL codebase you'll touch, and is fully supported by sqlc. `pgx` is technically nicer (richer types, actively developed, better perf) but the gains don't matter at this scale, and the unfamiliarity adds friction. Pick the stack you can be productive in.

**Why not other approaches:**
- Hand-writing queries without sqlc: more boilerplate; same SQL skill but slower for a multi-table project.
- ORMs (GORM, ent): you'd end up debugging ORM-specific issues instead of learning Postgres. Strongly avoid for this project.

**File organization in `sql/queries/`:** one file per domain area, e.g. `users.sql`, `sessions.sql`, `feeds.sql`, `items.sql`, `subscriptions.sql`. sqlc reads them all and generates one combined `internal/db/` package — file split is purely for your own readability.

**Implementation considerations:**
- **Connection pool:** Use `sql.Open("postgres", cfg.Database.URL)` to get a `*sql.DB`. `database/sql` manages a connection pool internally — defaults are fine for local dev. Tune later via `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime` only if you have a reason.
- **`sql.Open` is lazy — it does not actually connect.** Call `pool.Ping()` once at startup to fail fast if Postgres is unreachable. Without this, the first request would discover the connection problem instead of `main()`.
- **Mandatory side-effect import:** `_ "github.com/lib/pq"` at the top of `cmd/api/main.go`. Easy to forget; without it `sql.Open("postgres", ...)` fails at runtime with `unknown driver "postgres"`.
- **Interface for tests:** Your query functions should accept the interface sqlc generates (named `DBTX`), which both `*sql.DB` and `*sql.Tx` satisfy. This is what enables transaction-rollback test isolation (§10.5 in spec).
- **Context everywhere:** Every function takes `ctx context.Context` as first arg. Plumb it from the caller. No `context.Background()` deep in helper code.
- **Per-user scoping is a code-level invariant.** Every function that operates on user data takes `user_id` as a parameter and uses it in the WHERE clause. There is no "list all items" function — only "list items for user X". This makes per-user isolation a property of your API surface, not something you have to remember in each handler.
- **Don't expose `database/sql` types upward.** If your handlers manipulate `sql.NullString` / `sql.NullTime`, you've leaked the database choice into the application layer. sqlc generates structs that wrap these; convert to domain structs at the handler boundary if you want clean separation. (For a small project, leaking is survivable — just be aware.)
- **UUID generation:** Let Postgres generate UUIDs via `gen_random_uuid()` in your schema. No `github.com/google/uuid` dependency needed in Go unless you explicitly want client-side IDs (you don't, for this project).

**What to test (Tier 1):**
- Create user → look up by email → fields match.
- Insert item twice with same (feed_id, guid) → table has 1 row.
- Per-user isolation: insert subscriptions for two users, listing user A's subscriptions returns only user A's rows.

**Test setup pattern:**
- Have a `internal/db/dbtest` helper package that exposes `NewTestPool(t)` and uses a transaction-rollback pattern (each test gets its own tx, rolled back at end). The work to design this once is paid back across every later test.

---

## Milestone 4: Auth surface

**Goal:** All authentication flows from §6.3 working end-to-end via HTTP.

**Done when:**
- POST `/signup` creates an unverified user and sends a verification email.
- GET `/verify?token=...` marks the user verified.
- POST `/login` issues a session cookie.
- GET `/auth/me` (or similar) returns the current user when authenticated.
- POST `/logout` clears the session.
- POST `/forgot` sends a reset email (or pretends to, for unknown emails).
- GET `/reset?token=...` and POST `/reset` change the password.
- POST `/settings/password` (logged in) changes password and invalidates other sessions.
- Session middleware: handlers can read `user_id` from the request context.

**Spec reference:** §6.3 Authentication flows, §7 Security

**Library/tooling:**
- `golang.org/x/crypto/bcrypt` — password hashing.
- `crypto/rand` (stdlib) — token generation. `base64.URLEncoding.WithPadding(NoPadding)` to encode for URLs.
- `html/template` (stdlib) — both web pages and email bodies.
- `github.com/go-chi/chi/v5` — routing.
- SMTP: stdlib `net/smtp` is enough; alternatively a provider-specific SDK (e.g., `github.com/resendlabs/resend-go`).
- For dev: use a fake SMTP that captures emails (`github.com/mocktools/go-smtp-mock` works in-process), so you can test verify/reset flows without needing real email infra yet.

**Implementation considerations:**

- **Token generation:** 32 bytes from `crypto/rand`, base64-url encoded. Store the token directly in `email_tokens.token` and `sessions.token`. Don't hash tokens — they're not user-provided secrets, they're server-issued.
- **Cookie attributes:**
  - `HttpOnly: true` — prevents JS access (defense vs XSS)
  - `Secure: true` in production — HTTPS only
  - `Secure: false` in dev — HTTP on localhost
  - `SameSite: http.SameSiteLaxMode` — CSRF defense
  - `Path: "/"` — apply across the app
  - Don't set `Domain` — defaults to the current domain, which is correct
- **Session lookup is hot path.** Every request goes through it. Single PK lookup is fast (microseconds in Postgres), but keep it cheap: no joins, just `SELECT user_id, expires_at FROM sessions WHERE token = $1`.
- **Email enumeration:** Login and forgot-password must return identical responses regardless of whether the email exists. Don't even differentiate timing if you can avoid it (constant-time compare is overkill here, but never branch the response shape on "user exists").
- **bcrypt cost:** 12 is a good default for v1. The hashing should take ~250ms on a modern CPU — slow on purpose. If you find login feels sluggish, your cost is doing its job.
- **CSRF protection:** All state-changing handlers (POST, PUT, DELETE) need CSRF protection. Use `gorilla/csrf` middleware (small library, well-tested). Alternative: double-submit cookie pattern (write yourself, ~30 lines). For HTML forms, the middleware injects a hidden field; you embed `{{ .csrfField }}` in templates.
- **Validation:** Email must look like an email (basic regex or `net/mail.ParseAddress`). Password minimum length (8? 10?) — pick something reasonable. Don't enforce complex composition rules (NIST guidance is now "long > complex").

**Implementation order within this milestone:**
1. Password hashing helpers (`Hash(plaintext)`, `Compare(hash, plaintext)`).
2. Token generation helper.
3. Email helpers (one function per email type, takes the template data, sends via SMTP).
4. Signup handler + verify handler.
5. Login handler + session creation + logout.
6. Session middleware.
7. Forgot + reset handlers.
8. Change-password handler.

**What to test (Tier 1):**
- Password hashing roundtrip (hash, verify, verify-with-wrong-password-fails).
- Email enumeration resistance: `POST /login` with `nonexistent@example.com` and a fake password returns the same response as `POST /login` with an existing email and wrong password.
- Session middleware: request with valid cookie sets `user_id` in context; request with missing/expired cookie redirects to `/login`.
- Verify flow: create user, mark unverified, hit verify endpoint, user now verified.
- Reset flow: hit forgot, consume token, password is changed, old password doesn't work, new password does.
- Change password: logged in, change password, all other sessions for that user are invalidated (deleted), current session still works.

**This is the longest milestone. Don't rush it. Auth bugs are silent in tests and embarrassing in production.**

---

## Milestone 5: Sources management

**Goal:** Users can add, list, and remove RSS feed subscriptions.

**Done when:**
- POST `/sources` accepts a feed URL, validates it's a real feed (one-shot fetch + parse), upserts the `feeds` row, inserts `user_subscriptions`. Returns clear error on invalid URL.
- GET `/sources` lists the user's subscriptions with title, URL, last fetch status.
- DELETE `/sources/{feed_id}` (or POST with `_method=DELETE`) removes the subscription. The `feeds` row stays (other users may subscribe).
- HTML forms for add/remove work in the browser.

**Spec reference:** §6.4 Reading flow

**Library/tooling:**
- `github.com/mmcdole/gofeed` — RSS/Atom parser. Handles both formats uniformly.
- Reuse chi router, html/template from Milestone 4.

**Implementation considerations:**

- **Feed validation on add:** Don't trust the user's URL. Fetch it, parse it, fail if it's not a valid feed. Show a clear error: "That URL didn't return a valid RSS or Atom feed." This validation should also extract the feed's `title` and `site_url` to populate the `feeds` row.
- **Don't bulk-fetch items on add.** Just save the feed and let the next fetcher tick pick up items. Otherwise you'll fight rate limits and complicate the code.
- **Upsert pattern for feeds:**
  ```
  INSERT INTO feeds (feed_url, ...) VALUES (...)
  ON CONFLICT (feed_url) DO UPDATE SET title = EXCLUDED.title  -- refresh title
  RETURNING id
  ```
  This lets you get back the feed_id whether the row was new or existed.
- **HTTP timeout for validation fetch:** 10s. Don't let a slow remote server hang your handler.
- **User-Agent header:** Always set a descriptive User-Agent. `Lockedin/0.1 (https://yourdomain.com)` or similar. Be a good citizen.
- **DELETE via HTML forms:** Browsers can't natively send DELETE from a form. Either use a hidden `_method=DELETE` field + middleware, or use POST `/sources/{id}/delete`. Both are fine; pick one and stick with it.

**What to test:**
- Add a real feed URL → subscription exists.
- Add an invalid URL → clear error, no subscription.
- Delete a subscription → row gone; feeds row remains.
- User A can't delete user B's subscription (per-user scoping in the handler).

---

## Milestone 6: Fetcher core

**Goal:** The background fetcher pulls all feeds, dedupes items, and handles failures gracefully.

**Done when:**
- `cmd/fetcher` runs a loop on a 30-minute ticker (start with 1 minute in dev for testing).
- Each tick: fetches all feeds in parallel (capped at 10 concurrent), uses conditional GET headers, inserts new items, updates fetch status.
- Items dedupe correctly: re-running over the same feed inserts no duplicates.
- Per-feed failure isolation: introducing a broken feed (404, parse error, timeout) does not stop other feeds from fetching.
- `last_fetch_status` and `last_fetch_error` populate correctly per feed.

**Spec reference:** §6.1 Fetcher loop, §8 Error handling

**Library/tooling:**
- `github.com/mmcdole/gofeed` — RSS parsing.
- `golang.org/x/sync/errgroup` — parallel fetches with `SetLimit(10)`.
- stdlib `time.Ticker` for the loop.
- stdlib `context` for cancellation.
- stdlib `net/http` with explicit timeout.

**Implementation considerations:**

- **HTTP client setup:**
  - Explicit `http.Client{Timeout: 10 * time.Second}`. The default client has no timeout — production deathtrap.
  - Custom `Transport` if you want connection reuse and limits.
  - Set User-Agent on every request.
- **Conditional GETs:**
  - Send `If-None-Match: <etag>` and `If-Modified-Since: <last_modified>` if you have them stored.
  - On 304 response: just update `last_fetched_at`, skip parsing.
  - On 200: save the response's `ETag` and `Last-Modified` headers back to the `feeds` row for next time.
- **Parse before update:** Get the parsed items and the headers in hand before opening a transaction to update the DB. Keeps locks short.
- **Insert with ON CONFLICT:**
  ```
  INSERT INTO items (feed_id, guid, ...) VALUES ($1, $2, ...)
  ON CONFLICT (feed_id, guid) DO NOTHING
  RETURNING id
  ```
  The `RETURNING` lets you collect IDs of newly-inserted items if you need them.
- **errgroup pattern:**
  ```
  g, ctx := errgroup.WithContext(ctx)
  g.SetLimit(10)
  for _, feed := range feeds {
      feed := feed  // capture
      g.Go(func() error {
          return fetchOne(ctx, feed)  // returns error but we LOG it, don't propagate
      })
  }
  g.Wait()  // we don't care about the return — each feed handles its own errors
  ```
  Note: `errgroup` propagates the first error, but you don't *want* that — one bad feed shouldn't cancel others. Either log errors inside `fetchOne` and always return nil, or use a plain `sync.WaitGroup` with a `sem chan struct{}` for limiting.
- **Each feed's processing is its own function with its own error boundary.** Recover from panics if you're paranoid (RSS parsing on truly broken XML can sometimes panic), but per-feed errors should be `return error`, logged, and the loop continues.
- **No mid-tick retries.** A failed feed is retried on the next tick automatically. Don't add exponential backoff or circuit breakers.
- **Dev tip:** While iterating, set the ticker to 1 minute (or even 30 seconds) so you can see the loop in action. Make it config-driven: `FETCH_INTERVAL=30s` in dev, `FETCH_INTERVAL=30m` in prod.

**What to test (Tier 1):**
- Item dedupe: insert items twice via the fetcher's path, count is 1.
- Conditional GET: simulate a 304 response, verify no items inserted, `last_fetched_at` updated.
- Per-feed failure isolation: in a tick with 3 feeds where one returns 500, the other two complete and update status normally.
- Fetcher restart idempotency: cancel context mid-tick, restart, no duplicate items.
- RSS parsing edge cases: feed with missing `<guid>` (fall back to `<link>` if present, otherwise skip the item); feed with missing `<pubDate>` (use `fetched_at` as fallback); malformed XML (parse error caught, feed marked bad).

---

## Milestone 7: Web UI for reading

**Goal:** A minimal but functional web UI for landing, login, feed viewing, sources management, and settings.

**Done when:**
- GET `/` shows the landing page (with login form) when logged out, the personalized feed when logged in.
- Feed view shows items in reverse-chronological order, grouped by feed name or shown as a flat list (your call — the spec doesn't dictate).
- Each item: feed name, title (linked to original URL, `target="_blank" rel="noopener"`), published date, optional excerpt from `content`.
- `/sources` page works as a UI, not just as JSON endpoints.
- `/settings` page lets the user change their password.
- Logout is a button somewhere visible.
- CSS exists but is minimal — this is not a frontend project.

**Spec reference:** §6.4 Reading flow

**Library/tooling:**
- `html/template` (stdlib).
- **CSS:** `pico.css` (a classless CSS framework — include one CSS file, your HTML looks decent automatically). Don't pull in Tailwind, Bootstrap, or any JS framework for v1.
- `bluemonday` (`github.com/microcosm-cc/bluemonday`) — sanitize HTML from feeds before rendering, preventing XSS from a malicious feed's content.

**Implementation considerations:**

- **Template layout:**
  ```
  web/templates/
  ├── layout.html        (defines: title, content blocks)
  ├── landing.html       (login form + marketing)
  ├── feed.html          (logged-in home — the personalized feed)
  ├── sources.html       (manage subscriptions)
  ├── settings.html      (change password)
  └── auth/
      ├── signup.html
      ├── login.html
      ├── forgot.html
      └── reset.html
  ```
  Parse all templates at startup with `template.Must(template.ParseFS(...))`. Use embed.FS (`//go:embed web/templates`) so the binary is fully self-contained — no separate template files to ship.
- **Template data:** Define a struct per page (`type FeedPageData struct { ... }`) rather than passing `map[string]any`. Type-safe, refactorable.
- **Layout pattern:**
  ```html
  {{ define "layout" }}
  <html>...<body>{{ template "content" . }}</body></html>
  {{ end }}

  {{/* in feed.html: */}}
  {{ define "content" }}<ul>...</ul>{{ end }}
  ```
  Then render `tmpl.ExecuteTemplate(w, "layout", data)`.
- **CSRF on every form:** Embed `{{ .CSRFField }}` (gorilla/csrf provides it as a template func) inside every form.
- **HTML escaping:** `html/template` auto-escapes. To render trusted HTML (feed content after bluemonday sanitization), use `template.HTML(sanitizedString)` — this opts out of escaping for that specific value. Be very deliberate; this is the most common XSS source.
- **Content sanitization:** Run all `items.content` through `bluemonday.UGCPolicy()` before rendering. This strips `<script>` and other dangerous tags but keeps formatting.
- **Pagination:** Skip for v1. Show last 100 items. Add pagination if it becomes annoying.
- **Empty states:** Handle "no subscriptions yet" (link to `/sources`) and "no items yet" (encouraging copy) gracefully. These are easy to forget and look bad when first encountered.

**What to test (Tier 2 — nice-to-have):**
- Handler status codes (200 / 302 / 401 / 404).
- Logged-out `/` does NOT contain any private data.
- Per-user data isolation: when user A's session is in context, the rendered HTML never includes user B's subscriptions.

---

## Milestone 8: Notification path

**Goal:** After each fetcher tick, every user with new items gets exactly one digest email.

**Done when:**
- After a fetcher tick completes, the notifier runs.
- For each user with un-notified items, exactly one email is rendered and sent.
- After successful send, `item_notifications` rows are inserted for every emailed item.
- If SMTP fails, no notification rows are inserted (next tick retries).
- "No new items" produces no email.

**Spec reference:** §6.2 Notification path

**Library/tooling:**
- Same SMTP path as auth emails (reuse Milestone 4's email helpers).
- `html/template` for the email body.
- For plain-text alongside HTML, render two templates and use MIME multipart (`mime/multipart` stdlib).

**Implementation considerations:**

- **Query "what's un-notified for this user":** Use the exact query from §6.2 (LEFT JOIN + IS NULL). Index `item_notifications` if performance suffers.
- **Per-user iteration:** Naive loop is fine at v1 scale. If you grow to thousands of users, parallelize with the same errgroup pattern as feeds. Don't optimize prematurely.
- **Email subject:** `"X new items from your feeds"` or `"New: <first item title>"`. Whatever feels right — it's plain text in the inbox.
- **Email body:** Group items by feed name, list each with title + link + published date. Keep it skim-friendly.
- **Plain text version:** Most modern clients prefer HTML, but providing a plain-text alternative is good email hygiene and helps deliverability. Render the same data into a text template.
- **Critical order:**
  1. Render email.
  2. Send via SMTP.
  3. **Only if send succeeded:** insert `item_notifications` rows.
  If you insert before send, a failed send means items are marked notified but never emailed — they'll never be retried. Bug.
- **Idempotency under failure:** If you crash between step 2 and step 3, the user gets a duplicate email next tick. Annoying, not catastrophic. An outbox pattern fixes this; not worth it for v1.
- **Dev SMTP:** Continue using a local fake SMTP. Switch to real SMTP only when you're about to deploy.

**What to test (Tier 1):**
- Notification dedupe: run the fetcher → notifier twice with same items, user gets exactly one email (assertable by checking `item_notifications` count or by capturing send calls in a fake SMTP).
- No-new-items: run notifier with no new items, no email sent.
- SMTP failure: fake SMTP returns error, no `item_notifications` rows inserted.
- Per-user batching: with 3 users sharing 2 of 4 new items, each user gets one email containing their relevant items.

---

## Milestone 9: Test pass

**Goal:** All Tier 1 tests from §10 of the spec exist, pass reliably, and run in CI-ready isolation.

**Done when:**
- `go test ./...` passes.
- `go test -race ./...` passes (race detector finds nothing).
- Tier 1 test list (§10.1 in the spec) is fully covered.
- Integration tests use a real Postgres, isolated via transaction rollback.
- A single `make test` command runs everything end-to-end.

**Spec reference:** §10 Testing strategy

**Library/tooling:**
- `testing` stdlib + `github.com/stretchr/testify/require` for assertions. `require` halts on first failure per test (cleaner than `assert` for integration tests).
- `net/http/httptest` for handler tests.
- For integration tests' Postgres:
  - **Option A:** `testcontainers-go` — spins up a throwaway Postgres in Docker per test run. Clean, slower startup (~2-5s once).
  - **Option B:** A dedicated `lockedin_test` DB on your local Postgres, reset between runs. Faster, requires manual setup.
  Pick **A** if you want zero local setup; **B** if startup time matters.

**Implementation considerations:**
- **Transaction-rollback pattern:**
  ```go
  func setupTest(t *testing.T) (db.Querier, func()) {
      tx, _ := pool.Begin(context.Background())
      return db.New(tx), func() { tx.Rollback(context.Background()) }
  }
  ```
  Each test: `q, cleanup := setupTest(t); defer cleanup()`. Test changes never persist.
- **Test data fixtures:** Have a `dbtest.SeedUser(t, q)` helper that creates a test user and returns the ID. Avoid copy-pasting setup boilerplate across tests.
- **Run race detector:** `go test -race` catches concurrency bugs in the fetcher. Make it part of `make test`.
- **Avoid flaky tests like the plague.** If a test relies on timing (`time.Sleep`), redesign it. Use `context.WithTimeout` for waiting, not sleeps.

**What to test (the full Tier 1 list):**
- Item dedupe.
- Notification dedupe.
- Fetcher restart idempotency.
- Per-user data isolation.
- Password hashing roundtrip.
- Email enumeration resistance.
- Session middleware.
- RSS parsing edge cases.

Plus an end-to-end smoke test: seed two users, set up subscriptions, run fetcher → notifier with a fake feed source, verify emails went out correctly.

---

## Milestone 10: Containerize + deploy

**Goal:** The app runs on a real, internet-reachable server via HTTPS, and you've used it from a phone.

**Done when:**
- A `Dockerfile.api` and `Dockerfile.fetcher` exist with multi-stage builds → tiny final images.
- `docker-compose.yml` runs `api`, `fetcher`, `postgres` together.
- A `Caddyfile` (or Traefik config) terminates HTTPS via Let's Encrypt.
- Environment variables are loaded from a `.env` file (gitignored).
- The app is reachable on the public internet via HTTPS.
- You've signed up, verified your email, added a real RSS feed, and received the digest email — all on the deployed instance, from your phone.

**Spec reference:** §11 Deployment

**Library/tooling:**
- Docker, Docker Compose.
- Caddy (recommend — auto-HTTPS in 10 lines of config).
- Your chosen host (Oracle Cloud Free Tier, Hetzner, etc. — Milestone 10 is when you finalize this).
- Your chosen email provider (Resend recommended for the free tier).
- Your chosen domain (optional but recommended; sslip.io workaround if you skip).

**Implementation considerations:**

- **Multi-stage Dockerfile pattern:**
  ```
  Stage 1: golang:1.22-alpine — install deps, build binary.
  Stage 2: gcr.io/distroless/static (or alpine:latest) — copy ONLY the binary.
  ```
  Result: 10-20MB image instead of 800MB. Faster pulls, smaller attack surface.
- **Compile statically:** `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build` (or amd64 if your host is x86). Required for distroless.
- **Image architecture:** Match your host. If you picked Oracle/Hetzner ARM, build for `arm64`. Build on your dev machine: `docker buildx build --platform linux/arm64 ...`.
- **Compose service layout:**
  ```yaml
  services:
    postgres:
      image: postgres:16-alpine
      volumes: [pgdata:/var/lib/postgresql/data]
      env_file: .env
    api:
      build: { context: ., dockerfile: Dockerfile.api }
      depends_on: [postgres]
      env_file: .env
    fetcher:
      build: { context: ., dockerfile: Dockerfile.fetcher }
      depends_on: [postgres]
      env_file: .env
    caddy:
      image: caddy:2-alpine
      ports: ["80:80", "443:443"]
      volumes: [./Caddyfile:/etc/caddy/Caddyfile, caddy_data:/data]
      depends_on: [api]
  volumes:
    pgdata:
    caddy_data:
  ```
- **Caddyfile (minimal):**
  ```
  yourdomain.com {
      reverse_proxy api:8080
  }
  ```
  Caddy automatically handles HTTPS via Let's Encrypt. That's the whole config.
- **DNS:** Point your domain's A record at the host's public IP. Cloudflare DNS is free and good. Wait ~5 min for propagation before testing HTTPS.
- **Secret management:** `.env` file on the server (not committed). Keep a `.env.example` in the repo with empty values, documenting what's needed.
- **Database backup (minimum):** A daily cron on the host: `pg_dump -U postgres lockedin | gzip > /backups/lockedin-$(date +%Y%m%d).sql.gz`. Keep 7 days. Not great, but better than nothing.
- **Logging:** `docker compose logs -f api fetcher` is fine for v1. Don't set up centralized logging.
- **Deploy workflow:**
  1. SSH into host.
  2. `git pull` in the project directory.
  3. `docker compose up -d --build`.
  4. Watch logs to confirm.
  This is fine. It's a personal project.

**What to test:**
- Smoke test from your phone: sign up, verify, add a real feed (e.g., `https://news.ycombinator.com/rss`), wait for the next tick, receive email, click through.
- Check `feeds.last_fetch_status` for the first few real feeds. Investigate any that aren't `ok`.

**This is the milestone where the spec stops being a spec and becomes a thing you actually use. Celebrate it.**

---

## Cross-cutting reference

### Resources to read

In order, as needed:

1. **Mat Ryer — "How I write HTTP services after 13 years"** (search the title). Read before Milestone 4. Shapes how you structure the api binary.
2. **`golang-standards/project-layout` README** — read the disclaimer at the top. Use the rest as a menu, not a recipe.
3. **`sqlc` quickstart** — read before Milestone 3.
4. **`chi` README** — 15 minutes, skim before Milestone 4.
5. **A Philosophy of Software Design (Ousterhout)** — book, ~190 pages. Read *during* the project when you hit a "this feels wrong but I can't articulate why" moment. The vocabulary will click.

### Local dev workflow

A working setup looks like:

- One terminal: `docker compose -f docker-compose.dev.yml up postgres` (just Postgres in dev).
- Another terminal: `go run ./cmd/api`.
- Another: `go run ./cmd/fetcher` (with `FETCH_INTERVAL=1m` for fast iteration).
- A `Makefile` with: `make migrate-up`, `make migrate-down`, `make test`, `make run-api`, `make run-fetcher`.

### When you're stuck

Open the conversation with me with:
- **What you tried** (code or commands).
- **What you expected** to happen.
- **What actually happened** (error message, behavior).
- **Where you got to.**

I'll review what you wrote, explain what's broken, suggest the next step. I'll generally **not write the code for you**. If you ask "what library should I use for X?", I'll answer that. If you ask "can you write the auth middleware?", I'll push back.

Exception: if you've been stuck on the same thing for >30 minutes and explanations aren't unsticking you, ask for a worked example. Learning has diminishing returns past a certain frustration point.

### Things to track but not act on in v1

These came up during design discussions but aren't milestone work. Keep them in mind for v2:

- Rate-limiting login attempts (flagged in spec §7).
- Post-password-change security notification email (spec §14).
- OPML import/export (spec §14).
- Pagination on the feed view.
- Per-feed fetch intervals.
- AWS deployment migration.
- LLM summarization and relevance scoring.

### Done = done

After Milestone 10:
- The app is deployed.
- You're using it.
- The Tier 1 tests pass.
- You have a working personal aggregator that you built.

That's v1. Don't add anything else under the v1 banner — it just becomes "the next thing." Take a break. Use the app for a few weeks. Then decide what v2 should be based on real friction, not on the v2 list from the design phase.
