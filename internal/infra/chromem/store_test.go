package chromem

import (
	"context"
	"testing"
)

func TestListVacancies(t *testing.T) {
	store, err := NewChromemStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	ctx := context.Background()

	t.Run("empty collection returns empty slice", func(t *testing.T) {
		got, err := store.ListVacancies(ctx, []float32{0.1, 0.2, 0.3, 0.4})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected 0 vacancies, got: %d", len(got))
		}
	})

	vacancies := []string{"vaga 1", "vaga 2", "vaga 3"}
	embeddings := [][]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
	}
	if err := store.AddVacancies(ctx, vacancies, embeddings); err != nil {
		t.Fatalf("failed to seed vacancies: %v", err)
	}

	t.Run("returns every stored vacancy regardless of similarity ranking", func(t *testing.T) {
		got, err := store.ListVacancies(ctx, []float32{0.25, 0.25, 0.25, 0.25})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(vacancies) {
			t.Fatalf("expected %d vacancies, got: %d", len(vacancies), len(got))
		}

		gotTexts := make(map[string]bool, len(got))
		for _, v := range got {
			gotTexts[v.Text] = true
		}
		for _, want := range vacancies {
			if !gotTexts[want] {
				t.Errorf("expected vacancy %q to be present in list, got: %v", want, got)
			}
		}
	})

	t.Run("dimension mismatch returns an error", func(t *testing.T) {
		_, err := store.ListVacancies(ctx, []float32{1, 2})
		if err == nil {
			t.Fatal("expected error when query embedding dimension doesn't match stored embeddings")
		}
	})
}
