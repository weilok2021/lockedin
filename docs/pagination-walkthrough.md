# Pagination walkthrough — numbered pages

How `handlerHome` turns a `?page=N` query into "Showing 41–60 of 87 · Page 3 of 5".

The whole design rests on **one idea**: ask the database *how many items exist*
(a single number), then derive everything else by arithmetic. No probe rows, no
guessing.

---

## The two queries (and what each actually returns)

| Query | Returns | Why |
|---|---|---|
| `CountItemsForUser` | one integer, e.g. `87` | the **total** — drives page count + the "of N" labels |
| `ListItemsForUser(LIMIT, OFFSET)` | **one page** of rows, e.g. 20 items | the items actually shown on this page |

> `COUNT(*)` returns the *number* `87`, **not** 87 items. Item bodies never travel
> for the count. Only one page (≤ 20 rows) is ever fetched per request.

---

## The inputs

```
pageSize = 20          (constant — items per page)
page     = 3           (from the URL: /?page=3)
total    = 87          (from CountItemsForUser)
```

---

## The archive, sliced into pages

The user's items, newest → oldest. The DB holds all 87; each request renders just
one 20-row window.

```
 #1 #2 #3 ... #20 │ #21 ... #40 │ #41 ... #60 │ #61 ... #80 │ #81 ... #87
 └──── page 1 ────┘└── page 2 ──┘└── page 3 ──┘└── page 4 ──┘└── page 5 ──┘
   offset 0          offset 20     offset 40     offset 60     offset 80
   20 items          20 items      20 items      20 items      7 items
```

Page 5 is short (only 7 items) — that's the partial last page. Everything else is full.

---

## Step-by-step: a request for page 3

### 1. Read & sanitize the page number
```
page = 3            // /?page=3, and 3 > 1 so we keep it
```

### 2. Count, then compute how many pages exist
```
total      = 87
totalPages = (total + pageSize - 1) / pageSize      ← ceil division
           = (87 + 20 - 1) / 20
           = 106 / 20
           = 5                                       (integer division drops .3)
```

**Why `+ pageSize - 1`?** Integer division floors. Adding `pageSize-1` first turns a
*floor* into a *ceiling*, so a leftover partial page still counts as a whole page:

| total | `(total + 19) / 20` | pages |
|---|---|---|
| 80 | 99 / 20  | **4**  ← exact, no extra page |
| 81 | 100 / 20 | **5**  ← the 1 leftover item gets its own page |
| 87 | 106 / 20 | **5** |
| 20 | 39 / 20  | **1** |
| 0  | 19 / 20  | **0** → floored up to **1** (always at least one page) |

### 3. Clamp out-of-range pages
```
if page > totalPages { page = totalPages }
3 > 5 ? no  →  page stays 3
```
(So `/?page=999` quietly becomes page 5 instead of an empty screen.)

### 4. Compute the offset and fetch ONE page
```
offset = (page - 1) * pageSize
       = (3 - 1) * 20
       = 40

ListItemsForUser(LIMIT 20, OFFSET 40)  →  rows #41..#60  (len = 20)
```

The database does the slicing — it skips 40 rows and returns the next 20:

```
OFFSET 40  ├─────────── skip these 40 ───────────┤├─ LIMIT 20 ─┤
           #1 ........................... #40       #41 .... #60     #61 ... #87
                                                    └ returned ┘     └ untouched ┘
```

### 5. Build the clickable page numbers
```
pageNumbers = [1, 2, 3, 4, 5]      // 1 .. totalPages
```

### 6. Compute the human-readable range
```
firstItem = offset + 1          = 41        (unless total == 0, then 0)
lastItem  = offset + len(items) = 40 + 20   = 60
```
`lastItem` uses `len(items)`, not `page * pageSize` — that's what makes the partial
last page show "81–**87**" instead of "81–100".

### 7. The prev/next flags
```
HasPrevPage = page > 1          = (3 > 1) = true   →  PrevPage = 2
HasNextPage = page < totalPages = (3 < 5) = true   →  NextPage = 4
```
Read aloud: *"there's a next page if I'm not on the last one."* No probe row needed.

---

## The resulting `Pagination` struct (page 3 of 87 items)

```
Page        = 3
TotalPages  = 5
PageNumbers = [1 2 3 4 5]
FirstItem   = 41
LastItem    = 60
TotalItems  = 87
HasPrevPage = true    PrevPage = 2
HasNextPage = true    NextPage = 4
```

### Which renders as:

```
            Showing 41–60 of 87  ·  Page 3 of 5

        ← Newer    1   2  [3]  4   5    Older →
                          ▲
                   current page, filled clay
```

---

## Edge cases, walked

### First page (page 1) — "Newer" disabled
```
offset      = 0
items       = #1..#20
firstItem   = 1,  lastItem = 20
HasPrevPage = (1 > 1) = false      → "← Newer" greyed, unclickable
HasNextPage = (1 < 5) = true
```
```
        Showing 1–20 of 87  ·  Page 1 of 5
        ← Newer   [1]  2   3   4   5    Older →
        ───────                                  (greyed)
```

### Last page (page 5) — "Older" disabled + the end card
```
offset      = 80
items       = #81..#87   (len = 7, a partial page)
firstItem   = 81, lastItem = 80 + 7 = 87
HasNextPage = (5 < 5) = false      → "Older →" greyed
```
```
        Showing 81–87 of 87  ·  Page 5 of 5
        ← Newer   1   2   3   4  [5]   Older →
                                       ─────     (greyed)

        ┄┄┄┄┄┄┄┄ That's everything. ┄┄┄┄┄┄┄┄
        You've reached the last page.
```

### Over-shoot (`/?page=999`, 87 items)
```
totalPages = 5
page = 999 → clamped to 5  →  behaves exactly like page 5 above
```

### Exact multiple (80 items, no leftovers)
```
totalPages = (80 + 19) / 20 = 4      ← four full pages, no phantom 5th
page 4: offset 60, items #61..#80, HasNextPage = (4 < 4) = false
```

### Empty feed (0 items)
```
totalPages = (0 + 19) / 20 = 0  →  floored to 1
```
But the template renders the empty-state (`feedempty`) when `.Items` is empty, so the
pager itself never shows for a brand-new user — the `Pagination` math just stays safe.

---

## The flow, end to end

```
   GET /?page=3
        │
        ▼
   page := 3
        │
        ▼
   CountItemsForUser ─────────────► total = 87
        │
        ▼
   totalPages = ceil(87/20) = 5
        │
        ▼
   clamp page to [1, totalPages]  (3 is fine)
        │
        ▼
   offset = (3-1)*20 = 40
        │
        ▼
   ListItemsForUser(LIMIT 20, OFFSET 40) ──► rows #41..#60
        │
        ▼
   build Pagination{ range, page numbers, prev/next }
        │
        ▼
   render home.html  ─►  "Showing 41–60 of 87 · Page 3 of 5"
```

---

## Why this version is the simple one

- **`HasNextPage = page < totalPages`** — a plain comparison, not a "did the page come
  back full?" guess.
- **`Limit = pageSize`** exactly — no `+1` probe row. The count query answers
  "is there more?" instead.
- **Each page is independent.** Page 3 fetches rows 41–60 directly; it doesn't depend
  on pages 1–2 having been loaded. Nothing accumulates anywhere.
- **Zero JavaScript.** Every control is a plain `<a href="/?page=N">` — back-button
  and refresh friendly by construction.
