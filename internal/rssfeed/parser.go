// Package rssfeed parses RSS, Atom, and JSON podcast feeds.
package rssfeed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

const defaultMaxFeedBytes = 10 * 1024 * 1024

// ErrMalformedFeed marks input that cannot be parsed as a supported feed.
var ErrMalformedFeed = errors.New("malformed_feed")

// ParseOptions configures feed parsing.
type ParseOptions struct {
	FeedURL  string
	MaxBytes int64
}

// Feed is the stable feed model used by Moth acquisition code.
type Feed struct {
	Title string
	URL   string
	Items []Item
}

// Item is a stable feed entry with podcast acquisition fields.
type Item struct {
	Title      string
	Link       string
	GUID       string
	Duration   time.Duration
	Enclosures []Enclosure
}

// Enclosure describes one downloadable media resource from a feed item.
type Enclosure struct {
	URL    string
	Type   string
	Length int64
}

// Parser parses supported feed formats with bounded reads.
type Parser struct{}

// NewParser creates a feed parser.
func NewParser() Parser {
	return Parser{}
}

// Parse reads a feed with a byte cap and maps it to the Moth feed model.
func (parser Parser) Parse(ctx context.Context, reader io.Reader, options ParseOptions) (Feed, error) {
	if err := ctx.Err(); err != nil {
		return Feed{}, fmt.Errorf("parse feed: %w", err)
	}

	data, err := readFeed(reader, options.MaxBytes)
	if err != nil {
		return Feed{}, err
	}
	if err := ctx.Err(); err != nil {
		return Feed{}, fmt.Errorf("parse feed: %w", err)
	}

	trimmed := bytes.TrimSpace(data)
	if isJSONFeed(trimmed) {
		return parser.parseJSONFeed(trimmed, options)
	}

	return parser.parseXMLFeed(trimmed, options)
}

func isJSONFeed(data []byte) bool {
	return len(data) > 0 && data[0] == '{'
}
