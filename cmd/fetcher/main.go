package main

import (
	"context"
	"database/sql"
	"html"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
	"github.com/weilok2021/lockedin/internal/config"
	"github.com/weilok2021/lockedin/internal/database"

	_ "github.com/lib/pq"
)

type Fetcher struct {
	db      *sql.DB
	queries *database.Queries
	cfg     config.Config
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("postgres", cfg.DbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	fetcher := &Fetcher{
		db:      db,
		queries: database.New(db),
		cfg:     cfg,
	}

	ctx := context.Background()
	dbFeeds, err := fetcher.queries.ListFeeds(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, feed := range dbFeeds {
		if err := fetcher.fetchFeed(ctx, feed.ID, feed.FeedUrl); err != nil {
			log.Printf("fetch feed %s: %v", feed.FeedUrl, err)
			continue
		}
	}

}

// parse one feed, store all it's items into db
func (f *Fetcher) fetchFeed(ctx context.Context, feedID uuid.UUID, feedURL string) error {
	fp := gofeed.NewParser()
	fp.UserAgent = "Lockedin/0.1 (+https://github.com/weilok2021/lockedin)"

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	fetchedFeed, err := fp.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return err
	}

	for _, item := range fetchedFeed.Items {
		// PublishedParsed is a *time.Time and is nil when the feed omits/garbles
		// the date; guard before dereferencing so a missing date stores as NULL.
		var publishedAt sql.NullTime
		if item.PublishedParsed != nil {
			publishedAt = sql.NullTime{Time: *item.PublishedParsed, Valid: true}
		}

		// Authors is often empty (e.g. Google News); check len before indexing
		// to avoid an out-of-range panic. No author stores as NULL.
		var author sql.NullString
		if len(item.Authors) > 0 && item.Authors[0].Name != "" {
			author = sql.NullString{String: item.Authors[0].Name, Valid: true}
		}

		// guid is the dedupe key (feed_id, guid). Some feeds omit it; fall back
		// to the link so empty guids don't all collide and drop items.
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}

		text := item.Description
		if text == "" {
			text = item.Content
		}
		summary := plainText(text, 200)

		if err := f.queries.InsertItem(ctx, database.InsertItemParams{
			FeedID: feedID,
			Guid:   guid,
			Url:    item.Link,
			Title:  item.Title,
			Summary: sql.NullString{
				String: summary,
				Valid:  summary != "", // valid=true when content is not an empty string
			},
			Author:      author,
			PublishedAt: publishedAt,
		}); err != nil {
			log.Printf("insert item %q: %v", item.Title, err)
			continue
		}
	}
	return nil
}

// htmlTagRe matches HTML tags so they can be stripped from feed text.
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// plainText turns a feed's HTML summary/body (item.Description or item.Content)
// into a short, clean plain-text blurb for a card.
//
// Order matters: strip tags BEFORE unescaping entities, so entity-encoded text
// like "&lt;dl&gt;" survives as the literal "<dl>" instead of being decoded to a
// real-looking tag and then removed. max is a rune count, not bytes.
func plainText(s string, max int) string {
	s = htmlTagRe.ReplaceAllString(s, " ")   // 1. strip real HTML tags
	s = html.UnescapeString(s)               // 2. decode entities (&lt; -> <, &amp; -> &)
	s = strings.Join(strings.Fields(s), " ") // 3. collapse whitespace runs, trim ends
	if r := []rune(s); len(r) > max {        // 4. truncate rune-safely, add an ellipsis
		s = strings.TrimRight(string(r[:max]), " ") + "…"
	}
	return s
}
