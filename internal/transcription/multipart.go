package transcription

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

func multipartTranscriptionBody(request normalizedRequest, filePath string) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("model", request.Model); err != nil {
		return nil, "", fmt.Errorf("openai transcription multipart model: %w", err)
	}
	if request.Language != "" {
		if err := writer.WriteField("language", request.Language); err != nil {
			return nil, "", fmt.Errorf("openai transcription multipart language: %w", err)
		}
	}
	if err := writer.WriteField("response_format", request.ResponseFormat); err != nil {
		return nil, "", fmt.Errorf("openai transcription multipart response format: %w", err)
	}
	for _, granularity := range request.TimestampGranularities {
		if err := writer.WriteField("timestamp_granularities[]", granularity); err != nil {
			return nil, "", fmt.Errorf("openai transcription multipart timestamp granularity: %w", err)
		}
	}
	if err := writeMultipartFile(writer, filePath); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("openai transcription close multipart: %w", err)
	}

	return body.Bytes(), writer.FormDataContentType(), nil
}

func writeMultipartFile(writer *multipart.Writer, filePath string) error {
	//nolint:gosec // The caller intentionally selects the local audio file to upload.
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("openai transcription open audio file: %w", err)
	}
	defer func() { _ = file.Close() }()

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("openai transcription multipart file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("openai transcription copy audio file: %w", err)
	}

	return nil
}
