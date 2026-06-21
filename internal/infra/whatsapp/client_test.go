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
		cli := NewWhatsAppClient("", "", "")
		err := cli.SendMessage(context.Background(), "5511999999999", "hello")
		if err == nil {
			t.Fatal("expected error due to missing instance_id/token, got nil")
		}
	})

	t.Run("Successful send message", func(t *testing.T) {
		instanceID := "my-instance-id"
		token := "my-token"
		clientToken := "my-client-token"
		to := "5511999999999"
		message := "hello user!"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Validate URL path
			expectedPath := "/instances/" + instanceID + "/token/" + token + "/send-text"
			if r.URL.Path != expectedPath {
				t.Errorf("expected path %s, got: %s", expectedPath, r.URL.Path)
			}

			// Validate Headers
			if clientTokenHeader := r.Header.Get("Client-Token"); clientTokenHeader != clientToken {
				t.Errorf("expected Client-Token %s, got: %s", clientToken, clientTokenHeader)
			}
			if cType := r.Header.Get("Content-Type"); cType != "application/json" {
				t.Errorf("expected content type application/json, got: %s", cType)
			}

			// Validate Payload
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}

			var payload zapiTextMessagePayload
			if err := json.Unmarshal(bodyBytes, &payload); err != nil {
				t.Fatalf("failed to unmarshal request body: %v", err)
			}

			if payload.Phone != to || payload.Message != message {
				t.Errorf("unexpected request payload: %+v", payload)
			}

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"zaid": "some-id"}`))
		}))
		defer server.Close()

		// Instantiating internal client pointing to our test server
		cli := &whatsAppClient{
			instanceID:  instanceID,
			token:       token,
			clientToken: clientToken,
			apiURL:      server.URL,
			httpClient:  server.Client(),
		}

		err := cli.SendMessage(context.Background(), to, message)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("API returns HTTP Error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "Invalid parameter"}`))
		}))
		defer server.Close()

		cli := &whatsAppClient{
			instanceID:  "inst",
			token:       "tok",
			clientToken: "cli",
			apiURL:      server.URL,
			httpClient:  server.Client(),
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
