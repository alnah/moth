package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/content"
)

func TestResponseMetadataIsNormalizedAndBoundedForJSON(t *testing.T) {
	worker := newSurfaceWorker()
	worker.response = ResponseMetadata{
		URL:         "https://example.test/articles?id=1#fragment",
		Status:      http.StatusOK,
		ContentType: "text/html; charset=utf-8",
		Headers: http.Header{
			"Content-Type":   []string{"text/html; charset=utf-8"},
			"Set-Cookie":     []string{"session=secret"},
			"X-Trace":        []string{strings.Repeat("x", 256)},
			"Cache-Control":  []string{"max-age=60"},
			"X-Request-Time": []string{"42ms"},
		},
	}
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	got, err := pool.ResponseMetadata(context.Background(), ResponseMetadataRequest{
		URL:            "https://example.test/articles?id=1#fragment",
		MaxHeaderBytes: 96,
	})
	if err != nil {
		t.Fatalf("ResponseMetadata() error = %v, want nil", err)
	}
	if got.URL != "https://example.test/articles?id=1" {
		t.Fatalf("metadata URL = %q, want normalized URL without fragment", got.URL)
	}
	if got.Status != http.StatusOK || got.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("metadata = %#v, want status and content type", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal response metadata: %v", err)
	}
	jsonText := string(encoded)
	if strings.Contains(jsonText, "Set-Cookie") || strings.Contains(jsonText, "session=secret") {
		t.Fatalf("metadata JSON = %s, want sensitive cookies omitted", jsonText)
	}
	if len(jsonText) > 512 {
		t.Fatalf("metadata JSON length = %d, want bounded output", len(jsonText))
	}
	if !strings.Contains(jsonText, "cache-control") || !strings.Contains(jsonText, "content-type") {
		t.Fatalf("metadata JSON = %s, want normalized lower-case deterministic headers", jsonText)
	}
}

func TestProxyAndResourceBlockingOptionsReachWorkerCreation(t *testing.T) {
	var captured WorkerOptions
	created := false
	pool := NewPool(1,
		WithConfiguredWorkerFactory(func(_ context.Context, options WorkerOptions) (Worker, error) {
			captured = options
			created = true
			return newSurfaceWorker(), nil
		}),
		WithProxy("http://proxy.example.test:8080"),
		WithBlockedResources(ResourceImages, ResourceFonts, ResourceMedia),
	)
	defer func() { _ = pool.Close() }()

	_, err := pool.ResponseMetadata(context.Background(), ResponseMetadataRequest{URL: "https://example.test"})
	if err != nil {
		t.Fatalf("ResponseMetadata() error = %v, want nil", err)
	}
	if !created {
		t.Fatal("worker factory was not called")
	}
	if captured.ProxyURL != "http://proxy.example.test:8080" {
		t.Fatalf("worker proxy URL = %q, want configured proxy", captured.ProxyURL)
	}
	if !captured.BlockedResources.Has(ResourceImages) {
		t.Fatalf("worker blocked resources = %#v, want images blocked", captured.BlockedResources)
	}
	if !captured.BlockedResources.Has(ResourceFonts) {
		t.Fatalf("worker blocked resources = %#v, want fonts blocked", captured.BlockedResources)
	}
	if !captured.BlockedResources.Has(ResourceMedia) {
		t.Fatalf("worker blocked resources = %#v, want media blocked", captured.BlockedResources)
	}
}

func TestCaptchaDetectionReportsManualRequiredAndNeverSolves(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	worker.challenge = ManualChallengeResult{
		ManualRequired: true,
		Kind:           "captcha",
		Warnings:       []content.Warning{content.WarningCaptchaPossible},
	}
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	page := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "manual-login",
		URL:         "https://example.test/login",
	})
	got, err := pool.DetectManualChallenge(ctx, ManualChallengeRequest{
		ProfileName: "research",
		SessionName: "manual-login",
		PageID:      page.ID,
	})
	if err != nil {
		t.Fatalf("DetectManualChallenge() error = %v, want nil", err)
	}
	if !got.ManualRequired || got.Kind != "captcha" {
		t.Fatalf("manual challenge = %#v, want captcha manual-required state", got)
	}
	if got.Solved {
		t.Fatalf("manual challenge = %#v, want no automatic CAPTCHA solving", got)
	}
	if !hasWarning(got.Warnings, content.WarningCaptchaPossible) {
		t.Fatalf("manual challenge warnings = %#v, want captcha_possible", got.Warnings)
	}
}
