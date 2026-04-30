package tools

import "errors"

// Semantic runner and resolver errors.
var (
	ErrToolMissing = errors.New("tool_missing")
	ErrToolFailed  = errors.New("tool_failed")
)
