package domain

import (
	"context"
)

type Match struct {
	Index         int    `json:"index"`
	Reason        string `json:"reason"`
	ContactType   string `json:"contact_type"`
	ContactTarget string `json:"contact_target"`
}

type MatchResult struct {
	Matches []Match `json:"matches"`
}

type GeminiService interface {
	MatchResume(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*MatchResult, error)
	GetEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
	ExtractText(ctx context.Context, fileB64 string, fileMime string) (string, error)
}
