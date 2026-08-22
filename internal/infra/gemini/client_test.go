package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiClient_MatchResume(t *testing.T) {
	t.Run("Misconfiguration check", func(t *testing.T) {
		cli := NewGeminiClient("", "")
		_, err := cli.MatchResume(context.Background(), "b64", "application/pdf", []string{"vaga"})
		if err == nil {
			t.Fatal("expected error due to missing api key, got nil")
		}
	})

	t.Run("Successful match", func(t *testing.T) {
		apiKey := "test-key"
		vacancies := []string{"Vaga Go", "Vaga Python"}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key := r.URL.Query().Get("key"); key != apiKey {
				t.Errorf("expected key query param %s, got: %s", apiKey, key)
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"candidates": [
					{
						"content": {
							"parts": [
								{
									"text": "{\n  \"matches\": [\n    {\n      \"index\": 0,\n      \"reason\": \"Candidato experiente em Go\",\n      \"contact_type\": \"email\",\n      \"contact_target\": \"rh@empresa.com\"\n    }\n  ]\n}"
								}
							]
						}
					}
				]
			}`))
		}))
		defer server.Close()

		cli := &geminiClient{
			apiKey:     apiKey,
			model:      "gemini-2.5-flash",
			apiURL:     server.URL,
			httpClient: server.Client(),
		}

		res, err := cli.MatchResume(context.Background(), "aGVsbG8=", "application/pdf", vacancies)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if len(res.Matches) != 1 {
			t.Fatalf("expected 1 match, got: %d", len(res.Matches))
		}

		match := res.Matches[0]
		if match.Index != 0 || match.ContactType != "email" || match.ContactTarget != "rh@empresa.com" {
			t.Errorf("unexpected match results: %+v", match)
		}
	})
}
