package transcription

import (
	"context"

	"golang.org/x/sync/errgroup"
)

func (client *Client) transcribeChunks(
	ctx context.Context,
	apiKey string,
	request normalizedRequest,
	chunks []Chunk,
) ([]Result, error) {
	results := make([]Result, len(chunks))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(request.MaxParallelTranscriptions)

	for index, chunk := range chunks {
		group.Go(func() error {
			result, err := client.transcribeFile(groupCtx, apiKey, request, chunk.Path)
			if err != nil {
				return err
			}
			results[index] = result

			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}
