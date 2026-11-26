package supervisor

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDecodeCallReplyAcceptsLargeResult(t *testing.T) {
	html := strings.Repeat("a", 2<<20)
	payload := `{"result":{"url":"https://example.test/page","html":"` + html + `"}}`

	var result FetchRenderedResult
	if err := decodeCallReply(strings.NewReader(payload), &result); err != nil {
		t.Fatalf("decodeCallReply(large) error = %v, want nil", err)
	}
	if len(result.HTML) != len(html) {
		t.Fatalf("result.HTML length = %d, want %d", len(result.HTML), len(html))
	}
}

func TestDecodeCallReplyLimitRejectsOversizedResult(t *testing.T) {
	payload := `{"result":"` + strings.Repeat("a", 64) + `"}`

	var result string
	err := decodeCallReplyLimit(strings.NewReader(payload), 32, &result)
	if !errors.Is(err, ErrDaemonProtocol) {
		t.Fatalf("decodeCallReplyLimit(oversized) error = %v, want ErrDaemonProtocol", err)
	}
}

func TestDecodeParamsRejectsUnknownFieldAndTrailingJSON(t *testing.T) {
	tests := []struct {
		name   string
		params json.RawMessage
	}{
		{name: "unknown field", params: json.RawMessage(`{"url":"https://example.test","extra":1}`)},
		{name: "trailing document", params: json.RawMessage(`{"url":"https://example.test"} {}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var params FetchRenderedParams
			if err := decodeParams(tt.params, &params); !errors.Is(err, errInvalidParams) {
				t.Fatalf("decodeParams() error = %v, want errInvalidParams", err)
			}
		})
	}
}
