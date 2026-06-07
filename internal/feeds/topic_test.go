package feeds

import (
	"testing"
)

// FeedURLForTopic maps a user-entered topic to the subreddit feed URL
// https://www.reddit.com/r/<topic>/.rss, with source reported as "reddit".
// Normalization (lowercase, whitespace stripped) must fold near-identical
// topics onto one URL, because feeds.feed_url is UNIQUE and case-variant
// spellings would otherwise create duplicate feed rows.
func TestFeedURLForTopic_BuildsSubredditFeedURL(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		wantURL string
	}{
		{
			name:    "simple topic",
			topic:   "golang",
			wantURL: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name:    "spaces stripped",
			topic:   "Claude Code",
			wantURL: "https://www.reddit.com/r/claudecode/.rss",
		},
		{
			name:    "messy whitespace trimmed and stripped",
			topic:   "  Claude   Code  ",
			wantURL: "https://www.reddit.com/r/claudecode/.rss",
		},
		{
			name:    "mixed case lowered",
			topic:   "GoLang",
			wantURL: "https://www.reddit.com/r/golang/.rss",
		},
		{
			name:    "path-hostile characters escaped, no extra path segments",
			topic:   "go/lang",
			wantURL: "https://www.reddit.com/r/go%2Flang/.rss",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotSource, err := FeedURLForTopic(tt.topic)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotURL != tt.wantURL {
				t.Errorf("feedURL = %q, want %q", gotURL, tt.wantURL)
			}
			if gotSource != "reddit" {
				t.Errorf("source = %q, want reddit", gotSource)
			}
		})
	}
}

// A topic that is empty (or only whitespace) cannot become a subreddit and
// must return an error rather than the malformed feed URL /r//.rss.
func TestFeedURLForTopic_RejectsEmptyTopic(t *testing.T) {
	for _, topic := range []string{"", "   "} {
		_, _, err := FeedURLForTopic(topic)
		if err == nil {
			t.Errorf("FeedURLForTopic(%q) returned nil error, want an error", topic)
		}
	}
}
