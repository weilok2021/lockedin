# Signup & Email Verification — Manual Test Guide

## Prerequisites

- Postgres running with `lockedin_db` created
- Server built and ready to run

## 1. Start the server

```bash
go run ./cmd/api
```

You should see:

```
Starting server on :8080
```

## 2. Reset state (optional, for a clean run)

In another terminal:

```bash
curl -X POST http://localhost:8080/dev/reset
```

## 3. Test signup

```bash
curl -v -X POST http://localhost:8080/signup \
  -d 'email=test@example.com' \
  -d 'password=MyP@ssw0rd!23'
```

> `-d` sends form-encoded data (same as an HTML form).
> `-v` shows the full request/response so you can see the redirect.

**Expected:**

- Server log shows: `Verify email: http://localhost:8080/verify?token=<some-random-string>`
- curl receives: `303 See Other` — redirect to `/login?msg=check-email`

## 4. Verify user was created in the DB

```bash
psql lockedin_db -c "SELECT id, email, email_verified_at FROM users;"
```

Should show one row with `email_verified_at = NULL` (unverified).

## 5. Test email verification

Copy the token from the server log output, then:

```bash
curl -v "http://localhost:8080/verify?token=<paste-token-here>"
```

**Expected:**

- `302 Found` — redirect to `/login`
- No error in server logs

## 6. Verify user is now verified

```bash
psql lockedin_db -c "SELECT id, email, email_verified_at FROM users;"
```

`email_verified_at` should now be a timestamp (not NULL).

## 7. Edge cases

### Duplicate email (should redirect same as success — no info leak)

```bash
curl -v -X POST http://localhost:8080/signup \
  -d 'email=test@example.com' \
  -d 'password=MyP@ssw0rd!23'
```

### Weak password (should get 400)

```bash
curl -v -X POST http://localhost:8080/signup \
  -d 'email=test2@example.com' \
  -d 'password=short'
```

### Reuse same verify token (should get 400 — already consumed)

```bash
curl -v "http://localhost:8080/verify?token=<same-token-again>"
```
