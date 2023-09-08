package ui

import (
	"fmt"
	"net/url"

	"github.com/mmcdole/gofeed"
)

/*
Various methods for fetching items from different resource feeds
*/

const (
	// SchemeRSS is the URL scheme for RSS feeds with TLS
	SchemeRSS string = "rss"
	// SchemeRSSI is the URL scheme for RSS feeds without TLS
	SchemeRSSI string = "rssi"
)

// Item is the model of time tracking items for the purposes of the feed UI
type Item struct {
	Link        string // A URL for the item
	Description string // A description for the item
	Context     string // A context for the item
	Identifier  string // Identifier for the item
	Coordinal   string // Coordinal for the item
}

// A Feed is a source of items that can be automatically fetched
type Feed interface {
	FetchNew() ([]*Item, error) // Get items from the feed
}

// NewFeed takes in a URL for a feed and returns a Feed object that can fetch items
// from that URL
func NewFeed(urlS string, ctx string) (Feed, error) {
	parsed, err := url.Parse(urlS)
	if err != nil {
		return nil, fmt.Errorf("Error parsing URL: %s", err)
	}
	switch parsed.Scheme {
	case SchemeRSS:
		parsed.Scheme = "https"
		return &RSSFeed{
			Context: ctx,
			URL:     parsed,
		}, nil
	case SchemeRSSI:
		parsed.Scheme = "http"
		return &RSSFeed{
			Context: ctx,
			URL:     parsed,
		}, nil
	default:
		return nil, fmt.Errorf("Unknown URL scheme for feed: %s", parsed.Scheme)
	}
}

type RSSFeed struct {
	Context string
	URL     *url.URL
}

func (f *RSSFeed) FetchNew() ([]*Item, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(f.URL.String())
	if err != nil {
		return nil, err
	}
	items := make([]*Item, len(feed.Items))
	for i, item := range feed.Items {
		identifier := item.Title
		if f.Context != "" {
			identifier = fmt.Sprintf("[%s] %s", f.Context, item.Title)
		}
		items[i] = &Item{
			Identifier:  identifier,
			Context:     f.Context,
			Description: item.Title,
			Link:        item.Link,
		}
	}
	return items, nil

}
