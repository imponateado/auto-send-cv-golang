package db

import (
	"context"
	"path/filepath"
	"testing"

	"api/internal/domain"
)

func TestListEmailCredentials(t *testing.T) {
	sqlDB, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer sqlDB.Close()

	repo, err := NewSQLRepo(sqlDB)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	ctx := context.Background()

	t.Run("empty table returns empty slice", func(t *testing.T) {
		got, err := repo.ListEmailCredentials(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected 0 credentials, got: %d", len(got))
		}
	})

	creds := []*domain.EmailCredentials{
		{Email: "a@example.com", Provider: "google", RefreshToken: "ref-a", ClientID: "id-a", ClientSecret: "secret-a"},
		{Email: "b@example.com", Provider: "microsoft", RefreshToken: "ref-b", ClientID: "id-b", ClientSecret: "secret-b"},
	}
	for _, c := range creds {
		if err := repo.SaveEmailCredentials(ctx, c); err != nil {
			t.Fatalf("failed to seed credentials: %v", err)
		}
	}

	t.Run("returns every saved credential", func(t *testing.T) {
		got, err := repo.ListEmailCredentials(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(creds) {
			t.Fatalf("expected %d credentials, got: %d", len(creds), len(got))
		}

		byEmail := make(map[string]domain.EmailCredentials, len(got))
		for _, c := range got {
			byEmail[c.Email] = c
		}
		for _, want := range creds {
			got, ok := byEmail[want.Email]
			if !ok {
				t.Errorf("expected credential for %q to be present", want.Email)
				continue
			}
			if got.Provider != want.Provider || got.RefreshToken != want.RefreshToken {
				t.Errorf("unexpected credential for %q: %+v", want.Email, got)
			}
		}
	})
}
