package domain

import "context"

type EmailCredentials struct {
	Email        string `json:"email"`
	Provider     string `json:"provider"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// EmailService defines the domain contract for sending emails.
type EmailService interface {
	SendEmail(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error
}
