package supervisor

import (
	"fmt"

	"github.com/alnah/moth/internal/browser"
)

func newBrowserStatusResult(value browser.BrowserStatus) BrowserStatusResult {
	return BrowserStatusResult{
		Status:       value.Status,
		Scope:        value.Scope,
		DebugURL:     value.DebugURL,
		ChromePID:    value.ChromePID,
		Owned:        value.Owned,
		DataDir:      value.DataDir,
		ActivePageID: value.ActivePageID,
	}
}

func newBrowserPageResult(value browser.PageInfo) BrowserPageResult {
	return BrowserPageResult{
		ID:          value.ID,
		URL:         value.URL,
		Title:       value.Title,
		Active:      value.Active,
		ProfileName: value.ProfileName,
		SessionName: value.SessionName,
	}
}

func newBrowserAccessibilityResult(value browser.AccessibilityTree) BrowserAccessibilityResult {
	nodes := make([]BrowserAccessibilityNode, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		nodes = append(nodes, BrowserAccessibilityNode{Role: node.Role, Name: node.Name})
	}
	return BrowserAccessibilityResult{Nodes: nodes}
}

func newBrowserChallengeResult(value browser.ManualChallengeResult) BrowserChallengeResult {
	warnings := make([]string, 0, len(value.Warnings))
	for _, warning := range value.Warnings {
		warnings = append(warnings, string(warning))
	}
	return BrowserChallengeResult{
		ManualRequired: value.ManualRequired,
		Kind:           value.Kind,
		Solved:         value.Solved,
		Warnings:       warnings,
	}
}

func newBrowserMetadataResult(value browser.ResponseMetadata) BrowserMetadataResult {
	return BrowserMetadataResult{
		URL:         value.URL,
		Status:      value.Status,
		ContentType: value.ContentType,
		Headers:     value.Headers,
	}
}

func capturedByteCount(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case []byte:
		return int64(len(typed)), nil
	default:
		return 0, fmt.Errorf("unsupported download byte count %T", value)
	}
}
