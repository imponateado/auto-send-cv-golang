package gemini

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

type geminiClient struct {
	apiKey     string
	model      string
	apiURL     string
	httpClient *http.Client
}

// NewGeminiClient returns a domain.GeminiService backed by a *geminiClient using
// the native HTTP client.
func NewGeminiClient(apiKey, model string) domain.GeminiService {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &geminiClient{
		apiKey: apiKey,
		model:  model,
		apiURL: "https://generativelanguage.googleapis.com/v1beta",
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type geminiRequest struct {
	Contents         []content        `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	InlineData *inlineData `json:"inlineData,omitempty"`
	Text       string      `json:"text,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type generationConfig struct {
	ResponseMimeType string         `json:"responseMimeType"`
	ResponseSchema   responseSchema `json:"responseSchema"`
}

type responseSchema struct {
	Type        string                    `json:"type"`
	Properties  map[string]schemaProperty `json:"properties"`
	Required    []string                  `json:"required"`
	Items       *responseSchema           `json:"items,omitempty"`
	Description string                    `json:"description,omitempty"`
	Enum        []string                  `json:"enum,omitempty"`
}

type schemaProperty struct {
	Type        string          `json:"type"`
	Description string          `json:"description,omitempty"`
	Items       *responseSchema `json:"items,omitempty"`
	Enum        []string        `json:"enum,omitempty"`
}

type geminiResponse struct {
	Candidates []candidate `json:"candidates"`
}

type candidate struct {
	Content content `json:"content"`
}

func (c *geminiClient) MatchResume(ctx context.Context, fileB64 string, fileMime string, vacancies []string) (*domain.MatchResult, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("gemini client is misconfigured: api key is required")
	}

	if idx := strings.Index(fileB64, ","); idx != -1 {
		fileB64 = fileB64[idx+1:]
	}
	fileB64 = strings.Join(strings.Fields(fileB64), "")

	if fileMime == "" {
		fileMime = "application/pdf"
	}

	var builder strings.Builder
	builder.WriteString("Abaixo está o currículo de um candidato (em anexo) e uma lista de vagas de emprego.\n")
	builder.WriteString("Analise o currículo e compare-o com cada uma das vagas.\n")
	builder.WriteString("Retorne no formato JSON estruturado quais vagas dão match com o candidato.\n")
	builder.WriteString("Para cada match, identifique: o index da vaga na lista, o motivo, o canal de contato ('email' ou 'whatsapp') e o email ou telefone de destino indicado no texto da vaga.\n")
	builder.WriteString("Se o canal de contato for 'whatsapp', o contact_target deve conter APENAS dígitos, incluindo o código do país do Brasil (55) antes do DDD, sem parênteses, espaços ou hífens (ex: \"(61) 99205-5310\" vira \"5561992055310\").\n\n")
	builder.WriteString("Lista de Vagas:\n")
	for i, v := range vacancies {
		builder.WriteString(fmt.Sprintf("%d: %s\n", i, v))
	}

	reqPayload := geminiRequest{
		Contents: []content{
			{
				Parts: []part{
					{
						InlineData: &inlineData{
							MimeType: fileMime,
							Data:     fileB64,
						},
					},
					{
						Text: builder.String(),
					},
				},
			},
		},
		GenerationConfig: generationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema: responseSchema{
				Type:     "OBJECT",
				Required: []string{"matches"},
				Properties: map[string]schemaProperty{
					"matches": {
						Type: "ARRAY",
						Items: &responseSchema{
							Type:     "OBJECT",
							Required: []string{"index", "reason", "contact_type", "contact_target"},
							Properties: map[string]schemaProperty{
								"index": {
									Type:        "INTEGER",
									Description: "Índice numérico da vaga que deu match na lista recebida (iniciando em 0)",
								},
								"reason": {
									Type:        "STRING",
									Description: "Explicação em português de por que o currículo do candidato bate com os requisitos da vaga",
								},
								"contact_type": {
									Type:        "STRING",
									Enum:        []string{"email", "whatsapp"},
									Description: "Meio de contato identificado na vaga para se candidatar",
								},
								"contact_target": {
									Type:        "STRING",
									Description: "O e-mail ou o número de telefone da empresa informado na vaga. Se for telefone (contact_type 'whatsapp'), retornar somente dígitos com o código do país 55 antes do DDD (ex: 5561992055310)",
								},
							},
						},
					},
				},
			},
		},
	}

	log.Printf("[GeminiClient] Marshalling request payload for MatchResume with %d vacancies...", len(vacancies))
	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", c.apiURL, c.model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	log.Printf("[GeminiClient] Sending POST request to %s (payload size: %d bytes)...", url, len(jsonBytes))
	startHttp := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[GeminiClient] HTTP request failed after %s: %v", time.Since(startHttp), err)
		return nil, fmt.Errorf("http request to gemini failed: %w", err)
	}
	defer resp.Body.Close()

	latency := time.Since(startHttp)
	log.Printf("[GeminiClient] HTTP response received in %s. Status code: %d", latency, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		log.Printf("[GeminiClient] Error response body: %s", string(respBytes))
		return nil, fmt.Errorf("gemini api returned status code %d: %s", resp.StatusCode, string(respBytes))
	}

	var geminiRes geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiRes); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	if len(geminiRes.Candidates) == 0 || len(geminiRes.Candidates[0].Content.Parts) == 0 {
		log.Println("[GeminiClient] Error: Gemini returned an empty candidate list or empty parts")
		return nil, fmt.Errorf("gemini returned an empty response")
	}

	responseText := geminiRes.Candidates[0].Content.Parts[0].Text
	log.Printf("[GeminiClient] Raw response content from model:\n%s", responseText)

	var matchResult domain.MatchResult
	if err := json.Unmarshal([]byte(responseText), &matchResult); err != nil {
		log.Printf("[GeminiClient] Error unmarshalling structured JSON: %v", err)
		return nil, fmt.Errorf("failed to parse structured result from gemini response text: %w", err)
	}

	log.Printf("[GeminiClient] Successfully parsed %d matches from JSON", len(matchResult.Matches))
	return &matchResult, nil
}

func (c *geminiClient) GetEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("gemini embedding is deprecated in this project. Please configure Ollama as the embedding provider")
}

func (c *geminiClient) ExtractText(ctx context.Context, fileB64 string, fileMime string) (string, error) {
	return pdf.ExtractTextFromBase64(fileB64, fileMime)
}
