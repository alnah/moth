package content

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPackJSONIncludesContractFields(t *testing.T) {
	pack := Pack{
		Type: TypeContentPack,
		Items: []Item{
			{
				Kind:       KindPage,
				URL:        "https://example.fr/article",
				Title:      "Example title",
				Text:       "complete page text requested by the agent",
				Transcript: "complete transcript requested by the agent",
				Metadata: map[string]any{
					"source": "http",
				},
				Warnings: []Warning{WarningOCRUsed},
			},
		},
		Warnings: []Warning{WarningPartialContent},
	}

	encoded, err := json.Marshal(pack)
	if err != nil {
		t.Fatalf("marshal content pack: %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode marshaled content pack: %v", err)
	}

	if got := document["type"]; got != "content_pack" {
		t.Fatalf("type = %v, want content_pack", got)
	}
	if _, ok := document["items"]; !ok {
		t.Fatalf("items field missing in %s", encoded)
	}
	if _, ok := document["warnings"]; !ok {
		t.Fatalf("warnings field missing in %s", encoded)
	}
	if _, ok := document["cache"]; ok {
		t.Fatalf("cache field present in V1 content pack: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"text":"complete page text requested by the agent"`)) {
		t.Fatalf("complete item text missing from %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"transcript":"complete transcript requested by the agent"`)) {
		t.Fatalf("complete item transcript missing from %s", encoded)
	}
}

func TestItemKindJSONRoundTrip(t *testing.T) {
	kinds := []Kind{
		KindPage,
		KindPDF,
		KindImage,
		KindVideo,
		KindAudio,
		KindPodcast,
		KindSocialPost,
		KindSocialThread,
		KindSocialProfile,
		KindFeed,
		KindFile,
	}

	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			encoded, err := json.Marshal(Item{Kind: kind})
			if err != nil {
				t.Fatalf("marshal item: %v", err)
			}

			var decoded Item
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("decode item: %v", err)
			}
			if decoded.Kind != kind {
				t.Fatalf("kind = %q, want %q", decoded.Kind, kind)
			}
		})
	}
}

func TestWarningsUseTechnicalFactCodes(t *testing.T) {
	warnings := []Warning{
		WarningTimeout,
		WarningLoginRequired,
		WarningCaptchaPossible,
		WarningNoTranscriptFound,
		WarningFileTooLarge,
		WarningPartialContent,
		WarningToolMissing,
		WarningProviderRateLimited,
		WarningOCRUsed,
		WarningOCRFailed,
	}

	for _, warning := range warnings {
		t.Run(string(warning), func(t *testing.T) {
			encoded, err := json.Marshal(warning)
			if err != nil {
				t.Fatalf("marshal warning: %v", err)
			}

			var decoded Warning
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("decode warning: %v", err)
			}
			if decoded != warning {
				t.Fatalf("warning = %q, want %q", decoded, warning)
			}
			if strings.ContainsAny(string(decoded), " .") {
				t.Fatalf("warning %q is not a technical fact code", decoded)
			}
		})
	}
}

func TestUnknownOptionalJSONFieldsDoNotBreakDecoding(t *testing.T) {
	raw := []byte(`{
		"type": "content_pack",
		"cache": {"ignored_in_v1": true},
		"items": [
			{
				"kind": "page",
				"url": "https://example.fr/article",
				"title": "Example title",
				"text": "complete text",
				"metadata": {"source": "fixture"},
				"warnings": [],
				"unknown_optional": {"future": true}
			}
		],
		"warnings": [],
		"unknown_top_level": "ignored"
	}`)

	var pack Pack
	if err := json.Unmarshal(raw, &pack); err != nil {
		t.Fatalf("decode content pack with unknown optional fields: %v", err)
	}
	if pack.Type != TypeContentPack {
		t.Fatalf("type = %q, want %q", pack.Type, TypeContentPack)
	}
	if len(pack.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(pack.Items))
	}
	if pack.Items[0].Text != "complete text" {
		t.Fatalf("item text = %q, want complete text", pack.Items[0].Text)
	}
}
