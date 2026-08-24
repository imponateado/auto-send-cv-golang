package db

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"api/internal/domain"
)

func newTestVacancyStore(t *testing.T) (domain.VectorStore, context.Context) {
	t.Helper()
	sqlDB, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	store, err := NewVacancyStore(sqlDB)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	return store, context.Background()
}

// O round-trip do embedding é a base de tudo: se ele corromper o vetor, a busca
// vira ruído e nada estoura — os scores só despencam silenciosamente.
func TestEmbeddingRoundTrip(t *testing.T) {
	want := []float32{0, 1, -1, 0.5, -0.0001, 3.4028235e+38, 1.1754944e-38}

	got, err := decodeEmbedding(encodeEmbedding(want))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("dimensão mudou: want %d, got %d", len(want), len(got))
	}
	for i := range want {
		if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
			t.Errorf("elemento %d alterado: want %v, got %v", i, want[i], got[i])
		}
	}

	if _, err := decodeEmbedding([]byte{1, 2, 3}); err == nil {
		t.Error("blob com tamanho não múltiplo de 4 deve dar erro")
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float32
	}{
		{"idênticos", []float32{1, 2, 3}, []float32{1, 2, 3}, 1},
		{"ortogonais", []float32{1, 0}, []float32{0, 1}, 0},
		{"opostos", []float32{1, 0}, []float32{-1, 0}, -1},
		{"mesma direção, magnitudes diferentes", []float32{1, 1}, []float32{5, 5}, 1},
		{"dimensões diferentes", []float32{1, 2}, []float32{1, 2, 3}, 0},
		{"vazio", []float32{}, []float32{}, 0},
		{"vetor zero", []float32{0, 0}, []float32{1, 1}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(float64(got-tt.want)) > 1e-6 {
				t.Errorf("want %v, got %v", tt.want, got)
			}
		})
	}
}

func TestSearchSimilarityRanksAndFilters(t *testing.T) {
	store, ctx := newTestVacancyStore(t)

	// "perto" é idêntico à consulta, "meio" fica a 45°, "longe" é ortogonal.
	vacancies := []string{"perto", "meio", "longe"}
	embeddings := [][]float32{
		{1, 0, 0},
		{1, 1, 0},
		{0, 0, 1},
	}
	if err := store.AddVacancies(ctx, vacancies, embeddings); err != nil {
		t.Fatalf("failed to seed: %v", err)
	}

	query := []float32{1, 0, 0}

	got, err := store.SearchSimilarity(ctx, query, 10, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("threshold 0.5 deve deixar passar 'perto' (1.0) e 'meio' (~0.707), got %d: %+v", len(got), got)
	}
	if got[0].Text != "perto" || got[1].Text != "meio" {
		t.Errorf("resultado deve vir ordenado por similaridade desc, got %q e %q", got[0].Text, got[1].Text)
	}
	if got[0].Index != 0 || got[1].Index != 1 {
		t.Errorf("Index deve ser a posição no resultado, got %d e %d", got[0].Index, got[1].Index)
	}
	if got[0].Score < got[1].Score {
		t.Errorf("Score deve cair ao longo do resultado, got %v e %v", got[0].Score, got[1].Score)
	}

	limited, err := store.SearchSimilarity(ctx, query, 1, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(limited) != 1 || limited[0].Text != "perto" {
		t.Errorf("limit=1 deve devolver só a mais similar, got %+v", limited)
	}

	none, err := store.SearchSimilarity(ctx, query, 10, 0.99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(none) != 1 {
		t.Errorf("threshold 0.99 deve deixar passar só 'perto', got %d", len(none))
	}
}

func TestExistingVacancyIDs(t *testing.T) {
	store, ctx := newTestVacancyStore(t)

	if got, err := store.ExistingVacancyIDs(ctx, nil); err != nil || len(got) != 0 {
		t.Fatalf("lista vazia deve devolver map vazio sem erro, got %v, %v", got, err)
	}

	if err := store.AddVacancies(ctx, []string{"vaga a", "vaga b"}, [][]float32{{1, 0}, {0, 1}}); err != nil {
		t.Fatalf("failed to seed: %v", err)
	}

	presentID := vacancyID("vaga a")
	absentID := vacancyID("vaga que não existe")

	got, err := store.ExistingVacancyIDs(ctx, []string{presentID, absentID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got[presentID] {
		t.Errorf("esperava %q presente", presentID)
	}
	if got[absentID] {
		t.Errorf("não esperava %q presente", absentID)
	}

	// O ID ignora espaços nas pontas — é o que deduplica a mesma vaga repostada.
	if vacancyID("  vaga a  ") != presentID {
		t.Error("vacancyID deve ignorar espaços nas pontas")
	}
}

func TestListClearAndDelete(t *testing.T) {
	store, ctx := newTestVacancyStore(t)

	empty, err := store.ListVacancies(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if empty == nil {
		t.Error("tabela vazia deve devolver slice vazio, não nil (o handler serializa direto pra JSON)")
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 vacancies, got %d", len(empty))
	}

	if err := store.AddVacancies(ctx, []string{"vaga a", "vaga b"}, [][]float32{{1, 0}, {0, 1}}); err != nil {
		t.Fatalf("failed to seed: %v", err)
	}

	listed, err := store.ListVacancies(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 vacancies, got %d", len(listed))
	}

	if err := store.DeleteVacancy(ctx, vacancyID("vaga a")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after, err := store.ListVacancies(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(after) != 1 || after[0].Text != "vaga b" {
		t.Fatalf("esperava sobrar só 'vaga b', got %+v", after)
	}

	// Reinserir a mesma vaga não duplica: o ID é o hash do texto.
	if err := store.AddVacancies(ctx, []string{"vaga b"}, [][]float32{{0, 1}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if again, _ := store.ListVacancies(ctx); len(again) != 1 {
		t.Errorf("reinserir a mesma vaga não pode duplicar, got %d", len(again))
	}

	if err := store.Clear(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleared, _ := store.ListVacancies(ctx); len(cleared) != 0 {
		t.Errorf("Clear deve esvaziar a tabela, sobrou %d", len(cleared))
	}
}

func TestAddVacanciesRejectsSizeMismatch(t *testing.T) {
	store, ctx := newTestVacancyStore(t)

	if err := store.AddVacancies(ctx, []string{"a", "b"}, [][]float32{{1, 0}}); err == nil {
		t.Error("esperava erro quando a contagem de vagas e embeddings difere")
	}
}

// limit <= 0 significa "sem limite": quem filtra aptidão é a LLM adiante, e um
// corte em N aqui descartaria vagas com score praticamente idêntico às que
// passaram.
func TestSearchSimilarityUnlimited(t *testing.T) {
	store, ctx := newTestVacancyStore(t)

	vacancies := []string{"a", "b", "c", "d"}
	embeddings := [][]float32{{1, 0}, {1, 0}, {1, 0}, {1, 0}}
	if err := store.AddVacancies(ctx, vacancies, embeddings); err != nil {
		t.Fatalf("failed to seed: %v", err)
	}

	all, err := store.SearchSimilarity(ctx, []float32{1, 0}, 0, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("limit=0 deve devolver todas as vagas acima do threshold, got %d", len(all))
	}

	capped, err := store.SearchSimilarity(ctx, []float32{1, 0}, 2, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capped) != 2 {
		t.Errorf("limit=2 deve cortar em 2, got %d", len(capped))
	}
}
