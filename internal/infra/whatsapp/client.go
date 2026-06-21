package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"api/internal/domain"
)

type whatsAppClient struct {
	instanceID  string
	token       string
	clientToken string
	apiURL      string // e.g. "https://api.z-api.io"
	httpClient  *http.Client
}

// NewWhatsAppClient creates a new instance of domain.WhatsAppService pointing to Z-API.
func NewWhatsAppClient(instanceID, token, clientToken string) domain.WhatsAppService {
	return &whatsAppClient{
		instanceID:  instanceID,
		token:       token,
		clientToken: clientToken,
		apiURL:      "https://api.z-api.io",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type zapiTextMessagePayload struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

// SendMessage sends a text message via Z-API.
func (c *whatsAppClient) SendMessage(ctx context.Context, to string, message string) error {
	if c.instanceID == "" || c.token == "" {
		return fmt.Errorf("whatsapp client is misconfigured: instance_id and token are required")
	}

	url := fmt.Sprintf("%s/instances/%s/token/%s/send-text", c.apiURL, c.instanceID, c.token)

	payload := zapiTextMessagePayload{
		Phone:   to,
		Message: message,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal whatsapp payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.clientToken != "" {
		req.Header.Set("Client-Token", c.clientToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request to whatsapp api failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp api returned status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
