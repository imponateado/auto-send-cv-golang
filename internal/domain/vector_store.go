package domain

import "context"

// Vacancy é uma vaga do banco vetorial. ID (hash sha256 do texto) é a identidade
// estável; Index é só a posição no resultado desta consulta, para exibição.
type Vacancy struct {
	Index int     `json:"index"`
	Text  string  `json:"text"`
	ID    string  `json:"id"`
	Score float32 `json:"score"`
}

type VectorStore interface {
	Clear(ctx context.Context) error
	// ExistingVacancyIDs devolve quais dos ids já existem, numa consulta só.
	ExistingVacancyIDs(ctx context.Context, ids []string) (map[string]bool, error)
	AddVacancies(ctx context.Context, vacancies []string, embeddings [][]float32) error
	SearchSimilarity(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]Vacancy, error)
	ListVacancies(ctx context.Context) ([]Vacancy, error)
	DeleteVacancy(ctx context.Context, id string) error
}
