package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"api/internal/domain"
)

type deepseekClient struct {
	apiKey     string
	model      string
	apiURL     string
	httpClient *http.Client
}

// NewDeepSeekClient creates a new DeepSeek service adapter using the native HTTP client.
func NewDeepSeekClient(apiKey, model string) domain.GeminiService {
	if model == "" {
		model = "deepseek-v4-flash"
	}
	return &deepseekClient{
		apiKey: apiKey,
		model:  model,
		apiURL: "https://api.deepseek.com",
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // DeepSeek calls can take longer, especially thinking ones
		},
	}
}

// DeepSeek request structures
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

// DeepSeek response structures
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

	resumeText, err := ExtractTextFromBase64(fileB64, fileMime)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text from resume: %w", err)
	}

	var builder strings.Builder
	builder.WriteString("Abaixo está o texto extraído do currículo de um candidato e uma lista de vagas de emprego.\n")
	builder.WriteString("Analise o currículo e compare-o com cada uma das vagas.\n")
	builder.WriteString("Retorne no formato JSON estruturado quais vagas dão match com o candidato.\n")
	builder.WriteString("Para cada match, identifique: o index da vaga na lista, o motivo, o canal de contato ('email' ou 'whatsapp') e o email ou telefone de destino indicado no texto da vaga.\n\n")
	builder.WriteString("A resposta DEVE ser um objeto JSON válido seguindo estritamente este formato:\n")
	builder.WriteString("{\n  \"matches\": [\n    {\n      \"index\": 0,\n      \"reason\": \"motivo detalhado em português\",\n      \"contact_type\": \"email\" ou \"whatsapp\",\n      \"contact_target\": \"email ou telefone\"\n    }\n  ]\n}\n\n")
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request to deepseek failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("deepseek api returned status code %d: %s", resp.StatusCode, string(respBytes))
	}

	var dsRes deepseekResponse
	if err := json.NewDecoder(resp.Body).Decode(&dsRes); err != nil {
		return nil, fmt.Errorf("failed to decode deepseek response: %w", err)
	}

	if len(dsRes.Choices) == 0 || dsRes.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("deepseek returned an empty response")
	}

	responseText := dsRes.Choices[0].Message.Content

	var matchResult domain.MatchResult
	if err := json.Unmarshal([]byte(responseText), &matchResult); err != nil {
		return nil, fmt.Errorf("failed to parse structured result from deepseek response text: %w. Response was: %s", err, responseText)
	}

	return &matchResult, nil
}
