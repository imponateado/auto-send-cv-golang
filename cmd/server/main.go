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

// main monta as dependências (config, repositórios, LLM, WhatsApp, vector
// store), registra as rotas HTTP e bloqueia servindo requisições até receber um
// sinal de shutdown, encerrando o servidor graciosamente. Não retorna nada;
// termina o processo via log.Fatalf em caso de erro crítico de inicialização ou
// do servidor.
// @title API Go de Processamento de Texto e Documentos
// @version 1.0
// @description Monta as dependências, registra as rotas HTTP e serve requisições até receber um sinal de shutdown, encerrando o servidor graciosamente.
// @host localhost:8080
// @BasePath /
func main() {
	log.Println("Starting API server initialization...")

	cfg := config.Load()

	sqlDB, err := db.Open("./db/api.db")
	if err != nil {
		log.Fatalf("Failed to open sqlite database: %v", err)
	}

	credsRepo, err := db.NewSQLRepo(sqlDB)
	if err != nil {
		log.Fatalf("Failed to initialize sqlite credentials database: %v", err)
	}

	groupWatchRepo, err := db.NewGroupWatchRepo(sqlDB)
	if err != nil {
		log.Fatalf("Failed to initialize sqlite group watch database: %v", err)
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
	default:
		activeLLM = multillm.NewFallbackLLMService(geminiService, deepseekService)
	}

	matchingOrchestrator := service.NewOrchestrator(
		activeLLM,
		ollamaService,
		vectorStore,
		credsRepo,
		waManager,
	)

	groupWatcher := service.NewGroupWatcher(groupWatchRepo, waManager, matchingOrchestrator)
	if err := groupWatcher.Bootstrap(context.Background()); err != nil {
		log.Printf("Warning: failed to bootstrap group watchers: %v", err)
	}
	schedCtx, cancelSched := context.WithCancel(context.Background())
	defer cancelSched()
	go groupWatcher.RunScheduler(schedCtx)

	procHandler := handler.NewProcessorHandler(matchingOrchestrator)
	credsHandler := handler.NewCredentialsHandler(credsRepo, waManager)
	groupHandler := handler.NewGroupWatchHandler(groupWatcher)

	mux := http.NewServeMux()
	router := handler.RegisterRoutes(mux, procHandler, credsHandler, groupHandler)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
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

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Could not gracefully stop server: %v. Forcing close...", err)
			if err := server.Close(); err != nil {
				log.Fatalf("Error forcing server close: %v", err)
			}
		}
		log.Println("Server stopped gracefully.")
	}
}
