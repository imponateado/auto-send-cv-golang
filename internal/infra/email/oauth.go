package email

import (
	"api/internal/domain"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type oauthEmailService struct {
	creds  *domain.EmailCredentials
	client *http.Client
}

func NewOAuthEmailService(creds *domain.EmailCredentials) domain.EmailService {
	return &oauthEmailService{
		creds: creds,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func (s *oauthEmailService) refreshGoogleToken(ctx context.Context) (string, error) {
	data := url.Values{}
	data.Set("client_id", s.creds.ClientID)
	data.Set("client_secret", s.creds.ClientSecret)
	data.Set("refresh_token", s.creds.RefreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to refresh google token, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}

	return tr.AccessToken, nil
}

func (s *oauthEmailService) refreshMicrosoftToken(ctx context.Context) (string, error) {
	data := url.Values{}
	data.Set("client_id", s.creds.ClientID)
	data.Set("client_secret", s.creds.ClientSecret)
	data.Set("refresh_token", s.creds.RefreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://login.microsoftonline.com/common/oauth2/v2.0/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to refresh microsoft token, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}

	return tr.AccessToken, nil
}

func (s *oauthEmailService) SendEmail(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error {
	var accessToken string
	var err error

	prov := strings.ToLower(s.creds.Provider)
	if prov == "google" {
		accessToken, err = s.refreshGoogleToken(ctx)
	} else if prov == "microsoft" {
		accessToken, err = s.refreshMicrosoftToken(ctx)
	} else {
		return fmt.Errorf("unsupported email oauth provider: %s", s.creds.Provider)
	}

	if err != nil {
		return fmt.Errorf("oauth token refresh failed: %w", err)
	}

	if prov == "google" {
		return s.sendGoogleEmail(ctx, accessToken, to, subject, body, attachmentB64, attachmentName)
	}
	return s.sendMicrosoftEmail(ctx, accessToken, to, subject, body, attachmentB64, attachmentName)
}

func (s *oauthEmailService) sendGoogleEmail(ctx context.Context, token, to, subject, body, attachmentB64, attachmentName string) error {
	rawMsg, err := buildRawRFC822(s.creds.Email, to, subject, body, attachmentB64, attachmentName)
	if err != nil {
		return fmt.Errorf("failed to build RFC822 message: %w", err)
	}

	encodedMsg := base64.URLEncoding.EncodeToString(rawMsg)

	payload := map[string]string{
		"raw": encodedMsg,
	}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://gmail.googleapis.com/gmail/v1/users/me/messages/send", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gmail api returned status: %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

type graphRecipient struct {
	EmailAddress struct {
		Address string `json:"address"`
	} `json:"emailAddress"`
}

type graphAttachment struct {
	ODataType    string `json:"@odata.type"`
	Name         string `json:"name"`
	ContentType  string `json:"contentType"`
	ContentBytes string `json:"contentBytes"`
}

type graphMessage struct {
	Subject      string            `json:"subject"`
	Body         map[string]string `json:"body"`
	ToRecipients []graphRecipient  `json:"toRecipients"`
	Attachments  []graphAttachment `json:"attachments,omitempty"`
}

type graphPayload struct {
	Message         graphMessage `json:"message"`
	SaveToSentItems string       `json:"saveToSentItems"`
}

func (s *oauthEmailService) sendMicrosoftEmail(ctx context.Context, token, to, subject, body, attachmentB64, attachmentName string) error {
	base64Data := attachmentB64
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}
	base64Data = strings.Join(strings.Fields(base64Data), "")

	if attachmentName == "" {
		attachmentName = "curriculo.pdf"
	}

	recipient := graphRecipient{}
	recipient.EmailAddress.Address = to

	msg := graphMessage{
		Subject: subject,
		Body: map[string]string{
			"contentType": "HTML",
			"content":     fmt.Sprintf("<p>%s</p>", strings.ReplaceAll(body, "\n", "<br/>")),
		},
		ToRecipients: []graphRecipient{recipient},
	}

	if attachmentB64 != "" {
		msg.Attachments = []graphAttachment{
			{
				ODataType:    "#microsoft.graph.fileAttachment",
				Name:         attachmentName,
				ContentType:  "application/pdf",
				ContentBytes: base64Data,
			},
		}
	}

	payload := graphPayload{
		Message:         msg,
		SaveToSentItems: "true",
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://graph.microsoft.com/v1.0/me/sendMail", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("microsoft graph api returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func buildRawRFC822(from, to, subject, body, attachmentB64, attachmentName string) ([]byte, error) {
	var msg []byte
	if attachmentB64 != "" {
		boundary := "my-multipart-boundary-12345"
		base64Data := attachmentB64
		if idx := strings.Index(base64Data, ","); idx != -1 {
			base64Data = base64Data[idx+1:]
		}
		base64Data = strings.Join(strings.Fields(base64Data), "")
		if attachmentName == "" {
			attachmentName = "curriculo.pdf"
		}
		header := fmt.Sprintf("From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: multipart/mixed; boundary=%s\r\n"+
			"\r\n"+
			"--%s\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n"+
			"Content-Transfer-Encoding: 7bit\r\n"+
			"\r\n"+
			"%s\r\n"+
			"\r\n"+
			"--%s\r\n"+
			"Content-Type: application/octet-stream; name=\"%s\"\r\n"+
			"Content-Transfer-Encoding: base64\r\n"+
			"Content-Disposition: attachment; filename=\"%s\"\r\n"+
			"\r\n", from, to, subject, boundary, boundary, body, boundary, attachmentName, attachmentName)
		var buf bytes.Buffer
		buf.WriteString(header)
		for i := 0; i < len(base64Data); i += 76 {
			end := i + 76
			if end > len(base64Data) {
				end = len(base64Data)
			}
			buf.WriteString(base64Data[i:end])
			buf.WriteString("\r\n")
		}
		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
		msg = buf.Bytes()
	} else {
		msg = []byte(fmt.Sprintf("From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n"+
			"\r\n"+
			"%s\r\n", from, to, subject, body))
	}
	return msg, nil
}
