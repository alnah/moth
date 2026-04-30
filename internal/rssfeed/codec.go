package rssfeed

import (
	"bytes"
	"fmt"

	"github.com/mmcdole/gofeed"
	gofeedjson "github.com/mmcdole/gofeed/json"
)

func (parser Parser) parseXMLFeed(data []byte, options ParseOptions) (Feed, error) {
	parsed, err := gofeed.NewParser().Parse(bytes.NewReader(data))
	if err != nil {
		return Feed{}, fmt.Errorf("parse feed: %w", ErrMalformedFeed)
	}

	return mapUniversalFeed(parsed, options.FeedURL), nil
}

func (parser Parser) parseJSONFeed(data []byte, options ParseOptions) (Feed, error) {
	jsonParser := gofeedjson.Parser{}
	parsed, err := jsonParser.Parse(bytes.NewReader(data))
	if err != nil {
		return Feed{}, fmt.Errorf("parse JSON feed: %w", ErrMalformedFeed)
	}

	return mapJSONFeed(parsed, options.FeedURL), nil
}
