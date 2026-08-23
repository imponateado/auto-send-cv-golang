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

func TestDeleteVacancy(t *testing.T) {
	store, err := NewChromemStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	ctx := context.Background()

	vacancies := []string{"vaga a", "vaga b"}
	embeddings := [][]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
	}
	if err := store.AddVacancies(ctx, vacancies, embeddings); err != nil {
		t.Fatalf("failed to seed vacancies: %v", err)
	}

	before, err := store.ListVacancies(ctx, []float32{0.5, 0.5, 0, 0})
	if err != nil {
		t.Fatalf("unexpected error listing before delete: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("expected 2 vacancies before delete, got: %d", len(before))
	}

	var deletedID string
	for _, v := range before {
		if v.Text == "vaga a" {
			deletedID = v.ID
		}
	}
	if deletedID == "" {
		t.Fatal("could not find ID for 'vaga a'")
	}

	if err := store.DeleteVacancy(ctx, deletedID); err != nil {
		t.Fatalf("unexpected error deleting vacancy: %v", err)
	}

	after, err := store.ListVacancies(ctx, []float32{0.5, 0.5, 0, 0})
	if err != nil {
		t.Fatalf("unexpected error listing after delete: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("expected 1 vacancy after delete, got: %d", len(after))
	}
	if after[0].Text != "vaga b" {
		t.Errorf("expected remaining vacancy to be 'vaga b', got: %q", after[0].Text)
	}
}
