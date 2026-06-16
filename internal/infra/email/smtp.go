package email

import (
	"context"
	"fmt"
	"net/smtp"

	"api/internal/domain"
)

type smtpSender struct {
	host     string
	port     string
	username string
	password string
	sender   string
}

// NewSMTPSender creates a new instance of domain.EmailService using SMTP.
func NewSMTPSender(host, port, username, password, sender string) domain.EmailService {
	return &smtpSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		sender:   sender,
	}
}

// SendEmail sends an email using Go's native net/smtp package.
func (s *smtpSender) SendEmail(ctx context.Context, to string, subject string, body string) error {
	// Respect context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if s.host == "" || s.port == "" {
		return fmt.Errorf("smtp sender is misconfigured: host and port are required")
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	addr := fmt.Sprintf("%s:%s", s.host, s.port)

	// Format RFC 822 email message
	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: %s\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"\r\n"+
		"%s\r\n", to, subject, body))

	err := smtp.SendMail(addr, auth, s.sender, []string{to}, msg)
	if err != nil {
		return fmt.Errorf("smtp send mail failed: %w", err)
	}

	return nil
}
