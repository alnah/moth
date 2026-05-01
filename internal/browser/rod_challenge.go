package browser

import (
	"context"
	"fmt"
	"strings"

	"github.com/alnah/moth/internal/content"
)

func (worker *rodWorker) DetectManualChallenge(
	ctx context.Context,
	request ManualChallengeRequest,
) (ManualChallengeResult, error) {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return ManualChallengeResult{}, err
	}
	htmlText, err := page.Context(ctx).HTML()
	if err != nil {
		return ManualChallengeResult{}, fmt.Errorf("read challenge page: %w", err)
	}
	item, err := extractPageItem(LoadedPage{URL: page.String(), HTML: htmlText})
	if err != nil {
		return ManualChallengeResult{}, err
	}
	lowerText := strings.ToLower(item.Text)
	if strings.Contains(lowerText, "captcha") || strings.Contains(lowerText, "verify you are human") {
		return ManualChallengeResult{
			ManualRequired: true,
			Kind:           "captcha",
			Warnings:       []content.Warning{content.WarningCaptchaPossible},
		}, nil
	}
	return ManualChallengeResult{Warnings: []content.Warning{}}, nil
}
