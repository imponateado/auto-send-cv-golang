package domain

import (
	"context"
)

// Match é uma vaga que a LLM confirmou como compatível. Index é o índice na
// lista que foi enviada à LLM; VacancyID é preenchido pelo orchestrator a partir
// desse índice e é o que identifica a vaga de verdade.
type Match struct {
	Index         int    `json:"index"`
	VacancyID     string `json:"vacancy_id"`
	// Role é o cargo da vaga, como escrito no anúncio. É o único texto vindo da
	// LLM que chega ao recrutador, então o prompt pede uma transcrição, não uma
	// interpretação.
	Role          string `json:"role"`
	Reason        string `json:"reason"`
	ContactType   string `json:"contact_type"`
	ContactTarget string `json:"contact_target"`
	// Status do disparo. Nasce "Enfileirado" e é atualizado pelo dispatcher
	// conforme os envios acontecem — o POST /match responde antes de enviar, então
	// a confirmação vem por GET /api/v1/matches/{id}.
	Status string `json:"status"`
}

type MatchResult struct {
	Matches []Match `json:"matches"`
}

type GeminiService interface {
	MatchResume(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*MatchResult, error)
	GetEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
	ExtractText(ctx context.Context, fileB64 string, fileMime string) (string, error)
}
