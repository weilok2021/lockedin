# LockedIn

**Your corner of the internet, curated.**

A low-noise content aggregator: it pulls from a small set of trusted sources you
actually chose, and shows them as a calm, finite feed of preview cards that link
out to the original site. No algorithm, no infinite scroll, no junk.

> ⚠️ **Work in progress.** This is an active learning project and isn't finished
> or deployed yet. The core flow works locally (auth, catalog, reading feed,
> discover); a few pieces are still on the roadmap. See
> [Project status](#project-status) for the honest breakdown.

---

## Why I built this

<!--
  Personalize this section in your own voice before sharing. Drop in the real
  story: which 1–2 sources you actually love (a specific blog/channel/podcast),
  and the moment you got fed up enough to build this.
-->

You know the move: you grab your phone to read *one* article, and an hour later
you've got nothing to show for it. That's not an accident. The feeds we live in
are tuned to keep you scrolling, not to get you the handful of things you came
for.

I wanted to stay current on a few sources I genuinely trust — engineering blogs,
a couple of writers, the occasional channel — without wading through everything
the platforms wrap around them. So I built a front door for exactly those
sources and nothing else.

The thing I kept coming back to: the problem isn't engagement. Engagement is
fine when it's pointed at something worth your time. The problem is *noise* —
uncurated, low-quality stuff competing for the same attention. So LockedIn's job
is curation. You pick the sources, it gives you a clean shelf of what's new, and
when you reach the bottom it just says "that's the whole shelf" and lets you put
the phone down.

It's also my capstone for the Boot.dev backend track, so it leans on the stack I
wanted to practice: Go, Postgres, and (soon) Docker.

## What it is

LockedIn is a server-rendered Go web app. The shape of it:

- **A curated catalog.** The app owns a small, hand-picked list of trusted
  sources. You browse it and follow individual ones (like subscribing to a
  channel). You never paste in feed URLs.
- **A reading feed of link-out cards.** Each item is a preview card — title,
  source, short summary, thumbnail — that opens the original site to
  read/watch/listen. Nothing gets republished in-app, which keeps it clear of
  copyright and free-tier headaches and keeps the app a *front door*, not a
  copy.
- **A finite feed.** The feed is reverse-chronological with numbered pagination.
  When you hit the end, you hit the end. No bottomless scroll.
- **Discover.** Follow a Reddit community by topic with one tap (e.g. `golang`,
  `investing`, `selfimprovement`) and its latest posts get pulled into your feed
  right away.
- **Accounts.** Email + password signup with email verification, and
  cookie-based sessions.

It's multi-modal by design — the schema already distinguishes `article`,
`youtube`, and `podcast` source types so card styling and thumbnails can adapt
per kind. v1 implements `article` first; the other two are additive.

## How it works

```
                 ┌──────────────┐
   curated RSS   │   fetcher    │   reads every feed in the DB, parses it,
   feeds ───────▶│ (cmd/fetcher)│──▶ and stores new items.
                 └──────┬───────┘
                        │ writes
                        ▼
                 ┌──────────────┐
                 │   Postgres   │  feeds · items · users · sessions ·
                 │              │  user_subscriptions · email_tokens
                 └──────┬───────┘
                        │ reads
                        ▼
                 ┌──────────────┐
   browser ─────▶│  api server  │  server-rendered HTML: landing, auth,
                 │  (cmd/api)   │  catalog, reading feed, discover.
                 └──────────────┘
```

- **`cmd/fetcher`** is a run-once CLI. It walks every feed in the database,
  fetches and parses it (RSS/Atom via [`gofeed`](https://github.com/mmcdole/gofeed)),
  and inserts any new items. Fetching is deliberately on-demand, not a background
  ticker.
- **`cmd/api`** is the web server. It uses the Go standard library for routing
  (`net/http` method+path patterns) and `html/template` for rendering. No SPA, no
  framework.
- **Discover** is the one place the API fetches inline: following a topic builds
  the provider feed URL, validates it, upserts the feed, subscribes you, and
  pulls its items immediately so your feed isn't empty.

### Routes

| Method & path | What it does |
|---|---|
| `GET /` | Public landing page when logged out; your reading feed when logged in |
| `GET /signup` · `POST /signup` | Create an account (verification link logged to console in dev) |
| `GET /login` · `POST /login` | Sign in |
| `GET /verify` | Consume an email-verification token |
| `POST /logout` | End the session |
| `GET /catalog` | Browse the curated catalog; follow / unfollow sources |
| `POST /subscriptions/{feed_id}` · `POST /subscriptions/{feed_id}/delete` | Follow / unfollow a catalog source |
| `GET /subscriptions` | List the sources you follow |
| `POST /discover` | Follow a Reddit community by topic and pull its items |
| `GET /healthz` | Health check |
| `POST /dev/reset` | Wipe table data (dev only, `ENV=development`) |

## Tech stack

- **Language:** Go (1.25+)
- **HTTP:** `net/http` standard library routing — no framework
- **Templates:** `html/template`, server-side rendered
- **Database:** PostgreSQL via `database/sql` + [`lib/pq`](https://github.com/lib/pq) (not pgx)
- **Queries:** [`sqlc`](https://sqlc.dev) generates type-safe Go from SQL
- **Migrations:** [`goose`](https://github.com/pressly/goose)
- **Feed parsing:** [`gofeed`](https://github.com/mmcdole/gofeed)
- **Auth:** bcrypt password hashing (`golang.org/x/crypto`), `crypto/rand` tokens, cookie sessions
- **Config:** `.env` via [`godotenv`](https://github.com/joho/godotenv)

## Project status

What's working locally right now:

- ✅ Signup → email verification → login / logout, with sessions
- ✅ Curated catalog with follow / unfollow
- ✅ Paginated reading feed of link-out preview cards
- ✅ "That's the whole shelf" finite-feed end state
- ✅ Discover (follow a Reddit community by topic)
- ✅ Public landing page

Still on the roadmap:

- ⏳ Real email sending (verification links are logged to the console in dev)
- ⏳ Password reset (designed, not built)
- ⏳ Resurfacing email digest — an occasional "you might've missed these" nudge
- ⏳ YouTube and podcast modalities (schema is ready; `article` is built first)
- ⏳ Deployment (planned: Docker Compose on a single VPS)

## Getting started

### Prerequisites

- [Go](https://go.dev/dl/) 1.25 or newer
- [PostgreSQL](https://www.postgresql.org/)
- [`goose`](https://github.com/pressly/goose#install) — database migrations
- [`sqlc`](https://docs.sqlc.dev/en/latest/overview/install.html) — only needed if you edit the SQL queries

### 1. Clone and grab dependencies

```bash
git clone https://github.com/weilok2021/lockedin.git
cd lockedin
go mod download
```

### 2. Create the database

```bash
sudo -u postgres psql \
  -c "CREATE ROLE lockedin WITH LOGIN PASSWORD '<your-password>';" \
  -c "CREATE DATABASE lockedin OWNER lockedin;"
```

### 3. Configure the environment

```bash
cp .env.example .env
```

Then edit `.env` and set `DB_URL` (and a password). The defaults run the app on
port `8080` in development mode. `.env` is gitignored — never commit real
secrets.

### 4. Run the migrations

```bash
goose -dir sql/schema postgres "$DB_URL" up
```

(To roll back: `goose -dir sql/schema postgres "$DB_URL" down`.)

### 5. Seed the curated catalog

```bash
psql "$DB_URL" -f sql/seed/catalog.sql
```

This adds a couple of starter sources (Julia Evans and Simon Willison) so the
catalog isn't empty.

### 6. Pull some content

```bash
go run ./cmd/fetcher
```

The fetcher reads every feed in the database and stores the latest items.

### 7. Run the server

```bash
go run ./cmd/api    # run from the project root — templates use relative paths
```

Open <http://localhost:8080>.

### Using it

1. Sign up at `/signup`. In dev, the verification link is printed to the
   server's console — open it to verify your email.
2. Log in, then visit `/catalog` and follow a source or two.
3. Run `go run ./cmd/fetcher` again to pull their latest items, then refresh `/`
   to see your feed.
4. Or hit Discover and follow a Reddit topic — its posts get pulled in
   immediately, no separate fetch needed.

> **Tip (dev only):** `curl -X POST http://localhost:8080/dev/reset` wipes all
> table data except users, handy when you want a clean slate.

## Project layout

```
cmd/
  api/         # web server entrypoint + handlers
  fetcher/     # run-once RSS fetcher
internal/
  auth/        # password hashing, token generation
  config/      # .env loading
  database/    # sqlc-generated query code (do not edit by hand)
  fetcher/     # feed fetch + item ingest logic
  feeds/       # topic → provider feed URL mapping
web/
  templates/   # html/template files
  static/      # CSS and static assets
sql/
  schema/      # goose migrations
  queries/     # sqlc query sources
  seed/        # catalog seed data
```

After editing anything in `sql/queries/`, regenerate the Go code with `sqlc generate`.

## License

See [LICENSE](LICENSE).
