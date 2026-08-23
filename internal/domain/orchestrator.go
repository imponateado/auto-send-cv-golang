package domain

import (
	"context"
	"time"
)

type MatchRecord struct {
	ID        string         `json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	Result    *ProcessResult `json:"result"`
}

type Orchestrator interface {
	ClearVacancies(ctx context.Context) error
	PopulateVacancyTexts(ctx context.Context, texts []string) (int, error)
	ListVacancies(ctx context.Context) ([]Vacancy, error)
	DeleteVacancy(ctx context.Context, id string) error
	MatchResume(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*ProcessResult, error)
	ListMatches(ctx context.Context) ([]*MatchRecord, error)
	GetMatch(ctx context.Context, id string) (*MatchRecord, error)
	DeleteMatch(ctx context.Context, id string) error
}
