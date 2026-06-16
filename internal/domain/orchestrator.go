package domain

import (
	"context"
)

// Orchestrator defines the domain contract for processing files, matching with Gemini, and dispatching notifications.
type Orchestrator interface {
	RunMatchAndDispatch(ctx context.Context, req *ProcessRequest) (*ProcessResult, error)
}
