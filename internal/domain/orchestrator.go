package domain

import (
	"context"
)

type Orchestrator interface {
	ClearVacancies(ctx context.Context) error
	PopulateVacancyTexts(ctx context.Context, texts []string) (int, error)
	ListVacancies(ctx context.Context) ([]Vacancy, error)
	MatchResume(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*ProcessResult, error)
}
