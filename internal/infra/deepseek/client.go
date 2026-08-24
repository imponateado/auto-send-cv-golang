package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"api/internal/domain"
	"api/internal/infra/pdf"
)

type deepseekClient struct {
	apiKey     string
	model      string
	apiURL     string
	httpClient *http.Client
}

// NewDeepSeekClient returns a domain.GeminiService backed by a *deepseekClient
// using the native HTTP client.
func NewDeepSeekClient(apiKey, model string) domain.GeminiService {
	if model == "" {
		model = "deepseek-v4-flash"
	}
	return &deepseekClient{
		apiKey: apiKey,
		model:  model,
		apiURL: "https://api.deepseek.com",
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type deepseekRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type deepseekResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message chatMessage `json:"message"`
}

func (c *deepseekClient) MatchResume(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("deepseek client is misconfigured: api key is required")
	}

	resumeText, err := pdf.ExtractTextFromBase64(fileB64, fileMime)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text from resume: %w", err)
	}

	var builder strings.Builder
	builder.WriteString("Abaixo está o texto extraído do currículo de um candidato e uma lista de vagas de emprego.\n")
	builder.WriteString("Analise o currículo e compare-o com cada uma das vagas.\n")
	builder.WriteString("Retorne no formato JSON estruturado quais vagas dão match com o candidato.\n")
	builder.WriteString("Para cada match, identifique: o index da vaga na lista, o motivo, o canal de contato ('email' ou 'whatsapp') e o email ou telefone de destino indicado no texto da vaga.\n")
	builder.WriteString("Se o canal de contato for 'whatsapp', o contact_target deve conter APENAS dígitos, incluindo o código do país do Brasil (55) antes do DDD, sem parênteses, espaços ou hífens (ex: \"(61) 99205-5310\" vira \"5561992055310\").\n\n")
	builder.WriteString("A resposta DEVE ser um objeto JSON válido seguindo estritamente este formato:\n")
	builder.WriteString("{\n  \"matches\": [\n    {\n      \"index\": 0,\n      \"reason\": \"motivo detalhado em português\",\n      \"contact_type\": \"email\" ou \"whatsapp\",\n      \"contact_target\": \"email ou telefone (somente dígitos com código do país se whatsapp)\"\n    }\n  ]\n}\n\n")
	builder.WriteString("Currículo do Candidato:\n")
	builder.WriteString(resumeText)
	builder.WriteString("\n\nLista de Vagas:\n")
	for i, v := range vacancies {
		builder.WriteString(fmt.Sprintf("%d: %s\n", i, v))
	}

	reqPayload := deepseekRequest{
		Model: c.model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: "Você é um assistente de recrutamento e seleção especialista em compatibilidade de currículos com vagas de emprego. Você responde apenas com o JSON estruturado solicitado.",
			},
			{
				Role:    "user",
				Content: builder.String(),
			},
		},
		ResponseFormat: &responseFormat{
			Type: "json_object",
		},
	}

	log.Printf("[DeepSeekClient] Marshalling request payload for MatchResume with %d vacancies...", len(vacancies))
	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal deepseek request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.apiURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request to deepseek: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	log.Printf("[DeepSeekClient] Sending POST request to %s (model: %s, payload size: %d bytes)...", url, c.model, len(jsonBytes))
	startHttp := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[DeepSeekClient] HTTP request failed after %s: %v", time.Since(startHttp), err)
		return nil, fmt.Errorf("http request to deepseek failed: %w", err)
	}
	defer resp.Body.Close()

	latency := time.Since(startHttp)
	log.Printf("[DeepSeekClient] HTTP response received in %s. Status code: %d", latency, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		log.Printf("[DeepSeekClient] Error response body: %s", string(respBytes))
		return nil, fmt.Errorf("deepseek api returned status code %d: %s", resp.StatusCode, string(respBytes))
	}

	var dsRes deepseekResponse
	if err := json.NewDecoder(resp.Body).Decode(&dsRes); err != nil {
		return nil, fmt.Errorf("failed to decode deepseek response: %w", err)
	}

	if len(dsRes.Choices) == 0 || dsRes.Choices[0].Message.Content == "" {
		log.Println("[DeepSeekClient] Error: DeepSeek returned an empty candidate list or empty content")
		return nil, fmt.Errorf("deepseek returned an empty response")
	}

	responseText := dsRes.Choices[0].Message.Content
	log.Printf("[DeepSeekClient] Raw response content from model:\n%s", responseText)

	var matchResult domain.MatchResult
	if err := json.Unmarshal([]byte(responseText), &matchResult); err != nil {
		log.Printf("[DeepSeekClient] Error unmarshalling structured JSON: %v", err)
		return nil, fmt.Errorf("failed to parse structured result from deepseek response text: %w. Response was: %s", err, responseText)
	}

	log.Printf("[DeepSeekClient] Successfully parsed %d matches from JSON", len(matchResult.Matches))
	return &matchResult, nil
}

type deepseekEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type deepseekEmbedResponse struct {
	Object string              `json:"object"`
	Data   []deepseekEmbedData `json:"data"`
}

type deepseekEmbedData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

func (c *deepseekClient) GetEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("deepseek does not support embeddings (embeddings endpoint is not available on the official DeepSeek API). Please configure Gemini as the embedding provider")
}

func (c *deepseekClient) ExtractText(ctx context.Context, fileB64 string, fileMime string) (string, error) {
	return pdf.ExtractTextFromBase64(fileB64, fileMime)
}
