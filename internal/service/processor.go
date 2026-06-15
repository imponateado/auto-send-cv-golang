package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"api/internal/domain"
)

type textProcessor struct{}

// NewTextProcessor creates a new instance of the text payload processor.
func NewTextProcessor() domain.PayloadProcessor {
	return &textProcessor{}
}

// Process processes the text content, splitting it into smaller strings using the delimiter.
func (p *textProcessor) Process(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
	start := time.Now()

	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	// Respect context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	bytesProcessed := int64(len(req.Content))
	if bytesProcessed == 0 {
		return &domain.ProcessResult{
			BytesProcessed: 0,
			ItemsProcessed: 0,
			Items:          []string{},
			DurationMs:     time.Since(start).Milliseconds(),
			Status:         "success",
		}, nil
	}

	if req.Delimiter == "" {
		return nil, errors.New("delimiter cannot be empty")
	}

	// Split content into smaller strings using the delimiter
	items := strings.Split(req.Content, req.Delimiter)

	// If the last item is empty (caused by a trailing delimiter), we trim it to avoid empty entries
	if len(items) > 0 && items[len(items)-1] == "" {
		items = items[:len(items)-1]
	}

	return &domain.ProcessResult{
		BytesProcessed: bytesProcessed,
		ItemsProcessed: int64(len(items)),
		Items:          items,
		DurationMs:     time.Since(start).Milliseconds(),
		Status:         "success",
	}, nil
}
