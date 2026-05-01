package browser

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"github.com/alnah/moth/internal/content"
)

const warningReaderContentNotFound content.Warning = "reader_content_not_found"

// ExtractReaderContent returns article-like text or a clear visible-page fallback.
func ExtractReaderContent(loadedPage LoadedPage) (content.Item, error) {
	document, err := html.Parse(strings.NewReader(loadedPage.HTML))
	if err != nil {
		return content.Item{}, fmt.Errorf("parse reader html: %w", err)
	}

	if article := firstElement(document, "article"); article != nil {
		text := visibleText(article)
		if text != "" {
			return content.Item{
				Kind:     content.KindPage,
				URL:      loadedPage.URL,
				Title:    readerTitle(article, document),
				Text:     text,
				Warnings: []content.Warning{},
			}, nil
		}
	}

	return content.Item{
		Kind:     content.KindPage,
		URL:      loadedPage.URL,
		Title:    firstTitle(document),
		Text:     visibleText(document),
		Warnings: []content.Warning{warningReaderContentNotFound},
	}, nil
}

func readerTitle(article *html.Node, document *html.Node) string {
	for _, heading := range []string{"h1", "h2"} {
		if node := firstElement(article, heading); node != nil {
			if title := normalizedText(node); title != "" {
				return title
			}
		}
	}
	return firstTitle(document)
}

func firstElement(node *html.Node, elementName string) *html.Node {
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, elementName) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := firstElement(child, elementName); found != nil {
			return found
		}
	}
	return nil
}
