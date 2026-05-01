package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func (client *Client) transcribeSingleFile(
	ctx context.Context,
	apiKey string,
	request normalizedRequest,
) (Result, error) {
	result, err := client.transcribeFile(ctx, apiKey, request, request.FilePath)
	if err != nil {
		return Result{}, err
	}
	result.Metadata = transcriptionMetadata(request, 1)

	return result, nil
}

func (client *Client) transcribeFile(
	ctx context.Context,
	apiKey string,
	request normalizedRequest,
	filePath string,
) (Result, error) {
	if err := ensureUploadFileWithinLimit(filePath, request.MaxUploadBytes); err != nil {
		return Result{}, err
	}
	body, contentType, err := multipartTranscriptionBody(request, filePath)
	if err != nil {
		return Result{}, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.baseURL+"/audio/transcriptions",
		bytes.NewReader(body),
	)
	if err != nil {
		return Result{}, fmt.Errorf("openai transcription: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("openai transcription request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Result{}, openAIStatusError(resp, apiKey)
	}

	var response openAITranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return Result{}, fmt.Errorf("openai transcription decode response: %w", err)
	}

	return mapOpenAIResponse(response), nil
}

func ensureUploadFileWithinLimit(filePath string, maxUploadBytes int64) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("openai transcription: stat upload file: %w", err)
	}
	if info.Size() >= maxUploadBytes {
		return fmt.Errorf("openai transcription: upload file %q must be under %d bytes", filePath, maxUploadBytes)
	}

	return nil
}

type openAITranscriptionResponse struct {
	Text     string                  `json:"text"`
	Segments []openAIResponseSegment `json:"segments"`
}

type openAIResponseSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

func openAIStatusError(resp *http.Response, apiKey string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, openAIResponseBodyMax))
	responseText := strings.ReplaceAll(strings.TrimSpace(string(body)), apiKey, "[redacted]")

	return fmt.Errorf("openai transcription failed: status %d: %s", resp.StatusCode, responseText)
}

func mapOpenAIResponse(response openAITranscriptionResponse) Result {
	segments := make([]Segment, 0, len(response.Segments))
	for _, segment := range response.Segments {
		segments = append(segments, Segment{
			Start: secondsToDuration(segment.Start),
			End:   secondsToDuration(segment.End),
			Text:  segment.Text,
		})
	}

	result := Result{Text: response.Text}
	if len(segments) > 0 {
		result.Segments = segments
	}

	return result
}

func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
