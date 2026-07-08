package domain

import (
	"context"
)

type TaskStatus struct {
	ID             string `json:"id"`
	Status         string `json:"status"` // "processing", "completed", "failed"
	ItemsProcessed int    `json:"items_processed"`
	Error          string `json:"error,omitempty"`
}

// Orchestrator define o contrato de domínio para orquestrar o processamento de vagas e currículos.
type Orchestrator interface {
	ClearVacancies(ctx context.Context) error
	PopulateVacancies(ctx context.Context, content, delimiter string) (int, error)
	PopulateVacanciesAsync(ctx context.Context, content, delimiter string) (string, error)
	GetTaskStatus(ctx context.Context, taskID string) (*TaskStatus, error)
	MatchResume(ctx context.Context, fileB64, fileMime, candidateEmail, candidatePhone string) (*ProcessResult, error)
}
