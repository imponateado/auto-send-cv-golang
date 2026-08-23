package domain

// ProcessResult é o resultado de uma execução de match de currículo.
type ProcessResult struct {
	Matches    []Match `json:"matches,omitempty"`
	DurationMs int64   `json:"duration_ms"`
	Status     string  `json:"status"`
}
