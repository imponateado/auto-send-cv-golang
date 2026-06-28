package multillm

import (
	"context"
	"log"

	"api/internal/domain"
)

type fallbackLLMService struct {
	primary   domain.GeminiService
	secondary domain.GeminiService
}

// NewFallbackLLMService creates a new LLM matching service that tries a primary provider and falls back to a secondary if it fails.
func NewFallbackLLMService(primary, secondary domain.GeminiService) domain.GeminiService {
	return &fallbackLLMService{
		primary:   primary,
		secondary: secondary,
	}
}

func (s *fallbackLLMService) MatchResume(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error) {
	log.Println("[FallbackLLM] Attempting matching with primary LLM provider...")
	res, err := s.primary.MatchResume(ctx, fileB64, fileMime, vacancies)
	if err == nil {
		log.Println("[FallbackLLM] Primary LLM matched successfully.")
		return res, nil
	}

	log.Printf("[FallbackLLM] Primary LLM provider failed: %v. Falling back to secondary LLM provider...", err)
	res, err = s.secondary.MatchResume(ctx, fileB64, fileMime, vacancies)
	if err != nil {
		return nil, err
	}

	log.Println("[FallbackLLM] Secondary LLM matched successfully.")
	return res, nil
}
