package domain

import "context"

type Vacancy struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
	ID    string `json:"id"`
}

type VectorStore interface {
	Clear(ctx context.Context) error
	HasVacancy(ctx context.Context, id string) (bool, error)
	AddVacancies(ctx context.Context, vacancies []string, embeddings [][]float32) error
	SearchSimilarity(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]Vacancy, error)
	ListVacancies(ctx context.Context, queryEmbedding []float32) ([]Vacancy, error)
	DeleteVacancy(ctx context.Context, id string) error
}
