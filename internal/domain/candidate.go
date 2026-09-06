package domain

import (
	"context"
	"time"
)

// CandidateProfile é o currículo guardado de um candidato, com o canal por onde
// as candidaturas dele saem. É o que permite ao flush rodar o match sozinho, sem
// ninguém reenviar o PDF.
type CandidateProfile struct {
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
	// ResumeB64 nunca sai em JSON: é o PDF inteiro, e as rotas de listagem não
	// têm por que devolver megabytes de anexo.
	ResumeB64  string    `json:"-"`
	ResumeMime string    `json:"resume_mime"`
	Active     bool      `json:"active"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ProfileID é a identidade do candidato: o e-mail quando existe, senão o
// telefone. Vale como chave primária do perfil e como chave do registro de
// candidaturas já feitas, para que o match manual e o automático compartilhem o
// mesmo histórico.
func ProfileID(email, phone string) string {
	if email != "" {
		return email
	}
	return phone
}

type CandidateRepository interface {
	SaveProfile(ctx context.Context, p *CandidateProfile) error
	ListProfiles(ctx context.Context) ([]CandidateProfile, error)
	DeleteProfile(ctx context.Context, id string) error
	// AppliedVacancyIDs devolve as vagas às quais o candidato já se candidatou,
	// para não repetir o disparo.
	AppliedVacancyIDs(ctx context.Context, candidateID string) (map[string]bool, error)
	MarkApplied(ctx context.Context, candidateID string, vacancyIDs []string) error
}
