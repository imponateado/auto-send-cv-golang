package domain

import (
	"context"
)

type TaskStatus struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	ItemsProcessed int    `json:"items_processed"`
	Error          string `json:"error,omitempty"`
}

type Orchestrator interface {
	ClearVacancies(ctx context.Context) error
	PopulateVacancies(ctx context.Context, content, delimiter string) (int, error)
	PopulateVacanciesAsync(ctx context.Context, content, delimiter string) (string, error)
	PopulateVacancyTexts(ctx context.Context, texts []string) (int, error)
	GetTaskStatus(ctx context.Context, taskID string) (*TaskStatus, error)
	ListTasks(ctx context.Context) ([]*TaskStatus, error)
	MatchResume(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*ProcessResult, error)
}
