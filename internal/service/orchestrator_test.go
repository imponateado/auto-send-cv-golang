package service

import (
	"context"
	"testing"

	"api/internal/domain"
)

// Mock implementations
type mockProcessor struct {
	processFn func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error)
}

func (m *mockProcessor) Process(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
	return m.processFn(ctx, req)
}

type mockGemini struct {
	matchFn func(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error)
}

func (m *mockGemini) MatchResume(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error) {
	return m.matchFn(ctx, fileB64, fileMime, vacancies)
}

type mockEmail struct {
	sendFn func(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error
}

func (m *mockEmail) SendEmail(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error {
	return m.sendFn(ctx, to, subject, body, attachmentB64, attachmentName)
}

type mockWhatsApp struct {
	sendFn func(ctx context.Context, to string, message string) error
}

func (m *mockWhatsApp) SendMessage(ctx context.Context, to string, message string) error {
	return m.sendFn(ctx, to, message)
}

func TestOrchestrator_RunMatchAndDispatch(t *testing.T) {
	t.Run("No matching performed when file or vacancies missing", func(t *testing.T) {
		mProc := &mockProcessor{
			processFn: func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
				return &domain.ProcessResult{
					BytesProcessed: 0,
					ItemsProcessed: 0,
					Items:          []string{},
					Status:         "success",
				}, nil
			},
		}

		orch := NewOrchestrator(mProc, &mockGemini{}, &mockEmail{}, &mockWhatsApp{})
		req := &domain.ProcessRequest{
			Content:   "",
			Delimiter: "\n",
		}

		res, err := orch.RunMatchAndDispatch(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(res.Matches) != 0 {
			t.Errorf("expected 0 matches, got: %d", len(res.Matches))
		}
	})

	t.Run("Full matching and dispatch flow", func(t *testing.T) {
		mProc := &mockProcessor{
			processFn: func(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
				return &domain.ProcessResult{
					BytesProcessed: 20,
					ItemsProcessed: 2,
					Items:          []string{"vaga 1", "vaga 2"},
					File: &domain.FileMetadata{
						SizeInBytes: 100,
						MimeType:    "application/pdf",
						Status:      "decoded",
					},
					Status: "success",
				}, nil
			},
		}

		mGemini := &mockGemini{
			matchFn: func(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error) {
				return &domain.MatchResult{
					Matches: []domain.Match{
						{Index: 0, Reason: "perfeito", ContactType: "email", ContactTarget: "rh@empresa.com"},
						{Index: 1, Reason: "bom", ContactType: "whatsapp", ContactTarget: "5511999999999"},
					},
				}, nil
			},
		}

		emailCalls := 0
		mEmail := &mockEmail{
			sendFn: func(ctx context.Context, to string, subject string, body string, attachmentB64 string, attachmentName string) error {
				emailCalls++
				if to != "rh@empresa.com" || attachmentB64 != "aGVsbG8=" {
					t.Errorf("unexpected email call parameters: to=%s, b64=%s", to, attachmentB64)
				}
				return nil
			},
		}

		waCalls := 0
		mWhatsApp := &mockWhatsApp{
			sendFn: func(ctx context.Context, to string, message string) error {
				waCalls++
				if to != "5511999999999" {
					t.Errorf("unexpected whatsapp call parameters: to=%s", to)
				}
				return nil
			},
		}

		orch := NewOrchestrator(mProc, mGemini, mEmail, mWhatsApp)
		req := &domain.ProcessRequest{
			Content:    "vaga1\nvaga2",
			Delimiter:  "\n",
			FileBase64: "aGVsbG8=",
		}

		res, err := orch.RunMatchAndDispatch(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(res.Matches) != 2 {
			t.Errorf("expected 2 matches, got: %d", len(res.Matches))
		}

		if emailCalls != 1 {
			t.Errorf("expected 1 email call, got: %d", emailCalls)
		}

		if waCalls != 1 {
			t.Errorf("expected 1 whatsapp call, got: %d", waCalls)
		}
	})
}
