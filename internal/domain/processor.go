package domain

import (
	"context"
)

type ProcessRequest struct {
	Content    string `json:"content"`
	Delimiter  string `json:"delimiter"`
	FileBase64 string `json:"file_base64"`
}

type FileMetadata struct {
	SizeInBytes int64  `json:"size_in_bytes"`
	MimeType    string `json:"mime_type"`
	Status      string `json:"status"`
}

type ProcessResult struct {
	BytesProcessed int64         `json:"bytes_processed"`
	ItemsProcessed int64         `json:"items_processed"`
	Items          []string      `json:"items"`
	File           *FileMetadata `json:"file,omitempty"`
	Matches        []Match       `json:"matches,omitempty"`
	DurationMs     int64         `json:"duration_ms"`
	Status         string        `json:"status"`
}

type PayloadProcessor interface {
	Process(ctx context.Context, req *ProcessRequest) (*ProcessResult, error)
}
