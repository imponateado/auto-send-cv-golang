package service

import (
	"strings"
	"testing"
	"time"
)

func TestApplicationMessage(t *testing.T) {
	sp, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("fuso de teste indisponível: %v", err)
	}

	at := func(hour int) time.Time {
		return time.Date(2026, 8, 30, hour, 0, 0, 0, sp)
	}

	cases := []struct {
		name string
		when time.Time
		role string
		want string
	}{
		{"madrugada", at(3), "Dev Backend", "Boa noite,\n\nCandidato-me à vaga de Dev Backend. Currículo em anexo para avaliação."},
		{"manhã", at(9), "Dev Backend", "Bom dia,\n\nCandidato-me à vaga de Dev Backend. Currículo em anexo para avaliação."},
		{"meio-dia vira tarde", at(12), "Dev Backend", "Boa tarde,\n\nCandidato-me à vaga de Dev Backend. Currículo em anexo para avaliação."},
		{"18h vira noite", at(18), "Dev Backend", "Boa noite,\n\nCandidato-me à vaga de Dev Backend. Currículo em anexo para avaliação."},
		{"sem cargo", at(9), "", "Bom dia,\n\nCandidato-me à vaga divulgada. Currículo em anexo para avaliação."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := applicationMessage(tc.role, tc.when); got != tc.want {
				t.Errorf("applicationMessage()\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}

	// A saudação segue o fuso configurado, não o do relógio que chega: 23h em SP
	// é 02h do dia seguinte em UTC, e ainda assim é "Boa noite".
	if got := applicationMessage("", at(23).UTC()); !strings.HasPrefix(got, "Boa noite,") {
		t.Errorf("saudação ignorou o fuso configurado: %q", got)
	}
}

func TestSanitizeRoleUmaLinhaECurto(t *testing.T) {
	if got := sanitizeRole("  Dev\nBackend\tJava  "); got != "Dev Backend Java" {
		t.Errorf("sanitizeRole() = %q", got)
	}
	if got := sanitizeRole(strings.Repeat("ç", 200)); len([]rune(got)) != 80 {
		t.Errorf("sanitizeRole() devolveu %d runas, esperado 80", len([]rune(got)))
	}
	if got := applicationSubject(""); got != "Candidatura" {
		t.Errorf("applicationSubject(\"\") = %q", got)
	}
}
