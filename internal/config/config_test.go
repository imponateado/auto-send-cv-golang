package config

import (
	"os"
	"strings"
	"testing"
)

func TestParseEnv(t *testing.T) {
	defer func() {
		os.Unsetenv("TEST_PORT")
		os.Unsetenv("TEST_ENV")
		os.Unsetenv("TEST_QUOTED")
		os.Unsetenv("TEST_SINGLE_QUOTED")
		os.Unsetenv("TEST_PRESERVED")
	}()

	os.Setenv("TEST_PRESERVED", "system-value")

	envData := `
# This is a comment
TEST_PORT = 9000
TEST_ENV=staging

# Quoted values
TEST_QUOTED = "double-quoted"
TEST_SINGLE_QUOTED = 'single-quoted'

# Precedence check
TEST_PRESERVED = env-file-value
`

	reader := strings.NewReader(envData)
	err := ParseEnv(reader)
	if err != nil {
		t.Fatalf("unexpected error parsing env: %v", err)
	}

	if val := os.Getenv("TEST_PORT"); val != "9000" {
		t.Errorf("expected TEST_PORT to be 9000, got: %s", val)
	}

	if val := os.Getenv("TEST_ENV"); val != "staging" {
		t.Errorf("expected TEST_ENV to be staging, got: %s", val)
	}

	if val := os.Getenv("TEST_QUOTED"); val != "double-quoted" {
		t.Errorf("expected TEST_QUOTED to be double-quoted, got: %s", val)
	}

	if val := os.Getenv("TEST_SINGLE_QUOTED"); val != "single-quoted" {
		t.Errorf("expected TEST_SINGLE_QUOTED to be single-quoted, got: %s", val)
	}

	if val := os.Getenv("TEST_PRESERVED"); val != "system-value" {
		t.Errorf("expected TEST_PRESERVED to remain system-value (precedence), got: %s", val)
	}
}

// O prefixo de tarefa do modelo de embedding termina em espaço
// ("search_document: "), e ParseEnv faz TrimSpace na linha. Sem aspas o espaço
// morreria e o prefixo sairia colado no texto da vaga.
func TestParseEnvKeepsTrailingSpaceInsideQuotes(t *testing.T) {
	t.Setenv("EMBEDDING_DOC_PREFIX", "")
	os.Unsetenv("EMBEDDING_DOC_PREFIX")

	if err := ParseEnv(strings.NewReader(`EMBEDDING_DOC_PREFIX="search_document: "`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := os.Getenv("EMBEDDING_DOC_PREFIX"); got != "search_document: " {
		t.Errorf("aspas devem preservar o espaço final: want %q, got %q", "search_document: ", got)
	}
}
