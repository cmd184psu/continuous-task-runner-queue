package coordinator

import (
	"net/http"
	"strconv"

	"github.com/cmd184psu/ctrq/internal/models"
)

func (c *Coordinator) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	taskFilter := r.URL.Query().Get("task")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}
	execs, err := c.db.ListExecutions(taskFilter, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if execs == nil {
		execs = []*models.TaskExecution{}
	}
	writeJSON(w, http.StatusOK, execs)
}
