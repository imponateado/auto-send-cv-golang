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
	token         string
	phoneNumberID string
	apiURL        string // e.g. "https://graph.facebook.com/v19.0"
	httpClient    *http.Client
}

// NewWhatsAppClient creates a new instance of domain.WhatsAppService pointing to Meta Cloud API.
func NewWhatsAppClient(token, phoneNumberID string) domain.WhatsAppService {
	return &whatsAppClient{
		token:         token,
		phoneNumberID: phoneNumberID,
		apiURL:        "https://graph.facebook.com/v19.0",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type textMessagePayload struct {
	MessagingProduct string   `json:"messaging_product"`
	RecipientType    string   `json:"recipient_type"`
	To               string   `json:"to"`
	Type             string   `json:"type"`
	Text             textBody `json:"text"`
}

type textBody struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

// SendMessage sends a text message via Meta WhatsApp Cloud API.
func (c *whatsAppClient) SendMessage(ctx context.Context, to string, message string) error {
	if c.token == "" || c.phoneNumberID == "" {
		return fmt.Errorf("whatsapp client is misconfigured: token and phone_number_id are required")
	}

	url := fmt.Sprintf("%s/%s/messages", c.apiURL, c.phoneNumberID)

	payload := textMessagePayload{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "text",
		Text: textBody{
			PreviewURL: false,
			Body:       message,
		},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal whatsapp payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

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
