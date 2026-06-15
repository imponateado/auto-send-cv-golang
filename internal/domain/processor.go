package domain

import (
	"context"
)

// ProcessRequest represents the input payload for processing.
type ProcessRequest struct {
	Content   string `json:"content"`
	Delimiter string `json:"delimiter"`
}

// ProcessResult represents the statistics of the processed text stream.
type ProcessResult struct {
	BytesProcessed int64    `json:"bytes_processed"`
	ItemsProcessed int64    `json:"items_processed"`
	Items          []string `json:"items"`
	DurationMs     int64    `json:"duration_ms"`
	Status         string   `json:"status"`
}

// PayloadProcessor defines the domain contract for processing raw text payloads.
type PayloadProcessor interface {
	Process(ctx context.Context, req *ProcessRequest) (*ProcessResult, error)
}
