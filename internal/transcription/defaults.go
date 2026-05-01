package transcription

import "time"

const (
	defaultOpenAIBaseURL            = "https://api.openai.com/v1"
	defaultTranscriptionModel       = "gpt-4o-mini-transcribe"
	defaultTranscriptionFormat      = "json"
	defaultChunkDuration            = 10 * time.Minute
	defaultChunkOverlap             = 2 * time.Second
	defaultMaxUploadBytes           = 25 * 1024 * 1024
	defaultMaxParallelTranscription = 2
	openAIResponseBodyMax           = 4096
	toolOutputLimit                 = 1 << 20
)
