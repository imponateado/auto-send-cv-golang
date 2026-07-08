package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"api/internal/config"
	"api/internal/domain"
	"api/internal/handler"
	"api/internal/infra/chromem"
	"api/internal/infra/db"
	"api/internal/infra/deepseek"
	"api/internal/infra/gemini"
	"api/internal/infra/multillm"
	"api/internal/infra/ollama"
	"api/internal/infra/whatsapp"
	"api/internal/service"
)

// @title API Go de Processamento de Texto e Documentos
// @version 1.0
// @description API REST em Go com Clean Architecture para processamento de textos, análise de currículos com Gemini e disparos automáticos.
// @host localhost:8080
// @BasePath /
func main() {
	log.Println("Starting API server initialization...")

	cfg := config.Load()

	credsRepo, err := db.NewSQLRepo("./db/api.db")
	if err != nil {
		log.Fatalf("Failed to initialize sqlite credentials database: %v", err)
	}

	waManager, err := whatsapp.NewWhatsMeowManager("./db/whatsapp.db")
	if err != nil {
		log.Fatalf("Failed to initialize whatsmeow manager: %v", err)
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	geminiModel := os.Getenv("GEMINI_MODEL")
	geminiService := gemini.NewGeminiClient(geminiKey, geminiModel)

	deepseekKey := os.Getenv("DEEPSEEK_API_KEY")
	deepseekModel := os.Getenv("DEEPSEEK_MODEL")
	deepseekService := deepseek.NewDeepSeekClient(deepseekKey, deepseekModel)

	ollamaURL := os.Getenv("OLLAMA_API_URL")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	ollamaService := ollama.NewOllamaClient(ollamaURL, ollamaModel)

	vectorStore, err := chromem.NewChromemStore("./db")
	if err != nil {
		log.Fatalf("Failed to initialize chromem vector store: %v", err)
	}

	var activeLLM domain.GeminiService
	provider := os.Getenv("LLM_PROVIDER")
	if provider == "" {
		provider = "fallback"
	}

	switch provider {
	case "gemini":
		activeLLM = geminiService
	case "deepseek":
		activeLLM = deepseekService
	default: // fallback
		activeLLM = multillm.NewFallbackLLMService(geminiService, deepseekService)
	}

	matchingOrchestrator := service.NewOrchestrator(
		activeLLM,
		ollamaService,
		vectorStore, // Local Vector Database (chromem-go)
		credsRepo,
		waManager,
	)

	procHandler := handler.NewProcessorHandler(matchingOrchestrator)
	credsHandler := handler.NewCredentialsHandler(credsRepo, waManager)

	mux := http.NewServeMux()
	router := handler.RegisterRoutes(mux, procHandler, credsHandler)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf("Server listening on port %s (%s mode)...", cfg.Port, cfg.Env)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		log.Fatalf("Critical server error: %v", err)

	case sig := <-shutdownSignal:
		log.Printf("Received shutdown signal: %v. Initiating graceful shutdown...", sig)

		// Create a context with a timeout for the graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Attempt to gracefully shutdown the server
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Could not gracefully stop server: %v. Forcing close...", err)
			if err := server.Close(); err != nil {
				log.Fatalf("Error forcing server close: %v", err)
			}
		}
		log.Println("Server stopped gracefully.")
	}
}
