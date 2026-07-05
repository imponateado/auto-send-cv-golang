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

func (s *fallbackLLMService) GetEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	log.Println("[FallbackLLM] Getting embeddings from primary LLM provider...")
	res, err := s.primary.GetEmbeddings(ctx, texts)
	if err == nil {
		log.Println("[FallbackLLM] Primary LLM provider embeddings call succeeded.")
		return res, nil
	}

	log.Printf("[FallbackLLM] Primary LLM provider embeddings call failed: %v. Falling back to secondary LLM provider...", err)
	res, err = s.secondary.GetEmbeddings(ctx, texts)
	if err == nil {
		log.Println("[FallbackLLM] Secondary LLM provider embeddings call succeeded.")
		return res, nil
	}
	return nil, err
}

func (s *fallbackLLMService) ExtractText(ctx context.Context, fileB64 string, fileMime string) (string, error) {
	log.Println("[FallbackLLM] Extracting text from document using primary provider...")
	txt, err := s.primary.ExtractText(ctx, fileB64, fileMime)
	if err == nil {
		log.Println("[FallbackLLM] Primary provider text extraction succeeded.")
		return txt, nil
	}

	log.Printf("[FallbackLLM] Primary provider text extraction failed: %v. Falling back to secondary provider...", err)
	txt, err = s.secondary.ExtractText(ctx, fileB64, fileMime)
	if err == nil {
		log.Println("[FallbackLLM] Secondary provider text extraction succeeded.")
		return txt, nil
	}
	return "", err
}
