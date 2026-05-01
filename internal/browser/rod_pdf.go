package browser

import (
	"context"
	"fmt"
	"io"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (worker *rodWorker) CapturePDF(ctx context.Context, request PDFRequest) ([]byte, error) {
	var pdf []byte
	err := worker.withStatelessPage(ctx, request.URL, nil, "", func(page *rod.Page) error {
		reader, err := page.PDF(&proto.PagePrintToPDF{PrintBackground: true})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("capture pdf: %w", err)
		}
		defer func() { _ = reader.Close() }()
		pdf, err = io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("read pdf: %w", err)
		}
		return nil
	})
	return pdf, err
}
