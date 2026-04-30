package limits

import "time"

// Default root command limits.
const (
	DefaultTimeout    = 30 * time.Second
	DefaultMaxResults = 10
	DefaultMaxBytes   = 25 * 1024 * 1024
	DefaultRetries    = 0
	DefaultRetryBase  = 500 * time.Millisecond
	DefaultRetryMax   = 5 * time.Second
)

// Options contains conservative global command limits.
type Options struct {
	Timeout    time.Duration
	MaxResults int
	MaxBytes   int64
	Retries    int
	RetryBase  time.Duration
	RetryMax   time.Duration
}

// DefaultOptions returns conservative global limits for CLI commands.
func DefaultOptions() Options {
	return Options{
		Timeout:    DefaultTimeout,
		MaxResults: DefaultMaxResults,
		MaxBytes:   DefaultMaxBytes,
		Retries:    DefaultRetries,
		RetryBase:  DefaultRetryBase,
		RetryMax:   DefaultRetryMax,
	}
}
