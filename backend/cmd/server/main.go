package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpHandler "github.com/theweirdfulmurk/cfd-platform/internal/delivery/http"
	"github.com/theweirdfulmurk/cfd-platform/internal/infrastructure/k8s"
	"github.com/theweirdfulmurk/cfd-platform/internal/repository"
	"github.com/theweirdfulmurk/cfd-platform/internal/usecase"
)

func main() {
	// Configuration
	namespace := getEnv("K8S_NAMESPACE", "default")
	port := getEnv("PORT", "8082")

	// Initialize K8s client
	k8sClient, err := k8s.NewClient()
	if err != nil {
		log.Fatalf("Failed to create K8s client: %v", err)
	}

	// Infrastructure
	vizK8sManager := k8s.NewVisualizationManager(k8sClient, namespace)
	simK8sManager := k8s.NewSimulationManager(k8sClient, namespace)

	// Repositories
	vizRepo := repository.NewInMemoryVisualizationRepo()
	simRepo := repository.NewInMemorySimulationRepo()

	// Use Cases
	vizUseCase := usecase.NewVisualizationUseCase(vizRepo, vizK8sManager)
	simUseCase := usecase.NewSimulationUseCase(simRepo, simK8sManager)

	// HTTP Handlers
	vizHandler := httpHandler.NewVisualizationHandler(vizUseCase)
	simHandler := httpHandler.NewSimulationHandler(simUseCase)

	// Router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(corsMiddleware)

	// Routes
	r.Route("/api", func(r chi.Router) {
		// Simulation routes
		r.Route("/simulations", func(r chi.Router) {
			r.Post("/", simHandler.Create)
			r.Get("/", simHandler.List)
			r.Get("/{simId}", simHandler.Get)
			r.Delete("/{simId}", simHandler.Delete)

			// Visualization routes nested under simulation
			r.Get("/{simId}/visualizations", vizHandler.ListBySimulation)
		})

		// Visualization routes
		r.Route("/visualizations", func(r chi.Router) {
			r.Post("/", vizHandler.Create)
			r.Get("/{vizId}", vizHandler.GetStatus)
			r.Get("/{vizId}/ws-url", vizHandler.GetWebSocketURL)
			r.Delete("/{vizId}", vizHandler.Delete)
		})
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start server
	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}