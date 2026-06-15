package handler

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture the status code of responses.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RegisterRoutes sets up the endpoint and decorates it with logging and recovery middleware.
func RegisterRoutes(mux *http.ServeMux, procHandler *ProcessorHandler) http.Handler {
	// Register endpoint using Go 1.22+ ServeMux method-matching syntax
	mux.HandleFunc("POST /api/v1/process", procHandler.Process)

	var handler http.Handler = mux
	handler = loggingMiddleware(handler)
	handler = recoveryMiddleware(handler)

	return handler
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newResponseWriter(w)

		next.ServeHTTP(rw, r)

		log.Printf("[HTTP] %s %s - Status: %d - Duration: %v - IP: %s",
			r.Method, r.URL.Path, rw.statusCode, time.Since(start), r.RemoteAddr)
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[PANIC RECOVERED] error: %v\nstacktrace:\n%s", err, debug.Stack())
				respondWithError(w, http.StatusInternalServerError, "Internal Server Error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
