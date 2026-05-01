package browser

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/alnah/moth/internal/content"
)

type pageLink struct {
	URL  string `json:"url"`
	Text string `json:"text,omitempty"`
}

type mediaCandidate struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

func extractPageItem(loadedPage LoadedPage) (content.Item, error) {
	document, err := html.Parse(strings.NewReader(loadedPage.HTML))
	if err != nil {
		return content.Item{}, fmt.Errorf("parse rendered html: %w", err)
	}

	metadata := map[string]any{}
	links := extractLinks(document, loadedPage.URL)
	if len(links) > 0 {
		metadata["links"] = links
	}
	media := extractMediaCandidates(document, loadedPage.URL)
	if len(media) > 0 {
		metadata["media_candidates"] = media
	}

	return content.Item{
		Kind:     content.KindPage,
		URL:      loadedPage.URL,
		Title:    firstTitle(document),
		Text:     visibleText(document),
		Metadata: metadata,
		Warnings: []content.Warning{},
	}, nil
}

func firstTitle(node *html.Node) string {
	if node.Type == html.ElementNode && node.Data == "title" {
		return normalizedText(node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if title := firstTitle(child); title != "" {
			return title
		}
	}
	return ""
}

func visibleText(node *html.Node) string {
	return strings.Join(visibleTextFields(node, nil), " ")
}

func visibleTextFields(node *html.Node, fields []string) []string {
	if ignoredTextNode(node) {
		return fields
	}
	if node.Type == html.TextNode {
		return append(fields, strings.Fields(node.Data)...)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		fields = visibleTextFields(child, fields)
	}
	return fields
}

func ignoredTextNode(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}
	switch node.Data {
	case "head", "script", "style", "noscript", "template", "svg":
		return true
	default:
		return false
	}
}

func extractLinks(node *html.Node, baseURL string) []pageLink {
	links := []pageLink{}
	walkElements(node, func(element *html.Node) {
		if element.Data != "a" {
			return
		}
		href := attr(element, "href")
		absoluteURL := resolveReference(baseURL, href)
		if absoluteURL == "" {
			return
		}
		links = append(links, pageLink{URL: absoluteURL, Text: normalizedText(element)})
	})
	return links
}

func extractMediaCandidates(node *html.Node, baseURL string) []mediaCandidate {
	candidates := []mediaCandidate{}
	walkElements(node, func(element *html.Node) {
		src := attr(element, "src")
		if src == "" {
			return
		}
		mediaType := mediaTypeForElement(element.Data)
		if mediaType == "" {
			return
		}
		absoluteURL := resolveReference(baseURL, src)
		if absoluteURL == "" {
			return
		}
		candidates = append(candidates, mediaCandidate{URL: absoluteURL, Type: mediaType})
	})
	return candidates
}

func mediaTypeForElement(elementName string) string {
	switch elementName {
	case "img", "picture":
		return "image"
	case "video":
		return "video"
	case "audio":
		return "audio"
	default:
		return ""
	}
}

func walkElements(node *html.Node, visit func(*html.Node)) {
	if node.Type == html.ElementNode {
		visit(node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkElements(child, visit)
	}
}

func normalizedText(node *html.Node) string {
	return strings.Join(textFields(node, nil), " ")
}

func textFields(node *html.Node, fields []string) []string {
	if ignoredTextNode(node) {
		return fields
	}
	if node.Type == html.TextNode {
		return append(fields, strings.Fields(node.Data)...)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		fields = textFields(child, fields)
	}
	return fields
}

func attr(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, key) {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func resolveReference(baseURL string, reference string) string {
	if strings.TrimSpace(reference) == "" {
		return ""
	}
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	parsedReference, err := url.Parse(reference)
	if err != nil {
		return ""
	}
	return parsedBase.ResolveReference(parsedReference).String()
}
