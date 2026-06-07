package feeds

import (
	"errors"
	"net/url"
	"strings"
)

// FeedURLForTopic normalizes a user-entered topic and returns the Google
// News RSS search URL for it, along with the source identifier.
func FeedURLForTopic(topic string) (feedURL string, source string, err error) {
	// "Self Improvement" → "selfimprovement": subreddit names are lowercase, no spaces
	normalized := strings.ToLower(strings.Join(strings.Fields(topic), ""))
	if normalized == "" {
		return "", "", errors.New("no topic provided, please provide a topic")
	}
	feedURL = "https://www.reddit.com/r/" + url.PathEscape(normalized) + "/.rss"
	return feedURL, "reddit", nil
}
