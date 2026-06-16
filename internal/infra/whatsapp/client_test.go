package whatsapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWhatsAppClient_SendMessage(t *testing.T) {
	t.Run("Misconfiguration check", func(t *testing.T) {
		cli := NewWhatsAppClient("", "")
		err := cli.SendMessage(context.Background(), "5511999999999", "hello")
		if err == nil {
			t.Fatal("expected error due to missing token/id, got nil")
		}
	})

	t.Run("Successful send message", func(t *testing.T) {
		token := "my-auth-token"
		phoneID := "my-phone-id"
		to := "5511999999999"
		message := "hello user!"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Validate URL path
			expectedPath := "/" + phoneID + "/messages"
			if r.URL.Path != expectedPath {
				t.Errorf("expected path %s, got: %s", expectedPath, r.URL.Path)
			}

			// Validate Headers
			if auth := r.Header.Get("Authorization"); auth != "Bearer "+token {
				t.Errorf("expected auth Bearer %s, got: %s", token, auth)
			}
			if cType := r.Header.Get("Content-Type"); cType != "application/json" {
				t.Errorf("expected content type application/json, got: %s", cType)
			}

			// Validate Payload
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}

			var payload textMessagePayload
			if err := json.Unmarshal(bodyBytes, &payload); err != nil {
				t.Fatalf("failed to unmarshal request body: %v", err)
			}

			if payload.MessagingProduct != "whatsapp" || payload.To != to || payload.Text.Body != message {
				t.Errorf("unexpected request payload: %+v", payload)
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"messaging_product": "whatsapp", "contacts": [{"input": "5511999999999", "wa_id": "5511999999999"}], "messages": [{"id": "wamid.ID"}]}`))
		}))
		defer server.Close()

		// Instantiating internal client pointing to our test server
		cli := &whatsAppClient{
			token:         token,
			phoneNumberID: phoneID,
			apiURL:        server.URL,
			httpClient:    server.Client(),
		}

		err := cli.SendMessage(context.Background(), to, message)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("API returns HTTP Error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": {"message": "Invalid parameter"}}`))
		}))
		defer server.Close()

		cli := &whatsAppClient{
			token:         "tok",
			phoneNumberID: "id",
			apiURL:        server.URL,
			httpClient:    server.Client(),
		}

		err := cli.SendMessage(context.Background(), "123", "msg")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !strings.Contains(err.Error(), "status code 400") {
			t.Errorf("expected error message to contain status code, got: %v", err)
		}
	})
}
