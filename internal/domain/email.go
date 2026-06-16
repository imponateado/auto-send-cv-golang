package domain

import "context"

// EmailService defines the domain contract for sending emails.
type EmailService interface {
	SendEmail(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error
}
