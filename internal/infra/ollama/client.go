package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"api/internal/domain"
	"api/internal/infra/pdf"
)

type ollamaClient struct {
	apiURL     string
	model      string
	httpClient *http.Client
}

// NewOllamaClient creates a new Ollama service adapter for generating embeddings.
func NewOllamaClient(apiURL, model string) domain.GeminiService {
	if apiURL == "" {
		apiURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &ollamaClient{
		apiURL: apiURL,
		model:  model,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Vectorizing large batches locally might take some time
		},
	}
}

func (c *ollamaClient) MatchResume(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error) {
	return nil, fmt.Errorf("ollama client does not support matching resumes (only Gemini/DeepSeek LLM services are supported for matching)")
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func (c *ollamaClient) GetEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	log.Printf("[OllamaClient] Generating embeddings for %d text items using model %s...", len(texts), c.model)

	reqPayload := ollamaEmbedRequest{
		Model: c.model,
		Input: texts,
	}

	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ollama embedding request: %w", err)
	}

	url := fmt.Sprintf("%s/api/embed", c.apiURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request to ollama embeddings: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[OllamaClient] Sending embedding request to %s (payload: %d bytes)...", url, len(jsonBytes))
	startHttp := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[OllamaClient] Embedding request failed after %s: %v", time.Since(startHttp), err)
		return nil, fmt.Errorf("http request to ollama embeddings failed: %w. Is Ollama running?", err)
	}
	defer resp.Body.Close()

	latency := time.Since(startHttp)
	log.Printf("[OllamaClient] Embedding response received in %s. Status code: %d", latency, resp.StatusCode)

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ollama embedding response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[OllamaClient] Error embedding body: %s", string(respBytes))
		return nil, fmt.Errorf("ollama embedding api returned status code %d: %s", resp.StatusCode, string(respBytes))
	}

	var embedRes ollamaEmbedResponse
	if err := json.Unmarshal(respBytes, &embedRes); err != nil {
		return nil, fmt.Errorf("failed to decode ollama embedding response: %w. Response: %s", err, string(respBytes))
	}

	log.Printf("[OllamaClient] Successfully extracted %d embeddings.", len(embedRes.Embeddings))
	return embedRes.Embeddings, nil
}

func (c *ollamaClient) ExtractText(ctx context.Context, fileB64 string, fileMime string) (string, error) {
	return pdf.ExtractTextFromBase64(fileB64, fileMime)
}
