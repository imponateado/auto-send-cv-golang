package domain

import (
	"context"
)

// Match represents a single vacancy that matched the candidate's resume.
type Match struct {
	Index         int    `json:"index"`
	Reason        string `json:"reason"`
	ContactType   string `json:"contact_type"`   // "email" or "whatsapp"
	ContactTarget string `json:"contact_target"` // email address or phone number
}

// MatchResult represents the structured response from the Gemini matching operation.
type MatchResult struct {
	Matches []Match `json:"matches"`
}

// GeminiService defines the domain contract for communicating with Gemini.
type GeminiService interface {
	MatchResume(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*MatchResult, error)
	GetEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
	ExtractText(ctx context.Context, fileB64 string, fileMime string) (string, error)
}
