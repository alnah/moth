package content_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/alnah/moth/internal/content"
)

func TestPackJSONEncodesOmittedWarningsAsEmptyArrays(t *testing.T) {
	pack := content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindPage,
				URL:   "https://example.fr/no-warnings",
				Title: "No warnings",
			},
		},
	}

	encoded := marshalPackJSON(t, pack)
	var document struct {
		Warnings json.RawMessage `json:"warnings"`
		Items    []struct {
			Warnings json.RawMessage `json:"warnings"`
		} `json:"items"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode content pack JSON: %v", err)
	}

	assertRawJSON(t, document.Warnings, []byte(`[]`), "pack warnings")
	if len(document.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(document.Items))
	}
	assertRawJSON(t, document.Items[0].Warnings, []byte(`[]`), "item warnings")
}

func TestItemJSONEncodesOmittedWarningsAsEmptyArray(t *testing.T) {
	item := content.Item{
		Kind:  content.KindPage,
		URL:   "https://example.fr/item-no-warnings",
		Title: "No item warnings",
	}

	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal content item: %v", err)
	}
	var document struct {
		Warnings json.RawMessage `json:"warnings"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode content item JSON: %v", err)
	}

	assertRawJSON(t, document.Warnings, []byte(`[]`), "item warnings")
}

func TestPackJSONRoundTripsWarningValues(t *testing.T) {
	pack := content.Pack{
		Type:     content.TypeContentPack,
		Warnings: []content.Warning{content.WarningPartialContent},
		Items: []content.Item{
			{
				Kind:     content.KindPDF,
				URL:      "https://example.fr/report.pdf",
				Warnings: []content.Warning{content.WarningOCRUsed, content.WarningTimeout},
			},
		},
	}

	encoded := marshalPackJSON(t, pack)
	var decoded content.Pack
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode content pack JSON: %v", err)
	}

	if !slices.Equal(decoded.Warnings, pack.Warnings) {
		t.Fatalf("pack warnings = %#v, want %#v", decoded.Warnings, pack.Warnings)
	}
	if len(decoded.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(decoded.Items))
	}
	if !slices.Equal(decoded.Items[0].Warnings, pack.Items[0].Warnings) {
		t.Fatalf("item warnings = %#v, want %#v", decoded.Items[0].Warnings, pack.Items[0].Warnings)
	}
}

func marshalPackJSON(t *testing.T, pack content.Pack) []byte {
	t.Helper()

	encoded, err := json.Marshal(pack)
	if err != nil {
		t.Fatalf("marshal content pack: %v", err)
	}

	return encoded
}

func assertRawJSON(t *testing.T, got json.RawMessage, want []byte, label string) {
	t.Helper()

	if !bytes.Equal(got, want) {
		t.Fatalf("%s JSON = %s, want %s", label, got, want)
	}
}
