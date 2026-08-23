package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api/internal/domain"
)

type mockCredsRepo struct {
	listFn   func(ctx context.Context) ([]domain.EmailCredentials, error)
	getFn    func(ctx context.Context, email string) (*domain.EmailCredentials, error)
	deleteFn func(ctx context.Context, email string) error
}

func (m *mockCredsRepo) SaveEmailCredentials(ctx context.Context, creds *domain.EmailCredentials) error {
	return nil
}

func (m *mockCredsRepo) GetEmailCredentials(ctx context.Context, email string) (*domain.EmailCredentials, error) {
	if m.getFn != nil {
		return m.getFn(ctx, email)
	}
	return nil, context.DeadlineExceeded
}

func (m *mockCredsRepo) DeleteEmailCredentials(ctx context.Context, email string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, email)
	}
	return nil
}

func (m *mockCredsRepo) ListEmailCredentials(ctx context.Context) ([]domain.EmailCredentials, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func TestCredentialsHandler_ListCredentials(t *testing.T) {
	repo := &mockCredsRepo{
		listFn: func(ctx context.Context) ([]domain.EmailCredentials, error) {
			return []domain.EmailCredentials{
				{Email: "a@example.com", Provider: "google", RefreshToken: "should-not-leak", ClientSecret: "should-not-leak"},
			}, nil
		},
	}
	h := NewCredentialsHandler(repo, nil)

	req := httptest.NewRequest("GET", "/api/v1/credentials", nil)
	w := httptest.NewRecorder()
	h.ListCredentials(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got: %d", resp.StatusCode)
	}

	body := w.Body.String()
	if strings.Contains(body, "should-not-leak") {
		t.Fatalf("response leaked a secret field: %s", body)
	}

	var res []map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(res) != 1 || res[0]["email"] != "a@example.com" || res[0]["provider"] != "google" {
		t.Errorf("unexpected response content: %v", res)
	}
}

func TestCredentialsHandler_GetCredentials(t *testing.T) {
	t.Run("Found", func(t *testing.T) {
		repo := &mockCredsRepo{
			getFn: func(ctx context.Context, email string) (*domain.EmailCredentials, error) {
				return &domain.EmailCredentials{Email: email, Provider: "microsoft", RefreshToken: "should-not-leak"}, nil
			},
		}
		h := NewCredentialsHandler(repo, nil)

		req := httptest.NewRequest("GET", "/api/v1/credentials/a@example.com", nil)
		req.SetPathValue("email", "a@example.com")
		w := httptest.NewRecorder()
		h.GetCredentials(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got: %d", resp.StatusCode)
		}
		if strings.Contains(w.Body.String(), "should-not-leak") {
			t.Fatalf("response leaked a secret field: %s", w.Body.String())
		}
	})

	t.Run("Not found returns 404", func(t *testing.T) {
		h := NewCredentialsHandler(&mockCredsRepo{}, nil)

		req := httptest.NewRequest("GET", "/api/v1/credentials/missing@example.com", nil)
		req.SetPathValue("email", "missing@example.com")
		w := httptest.NewRecorder()
		h.GetCredentials(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected status 404, got: %d", resp.StatusCode)
		}
	})
}
