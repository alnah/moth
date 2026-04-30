package httpdownload

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func contextWithOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, timeout)
}

func wrapTimeout(operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", operation, ErrTimeout)
	}

	return fmt.Errorf("%s: %w", operation, err)
}
