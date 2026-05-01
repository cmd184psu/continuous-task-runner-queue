package coordinator

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/cmd184psu/ctrq/internal/models"
)

func (c *Coordinator) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	var req models.AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Passcode != c.cfg.Passcode {
		writeError(w, http.StatusUnauthorized, "invalid passcode")
		return
	}
	token, err := c.generateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	writeJSON(w, http.StatusOK, models.AuthResponse{Token: token})
}

func (c *Coordinator) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := c.db.Ping(); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (c *Coordinator) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := c.db.ListGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	statuses := make([]models.GroupStatus, 0, len(groups))
	for _, g := range groups {
		count, _ := c.db.CountRunningInGroup(g.Name)
		statuses = append(statuses, models.GroupStatus{Group: *g, RunningCount: count})
	}
	writeJSON(w, http.StatusOK, statuses)
}

func (c *Coordinator) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	g, err := c.db.GetGroup(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if g == nil {
		writeError(w, http.StatusNotFound, "group not found: "+name)
		return
	}
	count, _ := c.db.CountRunningInGroup(name)
	writeJSON(w, http.StatusOK, models.GroupStatus{Group: *g, RunningCount: count})
}

func (c *Coordinator) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var g models.Group
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if g.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if g.PoolLimit <= 0 {
		g.PoolLimit = 1
	}
	if g.AllowedTypes == nil {
		g.AllowedTypes = []string{}
	}
	if err := c.db.UpsertGroup(&g); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	created, _ := c.db.GetGroup(g.Name)
	writeJSON(w, http.StatusCreated, created)
}

func (c *Coordinator) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	existing, err := c.db.GetGroup(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "group not found: "+name)
		return
	}
	var updates struct {
		PoolLimit    *int     `json:"pool_limit"`
		AllowedTypes []string `json:"allowed_types"`
	}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if updates.PoolLimit != nil {
		existing.PoolLimit = *updates.PoolLimit
	}
	if updates.AllowedTypes != nil {
		existing.AllowedTypes = updates.AllowedTypes
	}
	if err := c.db.UpsertGroup(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, _ := c.db.GetGroup(name)
	writeJSON(w, http.StatusOK, updated)
}

func (c *Coordinator) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := c.db.DeleteGroup(name); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Coordinator) handlePauseGroup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	by := r.URL.Query().Get("by")
	if by == "" {
		by = "api"
	}
	if err := c.db.SetGroupPaused(name, true, by); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (c *Coordinator) handleResumeGroup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := c.db.SetGroupPaused(name, false, ""); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}
