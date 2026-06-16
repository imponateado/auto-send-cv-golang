package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"api/internal/domain"
)

type orchestrator struct {
	processor       domain.PayloadProcessor
	geminiService   domain.GeminiService
	emailService    domain.EmailService
	whatsAppService domain.WhatsAppService
}

// NewOrchestrator creates a new application use case orchestrator.
func NewOrchestrator(
	processor domain.PayloadProcessor,
	geminiService domain.GeminiService,
	emailService domain.EmailService,
	whatsAppService domain.WhatsAppService,
) domain.Orchestrator {
	return &orchestrator{
		processor:       processor,
		geminiService:   geminiService,
		emailService:    emailService,
		whatsAppService: whatsAppService,
	}
}

func (o *orchestrator) RunMatchAndDispatch(ctx context.Context, req *domain.ProcessRequest) (*domain.ProcessResult, error) {
	start := time.Now()

	// 1. Process the payload (Split vacancies and check resume metadata)
	res, err := o.processor.Process(ctx, req)
	if err != nil {
		return nil, err
	}

	// 2. We can only perform matching if both the resume and the vacancies list are provided
	if req.FileBase64 == "" || len(res.Items) == 0 {
		// Nothing to match, just return the text processing statistics
		res.DurationMs = time.Since(start).Milliseconds()
		return res, nil
	}

	// Get file mime type
	mimeType := "application/pdf"
	if res.File != nil && res.File.MimeType != "" {
		mimeType = res.File.MimeType
	}

	// 3. Request matching from Gemini
	log.Printf("[Orchestrator] Sending resume and %d vacancies to Gemini for matching...", len(res.Items))
	matchRes, err := o.geminiService.MatchResume(ctx, req.FileBase64, mimeType, res.Items)
	if err != nil {
		return nil, fmt.Errorf("gemini matching failed: %w", err)
	}
	log.Printf("[Orchestrator] Gemini matched %d vacancies.", len(matchRes.Matches))

	// 4. Dispatch applications based on match contacts
	for _, match := range matchRes.Matches {
		if match.Index < 0 || match.Index >= len(res.Items) {
			log.Printf("[Orchestrator] Warning: Gemini returned out-of-bounds index %d. Skipping.", match.Index)
			continue
		}

		switch match.ContactType {
		case "email":
			log.Printf("[Orchestrator] Sending email application for match index %d to %s...", match.Index, match.ContactTarget)
			subject := "Candidatura - Processamento Automático"
			body := fmt.Sprintf("Olá,\n\nEstou me candidatando à vaga de emprego número %d.\n\nMotivo da compatibilidade:\n%s\n\nEm anexo, envio meu currículo para avaliação.\n\nAtenciosamente,\nCandidato", match.Index+1, match.Reason)

			err := o.emailService.SendEmail(ctx, match.ContactTarget, subject, body, req.FileBase64, "curriculo.pdf")
			if err != nil {
				log.Printf("[Orchestrator] Error sending email to %s: %v", match.ContactTarget, err)
			} else {
				log.Printf("[Orchestrator] Email sent successfully to %s", match.ContactTarget)
			}

		case "whatsapp":
			log.Printf("[Orchestrator] Sending WhatsApp application for match index %d to %s...", match.Index, match.ContactTarget)
			body := fmt.Sprintf("Olá!\n\nEstou me candidatando à sua vaga de emprego.\n\n*Motivo do Match:*\n%s\n\nEnviei meu currículo por e-mail ou no formato correspondente para análise.", match.Reason)

			err := o.whatsAppService.SendMessage(ctx, match.ContactTarget, body)
			if err != nil {
				log.Printf("[Orchestrator] Error sending WhatsApp message to %s: %v", match.ContactTarget, err)
			} else {
				log.Printf("[Orchestrator] WhatsApp message sent successfully to %s", match.ContactTarget)
			}

		default:
			log.Printf("[Orchestrator] Unknown contact type '%s' returned by Gemini. Skipping.", match.ContactType)
		}
	}

	// 5. Populate final match results and duration
	res.Matches = matchRes.Matches
	res.DurationMs = time.Since(start).Milliseconds()

	return res, nil
}
