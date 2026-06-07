package main

// dbFeed is the feed metadata we fetched and store in db
// goFeed is the parsed feed struct after fetching the feed url

import (
	"context"
	"database/sql"
	"log"

	"github.com/weilok2021/lockedin/internal/config"
	"github.com/weilok2021/lockedin/internal/database"
	"github.com/weilok2021/lockedin/internal/fetcher"

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

	f := &Fetcher{
		db:      db,
		queries: database.New(db),
		cfg:     cfg,
	}

	ctx := context.Background()
	dbFeeds, err := f.queries.ListFeeds(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, dbFeed := range dbFeeds {
		goFeed, err := fetcher.FetchFeed(ctx, dbFeed.FeedUrl)
		if err != nil {
			log.Printf("fetch feed %s: %v", dbFeed.FeedUrl, err)
			continue
		}
		fetcher.StoreFeedItems(ctx, f.queries, goFeed, dbFeed.ID)
	}

}
