package feeds

import (
	"errors"
	"net/url"
	"strings"
)

// FeedURLForTopic normalizes a user-entered topic and returns the Google
// News RSS search URL for it, along with the source identifier.
func FeedURLForTopic(topic string) (feedURL string, source string, err error) {
	normalized := strings.Join(strings.Fields(topic), " ")
	if normalized == "" {
		return "", "", errors.New("no topic provided, please provide a topic")
	}

	q := url.Values{}
	q.Set("q", normalized)
	feedURL = "https://news.google.com/rss/search?" + q.Encode()
	return feedURL, "google_news", nil
}
