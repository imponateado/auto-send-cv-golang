package domain

import "context"

type Vacancy struct {
	Index int
	Text  string
}

type VectorStore interface {
	Clear(ctx context.Context) error
	HasVacancy(ctx context.Context, id string) (bool, error)
	AddVacancies(ctx context.Context, vacancies []string, embeddings [][]float32) error
	SearchSimilarity(ctx context.Context, queryEmbedding []float32, limit int, threshold float32) ([]Vacancy, error)
}
