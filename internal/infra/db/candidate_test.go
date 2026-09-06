package db

import (
	"context"
	"path/filepath"
	"testing"

	"api/internal/domain"
)

func TestCandidateRepo(t *testing.T) {
	sqlDB, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer sqlDB.Close()

	repo, err := NewCandidateRepo(sqlDB)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	ctx := context.Background()

	t.Run("saves and reads back the resume", func(t *testing.T) {
		p := &domain.CandidateProfile{
			ID: "eu@example.com", Email: "eu@example.com", Phone: "5511999999999",
			ResumeB64: "JVBERi0=", ResumeMime: "application/pdf", Active: true,
		}
		if err := repo.SaveProfile(ctx, p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := repo.ListProfiles(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 profile, got %d", len(got))
		}
		// O currículo precisa voltar íntegro: é ele que vira anexo do e-mail.
		if got[0].ResumeB64 != "JVBERi0=" || !got[0].Active || got[0].Phone != "5511999999999" {
			t.Errorf("perfil voltou diferente do salvo: %+v", got[0])
		}
	})

	t.Run("saving the same id replaces the profile", func(t *testing.T) {
		p := &domain.CandidateProfile{
			ID: "eu@example.com", Email: "eu@example.com",
			ResumeB64: "bm92bw==", ResumeMime: "application/pdf", Active: false,
		}
		if err := repo.SaveProfile(ctx, p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, _ := repo.ListProfiles(ctx)
		if len(got) != 1 || got[0].ResumeB64 != "bm92bw==" || got[0].Active {
			t.Errorf("esperava o perfil substituído, got: %+v", got)
		}
	})

	t.Run("applied vacancies are per candidate and idempotent", func(t *testing.T) {
		if err := repo.MarkApplied(ctx, "eu@example.com", []string{"vac_a", "vac_b"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Repetir o mesmo par não pode estourar a PK composta.
		if err := repo.MarkApplied(ctx, "eu@example.com", []string{"vac_a"}); err != nil {
			t.Fatalf("re-marcar a mesma vaga devia ser inofensivo: %v", err)
		}

		mine, err := repo.AppliedVacancyIDs(ctx, "eu@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mine) != 2 || !mine["vac_a"] || !mine["vac_b"] {
			t.Errorf("unexpected applied set: %v", mine)
		}

		other, _ := repo.AppliedVacancyIDs(ctx, "outro@example.com")
		if len(other) != 0 {
			t.Errorf("o histórico de um candidato não pode vazar para outro: %v", other)
		}
	})

	t.Run("deleting the profile keeps the applied history", func(t *testing.T) {
		if err := repo.DeleteProfile(ctx, "eu@example.com"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, _ := repo.ListProfiles(ctx)
		if len(got) != 0 {
			t.Errorf("expected no profiles, got %d", len(got))
		}
		// Recadastrar o currículo não pode reabrir as vagas já candidatadas.
		applied, _ := repo.AppliedVacancyIDs(ctx, "eu@example.com")
		if len(applied) != 2 {
			t.Errorf("o histórico devia sobreviver à remoção do perfil, got: %v", applied)
		}
	})
}
