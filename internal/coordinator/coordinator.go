package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/cmd184psu/ctrq/internal/db"
	"github.com/cmd184psu/ctrq/internal/models"
	"github.com/cmd184psu/ctrq/web"
)

type Coordinator struct {
	db     *db.DB
	cfg    *models.Config
	server *http.Server
}

func New(database *db.DB, cfg *models.Config) *Coordinator {
	return &Coordinator{db: database, cfg: cfg}
}

// Routes returns the HTTP handler (exported for testing).
func Routes(c *Coordinator) http.Handler { return c.routes() }

func (c *Coordinator) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public
	r.Post("/api/auth/token", c.handleAuthToken)
	r.Get("/api/health", c.handleHealth)

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(c.authMiddleware)

		r.Get("/api/groups", c.handleListGroups)
		r.Post("/api/groups", c.handleCreateGroup)
		r.Get("/api/groups/{name}", c.handleGetGroup)
		r.Put("/api/groups/{name}", c.handleUpdateGroup)
		r.Delete("/api/groups/{name}", c.handleDeleteGroup)
		r.Post("/api/groups/{name}/pause", c.handlePauseGroup)
		r.Post("/api/groups/{name}/resume", c.handleResumeGroup)

		r.Get("/api/tasks", c.handleListTasks)
		r.Post("/api/tasks", c.handleAddTask)
		r.Get("/api/tasks/{name}", c.handleGetTask)
		r.Put("/api/tasks/{name}", c.handleUpdateTask)
		r.Delete("/api/tasks/{name}", c.handleDeleteTask)
		r.Post("/api/tasks/{name}/pause", c.handlePauseTask)
		r.Post("/api/tasks/{name}/resume", c.handleResumeTask)
		r.Post("/api/tasks/{name}/enqueue", c.handleEnqueueTask)

		r.Get("/api/executions", c.handleListExecutions)
		r.Get("/api/executions/{id}/output", c.handleExecutionOutput)

		r.Get("/api/metrics", c.handleMetrics)
	})

	if c.cfg.UIEnabled {
		r.Handle("/*", web.Handler())
	}

	return r
}

func (c *Coordinator) Start(ctx context.Context) error {
	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", c.cfg.Port),
		Handler: c.routes(),
	}
	go func() {
		<-ctx.Done()
		c.server.Shutdown(context.Background())
	}()
	if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
