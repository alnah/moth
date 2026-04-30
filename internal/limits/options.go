package limits

import "time"

const (
	DefaultTimeout    = 30 * time.Second
	DefaultMaxResults = 10
	DefaultMaxBytes   = 25 * 1024 * 1024
	DefaultRetries    = 0
	DefaultRetryBase  = 500 * time.Millisecond
	DefaultRetryMax   = 5 * time.Second
)

type Options struct {
	Timeout    time.Duration
	MaxResults int
	MaxBytes   int64
	Retries    int
	RetryBase  time.Duration
	RetryMax   time.Duration
}

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
