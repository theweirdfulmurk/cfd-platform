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
	namespace := getEnv("K8S_NAMESPACE", "default")
	port := getEnv("PORT", "8080")

	typed, dyn, err := k8s.NewClients()
	if err != nil {
		log.Fatalf("k8s client: %v", err)
	}

	vizK8sManager := k8s.NewVisualizationManager(typed, namespace)
	simK8sManager := k8s.NewSimulationManager(typed, dyn, namespace)

	vizRepo := repository.NewInMemoryVisualizationRepo()
	// Persist the run index on the shared PVC (next to each case dir) so the
	// listing survives a backend restart instead of living only in pod RAM.
	simRepo := repository.NewInMemorySimulationRepo("/pvc/simulations/_index.json")

	vizUseCase := usecase.NewVisualizationUseCase(vizRepo, vizK8sManager)
	simUseCase := usecase.NewSimulationUseCase(simRepo, simK8sManager)

	vizHandler := httpHandler.NewVisualizationHandler(vizUseCase)
	simHandler := httpHandler.NewSimulationHandler(simUseCase)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(corsMiddleware)

	r.Route("/api", func(r chi.Router) {
		r.Route("/simulations", func(r chi.Router) {
			r.Post("/", simHandler.Create)
			r.Get("/", simHandler.List)
			r.Get("/{simId}", simHandler.Get)
			r.Delete("/{simId}", simHandler.Delete)
			r.Get("/{simId}/results", simHandler.DownloadResults)
			r.Get("/{simId}/surface", simHandler.Surface)
			r.Get("/{simId}/field-stats", simHandler.FieldStats)
			r.Get("/{simId}/visualizations", vizHandler.ListBySimulation)
		})
		r.Route("/visualizations", func(r chi.Router) {
			r.Post("/", vizHandler.Create)
			r.Get("/{vizId}", vizHandler.GetStatus)
			r.Get("/{vizId}/ws-url", vizHandler.GetWebSocketURL)
			r.Delete("/{vizId}", vizHandler.Delete)
		})
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	log.Printf("server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server: %v", err)
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
