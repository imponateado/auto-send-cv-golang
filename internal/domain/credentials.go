package domain

import "context"

type CredentialsRepository interface {
	SaveEmailCredentials(ctx context.Context, creds *EmailCredentials) error
	GetEmailCredentials(ctx context.Context, email string) (*EmailCredentials, error)
	DeleteEmailCredentials(ctx context.Context, email string) error
}
