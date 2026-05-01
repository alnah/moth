package browser

import (
	"context"
	"strings"
	"testing"
)

func TestUnsupportedWorkerSurfacesReportCapabilityErrors(t *testing.T) {
	ctx := context.Background()
	worker := &persistentOnlyWorker{}
	pool := NewPool(1, WithWorkerFactory(func(context.Context) (Worker, error) { return worker, nil }))
	defer func() { _ = pool.Close() }()

	page, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "capabilities",
		URL:         "https://example.test",
	})
	if err != nil {
		t.Fatalf("OpenPage(persistent-only worker) error = %v, want nil", err)
	}

	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "click",
			run: func() error {
				return pool.Click(ctx, InteractionRequest{
					ProfileName: "research",
					SessionName: "capabilities",
					PageID:      page.ID,
					Selector:    "button",
				})
			},
			want: "interactions",
		},
		{
			name: "input",
			run: func() error {
				return pool.Input(ctx, InputRequest{
					ProfileName: "research",
					SessionName: "capabilities",
					PageID:      page.ID,
					Selector:    "input",
					Text:        "query",
				})
			},
			want: "interactions",
		},
		{
			name: "wait",
			run: func() error {
				return pool.Wait(ctx, WaitRequest{
					ProfileName: "research",
					SessionName: "capabilities",
					PageID:      page.ID,
					Selector:    "main",
				})
			},
			want: "interactions",
		},
		{
			name: "accessibility",
			run: func() error {
				_, err := pool.AccessibilityTree(ctx, AccessibilityRequest{
					ProfileName: "research",
					SessionName: "capabilities",
					PageID:      page.ID,
				})
				return err
			},
			want: "accessibility",
		},
		{
			name: "download",
			run: func() error {
				_, err := pool.Download(ctx, DownloadRequest{
					ProfileName: "research",
					SessionName: "capabilities",
					PageID:      page.ID,
					Selector:    "a.download",
					Path:        t.TempDir() + "/download.bin",
				})
				return err
			},
			want: "downloads",
		},
		{
			name: "response metadata",
			run: func() error {
				_, err := pool.ResponseMetadata(ctx, ResponseMetadataRequest{URL: "https://example.test"})
				return err
			},
			want: "response metadata",
		},
		{
			name: "pdf",
			run: func() error {
				return pool.PDF(ctx, PDFRequest{URL: "https://example.test", Path: t.TempDir() + "/page.pdf"})
			},
			want: "pdf capture",
		},
		{
			name: "manual challenge",
			run: func() error {
				_, err := pool.DetectManualChallenge(ctx, ManualChallengeRequest{
					ProfileName: "research",
					SessionName: "capabilities",
					PageID:      page.ID,
				})
				return err
			},
			want: "manual challenge detection",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("surface error = %v, want %q", err, tc.want)
			}
		})
	}
}
