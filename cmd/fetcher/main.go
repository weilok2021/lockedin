package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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

		body := item.Content
		if body == "" {
			body = item.Description
		}
		if err := f.queries.InsertItem(ctx, database.InsertItemParams{
			FeedID: feedID,
			Guid:   guid,
			Url:    item.Link,
			Title:  item.Title,
			Content: sql.NullString{
				String: body,
				Valid:  body != "", // valid=true when content is not an empty string
			},
			Author:      author,
			PublishedAt: publishedAt,
		}); err != nil {
			log.Printf("insert item %q: %v", item.Title, err)
			continue
		}
		fmt.Println("item has been fetched: " + item.Description)
	}
	return nil
}
