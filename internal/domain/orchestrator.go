package domain

import (
	"context"
)

// Orchestrator define o contrato de domínio para orquestrar o processamento de vagas e currículos.
type Orchestrator interface {
	ClearVacancies(ctx context.Context) error
	PopulateVacancies(ctx context.Context, content, delimiter string) (int, error)
	MatchResume(ctx context.Context, fileB64, fileMime string) (*ProcessResult, error)
}
