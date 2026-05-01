package browser

import "testing"

func TestExtractPageItemResolvesMediaCandidates(t *testing.T) {
	item, err := extractPageItem(LoadedPage{URL: "https://example.test/articles/story", HTML: `<!doctype html>
<html><head><title>Media</title></head><body>
<picture src="/cover.avif"></picture>
<audio src="audio.mp3"></audio>
<video src="https://cdn.example.test/movie.mp4"></video>
</body></html>`})
	if err != nil {
		t.Fatalf("extractPageItem() error = %v, want nil", err)
	}
	candidates, ok := item.Metadata["media_candidates"].([]mediaCandidate)
	if !ok {
		t.Fatalf("media candidates = %#v, want []mediaCandidate", item.Metadata["media_candidates"])
	}
	if len(candidates) != 3 {
		t.Fatalf("media candidates len = %d, want 3: %#v", len(candidates), candidates)
	}
	if candidates[0].Type != "image" || candidates[1].Type != "audio" || candidates[2].Type != "video" {
		t.Fatalf("media candidate types = %#v, want image/audio/video", candidates)
	}
}

func TestResolveReferenceRejectsInvalidInputs(t *testing.T) {
	if got := resolveReference(":// bad base", "/path"); got != "" {
		t.Fatalf("resolve invalid base = %q, want empty", got)
	}
	if got := resolveReference("https://example.test", "%zz"); got != "" {
		t.Fatalf("resolve invalid reference = %q, want empty", got)
	}
	if got := resolveReference("https://example.test/base", ""); got != "" {
		t.Fatalf("resolve blank reference = %q, want empty", got)
	}
}
