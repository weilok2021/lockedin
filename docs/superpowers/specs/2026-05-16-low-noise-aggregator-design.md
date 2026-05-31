# Lockedin — Low-noise content aggregator v1 design

**Status:** Approved design, ready for implementation planning
**Date:** 2026-05-16
**Tracked:** This document lives in the repo and is kept in sync with the implementation.

> **⚠ Revised by [`2026-05-31-curated-multimodal-content-model-design.md`](2026-05-31-curated-multimodal-content-model-design.md).** The content model changed: topic→Google-News search was found unable to deliver readable content, so v1 moved to a **curated, multi-modal catalog** (article + youtube + podcast) that users follow source-by-source, and the "anti-engagement" thesis was corrected (the villain is junk, not engagement). The newer doc supersedes **§1 (thesis), §3 (non-goals), §5.2 (content tables), §6.4 (reading flow), and Milestone 7**. The rest of this spec — auth, multi-user model, architecture, email, deployment — still stands.

---

## 1. Background

Modern content platforms (algorithmic social feeds, YouTube home, X timelines) are engineered for engagement. They reward addictive use rather than focused consumption. The user wants to stay current on trusted sources — engineering blogs (e.g., Boris Cherny on Claude Code), trusted YouTubers, specific Substacks — without subjecting themselves to the platforms' attention-hijacking surfaces.

This project is a **personally-curated content aggregator**: the user adds specific sources they trust, the system pulls new items on a schedule, and delivers them through a low-friction surface (web page + email notification) that surfaces *only* what the user explicitly asked for.

**What "low-noise" constrains — and what it does not.** The enemy is *manipulation and uncurated volume*: algorithmic ranking, clickbait, infinite scroll, engagement-maximizing notifications, content the user never asked for. It is **not** content depth. For a topic the user deliberately chose, delivering the full, readable article *is* signal — the product should provide content as completely as the source allows, not withhold it. The anti-addictive stance constrains the feed's *mechanics* (chronological only, no ranking, no endless scroll, at most one email per cycle); it never limits how much of a good article the user can read. Reducing content richness in the name of being "anti-addictive" is a category error.

The project also serves as an applied capstone for the user's Boot.dev backend curriculum. The technology choices favor exercise of those skills (Go, Postgres, Docker, AWS) over the absolute minimum implementation cost.

## 2. Goals (v1)

1. **Multi-user web application** with password-based authentication and email verification.
2. **Topic-based subscription management** — users add, view, and remove *topics* (e.g. "Claude Code"); the system turns each topic into an RSS/Atom feed behind the scenes. Users never handle feed URLs directly.
3. **Periodic content fetcher** — a background process pulls new items from all subscribed feeds on a fixed schedule.
4. **Personalized web UI** — server-rendered HTML showing the user's items in reverse chronological order.
5. **Email notification on new items** — at most one email per user per fetch cycle, only if new items were found.
6. **Public landing page** for unauthenticated visitors.

## 3. Non-goals (v1)

Explicitly out of scope for v1, with notes on where each could plug in later:

- **LLM summarization** — additive change (new column on `items`); add in v2.
- **Relevance scoring** — additive change (new table); add in v2.
- **Scraping JS-rendered pages, Twitter/X, LinkedIn** — fragile and platform-hostile; deferred.
- **Read-state tracking ("mark as read")** — additive change (new table); add only if a real need emerges.
- **Native mobile apps** — out of scope.
- **Public content discovery surface** — conflicts with the project's anti-recommendation thesis. Note: *explicit topic subscription* (the user types a topic, the system maps it to a feed — see §6.4) is **in scope** and distinct from this. The user chooses; there is no algorithmic ranking, suggestion, or engagement surface. Turning a topic into a feed is plumbing, not recommendation.
- **AWS-specific services (RDS, ECS, SES, S3)** — deferred to v2 polish; v1 uses a single VPS with Docker Compose (see §11).
- **JWT-based auth** — server-side sessions are the correct tool for a single-backend web app.

## 4. Architecture

Two Go binaries, one Postgres database, SMTP via a transactional email provider.

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
│   │    render  │                       │  - feeds           │   │
│   └────────────┘                       │  - items           │   │
│         ▲                              │  - user_subs       │   │
│         │                              │  - item_notifs     │   │
│         │ browser                      └────────────────────┘   │
│         │                                       ▲               │
│   ┌─────┴──────┐                                │ reads/writes  │
│   │   User     │                                │               │
│   │ (browser)  │                       ┌────────┴───────────┐   │
│   └────────────┘                       │     fetcher        │   │
│                                        │      (Go)          │   │
│                                        │                    │   │
│                                        │  - cron loop       │   │
│                                        │  - RSS pull        │   │
│                                        │  - dedupe          │   │
│                                        │  - email sender    │   │
│                                        └────────┬───────────┘   │
│                                                 │               │
│                                                 ▼               │
│                                          ┌──────────────┐       │
│                                          │  SMTP        │       │
│                                          │  (Resend /   │       │
│                                          │   Postmark / │       │
│                                          │   Mailgun)   │       │
│                                          └──────────────┘       │
└─────────────────────────────────────────────────────────────────┘
```

### 4.1 Pattern: "app + worker, shared database"

This is the most common backend shape in industry, sitting between a strict monolith and microservices. It is **not** a monolith (two independently-deployable binaries) and **not** microservices (they share a database). The pattern teaches separation between web-tier and worker-tier responsibilities — a transferable skill across most production backends.

### 4.2 Component responsibilities

| Component | Responsibilities |
|---|---|
| `api` | Serve HTTP, handle authentication (signup, login, verify, reset, change, logout), render server-side HTML, manage subscriptions |
| `fetcher` | Periodic cron loop: pull RSS feeds, dedupe items, send notification emails |
| Postgres | Persistent store, source of truth, lightweight queue (notification state) |
| SMTP provider | Deliver verification emails, password-reset emails, notification emails |

Both Go binaries live in the same repository and share a `pkg/` of common code (database models, RSS parsing helpers, email template rendering).

### 4.3 v2 expansion path

When v2 features are added (LLM summarization, relevance scoring), a third process (Python summarizer service) reads from Postgres, writes back enriched data, and does not require schema migration of existing rows. The current data model is designed to accommodate this without rework.

## 5. Data model

Seven Postgres tables. SQL types below are illustrative; exact types resolved during implementation.

### 5.1 Authentication tables

```sql
users (
  id                  uuid PRIMARY KEY,
  email               text UNIQUE NOT NULL,
  hashed_password       text NOT NULL,           -- bcrypt or argon2id
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

- `email_tokens` is a single table reused for both email verification (signup) and password reset. The `purpose` column disambiguates.
- `sessions` are server-side; no JWT. Cookie carries only the random session token.
- Only `users` carries `updated_at` because it is the only table whose row state mutates over time without a more specific timestamp serving the same purpose.

### 5.2 Content tables (globally shared)

```sql
feeds (
  id                  uuid PRIMARY KEY,
  feed_url            text UNIQUE NOT NULL,
  title               text,                    -- from feed <title>
  site_url            text,                    -- from feed <link>
  etag                text,                    -- HTTP If-None-Match
  last_modified       text,                    -- HTTP If-Modified-Since
  last_fetched_at     timestamptz,
  last_fetch_status   text,                    -- 'ok' | 'http_error' | 'parse_error' | 'timeout'
  last_fetch_error    text,
  created_at          timestamptz NOT NULL DEFAULT now()
)

items (
  id            uuid PRIMARY KEY,
  feed_id       uuid REFERENCES feeds(id) NOT NULL,
  guid          text NOT NULL,                 -- publisher's stable identifier
  url           text NOT NULL,
  title         text NOT NULL,
  content       text,                          -- HTML body / summary from feed
  author        text,
  published_at  timestamptz,
  fetched_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (feed_id, guid)
)
```

**Key design choice:** feeds and items are **globally shared**, not per-user. If two users subscribe to the same blog, the fetcher pulls once and inserts items once. This is how real RSS aggregators (Feedly, Inoreader) work. Per-user duplication would be simpler to code but wrong at any non-toy scale.

**`feeds.last_fetched_at` serves as the table's "updated_at" semantically** — it captures the only mutation that matters.

### 5.3 Per-user tables

```sql
user_subscriptions (
  user_id          uuid REFERENCES users(id) NOT NULL,
  feed_id          uuid REFERENCES feeds(id) NOT NULL,
  custom_title     text,                       -- optional override
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

- `user_subscriptions` is the join that makes shared content + per-user views work.
- `item_notifications` is both an audit log and the notification queue's state. No row = needs to be emailed; row exists = already emailed. No separate queue table needed.

### 5.4 Explicitly absent from v1

- **No `read_state`** — tracking unread counts invites engagement-feed thinking. The email tells the user what is new; the web page shows everything chronologically.
- **No notification queue table** — `item_notifications` IS the queue's state.
- **No `summaries` or `scores`** — additive v2 changes that do not require migration.

## 6. Data flow

### 6.1 Fetcher loop (runs every 30 minutes)

```
Fetcher tick:
  1. SELECT * FROM feeds
  2. For each feed, in parallel (capped at N=10 concurrent):
       a. HTTP GET feed_url with conditional headers:
            If-None-Match: <feeds.etag>
            If-Modified-Since: <feeds.last_modified>
       b. If 304 Not Modified:
            UPDATE feeds SET last_fetched_at = now(),
                             last_fetch_status = 'ok'
       c. If 200 OK:
            Parse RSS/Atom with gofeed
            INSERT INTO items (...) ON CONFLICT (feed_id, guid) DO NOTHING
            UPDATE feeds SET etag, last_modified, last_fetched_at,
                             last_fetch_status = 'ok'
       d. If error (timeout, 4xx/5xx, DNS, parse failure):
            UPDATE feeds SET last_fetch_status = <category>,
                             last_fetch_error = <message>,
                             last_fetched_at = now()
            -- Do NOT abort the loop; the next feed proceeds.
  3. After all feeds processed, run notification step (6.2).
```

**Design properties:**

- **Conditional GETs** minimize bandwidth and respect publishers. Most RSS servers return `304` cheaply when nothing changed.
- **DB-layer dedupe** via `UNIQUE(feed_id, guid)` + `ON CONFLICT DO NOTHING` prevents duplicate items atomically, no race condition.
- **Parallel fetches** with a semaphore (Go `errgroup.SetLimit(10)`) — efficient but not abusive.
- **Per-feed failure isolation** — one broken feed never aborts the tick.
- **No mid-tick retries** — a flaky feed is retried automatically 30 minutes later.

### 6.2 Notification path

After every fetch tick, for each user:

```sql
SELECT i.*
FROM items i
JOIN user_subscriptions us ON us.feed_id = i.feed_id
LEFT JOIN item_notifications n
       ON n.user_id = us.user_id AND n.item_id = i.id
WHERE us.user_id = $1 AND n.notified_at IS NULL
ORDER BY i.published_at DESC;
```

If the result is non-empty:

1. Render a "what's new" HTML email (list of item titles + links + feed names).
2. Send via SMTP.
3. **On successful send only:** insert one row into `item_notifications` per emailed item.

If SMTP fails: notification rows are not inserted. The next fetch tick retries.

**Guarantee:** each user receives at most one email per fetch tick, only when new content exists. No item is ever notified twice (the `item_notifications` row is the receipt). The low-noise thesis is enforced at the data layer.

### 6.3 Authentication flows

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

**Forgot password (request):**
```
POST /forgot { email }
  → If user exists: INSERT email_tokens (token, user_id, purpose='password_reset', expires_at = now + 15min)
                    Send reset email
  → Always return identical response: "If that email exists, we sent a link"
```

**Forgot password (consume):**
```
GET  /reset?token=<t>   → render form for new password (after validating token)
POST /reset { token, new_password }
  → Validate token (matches above rules)
  → UPDATE users SET hashed_password = bcrypt(new_password), updated_at = now()
  → UPDATE email_tokens SET used_at = now()
  → DELETE FROM sessions WHERE user_id = <user>  -- invalidate all sessions
  → 302 → /login
```

**Change password (logged in):**
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

### 6.4 Reading flow

```
GET /
  → If logged in:  render personalized feed
                   (items JOIN user_subscriptions WHERE user_id = me, ordered by published_at DESC)
  → If logged out: render public landing page with login form
```

```
GET    /subscriptions                  → list user's subscriptions (show custom_title = topic label + last_fetch_status)
POST   /subscriptions { topic }        → map topic → feed_url (provider, v1 = Google News search RSS),
                                   validate (fetch + parse), upsert feeds row,
                                   INSERT user_subscriptions (custom_title = topic)
DELETE /subscriptions/{feed_id}        → DELETE FROM user_subscriptions WHERE user_id=me AND feed_id=$1
                                   (the feeds row stays — other users may subscribe)
```

The user submits a **topic**, never a URL. The backend turns the topic into a feed URL via a pluggable
"provider" (v1: Google News search RSS, `news.google.com/rss/search?q=<topic>`). One topic maps to one
feed; the topic label lives in `user_subscriptions.custom_title`. This requires no schema change and leaves
the fetcher (§6.1) untouched — a topic feed is just a feed. Multiple providers per topic is a v2 addition
(modeled as multiple subscriptions sharing one `custom_title`, no migration).

## 7. Security

These items are required, not optional:

1. **Password hashing** with bcrypt (cost ≥10) or argon2id (default params). Never sha256/md5 or any unsalted/fast hash.
2. **Email enumeration resistance** — login responses do not distinguish "no such email" from "wrong password." Forgot-password always returns the same response regardless of whether the email is registered.
3. **HTTP-only, Secure, SameSite=Lax cookies** for the session token. Immune to XSS-based theft.
4. **Server-side sessions, no JWT.** Logout is instantaneous (delete row).
5. **Session invalidation on password change.** Both forgot-password completion and logged-in password change delete other sessions for that user.
6. **Token entropy:** all random tokens (sessions, email_tokens) are at least 32 bytes from `crypto/rand`.
7. **Authorization checks on every per-user route.** Every query against `user_subscriptions` / `item_notifications` is scoped by `user_id` from the session. Authorization is not optional in any handler.

Optional for v1 (flag as a known gap if skipped):
- Rate-limiting login attempts (simple `(email, attempt_at)` table; lock for 15 min after 5 failures in 15 min). Without this, login is brute-forceable.

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

Each feed is processed in its own error boundary. One bad feed never aborts the tick. Status is surfaced in the `/subscriptions` UI so the user can see which subscriptions are broken.

### 8.2 Idempotency

The fetcher's loop is fully idempotent: killing and restarting mid-tick loses nothing and duplicates nothing. Two guarantees:

1. **Items dedupe at the DB layer** (`UNIQUE(feed_id, guid)` + `ON CONFLICT DO NOTHING`).
2. **Notifications dedupe via `item_notifications`.** Worst case (death between SMTP send and receipt insert) results in a single duplicate email — annoying, not catastrophic. An outbox pattern can be added later if it becomes a real problem.

### 8.3 SMTP failures

| Email type | Failure handling |
|---|---|
| Verification, password reset | Surface failure to the user immediately (500 with clear message). They retry by submitting the form again. |
| Notification | Do not insert `item_notifications` rows; next tick retries. Log the failure. |

### 8.4 Database failures

- Use `pgxpool` (handles transient drops automatically).
- DB fully unavailable → `api` returns 503; `fetcher` logs and sleeps until next tick.
- No exotic retry logic for v1.

### 8.5 Explicitly out of scope for v1

- Circuit breakers
- Dead-letter queues
- Exponential backoff (next tick already retries every 30 min)
- Graceful shutdown finesse (idempotency makes `Ctrl+C` safe)

## 9. Operational hygiene

Minimal but real:

- **Structured logging** with `log/slog`. Every log line carries relevant context (feed_id, user_id, request_id).
- **`/health` endpoint** on `api`: returns 200 if DB is reachable.
- **Daily fetcher summary log line:** "tick complete: N feeds checked, M new items, K errors."
- **No metrics/tracing/dashboards in v1.** The above is sufficient at this scale.

## 10. Testing strategy

Aim for **"tests on the parts where bugs would be silent, expensive, or embarrassing"** — not 100% coverage.

### 10.1 Tier 1 (must-have)

| Test | Type | Why |
|---|---|---|
| Item dedupe | Integration (real Postgres) | Verifies `ON CONFLICT DO NOTHING` |
| Notification dedupe | Integration | Same user, same item → exactly one email |
| Fetcher restart idempotency | Integration | Kill mid-tick → no duplicates, no losses |
| Per-user data isolation | Integration | User A cannot read user B's subscriptions (security) |
| Password hashing roundtrip | Unit | Catastrophic regression guard |
| Email enumeration resistance | Integration | Identical responses for valid/invalid emails on login + forgot |
| Session middleware | Integration | Valid cookie → user_id; missing/expired cookie → 302 |
| RSS parsing edge cases | Unit | Missing guid (fall back to link), missing pubDate, malformed XML |

### 10.2 Tier 2 (nice-to-have)

- HTTP handler responses (status codes, redirects) via `httptest`
- Email template rendering (golden-file test)
- Conditional GET behavior

### 10.3 Tier 3 (skip)

- Trivial CRUD wrappers
- Getters/setters
- Pure glue code

### 10.4 Tooling

- `testing` stdlib + `stretchr/testify/require` for assertions
- Real Postgres for integration tests (either `testcontainers-go` or a dedicated local `lockedin_test` DB)
- `net/http/httptest` for HTTP handlers
- Table-driven tests for multi-case scenarios

### 10.5 Test isolation pattern

Wrap each integration test in a transaction with `defer tx.Rollback()`. Test code uses the transaction instead of the database directly; rollback wipes all test data automatically. This requires `pkg/db` interfaces to accept `pgx.Tx` (designed in from the start).

### 10.6 Out of scope for v1

- End-to-end browser tests (Playwright/Selenium)
- Load tests
- Mutation testing, property-based testing, fuzzing

## 11. Deployment

v1 ships as a deployed, always-on service — not a local-only development build. Deployment splits into two layers: **architectural decisions** (locked in below; affect code structure) and **operational decisions** (deferred research; do not affect code).

### 11.1 Fixed architectural decisions

- **Containerization:** all v1 code ships as Docker images. Two Dockerfiles, one per Go binary, using a multi-stage build that produces a minimal final image (Alpine or `gcr.io/distroless/static`). Go binaries are statically compiled (`CGO_ENABLED=0`).
- **Orchestration:** Docker Compose on a single host. Three services: `api`, `fetcher`, `postgres`. A named volume backs Postgres data so container restarts do not lose data.
- **Configuration:** all environment-specific values via env vars — `DATABASE_URL`, `SESSION_SECRET`, `SMTP_API_KEY`, `BASE_URL`, etc. Loaded from a `.env` file referenced by `docker-compose.yml`. No secrets in code or in the repo.
- **Reverse proxy and TLS:** Caddy in front of `api`, with automatic Let's Encrypt certificate issuance and renewal. ~10 lines of Caddyfile config.
- **Deployment workflow:** SSH into host, `git pull`, `docker compose up -d --build`. No CI/CD pipeline, no zero-downtime deploys, no orchestration platform. Manual is fine at this scale.

These decisions are **independent of which cloud or VPS provider** is chosen — they describe how the app is *packaged and run*, not where.

### 11.2 Deferred research items (decide before launch)

These do not affect code structure, only operational setup. The user will research and finalize before the launch step:

**Hosting platform** — candidates ranked by cost:
- **Oracle Cloud Free Tier** — free forever (4 ARM cores, 24GB RAM split across 2 VMs, 200GB storage). Best free option. Caveat: account approval is unpredictable; some users get denied.
- **Hetzner CAX11** (ARM, €3.29/month) or **CX22** (x86, €4.51/month) — best paid value, reliable.
- **DigitalOcean / Linode / Vultr** — ~$5-6/month, well-known alternatives if Hetzner doesn't fit.
- **AWS / GCP / Azure** — possible but expensive after free-tier expires; defer to v2 if pursued at all.

**Email service** — candidates:
- **Resend** — 3,000 emails/month free, modern API, clean DX (recommended starting point).
- **AWS SES** — very cheap ($0.10 per 1k emails) but requires domain verification + production access approval.
- **Postmark, SendGrid, Mailgun** — workable alternatives.

**Domain name** — optional:
- $10-15/year for `.xyz` / `.app` / `.dev` TLDs via Cloudflare Registrar, Namecheap, or Porkbun.
- Free workarounds for testing: `sslip.io` (IP-based subdomain with Let's Encrypt support), `duckdns.org` (free pickable subdomain).

**Note:** Switching any of these later requires only configuration changes, never code changes. Go binaries cross-compile to ARM trivially (`GOOS=linux GOARCH=arm64`), Docker abstracts the host OS, and the SMTP interface is identical across providers.

## 12. v2 expansion path

The following are intentionally out of v1 but the data model is designed so each is additive:

| v2 feature | Required change |
|---|---|
| LLM summary per item | Add nullable `summary_text`, `summary_generated_at` columns to `items`. Python summarizer service reads `WHERE summary_text IS NULL`. |
| Relevance scoring | Add new `relevance_scores (user_id, item_id, score, generated_at)` table. UI does `LEFT JOIN` and orders by score when present. |
| Read state | Add `read_state (user_id, item_id, read_at)` table. |
| AWS-specific services (RDS, ECS, SES, S3) | Migrate from single-VPS Docker Compose to managed services. Pure operational change; no code change. |
| Per-feed schedule | Add `fetch_interval_minutes` column to `feeds`; fetcher selects by "due for refresh" predicate. |

None of these require migration of existing v1 data.

## 13. Implementation budget

Realistic estimate: **4 to 6 weekends**. Breakdown:

- ~1 weekend: multi-user auth surface (signup, verification, login, password reset, change password, session management).
- ~2-3 weekends: core feature work (schema, fetcher loop, RSS parsing, item dedupe, notification path, web UI rendering, subscriptions management).
- ~1 weekend: testing (Tier 1 coverage).
- ~1 weekend: containerization + first deployment (Dockerfiles, docker-compose, Caddy, host setup, domain/DNS).

## 14. Out of scope but worth tracking

These are not v1 work, but worth recording so they aren't lost:

- Post-password-change security notification email
- Audit log of authentication events (logins, password changes)
- Per-feed customization (display order, custom names — partially supported via `user_subscriptions.custom_title`)
- OPML import/export (industry-standard RSS subscription format)
- Two-factor authentication
- Account deletion / data export (GDPR-style, if ever shared beyond personal use)
- Rate-limiting login attempts (flagged in §7 as optional for v1; decision to be made during implementation planning)
