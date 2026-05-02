package cli

import (
	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/podcast"
	"github.com/alnah/moth/internal/transcription"
	"github.com/alnah/moth/internal/ytdlp"
)

type browserPageDocument struct {
	Type     string            `json:"type"`
	Page     browser.PageInfo  `json:"page"`
	Warnings []content.Warning `json:"warnings"`
}

type browserPagesDocument struct {
	Type     string             `json:"type"`
	Pages    []browser.PageInfo `json:"pages"`
	Warnings []content.Warning  `json:"warnings"`
}

type browserOperationDocument struct {
	Type     string            `json:"type"`
	OK       bool              `json:"ok"`
	Warnings []content.Warning `json:"warnings"`
}

type browserResponseMetadataDocument struct {
	Type     string                   `json:"type"`
	Response browser.ResponseMetadata `json:"response"`
	Warnings []content.Warning        `json:"warnings"`
}

type browserAccessibilityTreeDocument struct {
	Type     string                    `json:"type"`
	Tree     browser.AccessibilityTree `json:"tree"`
	Warnings []content.Warning         `json:"warnings"`
}

type browserChallengeDocument struct {
	Type      string                        `json:"type"`
	Challenge browser.ManualChallengeResult `json:"challenge"`
	Warnings  []content.Warning             `json:"warnings"`
}

func browserPageResult(page browser.PageInfo) browserPageDocument {
	return browserPageDocument{Type: "browser_page", Page: page, Warnings: []content.Warning{}}
}

func browserPagesResult(pages []browser.PageInfo) browserPagesDocument {
	return browserPagesDocument{Type: "browser_pages", Pages: pages, Warnings: []content.Warning{}}
}

func browserOperationResult() browserOperationDocument {
	return browserOperationDocument{Type: "browser_operation", OK: true, Warnings: []content.Warning{}}
}

func browserMetadataResult(response browser.ResponseMetadata) browserResponseMetadataDocument {
	return browserResponseMetadataDocument{
		Type:     "browser_response_metadata",
		Response: response,
		Warnings: []content.Warning{},
	}
}

func browserAccessibilityResult(tree browser.AccessibilityTree) browserAccessibilityTreeDocument {
	return browserAccessibilityTreeDocument{
		Type:     "browser_accessibility_tree",
		Tree:     tree,
		Warnings: []content.Warning{},
	}
}

func browserChallengeResult(challenge browser.ManualChallengeResult) browserChallengeDocument {
	return browserChallengeDocument{
		Type:      "browser_challenge",
		Challenge: challenge,
		Warnings:  challenge.Warnings,
	}
}

func contentPack(items ...content.Item) content.Pack {
	for index := range items {
		if items[index].Warnings == nil {
			items[index].Warnings = []content.Warning{}
		}
	}
	return content.Pack{Type: content.TypeContentPack, Items: items, Warnings: []content.Warning{}}
}

func subtitlePack(files ytdlp.SubtitleFiles) content.Pack {
	return contentPack(content.Item{
		Kind: content.KindFile,
		Metadata: map[string]any{
			"paths": files.Paths,
		},
	})
}

func audioPack(file ytdlp.AudioFile) content.Pack {
	return contentPack(content.Item{
		Kind: content.KindAudio,
		Metadata: map[string]any{
			"path": file.Path,
		},
	})
}

func podcastAudioPack(file podcast.AudioFile) content.Pack {
	return contentPack(content.Item{
		Kind: content.KindAudio,
		URL:  file.URL,
		Metadata: map[string]any{
			"content_type": file.ContentType,
			"bytes":        len(file.Bytes),
		},
	})
}

func transcriptionPack(result transcription.Result) content.Pack {
	return contentPack(content.Item{
		Kind:       content.KindAudio,
		Transcript: result.Text,
		Metadata: map[string]any{
			"segments": result.Segments,
			"metadata": result.Metadata,
		},
	})
}

func screenshotPack(request browser.ScreenshotRequest) content.Pack {
	return contentPack(content.Item{
		Kind: content.KindImage,
		URL:  request.URL,
		Metadata: map[string]any{
			"path": request.Path,
		},
	})
}

func browserPDFPack(request browser.PDFRequest) content.Pack {
	return contentPack(content.Item{
		Kind: content.KindPDF,
		URL:  request.URL,
		Metadata: map[string]any{
			"path": request.Path,
		},
	})
}

func downloadPack(result browser.CapturedDownload) content.Pack {
	return contentPack(content.Item{
		Kind: content.KindFile,
		Metadata: map[string]any{
			"path":         result.Path,
			"bytes":        result.Bytes,
			"content_type": result.ContentType,
		},
	})
}
