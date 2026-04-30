package limits

import (
	"testing"
	"time"
)

func TestDefaultOptionsUseConservativeRootLimits(t *testing.T) {
	options := DefaultOptions()

	if options.Timeout != 30*time.Second {
		t.Fatalf("timeout = %s, want 30s", options.Timeout)
	}
	if options.MaxResults != 10 {
		t.Fatalf("max results = %d, want 10", options.MaxResults)
	}
	if options.MaxBytes != 25*1024*1024 {
		t.Fatalf("max bytes = %d, want 25 MiB", options.MaxBytes)
	}
	if options.Retries != 0 {
		t.Fatalf("retries = %d, want 0", options.Retries)
	}
	if options.RetryBase != 500*time.Millisecond {
		t.Fatalf("retry base = %s, want 500ms", options.RetryBase)
	}
	if options.RetryMax != 5*time.Second {
		t.Fatalf("retry max = %s, want 5s", options.RetryMax)
	}
}
