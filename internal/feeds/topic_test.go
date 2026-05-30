package feeds

import (
	"net/url"
	"testing"
)

// A simple topic should become a Google News RSS search URL whose q
// parameter carries the topic, with source reported as "google_news".
func TestFeedURLForTopic_BuildsGoogleNewsSearchURL(t *testing.T) {
	gotURL, gotSource, err := FeedURLForTopic("Claude Code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u, err := url.Parse(gotURL)
	if err != nil {
		t.Fatalf("returned an unparseable URL %q: %v", gotURL, err)
	}

	if u.Host != "news.google.com" {
		t.Errorf("host = %q, want news.google.com", u.Host)
	}
	if u.Path != "/rss/search" {
		t.Errorf("path = %q, want /rss/search", u.Path)
	}
	if got := u.Query().Get("q"); got != "Claude Code" {
		t.Errorf("q param = %q, want %q", got, "Claude Code")
	}
	if gotSource != "google_news" {
		t.Errorf("source = %q, want google_news", gotSource)
	}
}

// Leading/trailing and repeated internal whitespace should be normalized
// so near-identical topics map to the same feed URL.
func TestFeedURLForTopic_NormalizesWhitespace(t *testing.T) {
	gotURL, _, err := FeedURLForTopic("  Claude   Code  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u, err := url.Parse(gotURL)
	if err != nil {
		t.Fatalf("returned an unparseable URL %q: %v", gotURL, err)
	}

	if got := u.Query().Get("q"); got != "Claude Code" {
		t.Errorf("q param = %q, want %q (whitespace should be trimmed and collapsed)", got, "Claude Code")
	}
}

// Special characters must survive round-trip through the query string.
func TestFeedURLForTopic_PreservesSpecialCharacters(t *testing.T) {
	gotURL, _, err := FeedURLForTopic("C++ & Go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u, err := url.Parse(gotURL)
	if err != nil {
		t.Fatalf("returned an unparseable URL %q: %v", gotURL, err)
	}

	if got := u.Query().Get("q"); got != "C++ & Go" {
		t.Errorf("q param = %q, want %q", got, "C++ & Go")
	}
}

// A topic that is empty (or only whitespace) cannot become a feed and
// must return an error.
func TestFeedURLForTopic_RejectsEmptyTopic(t *testing.T) {
	for _, topic := range []string{"", "   "} {
		_, _, err := FeedURLForTopic(topic)
		if err == nil {
			t.Errorf("FeedURLForTopic(%q) returned nil error, want an error", topic)
		}
	}
}
