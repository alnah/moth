package browser

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/content"
)

func TestFetchPageExtractsTitleTextLinksAndMediaCandidates(t *testing.T) {
	const pageURL = "https://example.test/articles/intro"
	const html = `<!doctype html>
<html>
<head>
  <title>Example article</title>
  <script>document.body.append("noise")</script>
</head>
<body>
  <nav><a href="/about">About us</a></nav>
  <main>
    <h1>Main heading</h1>
    <p>Useful body text.</p>
    <a href="https://cdn.example.test/file.pdf">Download PDF</a>
    <img src="/hero.jpg" alt="Hero image">
    <video src="/clip.mp4"></video>
  </main>
</body>
</html>`

	pool := NewPool(1, WithWorkerFactory(newQueuedWorkerFactory(t, &fakeWorker{
		openPage: func(_ context.Context, request PageRequest) (LoadedPage, error) {
			if request.URL != pageURL {
				t.Fatalf("OpenPage URL = %q, want %q", request.URL, pageURL)
			}
			return LoadedPage{URL: pageURL, HTML: html}, nil
		},
	})))
	defer func() { _ = pool.Close() }()

	got, err := pool.FetchPage(context.Background(), PageRequest{URL: pageURL})
	if err != nil {
		t.Fatalf("FetchPage() error = %v, want nil", err)
	}

	if got.Kind != content.KindPage {
		t.Fatalf("FetchPage() kind = %q, want page", got.Kind)
	}
	if got.URL != pageURL {
		t.Fatalf("FetchPage() URL = %q, want %q", got.URL, pageURL)
	}
	if got.Title != "Example article" {
		t.Fatalf("FetchPage() title = %q, want Example article", got.Title)
	}
	if !strings.Contains(got.Text, "Main heading") || !strings.Contains(got.Text, "Useful body text.") {
		t.Fatalf("FetchPage() text = %q, want visible heading and body", got.Text)
	}
	if strings.Contains(got.Text, "noise") {
		t.Fatalf("FetchPage() text = %q, want script text excluded", got.Text)
	}

	linksJSON := marshalMetadata(t, got.Metadata, "links")
	assertJSONContains(t, linksJSON, `"url":"https://example.test/about"`)
	assertJSONContains(t, linksJSON, `"text":"About us"`)
	assertJSONContains(t, linksJSON, `"url":"https://cdn.example.test/file.pdf"`)

	mediaJSON := marshalMetadata(t, got.Metadata, "media_candidates")
	assertJSONContains(t, mediaJSON, `"url":"https://example.test/hero.jpg"`)
	assertJSONContains(t, mediaJSON, `"type":"image"`)
	assertJSONContains(t, mediaJSON, `"url":"https://example.test/clip.mp4"`)
	assertJSONContains(t, mediaJSON, `"type":"video"`)
}

func TestScreenshotWritesBytesToRequestedPath(t *testing.T) {
	const pageURL = "https://example.test/chart"
	wantPath := filepath.Join(t.TempDir(), "nested", "chart.png")
	wantBytes := []byte("\x89PNG\r\nfake screenshot")

	pool := NewPool(1, WithWorkerFactory(newQueuedWorkerFactory(t, &fakeWorker{
		captureScreenshot: func(_ context.Context, request ScreenshotRequest) ([]byte, error) {
			if request.URL != pageURL {
				t.Fatalf("CaptureScreenshot URL = %q, want %q", request.URL, pageURL)
			}
			if request.Path != wantPath {
				t.Fatalf("CaptureScreenshot path = %q, want %q", request.Path, wantPath)
			}
			return wantBytes, nil
		},
	})))
	defer func() { _ = pool.Close() }()

	request := ScreenshotRequest{URL: pageURL, Path: wantPath, FullPage: true}
	if err := pool.Screenshot(context.Background(), request); err != nil {
		t.Fatalf("Screenshot() error = %v, want nil", err)
	}

	got, err := os.ReadFile(wantPath) //nolint:gosec // Test path is controlled under t.TempDir.
	if err != nil {
		t.Fatalf("read screenshot file: %v", err)
	}
	if string(got) != string(wantBytes) {
		t.Fatalf("screenshot file bytes = %q, want %q", got, wantBytes)
	}
}

func TestBrowserMissingErrorIsPropagated(t *testing.T) {
	pool := NewPool(1, WithWorkerFactory(func(context.Context) (Worker, error) {
		return nil, ErrBrowserMissing
	}))
	defer func() { _ = pool.Close() }()

	_, err := pool.FetchPage(context.Background(), PageRequest{URL: "https://example.test"})
	if err == nil {
		t.Fatal("FetchPage() error = nil, want browser missing")
	}
	if !errors.Is(err, ErrBrowserMissing) {
		t.Fatalf("FetchPage() error = %v, want ErrBrowserMissing", err)
	}
}

func marshalMetadata(t *testing.T, metadata map[string]any, key string) string {
	t.Helper()
	value, ok := metadata[key]
	if !ok {
		t.Fatalf("metadata missing %q: %#v", key, metadata)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal metadata[%q]: %v", key, err)
	}
	return string(encoded)
}

func assertJSONContains(t *testing.T, encoded string, want string) {
	t.Helper()
	if !strings.Contains(encoded, want) {
		t.Fatalf("JSON %s does not contain %s", encoded, want)
	}
}
