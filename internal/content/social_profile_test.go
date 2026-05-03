package content

import (
	"encoding/json"
	"testing"
)

func TestKindSocialProfileJSONValueIsStable(t *testing.T) {
	encoded, err := json.Marshal(Item{Kind: KindSocialProfile})
	if err != nil {
		t.Fatalf("marshal social profile item: %v", err)
	}

	var document struct {
		Kind Kind `json:"kind"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode social profile item JSON: %v", err)
	}
	if document.Kind != Kind("social_profile") {
		t.Fatalf("social profile kind JSON = %q, want social_profile", document.Kind)
	}
}
